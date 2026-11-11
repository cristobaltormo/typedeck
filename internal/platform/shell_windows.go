//go:build windows

package platform

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmCommand         = 0x0111
	wmPowerBroadcast  = 0x0218
	wmSessionChange   = 0x02B1
	wmTrayEvent       = 0x8001
	wmLButtonDblClk   = 0x0203
	wmRButtonUp       = 0x0205

	pbtSuspend         = 0x4
	pbtResumeSuspend   = 0x7
	pbtResumeAutomatic = 0x12

	wtsConsoleConnect    = 1
	wtsConsoleDisconnect = 2
	wtsRemoteConnect     = 3
	wtsRemoteDisconnect  = 4
	wtsSessionLock       = 7
	wtsSessionUnlock     = 8

	nimAdd, nimModify, nimDelete = 0, 1, 2
	nifMessage, nifIcon, nifTip  = 0x1, 0x2, 0x4
	nifInfo                      = 0x10

	menuOpen, menuRestart, menuQuit = 1, 2, 3
	trayID                          = 1
	tipTimer                        = 1
	swShowNA                        = 8
)

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            [16]byte
	BalloonIcon     uintptr
}

type point struct{ X, Y int32 }

var (
	shell32 = syscall.NewLazyDLL("shell32.dll")
	wtsapi  = syscall.NewLazyDLL("wtsapi32.dll")

	pShellNotifyIcon   = shell32.NewProc("Shell_NotifyIconW")
	pRegisterWTS       = wtsapi.NewProc("WTSRegisterSessionNotification")
	pRegisterWindowMsg = user32.NewProc("RegisterWindowMessageW")
	pCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	pAppendMenu        = user32.NewProc("AppendMenuW")
	pTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	pDestroyMenu       = user32.NewProc("DestroyMenu")
	pGetCursorPos      = user32.NewProc("GetCursorPos")
	pLoadImage         = user32.NewProc("LoadImageW")
	pLoadIcon          = user32.NewProc("LoadIconW")

	shellProcPtr = syscall.NewCallback(shellProc)

	shellMu    sync.Mutex
	shellHooks ShellHooks
	shellWnd   uintptr
	shellIcon  uintptr
	shellTip   string
	taskbarMsg uintptr
)

func StartShell(h ShellHooks) {
	shellMu.Lock()
	shellHooks = h
	shellMu.Unlock()
	go shellLoop()
}

func shellLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	inst, _, _ := pGetModuleHandle.Call(0)
	wc := wndClassEx{WndProc: shellProcPtr, Instance: inst, ClassName: shellClassString}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := pCreateWindowEx.Call(wsExToolWindow|wsExNoActivate, uintptr(unsafe.Pointer(shellClassString)), uintptr(unsafe.Pointer(utf16Ptr(shellTitle()))), wsPopup,
		0, 0, 0, 0, 0, 0, inst, 0)
	if hwnd == 0 {
		return
	}
	pShowWindow.Call(hwnd, swShowNA)
	taskbarMsg, _, _ = pRegisterWindowMsg.Call(uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated"))))
	pRegisterWTS.Call(hwnd, 0)
	shellIcon = loadTrayIcon(inst)
	shellMu.Lock()
	shellWnd = hwnd
	shellMu.Unlock()
	addTrayIcon(hwnd)
	pSetTimer.Call(hwnd, tipTimer, 5000, 0)
	var m msg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func loadTrayIcon(inst uintptr) uintptr {
	cx, _, _ := pGetSystemMetrics.Call(49)
	cy, _, _ := pGetSystemMetrics.Call(50)
	if h, _, _ := pLoadImage.Call(inst, 1, 1, cx, cy, 0); h != 0 {
		return h
	}
	h, _, _ := pLoadIcon.Call(0, 32512)
	return h
}

func hooks() ShellHooks {
	shellMu.Lock()
	defer shellMu.Unlock()
	return shellHooks
}

func statusText() string {
	text := "Typedeck"
	if s := hooks().Status; s != nil {
		if st := s(); st != "" {
			text += " - " + st
		}
	}
	return text
}

func trayData(hwnd uintptr, flags uint32) notifyIconData {
	d := notifyIconData{Wnd: hwnd, ID: trayID, Flags: flags}
	d.Size = uint32(unsafe.Sizeof(d))
	return d
}

func copyUTF16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) > len(dst) {
		u = append(u[:len(dst)-1], 0)
	}
	copy(dst, u)
}

