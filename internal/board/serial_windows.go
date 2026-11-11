package board

import (
	"errors"
	"io"
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

const fileFlagOverlapped = 0x40000000

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

func configurePort(h syscall.Handle) error {
	var d dcb
	d.DCBlength = uint32(unsafe.Sizeof(d))
	if r, _, e := pGetCommState.Call(uintptr(h), uintptr(unsafe.Pointer(&d))); r == 0 {
		return e
	}
	d.BaudRate = 115200
	d.ByteSize, d.Parity, d.StopBits = 8, 0, 0
	d.Flags = 1 | 1<<4 | 1<<12
	if r, _, e := pSetCommState.Call(uintptr(h), uintptr(unsafe.Pointer(&d))); r == 0 {
		return e
	}
	t := commTimeouts{ReadIntervalTimeout: 0xFFFFFFFF, ReadTotalTimeoutMultiplier: 0xFFFFFFFF, ReadTotalTimeoutConstant: 100, WriteTotalTimeoutConstant: 1000}
	pSetCommTimeouts.Call(uintptr(h), uintptr(unsafe.Pointer(&t)))
	pEscapeCommFunc.Call(uintptr(h), 5)
	pPurgeComm.Call(uintptr(h), 0xF)
	return nil
}

func openPort(path string) (io.ReadWriteCloser, error) {
	name, err := syscall.UTF16PtrFromString(`\\.\` + path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL|fileFlagOverlapped, 0)
	if err != nil {
		return nil, err
	}
	if err := configurePort(h); err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	f, err := newOverlappedFile(h)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	return f, nil
}
