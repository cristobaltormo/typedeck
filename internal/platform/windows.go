//go:build windows

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/cristobaltormo/typedeck/internal/hid"
	"github.com/cristobaltormo/typedeck/internal/winreg"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	powrprof = syscall.NewLazyDLL("powrprof.dll")

	pGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	pShowWindow               = user32.NewProc("ShowWindow")
	pKeybdEvent               = user32.NewProc("keybd_event")
	pSendMessageW             = user32.NewProc("SendMessageW")
	pSendMessageTimeout       = user32.NewProc("SendMessageTimeoutW")
	pGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	pLockWorkStation          = user32.NewProc("LockWorkStation")
	pOpenClipboard            = user32.NewProc("OpenClipboard")
	pCloseClipboard           = user32.NewProc("CloseClipboard")
	pEmptyClipboard           = user32.NewProc("EmptyClipboard")
	pGetClipboardData         = user32.NewProc("GetClipboardData")
	pSetClipboardData         = user32.NewProc("SetClipboardData")
	pMessageBeep              = user32.NewProc("MessageBeep")
	pSetSuspendState          = powrprof.NewProc("SetSuspendState")

	pOpenProcess        = kernel32.NewProc("OpenProcess")
	pQueryFullImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	pGlobalAlloc        = kernel32.NewProc("GlobalAlloc")
	pGlobalLock         = kernel32.NewProc("GlobalLock")
	pGlobalUnlock       = kernel32.NewProc("GlobalUnlock")
	pSetExecutionState  = kernel32.NewProc("SetThreadExecutionState")
	pAttachConsole      = kernel32.NewProc("AttachConsole")
)

const personalizeKey = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

func ptr(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

type windows struct{}

func newPlatform() Platform { return &windows{} }

func (*windows) Name() string { return "windows" }

func (*windows) Version() string {
	out, err := Run(5*time.Second, "", "cmd", "/C", "ver")
	if err != nil {
		return ""
	}
	if i := strings.Index(out, "[Version "); i >= 0 {
		return strings.TrimSuffix(out[i+len("[Version "):], "]")
	}
	return strings.TrimSpace(out)
}

func AttachConsole() {
	if h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil && h != 0 && h != syscall.InvalidHandle {
		return
	}
	if r, _, _ := pAttachConsole.Call(^uintptr(0)); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}

func (*windows) OpenURL(u string) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "mailto:") {
		return errors.New("only http, https or mailto links can be opened")
	}
	return Detached("rundll32.exe", "url.dll,FileProtocolHandler", u)
}

func (*windows) System(cmd string) error {
	switch cmd {
	case "lock":
		if r, _, e := pLockWorkStation.Call(); r == 0 {
			return e
		}
		return nil
	case "sleep":
		if r, _, e := pSetSuspendState.Call(0, 0, 0); r == 0 {
			return e
		}
		return nil
	case "sleepdisplay":
		pSendMessageW.Call(0xFFFF, 0x112, 0xF170, 2)
		return nil
	case "screensaver":
		pSendMessageW.Call(0xFFFF, 0x112, 0xF140, 0)
		return nil
	case "screenshot":
		return sendCombo([]string{"gui", "shift"}, 'S')
	case "darkmode":
		return toggleDarkMode()
	}
	return ErrUnsupported
}

func toggleDarkMode() error {
	light, ok := winreg.DWORD(winreg.CurrentUser, personalizeKey, "AppsUseLightTheme")
	next := uint32(0)
	if ok && light == 0 {
		next = 1
	}
	for _, v := range []string{"AppsUseLightTheme", "SystemUsesLightTheme"} {
		if err := winreg.SetDWORD(winreg.CurrentUser, personalizeKey, v, next); err != nil {
			return err
		}
	}
	var result uintptr
	pSendMessageTimeout.Call(0xFFFF, 0x001A, 0, uintptr(unsafe.Pointer(utf16Ptr("ImmersiveColorSet"))), 0x2, 3000, uintptr(unsafe.Pointer(&result)))
	return nil
}

func (*windows) LockStroke() (hid.Stroke, bool) { return hid.Stroke{}, false }

func (*windows) KeepAwake() (func(), error) {
	stop := make(chan struct{})
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		pSetExecutionState.Call(0x80000000 | 0x1 | 0x2)
		close(ready)
		<-stop
		pSetExecutionState.Call(0x80000000)
	}()
	<-ready
	return func() { close(stop) }, nil
}

func (*windows) Notify(title, body string, sound bool) {
	if title != "" || body != "" {
		balloon(title, body)
	}
	if sound {
		pMessageBeep.Call(0)
	}
}

func (*windows) AdaptStroke(st hid.Stroke, _ *hid.Layout) hid.Stroke { return st }

func (*windows) Info(l *hid.Layout) SystemInfo {
	return SystemInfo{TypingLayout: l.Name, ModRemap: []string{}}
}

func (*windows) ShellCommand(cmd string) (string, []string) { return "cmd", []string{"/C", cmd} }

func (*windows) QuoteArg(s string) string { return quoteCmdArg(s) }

func (*windows) LogFile() string { return filepath.Join(localDataDir(), "typedeck.log") }

func localDataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, "typedeck")
}

func (*windows) ConfigDir() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(base, "typedeck")
}

func PrivateDir(configDir string) string {
	if configDir != Current.ConfigDir() {
		return configDir
	}
	dir := localDataDir()
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

func MigratePrivate(configDir, name string) {
	dst := filepath.Join(PrivateDir(configDir), name)
	src := filepath.Join(configDir, name)
	if dst == src {
		return
	}
	if _, err := os.Stat(dst); err == nil {
		return
	}
	if _, err := os.Stat(src); err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(dst), 0o700) == nil && os.Rename(src, dst) != nil {
		if b, err := os.ReadFile(src); err == nil && os.WriteFile(dst, b, 0o600) == nil {
			_ = os.Remove(src)
		}
	}
}

func (*windows) Doctor() []Check {
	checks := []Check{
		{have("taskkill"), "taskkill available (closing applications)", "taskkill is missing: applications cannot be closed"},
		{have("schtasks"), "schtasks available (start with the session)", "schtasks is missing: typedeck install will not work"},
		{have("powershell.exe"), "PowerShell available (Microsoft Store apps in the list)", "optional: PowerShell is missing: Microsoft Store apps will not be listed"},
	}
	installed, how := autostartInstalled()
	return append(checks, Check{installed, "starts with your session (" + how + ")", "optional: not set to start with your session (run typedeck install)"})
}
