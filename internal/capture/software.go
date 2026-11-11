package capture

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

var (
	ErrUnsupported = errors.New("software capture is not available on this system")
	ErrNotRunning  = errors.New("software capture is not running")
)

const (
	retryBase    = 4 * time.Second
	retryCeiling = 2 * time.Minute
	noticeBuffer = 64
)

type Backend interface {
	Start(f *Filter) error
	Alive() bool
	Stop()
	Inject(s hid.Stroke, holdMS int) error
	Consumer(usage uint16) error
	TypeText(text string) error
	Latency() Latency
	Checks() []Check
}

type Reinstaller interface {
	Reinstall()
}

type Latency struct {
	Count uint64
	AvgUS int64
	MaxUS int64
}

type SoftwareSource struct {
	be     Backend
	filter *Filter

	life sync.Mutex

	mu      sync.Mutex
	h       Handlers
	wanted  bool
	running bool
	since   time.Time
	lastErr string

	notices chan bool
	wake    chan struct{}
	stop    chan struct{}
	once    sync.Once

	retryBase    time.Duration
	retryCeiling time.Duration
}

func NewSoftware() *SoftwareSource { return NewSoftwareWith(newBackend()) }

func NewSoftwareWith(be Backend) *SoftwareSource {
	return &SoftwareSource{
		be: be, filter: NewFilter(),
		notices: make(chan bool, noticeBuffer), wake: make(chan struct{}, 1), stop: make(chan struct{}),
		retryBase: retryBase, retryCeiling: retryCeiling,
	}
}

func (s *SoftwareSource) Filter() *Filter { return s.filter }

func SystemInfo() *Info {
	return &Info{Present: true, System: true, Prod: "System keyboard", Connected: time.Now()}
}

func (s *SoftwareSource) Kind() string { return Software }

func (s *SoftwareSource) Bind(h Handlers) {
	s.mu.Lock()
	s.h = h
	s.mu.Unlock()
}

func (s *SoftwareSource) handlers() Handlers {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h
}

func (s *SoftwareSource) Run() {
	go s.pump()
	wait := s.retryBase
	for {
		select {
		case <-s.stop:
			return
		case <-s.wake:
		case <-time.After(wait):
		}
		wait = s.supervise(wait)
	}
}

func nextRetry(cur, ceiling time.Duration) time.Duration {
	if cur*2 > ceiling {
		return ceiling
	}
	return cur * 2
}

func (s *SoftwareSource) supervise(wait time.Duration) time.Duration {
	s.life.Lock()
	defer s.life.Unlock()
	if s.isWanted() && s.isRunning() && !s.be.Alive() {
		s.halt()
	}
	if !s.isWanted() || s.isRunning() || s.start() == nil {
		return s.retryBase
	}
	return nextRetry(wait, s.retryCeiling)
}

func (s *SoftwareSource) isWanted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wanted
}

func (s *SoftwareSource) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *SoftwareSource) pump() {
	for {
		select {
		case <-s.stop:
			return
		case ev := <-s.filter.Events():
			s.deliver(ev)
		case up := <-s.notices:
			s.drainEvents()
			s.announce(up)
		}
	}
}

func (s *SoftwareSource) drainEvents() {
	for {
		select {
		case ev := <-s.filter.Events():
			s.deliver(ev)
		default:
			return
		}
	}
}

func (s *SoftwareSource) deliver(ev Event) {
	if fn := s.handlers().OnEvent; fn != nil {
		fn(ev)
	}
}

func (s *SoftwareSource) announce(up bool) {
	h := s.handlers()
	switch {
	case up && h.OnConnected != nil:
		h.OnConnected(SystemInfo())
	case !up && h.OnDisconnect != nil:
		h.OnDisconnect()
	}
}

func (s *SoftwareSource) notify(up bool) {
	select {
	case s.notices <- up:
	default:
	}
}

func (s *SoftwareSource) Enable() error {
	s.life.Lock()
	defer s.life.Unlock()
	s.mu.Lock()
	s.wanted = true
	s.mu.Unlock()
	return s.start()
}

func (s *SoftwareSource) Disable() {
	s.life.Lock()
	defer s.life.Unlock()
	s.mu.Lock()
	s.wanted = false
	s.mu.Unlock()
	s.halt()
}

func (s *SoftwareSource) start() error {
	if s.isRunning() {
		return nil
	}
	if err := s.be.Start(s.filter); err != nil {
		s.mu.Lock()
		s.lastErr = errorCode(err)
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.running, s.since, s.lastErr = true, time.Now(), ""
	mask := s.h.Mask
	s.mu.Unlock()
	if mask != nil {
		s.filter.SetMask(mask())
	}
	s.notify(true)
	return nil
}

func errorCode(err error) string {
	var coded interface{ Code() string }
	switch {
	case errors.As(err, &coded):
		return coded.Code()
	case errors.Is(err, ErrUnsupported):
		return "unsupported"
	}
	return "hook_failed"
}

func (s *SoftwareSource) halt() {
	s.mu.Lock()
	was := s.running
	s.running = false
	s.mu.Unlock()
	if !was {
		return
	}
	s.be.Stop()
	s.filter.Release()
	s.notify(false)
}

func (s *SoftwareSource) Close() {
	s.once.Do(func() {
		close(s.stop)
		s.Disable()
	})
}

func (s *SoftwareSource) Status() (bool, string, *Info, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return false, "", nil, time.Time{}
	}
	return true, "", SystemInfo(), s.since
}

func (s *SoftwareSource) Connected() bool { return s.isRunning() }

func (s *SoftwareSource) Error() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return ""
	}
	return s.lastErr
}

func (s *SoftwareSource) Warnings() []string {
	if w, ok := s.be.(Warner); ok && s.isRunning() {
		return w.Warnings()
	}
	return nil
}

func (s *SoftwareSource) SyncMask(bool) {
	if fn := s.handlers().Mask; fn != nil {
		s.filter.SetMask(fn())
	}
}

func (s *SoftwareSource) Keys(on bool) error {
	s.filter.SetReportAll(on)
	return nil
}

func (s *SoftwareSource) Tap(st hid.Stroke) error { return s.TapHeld(st, 0) }

func (s *SoftwareSource) TapHeld(st hid.Stroke, holdMS int) error {
	if !s.Connected() {
		return ErrNotRunning
	}
	return s.be.Inject(st, holdMS)
}

func (s *SoftwareSource) Consumer(usage uint16) error {
	if !s.Connected() {
		return ErrNotRunning
	}
	return s.be.Consumer(usage)
}

func (s *SoftwareSource) TypeText(text string) error {
	if !s.Connected() {
		return ErrNotRunning
	}
	return s.be.TypeText(text)
}

func (s *SoftwareSource) Pause() { s.filter.Pause() }

func (s *SoftwareSource) Resume() {
	s.filter.Resume()
	if r, ok := s.be.(Reinstaller); ok && s.isRunning() {
		r.Reinstall()
	}
}

func (s *SoftwareSource) Command(name string, _ int) ([]string, error) {
	switch name {
	case "ping":
		return []string{"PONG"}, nil
	case "stats":
		l := s.be.Latency()
		return []string{fmt.Sprintf("n=%d avg_us=%d max_us=%d captura=%d dropped=%d", l.Count, l.AvgUS, l.MaxUS, b2i(s.Connected()), s.filter.Dropped())}, nil
	}
	return nil, errors.New("command not available without a board")
}

func (s *SoftwareSource) Checks() []Check { return s.be.Checks() }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
