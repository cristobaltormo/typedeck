//go:build windows

package platform

import (
	"errors"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	cfText        = 1
	cfOemText     = 7
	cfUnicodeText = 13
	cfLocale      = 16

	wmRenderFormat = 0x0305
)

var (
	pEnumClipboardFormats = user32.NewProc("EnumClipboardFormats")
	pRegisterClipFormat   = user32.NewProc("RegisterClipboardFormatW")
	pGlobalSize           = kernel32.NewProc("GlobalSize")

	pasteMu      sync.Mutex
	pasteText    string
	pasteRender  chan struct{}
	historyNames = []string{"ExcludeClipboardContentFromMonitorProcessing", "CanIncludeInClipboardHistory", "CanUploadToCloudClipboard"}
)

func clipboardOwner() uintptr {
	shellMu.Lock()
	defer shellMu.Unlock()
	return shellWnd
}

func openClipboard(tries int) bool {
	for i := 0; i < tries; i++ {
		if r, _, _ := pOpenClipboard.Call(clipboardOwner()); r != 0 {
			return true
		}
		time.Sleep(15 * time.Millisecond)
	}
	return false
}

func clipboardIsPlainText() bool {
	f := uintptr(0)
	for {
		f, _, _ = pEnumClipboardFormats.Call(f)
		if f == 0 {
			return true
		}
		switch f {
		case cfText, cfOemText, cfUnicodeText, cfLocale:
		default:
			return false
		}
	}
}

func readUnicodeText() string {
	h, _, _ := pGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	size, _, _ := pGlobalSize.Call(h)
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(h)
	var u []uint16
	for i := uintptr(0); i+2 <= size; i += 2 {
		c := *(*uint16)(ptr(p + i))
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func (*windows) Clipboard() string {
	if !openClipboard(10) {
		return ""
	}
	defer pCloseClipboard.Call()
	return readUnicodeText()
}

func allocGlobal(data []byte) (uintptr, error) {
	h, _, _ := pGlobalAlloc.Call(0x2, uintptr(len(data)))
	if h == 0 {
		return 0, errors.New("out of memory for the clipboard")
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return 0, errors.New("could not allocate the clipboard")
	}
	copy(unsafe.Slice((*byte)(ptr(p)), len(data)), data)
	pGlobalUnlock.Call(h)
	return h, nil
}

func unicodeBytes(text string) []byte {
	u := utf16.Encode([]rune(text + "\x00"))
	return unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
}

func putUnicode(text string) error {
	h, err := allocGlobal(unicodeBytes(text))
	if err != nil {
		return err
	}
	if r, _, _ := pSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		return errors.New("could not write to the clipboard")
	}
	return nil
}

func excludeFromHistory() {
	for _, name := range historyNames {
		id, _, _ := pRegisterClipFormat.Call(uintptr(unsafe.Pointer(utf16Ptr(name))))
		if id == 0 {
			continue
		}
		if h, err := allocGlobal(make([]byte, 4)); err == nil {
			pSetClipboardData.Call(id, h)
		}
	}
}

func setClipboardText(text string) error {
	if !openClipboard(20) {
		return errors.New("could not open the clipboard")
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	if text == "" {
		return nil
	}
	if err := putUnicode(text); err != nil {
		return err
	}
	excludeFromHistory()
	return nil
}

func offerDelayedText(text string) error {
	if !openClipboard(20) {
		return errors.New("could not open the clipboard")
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	pSetClipboardData.Call(cfUnicodeText, 0)
	excludeFromHistory()
	return nil
}

func renderPaste() {
	pasteMu.Lock()
	text, done := pasteText, pasteRender
	pasteMu.Unlock()
	if done == nil {
		return
	}
	_ = putUnicode(text)
	select {
	case done <- struct{}{}:
	default:
	}
}

func snapshotClipboard() (text string, plain bool, ok bool) {
	if !openClipboard(20) {
		return "", false, false
	}
	defer pCloseClipboard.Call()
	if !clipboardIsPlainText() {
		return "", false, true
	}
	return readUnicodeText(), true, true
}

func (*windows) Paste(text string) error {
	old, plain, ok := snapshotClipboard()
	if !ok || !plain || clipboardOwner() == 0 {
		if ok && !plain {
			return typeUnicode(text)
		}
		return pasteImmediate(text, old)
	}
	done := make(chan struct{}, 1)
	pasteMu.Lock()
	pasteText, pasteRender = text, done
	pasteMu.Unlock()
	defer func() {
		pasteMu.Lock()
		pasteText, pasteRender = "", nil
		pasteMu.Unlock()
	}()
	if err := offerDelayedText(text); err != nil {
		return err
	}
	if err := sendCombo([]string{"ctrl"}, 'V'); err != nil {
		return err
	}
	select {
	case <-done:
		time.Sleep(150 * time.Millisecond)
	case <-time.After(2 * time.Second):
	}
	return setClipboardText(old)
}

func pasteImmediate(text, old string) error {
	if err := setClipboardText(text); err != nil {
		return err
	}
	err := sendCombo([]string{"ctrl"}, 'V')
	time.Sleep(300 * time.Millisecond)
	_ = setClipboardText(old)
	return err
}
