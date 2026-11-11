package capture

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type fakeBackend struct {
	mu       sync.Mutex
	startErr error
	alive    bool
	starts   int
	attempts int
	injected []hid.Stroke
	filter   *Filter
}

func (b *fakeBackend) Start(f *Filter) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempts++
	if b.startErr != nil {
		return b.startErr
	}
	b.alive, b.filter = true, f
	b.starts++
	return nil
}

func (b *fakeBackend) Alive() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.alive }
func (b *fakeBackend) Stop()       { b.mu.Lock(); b.alive = false; b.mu.Unlock() }
func (b *fakeBackend) Inject(s hid.Stroke, _ int) error {
	b.mu.Lock()
	b.injected = append(b.injected, s)
	b.mu.Unlock()
	return nil
}
func (b *fakeBackend) Consumer(uint16) error { return nil }
func (b *fakeBackend) TypeText(string) error { return nil }
func (b *fakeBackend) Latency() Latency      { return Latency{Count: 3, AvgUS: 5, MaxUS: 9} }
func (b *fakeBackend) Checks() []Check       { return nil }

type recorder struct {
	mu     sync.Mutex
	events []Event
	conn   int
	disc   int
	mask   [32]byte
}

func (r *recorder) handlers() Handlers {
	return Handlers{
		OnEvent:      func(ev Event) { r.mu.Lock(); r.events = append(r.events, ev); r.mu.Unlock() },
		Mask:         func() [32]byte { r.mu.Lock(); defer r.mu.Unlock(); return r.mask },
		OnConnected:  func(*Info) { r.mu.Lock(); r.conn++; r.mu.Unlock() },
		OnDisconnect: func() { r.mu.Lock(); r.disc++; r.mu.Unlock() },
	}
}

func (r *recorder) setMask(m [32]byte) { r.mu.Lock(); r.mask = m; r.mu.Unlock() }

func (r *recorder) snapshot() ([]Event, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...), r.conn, r.disc
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func newSoft(be *fakeBackend) (*SoftwareSource, *recorder) {
	s := NewSoftwareWith(be)
	r := &recorder{}
	s.Bind(r.handlers())
	go s.Run()
	return s, r
}

func TestSoftwareDeliversCapturedKeys(t *testing.T) {
	be := &fakeBackend{}
	s, r := newSoft(be)
	defer s.Close()
	r.setMask(maskOf(0x04))
	if err := s.Enable(); err != nil {
		t.Fatal(err)
	}
	if !s.Connected() {
		t.Fatal("not connected after Enable")
	}
	if !s.Filter().Handle(0x04, true) {
		t.Fatal("the mask of the engine must reach the filter on start")
	}
	s.Filter().Handle(0x04, false)
	waitFor(t, "events and the connected notice", func() bool { e, c, _ := r.snapshot(); return len(e) == 2 && c == 1 })
}

func TestSoftwareFollowsMaskAndKeysRequests(t *testing.T) {
	s, r := newSoft(&fakeBackend{})
	defer s.Close()
	_ = s.Enable()
	r.setMask(maskOf(0x10))
	s.SyncMask(false)
	if !s.Filter().Handle(0x10, true) {
		t.Fatal("new mask not applied")
	}
	_ = s.Keys(true)
	s.Filter().Handle(0x20, true)
	waitFor(t, "plain key", func() bool { e, _, _ := r.snapshot(); return len(e) == 2 && e[1].Kind == 'P' })
}

func TestSoftwareReportsHookFailureAndRetries(t *testing.T) {
	be := &fakeBackend{startErr: errors.New("denied")}
	s, r := newSoft(be)
	defer s.Close()
	if err := s.Enable(); err == nil {
		t.Fatal("expected an error")
	}
	if s.Connected() || s.Error() != "hook_failed" {
		t.Fatalf("connected=%v error=%q", s.Connected(), s.Error())
	}
	be.mu.Lock()
	be.startErr = nil
	be.mu.Unlock()
	s.wake <- struct{}{}
	waitFor(t, "retry", s.Connected)
	if s.Error() != "" {
		t.Fatalf("error after recovery: %q", s.Error())
	}
	waitFor(t, "connected callback", func() bool { _, c, _ := r.snapshot(); return c == 1 })
}

func TestSoftwareDisableReleasesAndNotifies(t *testing.T) {
	s, r := newSoft(&fakeBackend{})
	defer s.Close()
	r.setMask(maskOf(0x04))
	_ = s.Enable()
	s.Filter().Handle(0x04, true)
	s.Disable()
	if s.Connected() {
		t.Fatal("still connected")
	}
	waitFor(t, "release and disconnect", func() bool {
		e, _, d := r.snapshot()
		return d == 1 && len(e) == 2 && e[1].Kind == 'U'
	})
}

