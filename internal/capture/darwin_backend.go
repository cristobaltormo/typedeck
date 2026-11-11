package capture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const (
	HelperName         = "typedeck-capture"
	helperStartTimeout = 5 * time.Second
)

var ErrHelperMissing = errors.New("the macOS capture helper is missing")

type permissionError struct{ accessibility, input bool }

func (e *permissionError) Code() string { return "permission" }

func (e *permissionError) Error() string {
	var missing []string
	if !e.accessibility {
		missing = append(missing, "Accessibility")
	}
	if !e.input {
		missing = append(missing, "Input Monitoring")
	}
	return fmt.Sprintf("macOS has not allowed %s to read the keyboard: turn it on under %s in System Settings > Privacy & Security", HelperName, strings.Join(missing, " and "))
}

type helperMissingError struct{ path string }

func (e *helperMissingError) Code() string { return "helper_missing" }

func (e *helperMissingError) Error() string {
	return fmt.Sprintf("%s was not found at %s: it must sit next to the typedeck binary", HelperName, e.path)
}

func (e *helperMissingError) Is(target error) bool { return target == ErrHelperMissing }

func helperPath() string {
	exe, err := os.Executable()
	if err != nil {
		return HelperName
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Join(filepath.Dir(exe), HelperName)
}

type macBackend struct {
	path func() string

	mu       sync.Mutex
	proc     *helperProc
	filter   *Filter
	layout   atomic.Value
	prompted bool

	latN, latAvg, latMax atomic.Uint64
	tapResets            atomic.Uint64
}

func NewMacBackend(path func() string) Backend {
	b := &macBackend{path: path}
	b.layout.Store(layoutISO)
	return b
}

func (b *macBackend) currentLayout() string { return b.layout.Load().(string) }

func (b *macBackend) Start(f *Filter) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.proc != nil && b.proc.alive() {
		return nil
	}
	path := b.path()
	if _, err := os.Stat(path); err != nil {
		return &helperMissingError{path: path}
	}
	args := []string{"run"}
	if !b.prompted {
		args = append(args, "--prompt")
		b.prompted = true
	}
	first := make(chan helperMsg, 1)
	proc, err := startHelper(path, args, func(line string) { b.onLine(f, line, first) }, f.Release)
	if err != nil {
		return err
	}
	msg, err := waitFirst(proc, first)
	if err != nil {
		proc.stop()
		return err
	}
	b.layout.Store(msg.Text)
	b.proc, b.filter = proc, f
	_ = proc.send(configLine(f.Snapshot(), b.currentLayout()))
	go b.follow(proc, f)
	return nil
}

func waitFirst(proc *helperProc, first <-chan helperMsg) (helperMsg, error) {
	select {
	case m := <-first:
		if m.Kind == "READY" {
			return m, nil
		}
		return m, startError(m)
	case <-proc.done:
		select {
		case m := <-first:
			return m, startError(m)
		default:
			return helperMsg{}, errors.New("the capture helper stopped before it was ready")
		}
	case <-time.After(helperStartTimeout):
		return helperMsg{}, errors.New("the capture helper did not start")
	}
}

func startError(m helperMsg) error {
	if m.Kind == "ERR" && strings.HasPrefix(m.Text, "permission") {
		return &permissionError{accessibility: strings.Contains(m.Text, "accessibility=1"), input: strings.Contains(m.Text, "input_monitoring=1")}
	}
	if m.Kind == "ERR" {
		return errors.New("capture helper: " + m.Text)
	}
	return errors.New("the capture helper did not answer as expected")
}

func (b *macBackend) onLine(f *Filter, line string, first chan<- helperMsg) {
	m, ok := parseHelperLine(line)
	if !ok {
		return
	}
	switch m.Kind {
	case "READY", "ERR":
		select {
		case first <- m:
		default:
		}
	case "D", "U", "P", "R":
		if u := MacUsage(m.Code, b.currentLayout()); u != 0 {
			f.Observe(m.Kind[0], u, ModsFromFlags(m.Flags))
		}
	case "KBTYPE":
		b.layout.Store(m.Text)
		b.resend()
	case "TAPOFF":
		b.tapResets.Add(1)
		f.Release()
	case "LAT":
		b.latN.Store(m.Counts[0])
		b.latAvg.Store(m.Counts[1])
		b.latMax.Store(m.Counts[2])
	}
}

func (b *macBackend) resend() {
	b.mu.Lock()
	proc, f := b.proc, b.filter
	b.mu.Unlock()
	if proc != nil && f != nil {
		_ = proc.send(configLine(f.Snapshot(), b.currentLayout()))
	}
}

func (b *macBackend) follow(proc *helperProc, f *Filter) {
	for {
		select {
		case <-proc.done:
			return
		case <-f.Changed():
			_ = proc.send(configLine(f.Snapshot(), b.currentLayout()))
		}
	}
}

func (b *macBackend) Alive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.proc != nil && b.proc.alive()
}

func (b *macBackend) Stop() {
	b.mu.Lock()
	proc, f := b.proc, b.filter
	b.proc, b.filter = nil, nil
	b.mu.Unlock()
	if proc != nil {
		proc.stop()
	}
	if f != nil {
		f.Release()
	}
}

func (b *macBackend) Latency() Latency {
	return Latency{Count: b.latN.Load(), AvgUS: int64(b.latAvg.Load()), MaxUS: int64(b.latMax.Load())}
}

func (b *macBackend) send(lines ...string) error {
	b.mu.Lock()
	proc := b.proc
	b.mu.Unlock()
	if proc == nil || !proc.alive() {
		return ErrUnsupported
	}
	return proc.send(lines...)
}

var macInjectMu sync.Mutex

const macTapMS = 12

func (b *macBackend) Inject(s hid.Stroke, holdMS int) error {
	if s.Usage == 0x39 && s.Mods == 0 {
		macInjectMu.Lock()
		defer macInjectMu.Unlock()
		return b.send("CAPS")
	}
	down, up, err := strokePlan(s, b.currentLayout())
	if err != nil || len(down) == 0 {
		return err
	}
	macInjectMu.Lock()
	defer macInjectMu.Unlock()
	if err := b.send(stepLines(down)...); err != nil {
		return err
	}
	if holdMS <= 0 {
		holdMS = macTapMS
	}
	time.Sleep(time.Duration(holdMS) * time.Millisecond)
	return b.send(stepLines(up)...)
}

func stepLines(steps []keyStep) []string {
	out := make([]string, len(steps))
	for i, st := range steps {
		out[i] = keyLine(st)
	}
	return out
}

func (b *macBackend) Consumer(usage uint16) error {
	k, ok := ConsumerKeyType(usage)
	if !ok {
		return fmt.Errorf("media key %X has no macOS equivalent", usage)
	}
	macInjectMu.Lock()
	defer macInjectMu.Unlock()
	return b.send(mediaLine(k))
}

func (b *macBackend) TypeText(text string) error {
	macInjectMu.Lock()
	defer macInjectMu.Unlock()
	var lines []string
	var run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			lines = append(lines, textLine(run.String()))
			run.Reset()
		}
	}
	for _, r := range text {
		code := 0
		switch r {
		case '\n', '\r':
			code = codeReturn
		case '\t':
			code = codeTab
		default:
			run.WriteRune(r)
			continue
		}
		flush()
		lines = append(lines, keyLine(keyStep{true, code, 0}), keyLine(keyStep{false, code, 0}))
	}
	flush()
	if len(lines) == 0 {
		return nil
	}
	return b.send(lines...)
}