func addTrayIcon(hwnd uintptr) {
	d := trayData(hwnd, nifMessage|nifIcon|nifTip)
	d.CallbackMessage = wmTrayEvent
	d.Icon = shellIcon
	shellTip = statusText()
	copyUTF16(d.Tip[:], shellTip)
	pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&d)))
}

func refreshTip(hwnd uintptr) {
	tip := statusText()
	if tip == shellTip {
		return
	}
	shellTip = tip
	d := trayData(hwnd, nifTip)
	copyUTF16(d.Tip[:], tip)
	pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&d)))
}

func balloon(title, body string) bool {
	shellMu.Lock()
	hwnd := shellWnd
	shellMu.Unlock()
	if hwnd == 0 {
		return false
	}
	d := trayData(hwnd, nifInfo)
	d.InfoFlags = 1
	copyUTF16(d.InfoTitle[:], title)
	copyUTF16(d.Info[:], body)
	r, _, _ := pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&d)))
	return r != 0
}

func showMenu(hwnd uintptr) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	add := func(flags, id uintptr, text string) {
		pAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
	add(0x1, 0, statusText())
	add(0x800, 0, "")
	add(0, menuOpen, "Open editor")
	add(0, menuRestart, "Restart")
	add(0, menuQuit, "Quit")
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hwnd)
	pTrackPopupMenu.Call(menu, 0x2|0x20, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	pPostMessageW.Call(hwnd, 0, 0, 0)
}

func shutdownAndExit() {
	if f := hooks().Shutdown; f != nil {
		f()
	}
	os.Exit(0)
}

func restartSelf() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if f := hooks().Shutdown; f != nil {
		f()
	}
	releaseInstance()
	_ = Detached(exe, "--background")
	os.Exit(0)
}

func onCommand(id uintptr) {
	switch id & 0xFFFF {
	case menuOpen:
		if f := hooks().OpenEditor; f != nil {
			go f()
		}
	case menuRestart:
		restartSelf()
	case menuQuit:
		shutdownAndExit()
	}
}

func onSession(code uintptr) {
	h := hooks()
	switch code {
	case wtsSessionLock, wtsConsoleDisconnect, wtsRemoteDisconnect:
		if h.Release != nil {
			h.Release()
		}
	case wtsSessionUnlock, wtsConsoleConnect, wtsRemoteConnect:
		if h.Reclaim != nil {
			h.Reclaim()
		}
	}
}

func onPower(code uintptr) {
	h := hooks()
	switch code {
	case pbtSuspend:
		if h.Release != nil {
			h.Release()
		}
	case pbtResumeSuspend, pbtResumeAutomatic:
		if h.Reclaim != nil {
			h.Reclaim()
		}
	}
}

func shellProc(hwnd, m, wp, lp uintptr) uintptr {
	switch m {
	case wmClose:
		shutdownAndExit()
		return 0
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if wp != 0 {
			shutdownAndExit()
		}
		return 0
	case wmCommand:
		onCommand(wp)
		return 0
	case wmRenderFormat:
		if wp == cfUnicodeText {
			renderPaste()
		}
		return 0
	case wmSessionChange:
		onSession(wp)
		return 0
	case wmPowerBroadcast:
		onPower(wp)
		return 1
	case wmTimer:
		refreshTip(hwnd)
		return 0
	case wmTrayEvent:
		switch lp & 0xFFFF {
		case wmRButtonUp:
			showMenu(hwnd)
		case wmLButtonDblClk:
			onCommand(menuOpen)
		}
		return 0
	case wmDestroy:
		d := trayData(hwnd, 0)
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
		pPostQuitMessage.Call(0)
		return 0
	}
	if m == taskbarMsg && m != 0 {
		addTrayIcon(hwnd)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, m, wp, lp)
	return r
}
