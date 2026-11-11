package capture

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type fakeBoard struct {
	mu        sync.Mutex
	h         Handlers
	connected bool
	syncs     int
	keys      []bool
	taps      int
}

func (b *fakeBoard) Kind() string { return Board }
func (b *fakeBoard) Bind(h Handlers) {
	b.mu.Lock()
	b.h = h
	b.mu.Unlock()
}
func (b *fakeBoard) Run()   {}
func (b *fakeBoard) Close() {}
func (b *fakeBoard) Status() (bool, string, *Info, time.Time) {
	return b.connected, "COM3", &Info{Present: true, VID: "2341", PID: "8036"}, time.Time{}
}
func (b *fakeBoard) Connected() bool { return b.connected }
func (b *fakeBoard) Error() string   { return "" }
func (b *fakeBoard) SyncMask(bool) {
	b.mu.Lock()
	b.syncs++
	b.mu.Unlock()
}
func (b *fakeBoard) Keys(on bool) error {
	b.mu.Lock()
	b.keys = append(b.keys, on)
	b.mu.Unlock()
	return nil
}
func (b *fakeBoard) Tap(hid.Stroke) error                  { b.taps++; return nil }
func (b *fakeBoard) TapHeld(hid.Stroke, int) error         { b.taps++; return nil }
func (b *fakeBoard) Consumer(uint16) error                 { return nil }
func (b *fakeBoard) SetLayer(int)                          {}
func (b *fakeBoard) Pause()                                {}
func (b *fakeBoard) Resume()                               {}
func (b *fakeBoard) Command(string, int) ([]string, error) { return []string{"OK"}, nil }
func (b *fakeBoard) Do(string, bool, time.Duration) ([]string, error) {
	return nil, nil
}
func (b *fakeBoard) Sys() (Sys, error) { return Sys{Board: "leonardo"}, nil }

func (b *fakeBoard) emit(ev Event) { b.mu.Lock(); h := b.h; b.mu.Unlock(); h.OnEvent(ev) }
func (b *fakeBoard) mask() [32]byte {
	b.mu.Lock()
	h := b.h
	b.mu.Unlock()
	return h.Mask()
}

func newSelector(t *testing.T) (*Selector, *fakeBoard, *SoftwareSource, *recorder) {
	t.Helper()
	fb := &fakeBoard{connected: true}
	soft := NewSoftwareWith(&fakeBackend{})
	sel := NewSelector(fb, soft)
	r := &recorder{}
	r.setMask(maskOf(0x04))
	sel.Bind(r.handlers())
	go soft.Run()
	t.Cleanup(soft.Close)
	return sel, fb, soft, r
}

func TestSelectorStartsOnTheBoard(t *testing.T) {
	sel, fb, _, r := newSelector(t)
	if sel.Active() != Board || sel.Kind() != Board {
		t.Fatal("must default to the board")
	}
	fb.emit(Event{Kind: 'D', Usage: 4})
	if e, _, _ := r.snapshot(); len(e) != 1 {
		t.Fatalf("board events: %v", e)
	}
	if fb.mask() != maskOf(0x04) {
		t.Fatal("board gets the engine mask")
	}
}

func TestSwitchingToSoftwareSilencesTheBoard(t *testing.T) {
	sel, fb, soft, r := newSelector(t)
	if err := sel.Select(Software); err != nil {
		t.Fatal(err)
	}
	if sel.Active() != Software || !sel.Connected() {
		t.Fatal("software must be active and connected")
	}
	if fb.mask() != ([32]byte{}) {
		t.Fatal("an idle board must not capture anything")
	}
	fb.emit(Event{Kind: 'D', Usage: 4})
	if e, _, _ := r.snapshot(); len(e) != 0 {
		t.Fatalf("the idle board leaked events: %v", e)
	}
	if len(fb.keys) == 0 || fb.keys[len(fb.keys)-1] {
		t.Fatal("the idle board must stop reporting all keys")
	}
	soft.Filter().Handle(0x04, true)
	waitFor(t, "software event", func() bool { e, _, _ := r.snapshot(); return len(e) == 1 })
}

