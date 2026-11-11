package capture

import (
	"testing"
	"time"
)

func maskOf(usages ...byte) [32]byte {
	var m [32]byte
	for _, u := range usages {
		m[u>>3] |= 1 << (u & 7)
	}
	return m
}

func drain(f *Filter) []Event {
	var out []Event
	for {
		select {
		case ev := <-f.Events():
			out = append(out, ev)
		default:
			return out
		}
	}
}

func kinds(evs []Event) string {
	var b []byte
	for _, e := range evs {
		b = append(b, e.Kind)
	}
	return string(b)
}

func TestCapturedKeyIsSwallowedAndReported(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0x04))
	if !f.Handle(0x04, true) || !f.Handle(0x04, false) {
		t.Fatal("captured key must be swallowed on both edges")
	}
	evs := drain(f)
	if len(evs) != 2 || evs[0] != (Event{Kind: 'D', Usage: 0x04}) || evs[1] != (Event{Kind: 'U', Usage: 0x04}) {
		t.Fatalf("events: %+v", evs)
	}
}

func TestUncapturedKeyPassesAndIsInvisible(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0x04))
	if f.Handle(0x05, true) || f.Handle(0x05, false) {
		t.Fatal("uncaptured key must pass")
	}
	if evs := drain(f); len(evs) != 0 {
		t.Fatalf("the program must not see keys it does not need: %+v", evs)
	}
}

func TestReportAllShowsPlainKeys(t *testing.T) {
	f := NewFilter()
	f.SetReportAll(true)
	if f.Handle(0x05, true) || f.Handle(0x05, false) {
		t.Fatal("reported keys still pass")
	}
	if got := kinds(drain(f)); got != "PR" {
		t.Fatalf("events: %s", got)
	}
}

func TestAutorepeatOfSwallowedKeyIsSwallowedWithoutEvents(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0x04))
	f.Handle(0x04, true)
	for i := 0; i < 5; i++ {
		if !f.Handle(0x04, true) {
			t.Fatal("autorepeat leaked")
		}
	}
	f.Handle(0x04, false)
	if got := kinds(drain(f)); got != "DU" {
		t.Fatalf("events: %s", got)
	}
}

func TestMaskChangeWhileHeldKeepsTheDecision(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0x04))
	f.Handle(0x04, true)
	f.SetMask(maskOf())
	if !f.Handle(0x04, true) {
		t.Fatal("autorepeat of a swallowed key leaked after the mask changed")
	}
	if !f.Handle(0x04, false) {
		t.Fatal("the key-up of a swallowed key must be swallowed even if the mask changed")
	}
	f.SetMask(maskOf())
	f.Handle(0x05, true)
	f.SetMask(maskOf(0x05))
	if f.Handle(0x05, true) {
		t.Fatal("a key the system already saw going down must keep passing")
	}
	if f.Handle(0x05, false) {
		t.Fatal("its key-up must pass, or the system keeps the key stuck")
	}
	if got := kinds(drain(f)); got != "DU" {
		t.Fatalf("events: %s", got)
	}
}

func TestModifiersTravelWithTheEvent(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0x04))
	f.Handle(0xE0, true)
	f.Handle(0xE5, true)
	f.Handle(0x04, true)
	f.Handle(0x04, false)
	f.Handle(0xE0, false)
	f.Handle(0x04, true)
	evs := drain(f)
	if evs[0].Mods != 0x21 {
		t.Fatalf("mods with ctrl and right shift: %02X", evs[0].Mods)
	}
	if evs[2].Mods != 0x20 {
		t.Fatalf("mods after releasing ctrl: %02X", evs[2].Mods)
	}
}

func TestSwallowedModifierIsTracked(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0xE3, 0x04))
	if !f.Handle(0xE3, true) {
		t.Fatal("captured modifier must be swallowed")
	}
	f.Handle(0x04, true)
	if evs := drain(f); evs[1].Mods != 0x08 {
		t.Fatalf("mods: %+v", evs)
	}
}

func TestUnknownKeyUpPasses(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0x04))
	if f.Handle(0x04, false) {
		t.Fatal("a key-up with no matching down (pressed before the hook) must pass")
	}
	if f.Handle(0, true) {
		t.Fatal("usage 0 is never handled")
	}
}

func TestPauseReleasesAndPassesEverything(t *testing.T) {
	f := NewFilter()
	f.SetMask(maskOf(0x04))
	f.Handle(0x04, true)
	f.Pause()
	if f.Handle(0x04, true) {
		t.Fatal("paused filter must pass")
	}
	f.Resume()
	if got := kinds(drain(f)); got != "DU" {
		t.Fatalf("pause must release what was held: %s", got)
	}
}

func TestFullQueueNeverBlocks(t *testing.T) {
	f := NewFilter()
	f.SetReportAll(true)
	done := make(chan struct{})
	go func() {
		for i := 0; i < eventBuffer+50; i++ {
			f.Handle(0x05, true)
			f.Handle(0x05, false)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the hook callback blocked on a full queue")
	}
	if f.Dropped() == 0 {
		t.Fatal("drops must be counted")
	}
}

func TestConcurrentMaskUpdates(t *testing.T) {
	f := NewFilter()
	stop := make(chan struct{})
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				f.SetMask(maskOf(byte(4 + i%20)))
				f.SetReportAll(i%2 == 0)
			}
		}
	}()
	for i := 0; i < 5000; i++ {
		u := byte(4 + i%20)
		f.Handle(u, true)
		f.Handle(u, false)
		drain(f)
	}
	close(stop)
}
