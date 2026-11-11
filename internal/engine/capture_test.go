package engine

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hid"
)

type stubBackend struct {
	mu       sync.Mutex
	startErr error
	alive    bool
	injected []hid.Stroke
}

func (b *stubBackend) Start(*capture.Filter) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.startErr != nil {
		return b.startErr
	}
	b.alive = true
	return nil
}
func (b *stubBackend) Alive() bool { return b.alive }
func (b *stubBackend) Stop()       { b.alive = false }
func (b *stubBackend) Inject(s hid.Stroke, _ int) error {
	b.mu.Lock()
	b.injected = append(b.injected, s)
	b.mu.Unlock()
	return nil
}
func (b *stubBackend) Consumer(uint16) error    { return nil }
func (b *stubBackend) TypeText(string) error    { return nil }
func (b *stubBackend) Latency() capture.Latency { return capture.Latency{} }
func (b *stubBackend) Checks() []capture.Check  { return nil }
func (b *stubBackend) strokes() []hid.Stroke {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]hid.Stroke(nil), b.injected...)
}

type noBoard struct{}

func (noBoard) Open() (io.ReadWriteCloser, string, error) { return nil, "", board.ErrNoBoard }

func newSoftTest(t *testing.T, mutate func(*config.Config)) (*Engine, *rec, *capture.SoftwareSource, *stubBackend) {
	t.Helper()
	be := &stubBackend{}
	soft := capture.NewSoftwareWith(be)
	e, r := newTestWith(t, soft, mutate)
	go soft.Run()
	t.Cleanup(soft.Close)
	if err := soft.Enable(); err != nil {
		t.Fatal(err)
	}
	return e, r, soft, be
}

func press(f *capture.Filter, u byte) bool { return f.Handle(u, true) }

func release(f *capture.Filter, u byte) bool { return f.Handle(u, false) }

func TestSoftwareTapSwallowsTheKeyAndRunsTheAction(t *testing.T) {
	e, r, soft, _ := newSoftTest(t, func(c *config.Config) { c.Layers[0].Keys["04"] = config.KeyDef{Tap: act("a")} })
	_ = e
	f := soft.Filter()
	if !press(f, 0x04) || !release(f, 0x04) {
		t.Fatal("a key with a macro must not reach the application")
	}
	if press(f, 0x05) || release(f, 0x05) {
		t.Fatal("a key without a macro must reach the application")
	}
	settle()
	if got := r.snapshot(); len(got) != 1 || got[0] != "hud:a" {
		t.Fatalf("actions: %v", got)
	}
}

func TestSoftwareHoldAndDouble(t *testing.T) {
	_, r, soft, _ := newSoftTest(t, func(c *config.Config) {
		c.Layers[0].Keys["04"] = config.KeyDef{Tap: act("tap"), Hold: act("hold")}
		c.Layers[0].Keys["05"] = config.KeyDef{Tap: act("one"), Double: act("two")}
	})
	f := soft.Filter()
	press(f, 0x04)
	time.Sleep(350 * time.Millisecond)
	release(f, 0x04)
	press(f, 0x04)
	time.Sleep(30 * time.Millisecond)
	release(f, 0x04)
	press(f, 0x05)
	release(f, 0x05)
	time.Sleep(40 * time.Millisecond)
	press(f, 0x05)
	release(f, 0x05)
	time.Sleep(300 * time.Millisecond)
	got := r.snapshot()
	want := []string{"hud:hold", "hud:tap", "hud:two"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("actions: %v", got)
	}
}

func TestSoftwareMomentaryLayerCapturesKeysOfTheLayer(t *testing.T) {
	e, r, soft, _ := newSoftTest(t, func(c *config.Config) {
		c.Layers[0].Keys["39"] = config.KeyDef{Hold: &config.Action{Type: "layer", To: 1, Momentary: true}}
		c.Layers[1].Keys["04"] = config.KeyDef{Tap: act("layer2-a")}
	})
	f := soft.Filter()
	press(f, 0x39)
	time.Sleep(350 * time.Millisecond)
	if e.Status().Layer != 1 {
		t.Fatalf("layer: %d", e.Status().Layer)
	}
	soft.SyncMask(true)
	if !press(f, 0x04) || !release(f, 0x04) {
		t.Fatal("a key of the held layer must be swallowed")
	}
	release(f, 0x39)
	settle()
	if e.Status().Layer != 0 {
		t.Fatalf("layer after release: %d", e.Status().Layer)
	}
	if got := r.snapshot(); len(got) != 1 || got[0] != "hud:layer2-a" {
		t.Fatalf("actions: %v", got)
	}
}

func TestSoftwareHoldOnlyKeyIsTypedBackOnAShortPress(t *testing.T) {
	_, r, soft, be := newSoftTest(t, func(c *config.Config) { c.Layers[0].Keys["39"] = config.KeyDef{Hold: act("hold")} })
	f := soft.Filter()
	press(f, 0x39)
	time.Sleep(30 * time.Millisecond)
	release(f, 0x39)
	settle()
	if got := be.strokes(); len(got) != 1 || got[0].Usage != 0x39 {
		t.Fatalf("injected: %v", got)
	}
	if len(r.snapshot()) != 0 {
		t.Fatal("no action expected")
	}
}

