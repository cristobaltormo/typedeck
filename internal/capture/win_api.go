//go:build windows

package capture

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pSetWindowsHookEx   = user32.NewProc("SetWindowsHookExW")
	pUnhookWindowsHook  = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHookEx     = user32.NewProc("CallNextHookEx")
	pGetMessage         = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessage    = user32.NewProc("DispatchMessageW")
	pPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	pSendInput          = user32.NewProc("SendInput")
	pGetLastInputInfo   = user32.NewProc("GetLastInputInfo")
	pGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
	pGetCurrentProcess  = kernel32.NewProc("GetCurrentProcess")
	pGetTickCount       = kernel32.NewProc("GetTickCount")
	pProcessIDToSession = kernel32.NewProc("ProcessIdToSessionId")
	pGetCurrentPID      = kernel32.NewProc("GetCurrentProcessId")
	pOpenProcessToken   = advapi32.NewProc("OpenProcessToken")
	pGetTokenInfo       = advapi32.NewProc("GetTokenInformation")
)

const (
	whKeyboardLL   = 13
	hcAction       = 0
	wmKeyDown      = 0x0100
	wmKeyUp        = 0x0101
	wmSysKeyDown   = 0x0104
	wmSysKeyUp     = 0x0105
	wmApp          = 0x8000
	wmReinstall    = wmApp + 1
	llkhfExtended  = 0x01
	llkhfLowerIL   = 0x02
	llkhfInjected  = 0x10
	inputKeyboard  = 1
	keyeventfExt   = 0x0001
	keyeventfUp    = 0x0002
	keyeventfUni   = 0x0004
	keyeventfScan  = 0x0008
	tokenQuery     = 0x0008
	tokenElevation = 20
)

type kbdllhook struct {
	vkCode    uint32
	scanCode  uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

type keybdInput struct {
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	extra uintptr
}

type input struct {
	kind uint32
	ki   keybdInput
	_    [8]byte
}

type lastInput struct {
	size uint32
	time uint32
}

func ptr[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }
