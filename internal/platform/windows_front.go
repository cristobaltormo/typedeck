//go:build windows

package platform

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	pEnumWindows         = user32.NewProc("EnumWindows")
	pGetWindowTextW      = user32.NewProc("GetWindowTextW")
	pGetClassNameW       = user32.NewProc("GetClassNameW")
	pIsWindowVisible     = user32.NewProc("IsWindowVisible")
	pIsIconic            = user32.NewProc("IsIconic")
	pGetWindow           = user32.NewProc("GetWindow")
	pGetWindowLongW      = user32.NewProc("GetWindowLongW")
	pPostMessageW        = user32.NewProc("PostMessageW")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pDwmGetAttribute     = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")

	enumMu       sync.Mutex
	enumVisit    func(hwnd uintptr) bool
	enumCallback = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if enumVisit(hwnd) {
			return 1
		}
		return 0
	})
)

var browserImages = map[string]bool{"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true, "opera.exe": true, "vivaldi.exe": true, "arc.exe": true, "librewolf.exe": true}

var shellClasses = map[string]bool{"Progman": true, "WorkerW": true, "Shell_TrayWnd": true, "Shell_SecondaryTrayWnd": true}

type topWindow struct {
	hwnd  uintptr
	pid   uint32
	title string
	class string
}

func enumerate(visit func(hwnd uintptr) bool) {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumVisit = visit
	pEnumWindows.Call(enumCallback, 0)
	enumVisit = nil
}

func windowText(hwnd uintptr) string {
	buf := make([]uint16, 512)
	n, _, _ := pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func windowClass(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := pGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func isCloaked(hwnd uintptr) bool {
	var cloaked uint32
	if r, _, _ := pDwmGetAttribute.Call(hwnd, 14, uintptr(unsafe.Pointer(&cloaked)), 4); r != 0 {
		return false
	}
	return cloaked != 0
}

func isAppWindow(hwnd uintptr) bool {
	if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
		return false
	}
	if owner, _, _ := pGetWindow.Call(hwnd, 4); owner != 0 {
		return false
	}
	if ex, _, _ := pGetWindowLongW.Call(hwnd, ^uintptr(19)); ex&0x80 != 0 {
		return false
	}
	return !isCloaked(hwnd)
}

func topWindows() []topWindow {
	var out []topWindow
	enumerate(func(hwnd uintptr) bool {
		if !isAppWindow(hwnd) {
			return true
		}
		w := topWindow{hwnd: hwnd, title: windowText(hwnd), class: windowClass(hwnd)}
		if w.title == "" || shellClasses[w.class] {
			return true
		}
		pGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&w.pid)))
		out = append(out, w)
		return true
	})
	return out
}

func windowsOfImage(exe string) []topWindow {
	images := map[uint32]string{}
	var out []topWindow
	for _, w := range topWindows() {
		img, ok := images[w.pid]
		if !ok {
			img = strings.ToLower(filepath.Base(processImage(w.pid)))
			images[w.pid] = img
		}
		if img == strings.ToLower(exe) {
			out = append(out, w)
		}
	}
	return out
}

func closeWindows(ws []topWindow) {
	for _, w := range ws {
		pPostMessageW.Call(w.hwnd, wmClose, 0, 0)
	}
}

func bringToFront(hwnd uintptr) bool {
	if min, _, _ := pIsIconic.Call(hwnd); min != 0 {
		pShowWindow.Call(hwnd, 9)
	}
	keyEvent(0x12, false)
	keyEvent(0x12, true)
	r, _, _ := pSetForegroundWindow.Call(hwnd)
	return r != 0
}

func (*windows) FocusEditor(string) bool {
	for _, w := range topWindows() {
		if !strings.Contains(w.title, "Typedeck") || !browserImages[strings.ToLower(filepath.Base(processImage(w.pid)))] {
			continue
		}
		return bringToFront(w.hwnd)
	}
	return false
}

func foreground() (hwnd uintptr, pid uint32) {
	hwnd, _, _ = pGetForegroundWindow.Call()
	if hwnd != 0 {
		pGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	}
	return
}

func (*windows) HideApp(name string) error {
	hwnd, _ := foreground()
	if hwnd == 0 {
		return ErrUnsupported
	}
	pShowWindow.Call(hwnd, 6)
	return nil
}

func processImage(pid uint32) string {
	h, _, _ := pOpenProcess.Call(0x1000, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if r, _, _ := pQueryFullImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func (*windows) FrontID() string {
	hwnd, pid := foreground()
	if hwnd == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(hwnd), 16) + ":" + strconv.FormatUint(uint64(pid), 10)
}

func (*windows) FrontNames(id string) []string {
	_, pid := foreground()
	if pid == 0 {
		return nil
	}
	img := processImage(pid)
	if img == "" {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(img), filepath.Ext(img))
	return append([]string{base}, shortcutNamesFor(img, base)...)
}

func topWindowsTitled(title string) []topWindow {
	var out []topWindow
	for _, w := range topWindows() {
		if strings.Contains(w.title, title) {
			out = append(out, w)
		}
	}
	return out
}