func TestSwitchingBackRestoresTheBoardAndNotifies(t *testing.T) {
	sel, fb, _, r := newSelector(t)
	_ = sel.Select(Software)
	waitFor(t, "software notification", func() bool { _, c, _ := r.snapshot(); return c == 1 })
	before := fb.syncs
	if err := sel.Select(Board); err != nil {
		t.Fatal(err)
	}
	if sel.Active() != Board || fb.syncs == before {
		t.Fatal("the mask of the board must be pushed again")
	}
	if fb.mask() != maskOf(0x04) {
		t.Fatal("board mask not restored")
	}
	waitFor(t, "connected notification for the board", func() bool { _, c, _ := r.snapshot(); return c >= 2 })
}

func TestSelectFailureFallsBackToTheBoard(t *testing.T) {
	fb := &fakeBoard{connected: true}
	be := &fakeBackend{startErr: errors.New("denied")}
	soft := NewSoftwareWith(be)
	sel := NewSelector(fb, soft)
	r := &recorder{}
	r.setMask(maskOf(0x04))
	sel.Bind(r.handlers())
	if err := sel.Select(Software); err == nil {
		t.Fatal("expected the hook error")
	}
	if sel.Active() != Board {
		t.Fatal("a failed switch must leave the board active")
	}
	if fb.mask() != maskOf(0x04) {
		t.Fatal("the board must get its mask back")
	}
	be.mu.Lock()
	be.startErr = nil
	be.mu.Unlock()
	soft.supervise(time.Second)
	if soft.Connected() {
		t.Fatal("the failed software source must not keep retrying in the background")
	}
	if sel.Select("quantum") == nil {
		t.Fatal("unknown modes are rejected")
	}
}

func TestConcurrentSelectsLeaveAConsistentState(t *testing.T) {
	sel, _, soft, _ := newSelector(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = sel.Select(Software) }()
		go func() { defer wg.Done(); _ = sel.Select(Board) }()
	}
	wg.Wait()
	_ = sel.Select(Board)
	if sel.Active() != Board || soft.Connected() || soft.isWanted() {
		t.Fatalf("active=%s connected=%v wanted=%v", sel.Active(), soft.Connected(), soft.isWanted())
	}
	_ = sel.Select(Software)
	if sel.Active() != Software || !soft.Connected() {
		t.Fatal("software must be running after the last request")
	}
}

type pauseBoard struct {
	fakeBoard
	paused, resumed int
}

func (b *pauseBoard) Pause()  { b.paused++ }
func (b *pauseBoard) Resume() { b.resumed++ }

func TestPauseAndResumeReachBothSources(t *testing.T) {
	pb := &pauseBoard{fakeBoard: fakeBoard{connected: true}}
	soft := NewSoftwareWith(&fakeBackend{})
	sel := NewSelector(pb, soft)
	sel.Bind((&recorder{}).handlers())
	_ = sel.Select(Software)
	soft.Filter().SetMask(maskOf(0x04))
	sel.Pause()
	if soft.Filter().Handle(0x04, true) {
		t.Fatal("the paused software filter must pass keys")
	}
	_ = sel.Select(Board)
	sel.Resume()
	if pb.paused != 1 || pb.resumed != 1 {
		t.Fatalf("board paused=%d resumed=%d", pb.paused, pb.resumed)
	}
	if !soft.Filter().Handle(0x04, true) {
		t.Fatal("a mode change between pause and resume must not leave the software source paused")
	}
}

func TestBoardOnlyCallsFailCleanlyInSoftwareMode(t *testing.T) {
	sel, _, _, _ := newSelector(t)
	_ = sel.Select(Software)
	if _, err := sel.Do("WHO", false, time.Second); err == nil {
		t.Fatal("raw board lines need the board")
	}
	if _, err := sel.Sys(); err == nil {
		t.Fatal("board system data needs the board")
	}
	if out, err := sel.Command("ping", 0); err != nil || out[0] != "PONG" {
		t.Fatalf("%v %v", out, err)
	}
}

func TestSelectorRoutesInjectionToTheActiveSource(t *testing.T) {
	sel, fb, _, _ := newSelector(t)
	_ = sel.Tap(hid.Stroke{Usage: 4})
	if fb.taps != 1 {
		t.Fatal("board active: injection goes to the board")
	}
	_ = sel.Select(Software)
	_ = sel.Tap(hid.Stroke{Usage: 4})
	if fb.taps != 1 {
		t.Fatal("software active: the board must not be used")
	}
	if err := sel.TypeText("ñ"); err != nil {
		t.Fatal(err)
	}
}
