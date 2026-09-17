package board

import (
	"sort"
	"syscall"
	"unsafe"
)

// systemPorts lists USB serial ports (usbser.sys) from HKLM\HARDWARE\DEVICEMAP\SERIALCOMM, skipping Bluetooth and real ports
// that could stall the probe.
func systemPorts() []string {
	var key syscall.Handle
	k, _ := syscall.UTF16PtrFromString(`HARDWARE\DEVICEMAP\SERIALCOMM`)
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, k, 0, syscall.KEY_READ, &key) != nil {
		return nil
	}
	defer syscall.RegCloseKey(key)
	enum := syscall.NewLazyDLL("advapi32.dll").NewProc("RegEnumValueW")
	var out []string
	for i := uint32(0); ; i++ {
		name := make([]uint16, 256)
		data := make([]uint16, 256)
		nl, dl := uint32(len(name)), uint32(len(data)*2)
		var typ uint32
		r, _, _ := enum.Call(uintptr(key), uintptr(i), uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&nl)), 0,
			uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&dl)))
		if r != 0 {
			break
		}
		dev := syscall.UTF16ToString(name[:nl])
		if len(dev) >= 6 && (contains(dev, "USBSER") || contains(dev, "VCP")) {
			out = append(out, syscall.UTF16ToString(data))
		}
	}
	sort.Strings(out)
	return out
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
