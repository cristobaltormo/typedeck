package capture

import (
	"sync"
	"sync/atomic"
)

const (
	held      = 1
	swallowed = 2
	reported  = 4
)

const eventBuffer = 1024

type Filter struct {
	mask      [4]atomic.Uint64
	reportAll atomic.Bool
	paused    atomic.Bool
	dropped   atomic.Uint64
	events    chan Event
	changed   chan struct{}

	mu    sync.Mutex
	state [256]byte
	mods  byte
}

func NewFilter() *Filter {
	return &Filter{events: make(chan Event, eventBuffer), changed: make(chan struct{}, 1)}
}

func (f *Filter) Changed() <-chan struct{} { return f.changed }

func (f *Filter) notify() {
	select {
	case f.changed <- struct{}{}:
	default:
	}
}

type Snapshot struct {
	Mask      [32]byte
	ReportAll bool
	Inactive  bool
}

func (f *Filter) Snapshot() Snapshot {
	var s Snapshot
	for i := range f.mask {
		w := f.mask[i].Load()
		for b := 0; b < 8; b++ {
			s.Mask[i*8+b] = byte(w >> (8 * b))
		}
	}
	s.ReportAll = f.reportAll.Load()
	s.Inactive = f.paused.Load()
	return s
}

func (f *Filter) Observe(kind, usage, mods byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.state[usage]
	switch kind {
	case 'D':
		if st&held != 0 {
			return
		}
		f.state[usage] = held | swallowed
		f.emit(Event{Kind: 'D', Usage: usage, Mods: mods})
	case 'P':
		if st&held != 0 {
			return
		}
		f.state[usage] = held | reported
		f.emit(Event{Kind: 'P', Usage: usage})
	case 'U':
		if st&swallowed != 0 {
			f.state[usage] = 0
			f.emit(Event{Kind: 'U', Usage: usage})
		}
	case 'R':
		if st&reported != 0 {
			f.state[usage] = 0
			f.emit(Event{Kind: 'R', Usage: usage})
		}
	}
}

func (f *Filter) Events() <-chan Event { return f.events }

func (f *Filter) Dropped() uint64 { return f.dropped.Load() }

func (f *Filter) SetMask(m [32]byte) {
	for i := range f.mask {
		var w uint64
		for b := 0; b < 8; b++ {
			w |= uint64(m[i*8+b]) << (8 * b)
		}
		f.mask[i].Store(w)
	}
	f.notify()
}

func (f *Filter) SetReportAll(on bool) {
	f.reportAll.Store(on)
	f.notify()
}

func (f *Filter) captured(u byte) bool {
	return f.mask[u>>6].Load()>>(u&63)&1 == 1
}

func (f *Filter) Handle(u byte, down bool) bool {
	if u == 0 || f.paused.Load() {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if down {
		return f.press(u)
	}
	return f.release(u)
}

func (f *Filter) press(u byte) bool {
	st := f.state[u]
	if st&held != 0 {
		return st&swallowed != 0
	}
	f.trackMods(u, true)
	switch {
	case f.captured(u):
		f.state[u] = held | swallowed
		f.emit(Event{Kind: 'D', Usage: u, Mods: f.mods})
		return true
	case f.reportAll.Load():
		f.state[u] = held | reported
		f.emit(Event{Kind: 'P', Usage: u})
	default:
		f.state[u] = held
	}
	return false
}

func (f *Filter) release(u byte) bool {
	st := f.state[u]
	if st&held == 0 {
		return false
	}
	f.state[u] = 0
	f.trackMods(u, false)
	switch {
	case st&swallowed != 0:
		f.emit(Event{Kind: 'U', Usage: u})
		return true
	case st&reported != 0:
		f.emit(Event{Kind: 'R', Usage: u})
	}
	return false
}

func (f *Filter) trackMods(u byte, down bool) {
	if u < 0xE0 || u > 0xE7 {
		return
	}
	bit := byte(1) << (u - 0xE0)
	if down {
		f.mods |= bit
	} else {
		f.mods &^= bit
	}
}

func (f *Filter) emit(ev Event) {
	select {
	case f.events <- ev:
	default:
		f.dropped.Add(1)
	}
}

func (f *Filter) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for u := range f.state {
		st := f.state[u]
		switch {
		case st&swallowed != 0:
			f.emit(Event{Kind: 'U', Usage: byte(u)})
		case st&reported != 0:
			f.emit(Event{Kind: 'R', Usage: byte(u)})
		}
		f.state[u] = 0
	}
	f.mods = 0
}

func (f *Filter) Pause() {
	f.paused.Store(true)
	f.notify()
	f.Release()
}

func (f *Filter) Resume() {
	f.paused.Store(false)
	f.notify()
}
