//go:build windows

package winreg

import (
	"syscall"
	"unsafe"
)

const (
	LocalMachine = syscall.HKEY_LOCAL_MACHINE
	CurrentUser  = syscall.HKEY_CURRENT_USER

	regSZ    = 1
	regDWORD = 4
)

var (
	advapi32       = syscall.NewLazyDLL("advapi32.dll")
	pCreateKeyEx   = advapi32.NewProc("RegCreateKeyExW")
	pSetValueEx    = advapi32.NewProc("RegSetValueExW")
	pDeleteValue   = advapi32.NewProc("RegDeleteValueW")
	pEnumValue     = advapi32.NewProc("RegEnumValueW")
	errNoSuchValue = syscall.Errno(2)
)

func openKey(root syscall.Handle, path string, access uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(root, p, 0, access, &key); err != nil {
		return 0, err
	}
	return key, nil
}

func queryRaw(root syscall.Handle, path, name string) ([]byte, uint32, bool) {
	key, err := openKey(root, path, syscall.KEY_READ)
	if err != nil {
		return nil, 0, false
	}
	defer syscall.RegCloseKey(key)
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, 0, false
	}
	var typ, size uint32
	if syscall.RegQueryValueEx(key, n, nil, &typ, nil, &size) != nil || size == 0 {
		return nil, 0, false
	}
	buf := make([]byte, size)
	if syscall.RegQueryValueEx(key, n, nil, &typ, &buf[0], &size) != nil {
		return nil, 0, false
	}
	return buf[:size], typ, true
}

func String(root syscall.Handle, path, name string) (string, bool) {
	buf, typ, ok := queryRaw(root, path, name)
	if !ok || typ != regSZ || len(buf) < 2 {
		return "", false
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), len(buf)/2)
	return syscall.UTF16ToString(u), true
}

func DWORD(root syscall.Handle, path, name string) (uint32, bool) {
	buf, typ, ok := queryRaw(root, path, name)
	if !ok || typ != regDWORD || len(buf) < 4 {
		return 0, false
	}
	return *(*uint32)(unsafe.Pointer(&buf[0])), true
}

func SubKeys(root syscall.Handle, path string) []string {
	key, err := openKey(root, path, syscall.KEY_READ)
	if err != nil {
		return nil
	}
	defer syscall.RegCloseKey(key)
	var out []string
	for i := uint32(0); ; i++ {
		name := make([]uint16, 256)
		n := uint32(len(name))
		if syscall.RegEnumKeyEx(key, i, &name[0], &n, nil, nil, nil, nil) != nil {
			return out
		}
		out = append(out, syscall.UTF16ToString(name[:n]))
	}
}

type Value struct{ Name, Data string }

func StringValues(root syscall.Handle, path string) []Value {
	key, err := openKey(root, path, syscall.KEY_READ)
	if err != nil {
		return nil
	}
	defer syscall.RegCloseKey(key)
	var out []Value
	for i := uint32(0); ; i++ {
		name := make([]uint16, 256)
		data := make([]uint16, 256)
		nl, dl := uint32(len(name)), uint32(len(data)*2)
		var typ uint32
		r, _, _ := pEnumValue.Call(uintptr(key), uintptr(i), uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&nl)), 0,
			uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&dl)))
		if r != 0 {
			return out
		}
		if typ == regSZ {
			out = append(out, Value{syscall.UTF16ToString(name[:nl]), syscall.UTF16ToString(data)})
		}
	}
}

func createKey(root syscall.Handle, path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var key syscall.Handle
	r, _, _ := pCreateKeyEx.Call(uintptr(root), uintptr(unsafe.Pointer(p)), 0, 0, 0, syscall.KEY_WRITE, 0, uintptr(unsafe.Pointer(&key)), 0)
	if r != 0 {
		return 0, syscall.Errno(r)
	}
	return key, nil
}

func setRaw(root syscall.Handle, path, name string, typ uint32, data []byte) error {
	key, err := createKey(root, path)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, _ := pSetValueEx.Call(uintptr(key), uintptr(unsafe.Pointer(n)), 0, uintptr(typ), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func SetDWORD(root syscall.Handle, path, name string, v uint32) error {
	b := (*[4]byte)(unsafe.Pointer(&v))[:]
	return setRaw(root, path, name, regDWORD, append([]byte(nil), b...))
}

func SetString(root syscall.Handle, path, name, v string) error {
	u, err := syscall.UTF16FromString(v)
	if err != nil {
		return err
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
	return setRaw(root, path, name, regSZ, append([]byte(nil), b...))
}

func DeleteValue(root syscall.Handle, path, name string) error {
	key, err := openKey(root, path, syscall.KEY_WRITE)
	if err != nil {
		if err == errNoSuchValue {
			return nil
		}
		return err
	}
	defer syscall.RegCloseKey(key)
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, _ := pDeleteValue.Call(uintptr(key), uintptr(unsafe.Pointer(n)))
	if r != 0 && syscall.Errno(r) != errNoSuchValue {
		return syscall.Errno(r)
	}
	return nil
}
