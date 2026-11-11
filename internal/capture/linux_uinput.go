//go:build linux

package capture

import (
	"errors"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const (
	busVirtual = 0x06
	setupSize  = 92
)

var (
	uiDevCreate  = ioc(0, 'U', 1, 0)
	uiDevDestroy = ioc(0, 'U', 2, 0)
	uiDevSetup   = ioc(iocWrite, 'U', 3, setupSize)
	uiSetEvbit   = ioc(iocWrite, 'U', 100, 4)
	uiSetKeybit  = ioc(iocWrite, 'U', 101, 4)
	uiSetMscbit  = ioc(iocWrite, 'U', 104, 4)
	uiSetLedbit  = ioc(iocWrite, 'U', 105, 4)
	uiSetPhys    = ioc(iocWrite, 'U', 108, unsafe.Sizeof(uintptr(0)))
)

var uinputPaths = []string{"/dev/uinput", "/dev/input/uinput"}

type uinputSetup struct {
	Bustype, Vendor, Product, Version uint16
	Name                              [80]byte
	FFEffectsMax                      uint32
}

type virtualKeyboard struct {
	f     *os.File
	mu    sync.Mutex
	onLED func(code uint16, value int32)
}

func openUinput() (*os.File, error) {
	var first error
	for _, p := range uinputPaths {
		fd, err := syscall.Open(p, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err == nil {
			return os.NewFile(uintptr(fd), p), nil
		}
		if first == nil || errors.Is(first, syscall.ENOENT) {
			first = &os.PathError{Op: "open", Path: p, Err: err}
		}
	}
	return nil, first
}

func virtualKeys() []int {
	var codes []int
	for _, r := range virtualKeyRanges {
		for code := r[0]; code <= r[1]; code++ {
			codes = append(codes, code)
		}
	}
	return codes
}

func newVirtualKeyboard(onLEDD func(code uint16, value int32)) (*virtualKeyboard, error) {
	f, err := openUinput()
	if err != nil {
		return nil, err
	}
	v := &virtualKeyboard{f: f, onLED: onLEDD}
	if err := v.create(); err != nil {
		f.Close()
		return nil, err
	}
	go v.readLEDs()
	return v, nil
}

func (v *virtualKeyboard) create() error {
	for _, ev := range []int{evKey, evMsc, evLed, evSyn} {
		if err := ioctlValue(v.f, uiSetEvbit, uintptr(ev)); err != nil {
			return err
		}
	}
	if err := ioctlValue(v.f, uiSetMscbit, mscScan); err != nil {
		return err
	}
	for _, code := range virtualKeys() {
		if err := ioctlValue(v.f, uiSetKeybit, uintptr(code)); err != nil {
			return err
		}
	}
	for led := 0; led <= ledMax; led++ {
		if err := ioctlValue(v.f, uiSetLedbit, uintptr(led)); err != nil {
			return err
		}
	}
	phys := append([]byte(virtualPh), 0)
	if err := ioctl(v.f, uiSetPhys, unsafe.Pointer(&phys[0])); err != nil {
		return err
	}
	setup := uinputSetup{Bustype: busVirtual}
	copy(setup.Name[:79], "Typedeck virtual keyboard")
	if err := ioctl(v.f, uiDevSetup, unsafe.Pointer(&setup)); err != nil {
		return err
	}
	return ioctlValue(v.f, uiDevCreate, 0)
}

func (v *virtualKeyboard) write(ev []inputEvent) error {
	b := encodeEvents(ev)
	v.mu.Lock()
	defer v.mu.Unlock()
	_, err := v.f.Write(b)
	return err
}

func (v *virtualKeyboard) key(code uint16, value int32) error {
	return v.write([]inputEvent{
		{Type: evKey, Code: code, Value: value},
		{Type: evSyn, Code: synReport},
	})
}

func (v *virtualKeyboard) keys(codes []uint16, value int32) error {
	ev := make([]inputEvent, 0, len(codes)+1)
	for _, c := range codes {
		ev = append(ev, inputEvent{Type: evKey, Code: c, Value: value})
	}
	return v.write(append(ev, inputEvent{Type: evSyn, Code: synReport}))
}

func (v *virtualKeyboard) readLEDs() {
	buf := make([]inputEvent, 16)
	raw := encodeEvents(buf)
	for {
		n, err := v.f.Read(raw)
		if err != nil {
			return
		}
		for _, ev := range decodeEvents(raw[:n-n%eventSize]) {
			if ev.Type == evLed && v.onLED != nil {
				v.onLED(ev.Code, ev.Value)
			}
		}
	}
}

func (v *virtualKeyboard) close() {
	_ = ioctlValue(v.f, uiDevDestroy, 0)
	v.f.Close()
}
