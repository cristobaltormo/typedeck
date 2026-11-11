package board

import (
	"io"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	pCreateEvent      = kernel32.NewProc("CreateEventW")
	pGetOverlappedRes = kernel32.NewProc("GetOverlappedResult")
	pCancelIoEx       = kernel32.NewProc("CancelIoEx")
)

const ioBufferSize = 4096

type overlappedFile struct {
	h      syscall.Handle
	mu     sync.RWMutex
	rmu    sync.Mutex
	wmu    sync.Mutex
	closed atomic.Bool

	rov, wov   syscall.Overlapped
	rbuf, wbuf []byte
}

func newOverlappedFile(h syscall.Handle) (*overlappedFile, error) {
	f := &overlappedFile{h: h, rbuf: make([]byte, ioBufferSize)}
	for _, ov := range []*syscall.Overlapped{&f.rov, &f.wov} {
		ev, _, err := pCreateEvent.Call(0, 1, 0, 0)
		if ev == 0 {
			return nil, err
		}
		ov.HEvent = syscall.Handle(ev)
	}
	return f, nil
}

func (f *overlappedFile) finish(ov *syscall.Overlapped, err error) (int, error) {
	var n uint32
	if err == nil {
		r, _, e := pGetOverlappedRes.Call(uintptr(f.h), uintptr(unsafe.Pointer(ov)), uintptr(unsafe.Pointer(&n)), 0)
		if r == 0 {
			err = e
		}
		return int(n), err
	}
	if err != syscall.ERROR_IO_PENDING {
		return 0, err
	}
	r, _, e := pGetOverlappedRes.Call(uintptr(f.h), uintptr(unsafe.Pointer(ov)), uintptr(unsafe.Pointer(&n)), 1)
	if r == 0 {
		return int(n), e
	}
	return int(n), nil
}

func (f *overlappedFile) Read(b []byte) (int, error) {
	for {
		if f.closed.Load() {
			return 0, io.EOF
		}
		n, err := f.readLocked(b)
		if f.closed.Load() {
			return 0, io.EOF
		}
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return n, nil
		}
	}
}

func (f *overlappedFile) readLocked(b []byte) (int, error) {
	f.rmu.Lock()
	defer f.rmu.Unlock()
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed.Load() {
		return 0, io.EOF
	}
	return f.readOnce(b)
}

func (f *overlappedFile) readOnce(b []byte) (int, error) {
	size := min(len(b), len(f.rbuf))
	if size == 0 {
		return 0, nil
	}
	f.rov.Internal, f.rov.InternalHigh, f.rov.Offset, f.rov.OffsetHigh = 0, 0, 0, 0
	err := syscall.ReadFile(f.h, f.rbuf[:size], nil, &f.rov)
	n, err := f.finish(&f.rov, err)
	copy(b, f.rbuf[:n])
	return n, err
}

func (f *overlappedFile) Write(b []byte) (int, error) {
	if f.closed.Load() {
		return 0, errPortClosed
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed.Load() {
		return 0, errPortClosed
	}
	f.wmu.Lock()
	defer f.wmu.Unlock()
	if cap(f.wbuf) < len(b) {
		f.wbuf = make([]byte, len(b))
	}
	f.wbuf = f.wbuf[:len(b)]
	copy(f.wbuf, b)
	f.wov.Internal, f.wov.InternalHigh, f.wov.Offset, f.wov.OffsetHigh = 0, 0, 0, 0
	err := syscall.WriteFile(f.h, f.wbuf, nil, &f.wov)
	n, err := f.finish(&f.wov, err)
	return n, checkWritten(n, len(b), err)
}

func (f *overlappedFile) Close() error {
	if !f.closed.CompareAndSwap(false, true) {
		return nil
	}
	for !f.mu.TryLock() {
		pCancelIoEx.Call(uintptr(f.h), 0)
		time.Sleep(time.Millisecond)
	}
	defer f.mu.Unlock()
	syscall.CloseHandle(f.rov.HEvent)
	syscall.CloseHandle(f.wov.HEvent)
	return syscall.CloseHandle(f.h)
}
