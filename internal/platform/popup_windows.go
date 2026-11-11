//go:build windows

package platform

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type rect struct{ Left, Top, Right, Bottom int32 }

type paintStruct struct {
	HDC       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      [2]int32
}

var (
	gdi32 = syscall.NewLazyDLL("gdi32.dll")

	pRegisterClassEx  = user32.NewProc("RegisterClassExW")
	pCreateWindowEx   = user32.NewProc("CreateWindowExW")
	pDefWindowProc    = user32.NewProc("DefWindowProcW")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pGetMessage       = user32.NewProc("GetMessageW")
	pDispatchMessage  = user32.NewProc("DispatchMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pSetTimer         = user32.NewProc("SetTimer")
	pKillTimer        = user32.NewProc("KillTimer")
	pShowWindow2      = user32.NewProc("ShowWindow")
	pSetLayeredAttr   = user32.NewProc("SetLayeredWindowAttributes")
	pBeginPaint       = user32.NewProc("BeginPaint")
	pEndPaint         = user32.NewProc("EndPaint")
	pFillRect         = user32.NewProc("FillRect")
	pDrawText         = user32.NewProc("DrawTextW")
	pGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	pSetWindowRgn     = user32.NewProc("SetWindowRgn")
	pGetDpiForWindow  = user32.NewProc("GetDpiForWindow")
	pMonitorFromWin   = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo   = user32.NewProc("GetMonitorInfoW")
	pGetDpiForMonitor = syscall.NewLazyDLL("shcore.dll").NewProc("GetDpiForMonitor")
	pLoadCursor       = user32.NewProc("LoadCursorW")
	pGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject     = gdi32.NewProc("DeleteObject")
	pSelectObject     = gdi32.NewProc("SelectObject")
	pSetBkMode        = gdi32.NewProc("SetBkMode")
	pSetTextColor     = gdi32.NewProc("SetTextColor")
	pCreateFont       = gdi32.NewProc("CreateFontW")
	pCreateRoundRgn   = gdi32.NewProc("CreateRoundRectRgn")
	pEllipse          = gdi32.NewProc("Ellipse")
	pCreatePen        = gdi32.NewProc("CreatePen")
)

const (
	wsPopup         = 0x80000000
	wsExTopmost     = 0x8
	wsExToolWindow  = 0x80
	wsExNoActivate  = 0x08000000
	wsExLayered     = 0x80000
	wsExTransparent = 0x20
	wmPaint         = 0x000F
	wmTimer         = 0x0113
	wmDestroy       = 0x0002
	swShowNoActive  = 4
	dtCenter        = 0x1
	dtVCenter       = 0x4
	dtSingleLine    = 0x20
	dtEndEllipsis   = 0x8000
)

var (
	popMu     sync.Mutex
	popTexts  = map[uintptr]PopupOptions{}
	popClass  *uint16
	popOnce   sync.Once
	popProcPt uintptr
	popCur    uintptr
)

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

func rgb(r, g, b byte) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func parseHex(c string) (r, g, b byte, ok bool) {
	c = strings.TrimPrefix(c, "#")
	if len(c) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(c, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return byte(v >> 16), byte(v >> 8), byte(v), true
}

func popupProc(hwnd, m, wp, lp uintptr) uintptr {
	switch m {
	case wmPaint:
		paintPopup(hwnd)
		return 0
	case wmTimer:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pKillTimer.Call(hwnd, 1)
		popMu.Lock()
		delete(popTexts, hwnd)
		if popCur == hwnd {
			popCur = 0
		}
		popMu.Unlock()
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, m, wp, lp)
	return r
}

func windowScale(hwnd uintptr) float64 {
	if d, _, _ := pGetDpiForWindow.Call(hwnd); d >= 96 {
		return float64(d) / 96
	}
	return 1
}

func activeMonitor() (work rect, scale float64) {
	fg, _, _ := pGetForegroundWindow.Call()
	mon, _, _ := pMonitorFromWin.Call(fg, 2)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, _ := pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		w, _, _ := pGetSystemMetrics.Call(0)
		h, _, _ := pGetSystemMetrics.Call(1)
		return rect{0, 0, int32(w), int32(h)}, 1
	}
	var dx, dy uint32
	scale = 1
	if r, _, _ := pGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx >= 96 {
		scale = float64(dx) / 96
	}
	return mi.Work, scale
}

func font(height int32, weight int32) uintptr {
	face, _ := syscall.UTF16PtrFromString("Segoe UI")
	f, _, _ := pCreateFont.Call(uintptr(-height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	return f
}

func paintPopup(hwnd uintptr) {
	popMu.Lock()
	o := popTexts[hwnd]
	popMu.Unlock()
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	sc := windowScale(hwnd)
	var rc rect
	rc.Right, rc.Bottom = ps.Paint.Right, ps.Paint.Bottom
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	bg, _, _ := pCreateSolidBrush.Call(rgb(0x1c, 0x1f, 0x25))
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), bg)
	pDeleteObject.Call(bg)
	ar, ag, ab, ok := parseHex(o.Accent)
	if !ok {
		ar, ag, ab = 0x25, 0x63, 0xeb
	}
	br, _, _ := pCreateSolidBrush.Call(rgb(ar, ag, ab))
	bar := rect{0, 0, int32(6 * sc), rc.Bottom}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&bar)), br)
	pDeleteObject.Call(br)
	pSetBkMode.Call(hdc, 1)
	draw := func(text string, f uintptr, color uintptr, r rect) {
		old, _, _ := pSelectObject.Call(hdc, f)
		pSetTextColor.Call(hdc, color)
		t, _ := syscall.UTF16PtrFromString(text)
		pDrawText.Call(hdc, uintptr(unsafe.Pointer(t)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
		pSelectObject.Call(hdc, old)
		pDeleteObject.Call(f)
	}
	pad := int32(16 * sc)
	hasSub := strings.TrimSpace(o.Subtitle) != ""
	dotsH := int32(0)
	if o.Dots > 1 {
		dotsH = int32(20 * sc)
	}
	titleBox := rect{pad + bar.Right, int32(8 * sc), rc.Right - pad, int32(8*sc) + int32(34*sc)}
	if !hasSub {
		titleBox.Top, titleBox.Bottom = 0, rc.Bottom-dotsH
	}
	draw(o.Title, font(int32(22*sc), 600), rgb(0xf5, 0xf6, 0xf7), titleBox)
	if hasSub {
		draw(o.Subtitle, font(int32(15*sc), 400), rgb(0x94, 0x9e, 0xa6), rect{pad + bar.Right, titleBox.Bottom, rc.Right - pad, rc.Bottom - dotsH - int32(6*sc)})
	}
	if o.Dots > 1 {
		d := int32(8 * sc)
		gap := int32(8 * sc)
		total := int32(o.Dots)*d + int32(o.Dots-1)*gap
		x := (rc.Right+bar.Right)/2 - total/2
		y := rc.Bottom - int32(18*sc)
		for i := 0; i < o.Dots; i++ {
			col := rgb(0x4a, 0x52, 0x5a)
			if i == o.Active {
				col = rgb(ar, ag, ab)
			}
			b, _, _ := pCreateSolidBrush.Call(col)
			pen, _, _ := pCreatePen.Call(5, 0, 0)
			ob, _, _ := pSelectObject.Call(hdc, b)
			op, _, _ := pSelectObject.Call(hdc, pen)
			pEllipse.Call(hdc, uintptr(x), uintptr(y), uintptr(x+d), uintptr(y+d))
			pSelectObject.Call(hdc, ob)
			pSelectObject.Call(hdc, op)
			pDeleteObject.Call(b)
			pDeleteObject.Call(pen)
			x += d + gap
		}
	}
}

var pGetClientRect = user32.NewProc("GetClientRect")

func (*windows) Popup(o PopupOptions) bool {
	if strings.TrimSpace(o.Title) == "" && strings.TrimSpace(o.Subtitle) == "" {
		return true
	}
	ready := make(chan bool, 1)
	go showPopup(o, ready)
	select {
	case ok := <-ready:
		return ok
	case <-time.After(2 * time.Second):
		return false
	}
}

func popupRect(o PopupOptions) (x, y, w, h int32) {
	work, sc := activeMonitor()
	w = int32(440 * sc)
	h = int32(70 * sc)
	if strings.TrimSpace(o.Subtitle) != "" {
		h = int32(96 * sc)
	}
	if o.Dots > 1 {
		h += int32(18 * sc)
	}
	x = work.Left + (work.Right-work.Left-w)/2
	y = work.Bottom - h - int32(110*sc)
	switch o.Position {
	case "top":
		y = work.Top + int32(70*sc)
	case "center":
		y = work.Top + (work.Bottom-work.Top-h)/2
	}
	return
}

func registerPopupClass() {
	popOnce.Do(func() {
		popProcPt = syscall.NewCallback(popupProc)
		popClass, _ = syscall.UTF16PtrFromString("TypedeckPopup")
		inst, _, _ := pGetModuleHandle.Call(0)
		cur, _, _ := pLoadCursor.Call(0, 32512)
		wc := wndClassEx{WndProc: popProcPt, Instance: inst, Cursor: cur, ClassName: popClass}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	})
}

func replacePopup(hwnd uintptr) {
	popMu.Lock()
	prev := popCur
	popCur = hwnd
	popMu.Unlock()
	if prev != 0 {
		pPostMessageW.Call(prev, wmClose, 0, 0)
	}
}

func showPopup(o PopupOptions, ready chan<- bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	registerPopupClass()
	x, y, w, h := popupRect(o)
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolWindow|wsExNoActivate|wsExLayered|wsExTransparent, uintptr(unsafe.Pointer(popClass)), 0, wsPopup,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, 0, 0)
	if hwnd == 0 {
		ready <- false
		return
	}
	popMu.Lock()
	popTexts[hwnd] = o
	popMu.Unlock()
	replacePopup(hwnd)
	pSetLayeredAttr.Call(hwnd, 0, 240, 2)
	rgn, _, _ := pCreateRoundRgn.Call(0, 0, uintptr(w), uintptr(h), uintptr(int32(18*windowScale(hwnd))), uintptr(int32(18*windowScale(hwnd))))
	pSetWindowRgn.Call(hwnd, rgn, 1)
	secs := o.Seconds
	if secs < 0.6 {
		secs = 1.2
	}
	pSetTimer.Call(hwnd, 1, uintptr(secs*1000), 0)
	pShowWindow2.Call(hwnd, swShowNoActive)
	ready <- true
	var m msg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}
