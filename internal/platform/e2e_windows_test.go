package platform

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

const cfDIB = 8

func e2eDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("TYPEDECK_E2E_DIR")
	if dir == "" {
		t.Skip("set TYPEDECK_E2E_DIR to the folder of the test box to run the interactive checks")
	}
	return dir
}

func boxText(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "testbox.txt"))
	return strings.TrimPrefix(strings.ReplaceAll(string(b), "\r\n", "\n"), string(rune(0xFEFF)))
}

func waitBox(t *testing.T, dir, want string) string {
	t.Helper()
	var got string
	for i := 0; i < 20; i++ {
		if got = boxText(t, dir); got == want {
			return got
		}
		time.Sleep(250 * time.Millisecond)
	}
	return got
}

func clearBox(t *testing.T) {
	t.Helper()
	_ = sendCombo([]string{"ctrl"}, 'A')
	_ = sendCombo(nil, 0x08)
	time.Sleep(400 * time.Millisecond)
}

func setTestImage(t *testing.T) {
	t.Helper()
	dib := make([]byte, 40+16)
	binary.LittleEndian.PutUint32(dib, 40)
	binary.LittleEndian.PutUint32(dib[4:], 2)
	binary.LittleEndian.PutUint32(dib[8:], 2)
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	if !openClipboard(20) {
		t.Fatal("could not open the clipboard")
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, err := allocGlobal(dib)
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _ := pSetClipboardData.Call(cfDIB, h); r == 0 {
		t.Fatal("could not put the image on the clipboard")
	}
}

func hasFormat(f uintptr) bool {
	if !openClipboard(20) {
		return false
	}
	defer pCloseClipboard.Call()
	r, _, _ := user32.NewProc("IsClipboardFormatAvailable").Call(f)
	return r != 0
}

func TestE2EPasteTextRestoresClipboard(t *testing.T) {
	dir := e2eDir(t)
	clearBox(t)
	if err := setClipboardText("previous clipboard"); err != nil {
		t.Fatal(err)
	}
	want := "Hola, ¿qué tal? Ñandú\nsegunda línea"
	if err := Current.Paste(want); err != nil {
		t.Fatal(err)
	}
	if got := waitBox(t, dir, want); got != want {
		t.Fatalf("the box got %q", got)
	}
	if got := Current.Clipboard(); got != "previous clipboard" {
		t.Fatalf("the clipboard was not restored: %q", got)
	}
}

func TestE2EPasteKeepsAnImageOnTheClipboard(t *testing.T) {
	dir := e2eDir(t)
	clearBox(t)
	setTestImage(t)
	want := "typed without touching the clipboard"
	if err := Current.Paste(want); err != nil {
		t.Fatal(err)
	}
	if got := waitBox(t, dir, want); got != want {
		t.Fatalf("the box got %q", got)
	}
	if !hasFormat(cfDIB) {
		t.Fatal("the image on the clipboard was lost")
	}
}

func TestE2EPasteIsHiddenFromClipboardHistory(t *testing.T) {
	e2eDir(t)
	clearBox(t)
	if err := offerDelayedText("x"); err != nil {
		t.Fatal(err)
	}
	for _, name := range historyNames {
		id, _, _ := pRegisterClipFormat.Call(uintptr(unsafe.Pointer(utf16Ptr(name))))
		if !hasFormat(id) {
			t.Errorf("format %s missing", name)
		}
	}
}

func TestE2ESoftHotkeyReachesTheFocusedWindow(t *testing.T) {
	dir := e2eDir(t)
	clearBox(t)
	if err := Current.Paste("abc"); err != nil {
		t.Fatal(err)
	}
	waitBox(t, dir, "abc")
	if err := Current.SoftHotkey("ctrl+a"); err != nil {
		t.Fatal(err)
	}
	if err := Current.SoftHotkey("delete"); err != nil {
		t.Fatal(err)
	}
	if got := waitBox(t, dir, ""); got != "" {
		t.Fatalf("select all and delete left %q", got)
	}
}

func TestE2EOpenAppDoesNotDuplicate(t *testing.T) {
	e2eDir(t)
	const exe = "charmap.exe"
	closeWindows(windowsOfImage(exe))
	time.Sleep(time.Second)
	for i := 0; i < 3; i++ {
		if err := Current.OpenApp("charmap"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1500 * time.Millisecond)
	}
	if n := len(windowsOfImage(exe)); n != 1 {
		t.Fatalf("expected one window, found %d", n)
	}
	if err := Current.QuitApp("charmap"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if n := len(windowsOfImage(exe)); n != 0 {
		t.Fatalf("the window did not close (%d left)", n)
	}
}

func TestE2EQuitAppIgnoresTitlesOfOtherPrograms(t *testing.T) {
	e2eDir(t)
	if err := Current.QuitApp("test box"); err == nil {
		t.Fatal("a title fragment must not close another program's window")
	}
	if len(topWindowsTitled("Typedeck test box")) == 0 {
		t.Fatal("the test box was closed by a title match")
	}
}

func TestE2EPopupReplacesThePreviousOne(t *testing.T) {
	e2eDir(t)
	for i := 0; i < 3; i++ {
		if !Current.Popup(PopupOptions{Title: "Layer " + string(rune('1'+i)), Seconds: 3}) {
			t.Fatal("the popup was not shown")
		}
	}
	time.Sleep(500 * time.Millisecond)
	n := 0
	enumerate(func(h uintptr) bool {
		if windowClass(h) == "TypedeckPopup" {
			n++
		}
		return true
	})
	if n != 1 {
		t.Fatalf("expected one popup on screen, found %d", n)
	}
}

func TestE2ENotifyBalloonNeedsNoPowerShell(t *testing.T) {
	e2eDir(t)
	StartShell(ShellHooks{Status: func() string { return "test" }})
	time.Sleep(time.Second)
	if !balloon("Typedeck", "balloon from the tray icon") {
		t.Fatal("the tray icon did not accept the balloon")
	}
}

func TestE2ESystemLayoutIsResolved(t *testing.T) {
	e2eDir(t)
	t.Logf("layout: %q, HKL %08X", Current.SystemLayout(), uint32(foregroundLayout()))
}

func TestE2EStoreAppsListed(t *testing.T) {
	e2eDir(t)
	for i := 0; i < 20 && len(storeApps()) == 0; i++ {
		time.Sleep(time.Second)
	}
	apps := storeApps()
	t.Logf("%d store apps, first: %+v", len(apps), apps[:min(3, len(apps))])
	if len(apps) == 0 {
		t.Fatal("no Microsoft Store apps found")
	}
}
