//go:build linux

package capture

import (
	"log"
	"os"
	"syscall"
	"time"
)

const idlePoll = 100 * time.Millisecond

type keyboard struct {
	s       *session
	path    string
	f       *os.File
	pending []inputEvent
	out     []inputEvent

	swallowed map[uint16]bool
	forwarded map[uint16]bool
	dropped   bool
}

func newKeyboard(s *session, path string, f *os.File) *keyboard {
	return &keyboard{s: s, path: path, f: f, swallowed: map[uint16]bool{}, forwarded: map[uint16]bool{}}
}

func (k *keyboard) run() {
	defer k.s.wg.Done()
	defer k.s.forget(k)
	defer k.f.Close()
	if !k.waitIdle() {
		return
	}
	k.drain()
	if err := ioctlValue(k.f, eviocgrab, 1); err != nil {
		log.Printf("capture: cannot take %s: %v", k.path, err)
		k.s.grabFailed(k.path, err)
		return
	}
	k.s.grabbed(k.path)
	k.read()
	k.releaseAll()
}

func (k *keyboard) waitIdle() bool {
	for {
		bits, err := pressedKeys(k.f)
		if err != nil {
			return false
		}
		if !anyBit(bits) {
			return true
		}
		select {
		case <-k.s.stop:
			return false
		case <-time.After(idlePoll):
		}
	}
}

func (k *keyboard) drain() {
	buf := make([]byte, eventSize*64)
	_ = control(k.f, func(fd uintptr) error {
		for {
			if n, err := syscall.Read(int(fd), buf); err != nil || n <= 0 {
				return err
			}
		}
	})
}

func (k *keyboard) read() {
	evs := make([]inputEvent, 64)
	raw := encodeEvents(evs)
	for {
		n, err := k.f.Read(raw)
		if err != nil {
			return
		}
		start := time.Now()
		k.handle(evs[:n/eventSize], start)
	}
}

func (k *keyboard) handle(evs []inputEvent, start time.Time) {
	for _, ev := range evs {
		switch {
		case ev.Type == evSyn && ev.Code == synDropped:
			k.dropped = true
			k.pending = k.pending[:0]
		case ev.Type == evSyn && ev.Code == synReport:
			if k.dropped {
				k.dropped = false
				k.resync()
			}
			k.flush(start)
		case !k.dropped:
			k.pending = append(k.pending, ev)
		}
	}
}

func (k *keyboard) flush(start time.Time) {
	out, sawKey := k.out[:0], false
	for _, ev := range k.pending {
		if ev.Type == evKey {
			sawKey = true
			if k.consume(ev) {
				out = dropScan(out)
				continue
			}
		}
		out = append(out, ev)
	}
	k.pending = k.pending[:0]
	if len(out) == 0 {
		return
	}
	out = append(out, inputEvent{Type: evSyn, Code: synReport})
	_ = k.s.virt.write(out)
	k.out = out[:0]
	if sawKey {
		k.s.lat.record(time.Since(start))
	}
}

func dropScan(out []inputEvent) []inputEvent {
	if n := len(out); n > 0 && out[n-1].Type == evMsc && out[n-1].Code == mscScan {
		return out[:n-1]
	}
	return out
}

func (k *keyboard) consume(ev inputEvent) bool {
	switch ev.Value {
	case 1:
		if k.s.filter.Handle(UsageFromEvdev(ev.Code), true) {
			k.swallowed[ev.Code] = true
			return true
		}
		k.forwarded[ev.Code] = true
		return false
	case 0:
		wasSwallowed := k.swallowed[ev.Code]
		delete(k.swallowed, ev.Code)
		delete(k.forwarded, ev.Code)
		k.s.filter.Handle(UsageFromEvdev(ev.Code), false)
		return wasSwallowed
	}
	return k.swallowed[ev.Code]
}

func (k *keyboard) resync() {
	bits, err := pressedKeys(k.f)
	if err != nil {
		return
	}
	for code := range k.known() {
		if !bitSet(bits, int(code)) {
			k.pending = append(k.pending, inputEvent{Type: evKey, Code: code})
		}
	}
	for code := 1; code <= keyMax; code++ {
		if bitSet(bits, code) && !k.swallowed[uint16(code)] && !k.forwarded[uint16(code)] {
			k.pending = append(k.pending, inputEvent{Type: evKey, Code: uint16(code), Value: 1})
		}
	}
}

func (k *keyboard) known() map[uint16]bool {
	all := map[uint16]bool{}
	for c := range k.swallowed {
		all[c] = true
	}
	for c := range k.forwarded {
		all[c] = true
	}
	return all
}

func (k *keyboard) releaseAll() {
	for code := range k.known() {
		k.pending = append(k.pending, inputEvent{Type: evKey, Code: code})
	}
	k.flush(time.Now())
}

func (k *keyboard) setLED(code uint16, value int32) {
	ev := []inputEvent{{Type: evLed, Code: code, Value: value}, {Type: evSyn, Code: synReport}}
	_, _ = k.f.Write(encodeEvents(ev))
}