func TestSoftwareEditorAloneDoesNotReportKeysWithoutAMacro(t *testing.T) {
	e, _, soft, _ := newSoftTest(t, func(c *config.Config) { c.Layers[0].Keys["04"] = config.KeyDef{Tap: act("a")} })
	var seen []string
	var mu sync.Mutex
	e.OnPlainKey = func(u byte, down bool) { mu.Lock(); seen = append(seen, hexKey(u)); mu.Unlock() }
	f := soft.Filter()
	e.SetEditorOpen(true)
	time.Sleep(30 * time.Millisecond)
	press(f, 0x05)
	release(f, 0x05)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 0 {
		t.Fatalf("an open editor must not make software capture report every key: %v", seen)
	}
}

func TestSoftwareReportsEveryKeyOnlyForHistoryOrLearning(t *testing.T) {
	e, _, soft, _ := newSoftTest(t, func(c *config.Config) { c.Settings.KeyHistory = true })
	var seen []string
	var mu sync.Mutex
	e.OnPlainKey = func(u byte, down bool) { mu.Lock(); seen = append(seen, hexKey(u)); mu.Unlock() }
	e.syncKeysAsync()
	time.Sleep(30 * time.Millisecond)
	f := soft.Filter()
	press(f, 0x05)
	release(f, 0x05)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	got := len(seen)
	mu.Unlock()
	if got != 2 {
		t.Fatalf("with the history on, plain keys are reported: %d", got)
	}
	cfg := e.Config()
	cfg.Settings.KeyHistory = false
	if _, err := e.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	press(f, 0x06)
	release(f, 0x06)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("with the history off nothing more is reported: %v", seen)
	}
}

func TestBoardModeStillReportsEveryKeyWhileTheEditorIsOpen(t *testing.T) {
	be := &stubBackend{}
	kb := &keysBoard{}
	sel := capture.NewSelector(kb, capture.NewSoftwareWith(be))
	e, _ := newTestWith(t, sel, nil)
	e.SetEditorOpen(true)
	time.Sleep(30 * time.Millisecond)
	if !kb.last() {
		t.Fatal("on the board an open editor asks for every key, as before")
	}
}

func TestSoftwareLearningCapturesEverything(t *testing.T) {
	e, _, soft, _ := newSoftTest(t, nil)
	if err := e.StartLearn(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	f := soft.Filter()
	if !press(f, 0x1A) {
		t.Fatal("learning swallows the keys so they do not act")
	}
	release(f, 0x1A)
	time.Sleep(30 * time.Millisecond)
	if e.Seen()["1A"] != 1 {
		t.Fatalf("seen: %v", e.Seen())
	}
	e.StopLearn()
	time.Sleep(30 * time.Millisecond)
	if press(f, 0x1A) {
		t.Fatal("after learning keys pass again")
	}
}

func newSwitchable(t *testing.T, be *stubBackend) (*Engine, *capture.Selector) {
	t.Helper()
	sel := capture.NewSelector(board.New(noBoard{}), capture.NewSoftwareWith(be))
	e, _ := newTestWith(t, sel, nil)
	return e, sel
}

func TestSetCapturePersistsAndSwitches(t *testing.T) {
	e, sel := newSwitchable(t, &stubBackend{})
	if e.CaptureMode() != capture.Board {
		t.Fatal("must start on the board")
	}
	if err := e.SetCapture(capture.Software, true); err != nil {
		t.Fatal(err)
	}
	if sel.Active() != capture.Software || e.Config().Settings.Capture != capture.Software {
		t.Fatal("mode and config must both change")
	}
	saved, _ := config.Load(e.paths)
	if saved.Settings.Capture != capture.Software {
		t.Fatalf("not persisted: %q", saved.Settings.Capture)
	}
	if err := e.SetCapture(capture.Board, false); err != nil || sel.Active() != capture.Board || e.Config().Settings.Capture != capture.Board {
		t.Fatalf("going back: %v", err)
	}
}

func TestSetCaptureFailureChangesNothing(t *testing.T) {
	e, sel := newSwitchable(t, &stubBackend{startErr: errors.New("denied")})
	if err := e.SetCapture(capture.Software, true); err == nil {
		t.Fatal("expected an error")
	}
	if sel.Active() != capture.Board || e.Config().Settings.Capture != capture.Board {
		t.Fatal("a failed switch must leave the board mode in place")
	}
}

func TestSetCaptureNeedsConsentAndRecordsIt(t *testing.T) {
	e, sel := newSwitchable(t, &stubBackend{})
	if err := e.SetCapture(capture.Software, false); !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("got %v", err)
	}
	if sel.Active() != capture.Board || config.HasCaptureConsent(e.paths) {
		t.Fatal("refusing must change and record nothing")
	}
	if err := e.SetCapture(capture.Software, true); err != nil {
		t.Fatal(err)
	}
	if !config.HasCaptureConsent(e.paths) {
		t.Fatal("the consent must be stored in the same act")
	}
}