func TestSoftwareInjectionNeedsTheHook(t *testing.T) {
	be := &fakeBackend{}
	s, _ := newSoft(be)
	defer s.Close()
	if err := s.TapHeld(hid.Stroke{Usage: 0x04}, 10); err == nil {
		t.Fatal("injecting with the capture off must fail")
	}
	_ = s.Enable()
	if err := s.Tap(hid.Stroke{Usage: 0x04, Mods: 1}); err != nil || len(be.injected) != 1 {
		t.Fatalf("err=%v injected=%v", err, be.injected)
	}
}

func TestSoftwareIdentityAndStats(t *testing.T) {
	s, _ := newSoft(&fakeBackend{})
	defer s.Close()
	_ = s.Enable()
	ok, port, info, _ := s.Status()
	if !ok || port != "" || info == nil || !info.System || !info.Present || info.ID() != "system" {
		t.Fatalf("status: %v %q %+v", ok, port, info)
	}
	out, err := s.Command("stats", 0)
	if err != nil || out[0] != "n=3 avg_us=5 max_us=9 captura=1 dropped=0" {
		t.Fatalf("stats: %v %v", out, err)
	}
	if _, err := s.Command("reboot", 0); err == nil {
		t.Fatal("board commands do not exist in software mode")
	}
}

func TestSoftwareCloseIsIdempotent(t *testing.T) {
	s, _ := newSoft(&fakeBackend{})
	_ = s.Enable()
	s.Close()
	s.Close()
}

func TestSoftwareStatusNeverWaitsForAHookThatIsStopping(t *testing.T) {
	be := &slowStopBackend{fakeBackend: &fakeBackend{}, release: make(chan struct{}), stopping: make(chan struct{})}
	s := NewSoftwareWith(be)
	s.Bind((&recorder{}).handlers())
	_ = s.Enable()
	go s.Disable()
	<-be.stopping
	done := make(chan struct{})
	go func() { s.Status(); s.Connected(); _ = s.Error(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Status blocked while the hook was being stopped")
	}
	close(be.release)
}

type slowStopBackend struct {
	*fakeBackend
	release  chan struct{}
	stopping chan struct{}
}

func (b *slowStopBackend) Stop() {
	close(b.stopping)
	<-b.release
	b.fakeBackend.Stop()
}

func TestRetriesBackOffWhileTheHookKeepsFailing(t *testing.T) {
	be := &fakeBackend{startErr: errors.New("denied")}
	s := NewSoftwareWith(be)
	s.retryBase, s.retryCeiling = 5*time.Millisecond, 40*time.Millisecond
	s.Bind((&recorder{}).handlers())
	go s.Run()
	defer s.Close()
	_ = s.Enable()
	time.Sleep(400 * time.Millisecond)
	be.mu.Lock()
	n := be.attempts
	be.mu.Unlock()
	if n < 3 || n > 16 {
		t.Fatalf("%d start attempts in 400 ms: the delay must grow up to its ceiling", n)
	}
}

func TestNextRetryDoublesUpToTheCeiling(t *testing.T) {
	d := retryBase
	var seen []time.Duration
	for i := 0; i < 8; i++ {
		d = nextRetry(d, retryCeiling)
		seen = append(seen, d)
	}
	if seen[0] != 2*retryBase || seen[len(seen)-1] != retryCeiling {
		t.Fatalf("%v", seen)
	}
}

func TestStateNoticesArriveInOrder(t *testing.T) {
	s, r := newSoft(&fakeBackend{})
	defer s.Close()
	var mu sync.Mutex
	var order []string
	h := r.handlers()
	h.OnConnected = func(*Info) { mu.Lock(); order = append(order, "up"); mu.Unlock() }
	h.OnDisconnect = func() { mu.Lock(); order = append(order, "down"); mu.Unlock() }
	s.Bind(h)
	for i := 0; i < 5; i++ {
		_ = s.Enable()
		s.Disable()
	}
	waitFor(t, "ten notices", func() bool { mu.Lock(); defer mu.Unlock(); return len(order) == 10 })
	for i, got := range order {
		if want := []string{"up", "down"}[i%2]; got != want {
			t.Fatalf("notice %d is %s, want %s: %v", i, got, want, order)
		}
	}
}

func TestInjectionWithoutTheHookSaysItIsNotRunning(t *testing.T) {
	s, _ := newSoft(&fakeBackend{})
	defer s.Close()
	if err := s.Tap(hid.Stroke{Usage: 4}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("got %v", err)
	}
}
