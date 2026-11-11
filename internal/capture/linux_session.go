//go:build linux

package capture

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	inputDir   = "/dev/input"
	settleWait = 150 * time.Millisecond
	retryWait  = 3 * time.Second
)

type latencyStats struct {
	count   atomic.Uint64
	totalNS atomic.Int64
	maxNS   atomic.Int64
}

func (l *latencyStats) record(d time.Duration) {
	ns := d.Nanoseconds()
	l.count.Add(1)
	l.totalNS.Add(ns)
	for {
		cur := l.maxNS.Load()
		if ns <= cur || l.maxNS.CompareAndSwap(cur, ns) {
			return
		}
	}
}

func (l *latencyStats) snapshot() Latency {
	n := l.count.Load()
	out := Latency{Count: n, MaxUS: l.maxNS.Load() / 1000}
	if n > 0 {
		out.AvgUS = l.totalNS.Load() / int64(n) / 1000
	}
	return out
}

type session struct {
	filter *Filter
	virt   *virtualKeyboard
	lat    *latencyStats
	stop   chan struct{}
	done   chan struct{}
	wg     sync.WaitGroup

	mu       sync.Mutex
	closed   bool
	boards   map[string]*keyboard
	grabFail map[string]string
}

var (
	probeInputs = probeKeyboards
	makeVirtual = newVirtualKeyboard
)

func startSession(f *Filter, lat *latencyStats) (*session, error) {
	s := &session{filter: f, lat: lat, stop: make(chan struct{}), done: make(chan struct{}), boards: map[string]*keyboard{}, grabFail: map[string]string{}}
	if readable, denied := probeInputs(); readable == 0 && denied > 0 {
		return nil, errReadAccess
	}
	virt, err := makeVirtual(s.setLED)
	if err != nil {
		return nil, uinputError(err)
	}
	s.virt = virt
	_, denied := s.scan()
	watch, err := watchInputDir()
	if err != nil {
		s.close()
		return nil, err
	}
	go s.watch(watch, denied > 0)
	return s, nil
}

func (s *session) alive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *session) scan() (opened, denied int) {
	names, _ := filepath.Glob(filepath.Join(inputDir, "event*"))
	for _, path := range names {
		if s.tracked(path) {
			opened++
			continue
		}
		f, err := openEvdev(path)
		switch {
		case errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM):
			denied++
		case err != nil:
		case !isKeyboard(f):
			f.Close()
		default:
			s.add(path, f)
			opened++
		}
	}
	return opened, denied
}

func (s *session) tracked(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.boards[path]
	return ok
}

func (s *session) add(path string, f *os.File) {
	k := newKeyboard(s, path, f)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		f.Close()
		return
	}
	s.boards[path] = k
	s.wg.Add(1)
	go k.run()
}

func (s *session) forget(k *keyboard) {
	s.mu.Lock()
	if s.boards[k.path] == k {
		delete(s.boards, k.path)
	}
	s.mu.Unlock()
}

func (s *session) grabFailed(path string, err error) {
	s.mu.Lock()
	s.grabFail[path] = err.Error()
	s.mu.Unlock()
}

func (s *session) grabbed(path string) {
	s.mu.Lock()
	delete(s.grabFail, path)
	s.mu.Unlock()
}

func (s *session) grabFailures() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.grabFail))
	for p, e := range s.grabFail {
		out[p] = e
	}
	return out
}

func (s *session) setLED(code uint16, value int32) {
	s.mu.Lock()
	boards := make([]*keyboard, 0, len(s.boards))
	for _, k := range s.boards {
		boards = append(boards, k)
	}
	s.mu.Unlock()
	for _, k := range boards {
		k.setLED(code, value)
	}
}

func watchInputDir() (*os.File, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC)
	if err != nil {
		return nil, err
	}
	mask := uint32(syscall.IN_CREATE | syscall.IN_DELETE | syscall.IN_ATTRIB)
	if _, err := syscall.InotifyAddWatch(fd, inputDir, mask); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "inotify"), nil
}

func (s *session) watch(w *os.File, retry bool) {
	defer close(s.done)
	changed := make(chan struct{}, 1)
	go readChanges(w, changed)
	var timer <-chan time.Time
	if retry {
		timer = time.After(retryWait)
	}
	for {
		select {
		case <-s.stop:
			w.Close()
			return
		case <-changed:
			timer = time.After(settleWait)
		case <-timer:
			_, denied := s.scan()
			timer = nil
			if denied > 0 {
				timer = time.After(retryWait)
			}
		}
	}
}

func readChanges(w *os.File, changed chan<- struct{}) {
	buf := make([]byte, 4096)
	for {
		if _, err := w.Read(buf); err != nil {
			return
		}
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}

func (s *session) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.stop)
	for _, k := range s.boards {
		k.f.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	s.virt.close()
}

type codedError struct {
	code string
	msg  string
}

func (e *codedError) Error() string { return e.msg }
func (e *codedError) Code() string  { return e.code }

func uinputError(err error) error {
	switch {
	case errors.Is(err, syscall.ENOENT):
		return &codedError{"no_uinput", "the uinput module is not loaded: run 'sudo modprobe uinput'"}
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return &codedError{"uinput_access", noUinputAccess}
	}
	return err
}

const accessHint = "add your user to the input group ('sudo usermod -aG input $USER') and sign in again; the package installs the udev rule that gives that group access"

var (
	noUinputAccess = "no permission to open /dev/uinput: " + accessHint
	noReadAccess   = "no permission to read the keyboards in /dev/input: " + accessHint
	errReadAccess  = &codedError{"input_access", noReadAccess}
)