func TestRestoredConfigCannotTurnOnSoftwareCaptureWithoutConsent(t *testing.T) {
	e, sel := newSwitchable(t, &stubBackend{})
	c := e.Config()
	c.Settings.Capture = capture.Software
	got := e.ApplyRestored(c)
	if sel.Active() != capture.Board || got.Settings.Capture != capture.Board || e.Config().Settings.Capture != capture.Board {
		t.Fatal("a restored configuration must not turn on a keyboard hook by itself")
	}
	saved, _ := config.Load(e.paths)
	if saved.Settings.Capture != capture.Board {
		t.Fatalf("the file must not keep the unconsented mode: %q", saved.Settings.Capture)
	}
}

func TestRestoredConfigKeepsSoftwareCaptureAfterConsent(t *testing.T) {
	e, sel := newSwitchable(t, &stubBackend{})
	if err := config.RecordCaptureConsent(e.paths); err != nil {
		t.Fatal(err)
	}
	c := e.Config()
	c.Settings.Capture = capture.Software
	e.ApplyRestored(c)
	if sel.Active() != capture.Software {
		t.Fatal("with the consent of this installation a restored configuration brings its mode")
	}
}

func TestSetConfigCannotTurnOnSoftwareCaptureWithoutConsent(t *testing.T) {
	e, sel := newSwitchable(t, &stubBackend{})
	c := e.Config()
	c.Settings.Capture = capture.Software
	loaded, err := e.SetConfig(c)
	if err != nil || loaded.Settings.Capture != capture.Board || sel.Active() != capture.Board {
		t.Fatalf("%v %q %s", err, loaded.Settings.Capture, sel.Active())
	}
}

func TestStartupWithSoftwareCaptureAndNoConsentFallsBackToTheBoard(t *testing.T) {
	be := &stubBackend{}
	sel := capture.NewSelector(board.New(noBoard{}), capture.NewSoftwareWith(be))
	e, _ := newTestWith(t, sel, func(c *config.Config) { c.Settings.Capture = capture.Software })
	if e.Config().Settings.Capture != capture.Board {
		t.Fatal("the loaded configuration must be normalised")
	}
	saved, _ := config.Load(e.paths)
	if saved.Settings.Capture != capture.Board {
		t.Fatal("and the file too")
	}
	e.Run(closedChan())
	if sel.Active() != capture.Board {
		t.Fatal("startup must not hook the keyboard")
	}
}

func TestStartupWithSoftwareCaptureAndConsentStartsTheHook(t *testing.T) {
	be := &stubBackend{}
	sel := capture.NewSelector(board.New(noBoard{}), capture.NewSoftwareWith(be))
	p := config.Paths{Dir: t.TempDir()}
	c := config.Default()
	c.Settings.Capture = capture.Software
	if err := config.Save(p, c, false, false); err != nil {
		t.Fatal(err)
	}
	if err := config.RecordCaptureConsent(p); err != nil {
		t.Fatal(err)
	}
	e, err := New(p, sel)
	if err != nil {
		t.Fatal(err)
	}
	e.Run(closedChan())
	if sel.Active() != capture.Software {
		t.Fatal("an installation that agreed keeps its mode across restarts")
	}
}

func TestFailedStartupSwitchGoesBackToTheBoardAndWarns(t *testing.T) {
	be := &stubBackend{startErr: errors.New("denied")}
	sel := capture.NewSelector(board.New(noBoard{}), capture.NewSoftwareWith(be))
	e, r := newTestWith(t, sel, nil)
	if err := config.RecordCaptureConsent(e.paths); err != nil {
		t.Fatal(err)
	}
	c := e.Config()
	c.Settings.Capture = capture.Software
	e.ApplyRestored(c)
	if sel.Active() != capture.Board {
		t.Fatal("a failed switch must leave the board active")
	}
	settle()
	if got := r.hudsSnapshot(); len(got) == 0 {
		t.Fatal("the user must be told")
	}
}

func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

type keysBoard struct {
	mu    sync.Mutex
	asked []bool
}

func (b *keysBoard) last() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.asked) > 0 && b.asked[len(b.asked)-1]
}

func (b *keysBoard) Keys(on bool) error {
	b.mu.Lock()
	b.asked = append(b.asked, on)
	b.mu.Unlock()
	return nil
}

func (b *keysBoard) Kind() string          { return capture.Board }
func (b *keysBoard) Bind(capture.Handlers) {}
func (b *keysBoard) Run()                  {}
func (b *keysBoard) Close()                {}
func (b *keysBoard) Status() (bool, string, *capture.Info, time.Time) {
	return true, "", nil, time.Time{}
}
func (b *keysBoard) Connected() bool               { return true }
func (b *keysBoard) Error() string                 { return "" }
func (b *keysBoard) SyncMask(bool)                 {}
func (b *keysBoard) Pause()                        {}
func (b *keysBoard) Resume()                       {}
func (b *keysBoard) Tap(hid.Stroke) error          { return nil }
func (b *keysBoard) TapHeld(hid.Stroke, int) error { return nil }
func (b *keysBoard) Consumer(uint16) error         { return nil }

func (r *rec) hudsSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.huds...)
}
