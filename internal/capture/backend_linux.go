//go:build linux

package capture

import (
	"fmt"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

const defaultTapMS = 12

type linuxBackend struct {
	mu     sync.Mutex
	sess   *session
	lat    latencyStats
	inject sync.Mutex
}

func newBackend() Backend { return &linuxBackend{} }

func (b *linuxBackend) current() *session {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sess
}

func (b *linuxBackend) Start(f *Filter) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sess != nil && b.sess.alive() {
		return nil
	}
	if b.sess != nil {
		b.sess.close()
		b.sess = nil
	}
	s, err := startSession(f, &b.lat)
	if err != nil {
		return err
	}
	b.sess = s
	signal.Ignore(syscall.SIGTSTP)
	platform.SetPasteKeys(b.pasteKeys)
	return nil
}

func (b *linuxBackend) Alive() bool {
	s := b.current()
	return s != nil && s.alive()
}

func (b *linuxBackend) Stop() {
	b.mu.Lock()
	s := b.sess
	b.sess = nil
	b.mu.Unlock()
	platform.SetPasteKeys(nil)
	signal.Reset(syscall.SIGTSTP)
	if s != nil {
		s.close()
	}
}

func (b *linuxBackend) Warnings() []string {
	if s := b.current(); s != nil && len(s.grabFailures()) > 0 {
		return []string{"grab_failed"}
	}
	return nil
}

func (b *linuxBackend) Latency() Latency { return b.lat.snapshot() }

func (b *linuxBackend) Inject(st hid.Stroke, holdMS int) error {
	codes, err := strokeCodes(st)
	if err != nil || len(codes) == 0 {
		return err
	}
	s := b.current()
	if s == nil {
		return ErrNotRunning
	}
	b.inject.Lock()
	defer b.inject.Unlock()
	err = s.virt.keys(codes, 1)
	if holdMS <= 0 {
		holdMS = defaultTapMS
	}
	time.Sleep(time.Duration(holdMS) * time.Millisecond)
	if rel := s.virt.keys(reversed(codes), 0); err == nil {
		err = rel
	}
	return err
}

func (b *linuxBackend) Consumer(usage uint16) error {
	code, ok := ConsumerEvdev(usage)
	if !ok {
		return fmt.Errorf("media key %X has no Linux equivalent", usage)
	}
	s := b.current()
	if s == nil {
		return ErrNotRunning
	}
	b.inject.Lock()
	defer b.inject.Unlock()
	err := s.virt.keys([]uint16{code}, 1)
	if rel := s.virt.keys([]uint16{code}, 0); err == nil {
		err = rel
	}
	return err
}

func (b *linuxBackend) TypeText(text string) error {
	return platform.Current.Paste(text)
}

func (b *linuxBackend) pasteKeys() error {
	return b.Inject(hid.Stroke{Mods: hid.ModCtrl, Usage: 0x19}, 0)
}

func strokeCodes(st hid.Stroke) ([]uint16, error) {
	var codes []uint16
	for bit := byte(0); bit < 8; bit++ {
		if st.Mods&(1<<bit) != 0 {
			c, _ := EvdevFromUsage(0xE0 + bit)
			codes = append(codes, c)
		}
	}
	if st.Usage != 0 {
		c, ok := EvdevFromUsage(st.Usage)
		if !ok {
			return nil, fmt.Errorf("key %02X has no Linux equivalent", st.Usage)
		}
		codes = append(codes, c)
	}
	return codes, nil
}

func reversed(codes []uint16) []uint16 {
	out := make([]uint16, len(codes))
	for i, c := range codes {
		out[len(codes)-1-i] = c
	}
	return out
}
