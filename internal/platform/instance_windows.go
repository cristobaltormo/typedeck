//go:build windows

package platform

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	shellClassName   = "TypedeckShell"
	errAlreadyExists = 183
	synchronize      = 0x100000
)

var (
	pCreateMutex     = kernel32.NewProc("CreateMutexW")
	pReleaseMutex    = kernel32.NewProc("ReleaseMutex")
	pFindWindow      = user32.NewProc("FindWindowW")
	pMessageBox      = user32.NewProc("MessageBoxW")
	instanceMutex    uintptr
	shellClassString = utf16Ptr(shellClassName)
)

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func instanceKey() string {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(Current.ConfigDir())))
	return fmt.Sprintf("%08x", h.Sum32())
}

func shellTitle() string { return "Typedeck " + instanceKey() }

func AcquireInstance() bool {
	h, _, err := pCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(utf16Ptr(`Local\Typedeck-`+instanceKey()))))
	if h == 0 {
		return true
	}
	if errno, ok := err.(syscall.Errno); ok && errno == errAlreadyExists {
		syscall.CloseHandle(syscall.Handle(h))
		return false
	}
	instanceMutex = h
	return true
}

func releaseInstance() {
	if instanceMutex != 0 {
		pReleaseMutex.Call(instanceMutex)
		syscall.CloseHandle(syscall.Handle(instanceMutex))
		instanceMutex = 0
	}
}

func StartedByUser() bool {
	parent := parentImage()
	return strings.EqualFold(filepath.Base(parent), "explorer.exe")
}

func parentImage() string {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(snap)
	self := uint32(syscall.Getpid())
	var parent uint32
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err := syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		if e.ProcessID == self {
			parent = e.ParentProcessID
			break
		}
	}
	if parent == 0 {
		return ""
	}
	return processImage(parent)
}

func ShowError(title, text string) {
	pMessageBox.Call(0, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(title))), 0x10|0x10000)
}

func StopRunning() bool {
	hwnd, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(shellClassString)), uintptr(unsafe.Pointer(utf16Ptr(shellTitle()))))
	if hwnd == 0 {
		return true
	}
	var pid uint32
	pGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	h, _, _ := pOpenProcess.Call(synchronize, 0, uintptr(pid))
	if h == 0 {
		return true
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	pPostMessageW.Call(hwnd, wmClose, 0, 0)
	ev, _ := syscall.WaitForSingleObject(syscall.Handle(h), uint32((8 * time.Second).Milliseconds()))
	return ev == syscall.WAIT_OBJECT_0
}
