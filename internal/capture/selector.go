package capture

import (
	"errors"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type Soft interface {
	Source
	Enable() error
	Disable()
}

type Selector struct {
	board Source
	soft  Soft

	sel    sync.Mutex
	mu     sync.RWMutex
	active string
	h      Handlers
}

func NewSelector(board Source, soft Soft) *Selector {
	return &Selector{board: board, soft: soft, active: Board}
}

func (s *Selector) current() Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.active == Software {
		return s.soft
	}
	return s.board
}

func (s *Selector) Active() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

func (s *Selector) isActive(kind string) bool { return s.Active() == kind }

func (s *Selector) Kind() string { return s.Active() }

func (s *Selector) Bind(h Handlers) {
	s.mu.Lock()
	s.h = h
	s.mu.Unlock()
	s.board.Bind(s.handlersFor(Board))
	s.soft.Bind(s.handlersFor(Software))
}

func (s *Selector) handlersFor(kind string) Handlers {
	return Handlers{
		OnEvent: func(ev Event) {
			if fn := s.handlers().OnEvent; fn != nil && s.isActive(kind) {
				fn(ev)
			}
		},
		Mask: func() [32]byte {
			if fn := s.handlers().Mask; fn != nil && s.isActive(kind) {
				return fn()
			}
			return [32]byte{}
		},
		OnConnected: func(i *Info) {
			if fn := s.handlers().OnConnected; fn != nil && s.isActive(kind) {
				fn(i)
			}
		},
		OnDisconnect: func() {
			if fn := s.handlers().OnDisconnect; fn != nil && s.isActive(kind) {
				fn()
			}
		},
	}
}

func (s *Selector) handlers() Handlers {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.h
}

func (s *Selector) swap(kind string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.active
	s.active = kind
	return prev
}

func (s *Selector) Select(kind string) error {
	if kind != Board && kind != Software {
		return errors.New("unknown capture mode")
	}
	s.sel.Lock()
	defer s.sel.Unlock()
	if s.swap(kind) == kind {
		return nil
	}
	if kind == Board {
		s.backToBoard()
		return nil
	}
	s.board.SyncMask(true)
	_ = s.board.Keys(false)
	if err := s.soft.Enable(); err != nil {
		s.backToBoard()
		return err
	}
	return nil
}

func (s *Selector) backToBoard() {
	s.swap(Board)
	s.soft.Disable()
	s.board.SyncMask(true)
	s.notifyCurrent()
}

func (s *Selector) notifyCurrent() {
	h := s.handlers()
	connected, _, info, _ := s.board.Status()
	switch {
	case connected && info != nil && h.OnConnected != nil:
		h.OnConnected(info)
	case !connected && h.OnDisconnect != nil:
		h.OnDisconnect()
	}
}

func (s *Selector) Run() {
	go s.soft.Run()
	s.board.Run()
}

func (s *Selector) Close() {
	s.soft.Close()
	s.board.Close()
}

func (s *Selector) Status() (bool, string, *Info, time.Time) { return s.current().Status() }
func (s *Selector) Connected() bool                          { return s.current().Connected() }
func (s *Selector) Error() string                            { return s.current().Error() }
func (s *Selector) SyncMask(force bool)                      { s.current().SyncMask(force) }
func (s *Selector) Keys(on bool) error                       { return s.current().Keys(on) }
func (s *Selector) Tap(st hid.Stroke) error                  { return s.current().Tap(st) }
func (s *Selector) TapHeld(st hid.Stroke, ms int) error      { return s.current().TapHeld(st, ms) }
func (s *Selector) Consumer(usage uint16) error              { return s.current().Consumer(usage) }

func (s *Selector) Pause() {
	s.board.Pause()
	s.soft.Pause()
}

func (s *Selector) Resume() {
	s.board.Resume()
	s.soft.Resume()
}

func (s *Selector) SetLayer(i int) {
	if l, ok := s.current().(LayerSetter); ok {
		l.SetLayer(i)
	}
}

func (s *Selector) Sys() (Sys, error) {
	if r, ok := s.current().(SysReporter); ok {
		return r.Sys()
	}
	return Sys{}, errNeedsBoard
}

func (s *Selector) Command(name string, arg int) ([]string, error) {
	if c, ok := s.current().(Commander); ok {
		return c.Command(name, arg)
	}
	return nil, errNeedsBoard
}

func (s *Selector) Do(cmd string, multi bool, timeout time.Duration) ([]string, error) {
	if d, ok := s.current().(RawDoer); ok {
		return d.Do(cmd, multi, timeout)
	}
	return nil, errNeedsBoard
}

func (s *Selector) Warnings() []string {
	if w, ok := s.current().(Warner); ok {
		return w.Warnings()
	}
	return nil
}

var errNeedsBoard = errors.New("not available without a board")

func (s *Selector) TypeText(text string) error {
	if t, ok := s.current().(TextTyper); ok {
		return t.TypeText(text)
	}
	return errors.New("not supported")
}
