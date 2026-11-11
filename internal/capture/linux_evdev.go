//go:build linux

package capture

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02
	evAbs = 0x03
	evMsc = 0x04
	evLed = 0x11

	synReport  = 0
	synDropped = 3
	mscScan    = 4

	keyMax    = 0x2FF
	ledMax    = 0x0F
	virtualPh = "typedeck-virtual"
)

const (
	iocWrite = 1
	iocRead  = 2
)

func ioc(dir, kind, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | kind<<8 | nr
}

func eviocgbit(ev, size uintptr) uintptr { return ioc(iocRead, 'E', 0x20+ev, size) }
func eviocgname(size uintptr) uintptr    { return ioc(iocRead, 'E', 0x06, size) }
func eviocgphys(size uintptr) uintptr    { return ioc(iocRead, 'E', 0x07, size) }
func eviocgkey(size uintptr) uintptr     { return ioc(iocRead, 'E', 0x18, size) }

var virtualKeyRanges = [][2]int{{1, 0xFF}, {0x160, 0x2BF}}

var eviocgrab = ioc(iocWrite, 'E', 0x90, 4)

type inputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

var eventSize = int(unsafe.Sizeof(inputEvent{}))

func rawIoctl(fd uintptr, req uintptr, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctl(f *os.File, req uintptr, arg unsafe.Pointer) error {
	return control(f, func(fd uintptr) error { return rawIoctl(fd, req, uintptr(arg)) })
}

func ioctlValue(f *os.File, req uintptr, v uintptr) error {
	return control(f, func(fd uintptr) error { return rawIoctl(fd, req, v) })
}

func control(f *os.File, fn func(fd uintptr) error) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var inner error
	if err := rc.Control(func(fd uintptr) { inner = fn(fd) }); err != nil {
		return err
	}
	return inner
}

func ioctlString(f *os.File, req uintptr) string {
	buf := make([]byte, 256)
	if err := ioctl(f, req, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	return strings.TrimRight(string(buf), "\x00")
}

func bitSet(bits []byte, n int) bool {
	return n/8 < len(bits) && bits[n/8]>>(n%8)&1 == 1
}

func eventBits(f *os.File, ev uintptr, size int) []byte {
	bits := make([]byte, size)
	if err := ioctl(f, eviocgbit(ev, uintptr(size)), unsafe.Pointer(&bits[0])); err != nil {
		return nil
	}
	return bits
}

func pressedKeys(f *os.File) ([]byte, error) {
	bits := make([]byte, keyMax/8+1)
	err := ioctl(f, eviocgkey(uintptr(len(bits))), unsafe.Pointer(&bits[0]))
	return bits, err
}

func anyBit(bits []byte) bool {
	for _, b := range bits {
		if b != 0 {
			return true
		}
	}
	return false
}

func openEvdev(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

func isKeyboard(f *os.File) bool {
	types := eventBits(f, 0, 4)
	if !bitSet(types, evKey) || bitSet(types, evRel) || bitSet(types, evAbs) {
		return false
	}
	keys := eventBits(f, evKey, keyMax/8+1)
	for _, k := range evdevTable[:26] {
		if !bitSet(keys, int(k.code)) {
			return false
		}
	}
	return !strings.HasPrefix(ioctlString(f, eviocgphys(256)), virtualPh)
}

func decodeEvents(buf []byte) []inputEvent {
	n := len(buf) / eventSize
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*inputEvent)(unsafe.Pointer(&buf[0])), n)
}

func encodeEvents(ev []inputEvent) []byte {
	if len(ev) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&ev[0])), len(ev)*eventSize)
}
