package board

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	pGetCommState    = kernel32.NewProc("GetCommState")
	pSetCommState    = kernel32.NewProc("SetCommState")
	pSetCommTimeouts = kernel32.NewProc("SetCommTimeouts")
	pEscapeCommFunc  = kernel32.NewProc("EscapeCommFunction")
	pPurgeComm       = kernel32.NewProc("PurgeComm")
	errPortClosed    = errors.New("port closed")
)

type dcb struct {
	DCBlength  uint32
	BaudRate   uint32
	Flags      uint32
	wReserved  uint16
	XonLim     uint16
	XoffLim    uint16
	ByteSize   uint8
	Parity     uint8
	StopBits   uint8
	XonChar    int8
	XoffChar   int8
	ErrorChar  int8
	EofChar    int8
	EvtChar    int8
	wReserved1 uint16
}

type commTimeouts struct {
	ReadIntervalTimeout         uint32
	ReadTotalTimeoutMultiplier  uint32
	ReadTotalTimeoutConstant    uint32
	WriteTotalTimeoutMultiplier uint32
	WriteTotalTimeoutConstant   uint32
}

type comPort struct {
	h      syscall.Handle
	mu     sync.RWMutex
	closed atomic.Bool
}

// openPort opens a COM port raw at 115200 baud with DTR asserted: the board only talks when DTR is on. Reads time out after
// 100 ms and loop, so Close never waits on a blocked read.
func openPort(path string) (io.ReadWriteCloser, error) {
	name, err := syscall.UTF16PtrFromString(`\\.\` + path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	var d dcb
	d.DCBlength = uint32(unsafe.Sizeof(d))
	if r, _, e := pGetCommState.Call(uintptr(h), uintptr(unsafe.Pointer(&d))); r == 0 {
		syscall.CloseHandle(h)
		return nil, e
	}
	d.BaudRate = 115200
	d.ByteSize, d.Parity, d.StopBits = 8, 0, 0
	d.Flags = 1 | 1<<4 | 1<<12
	if r, _, e := pSetCommState.Call(uintptr(h), uintptr(unsafe.Pointer(&d))); r == 0 {
		syscall.CloseHandle(h)
		return nil, e
	}
	t := commTimeouts{ReadIntervalTimeout: 0xFFFFFFFF, ReadTotalTimeoutMultiplier: 0xFFFFFFFF, ReadTotalTimeoutConstant: 100, WriteTotalTimeoutConstant: 1000}
	pSetCommTimeouts.Call(uintptr(h), uintptr(unsafe.Pointer(&t)))
	pEscapeCommFunc.Call(uintptr(h), 5)
	pPurgeComm.Call(uintptr(h), 0xF)
	return &comPort{h: h}, nil
}

func (p *comPort) Read(b []byte) (int, error) {
	for {
		if p.closed.Load() {
			return 0, io.EOF
		}
		p.mu.RLock()
		var n uint32
		err := syscall.ReadFile(p.h, b, &n, nil)
		p.mu.RUnlock()
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return int(n), nil
		}
	}
}

func (p *comPort) Write(b []byte) (int, error) {
	if p.closed.Load() {
		return 0, errPortClosed
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	var n uint32
	err := syscall.WriteFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *comPort) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return syscall.CloseHandle(p.h)
}
