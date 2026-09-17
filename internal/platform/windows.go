//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	pShowWindow               = user32.NewProc("ShowWindow")
	pKeybdEvent               = user32.NewProc("keybd_event")
	pVkKeyScanW               = user32.NewProc("VkKeyScanW")
	pSendMessageW             = user32.NewProc("SendMessageW")
	pGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	pOpenClipboard            = user32.NewProc("OpenClipboard")
	pCloseClipboard           = user32.NewProc("CloseClipboard")
	pEmptyClipboard           = user32.NewProc("EmptyClipboard")
	pGetClipboardData         = user32.NewProc("GetClipboardData")
	pSetClipboardData         = user32.NewProc("SetClipboardData")
	pMessageBeep              = user32.NewProc("MessageBeep")

	pOpenProcess        = kernel32.NewProc("OpenProcess")
	pQueryFullImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	pGlobalAlloc        = kernel32.NewProc("GlobalAlloc")
	pGlobalLock         = kernel32.NewProc("GlobalLock")
	pGlobalUnlock       = kernel32.NewProc("GlobalUnlock")
	pSetExecutionState  = kernel32.NewProc("SetThreadExecutionState")
	pAttachConsole      = kernel32.NewProc("AttachConsole")
)

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

// AttachConsole reconnects stdout to the parent console. The executable is built for the GUI subsystem so that it starts
// with the session without opening a window, but redirected or piped output (ssh, files) is left alone.
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

var badChars = strings.NewReplacer("&", "", "|", "", "<", "", ">", "", "^", "", "\"", "", "%", "")

func (*windows) OpenURL(u string) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "mailto:") {
		return errors.New("solo se abren enlaces http, https o mailto")
	}
	return Detached("rundll32.exe", "url.dll,FileProtocolHandler", u)
}

func startMenuDirs() []string {
	var dirs []string
	if p := os.Getenv("ProgramData"); p != "" {
		dirs = append(dirs, filepath.Join(p, `Microsoft\Windows\Start Menu\Programs`))
	}
	if p := os.Getenv("APPDATA"); p != "" {
		dirs = append(dirs, filepath.Join(p, `Microsoft\Windows\Start Menu\Programs`))
	}
	return dirs
}

type shortcut struct{ Name, Path string }

var (
	lnkMu sync.Mutex
	lnks  []shortcut
	lnkAt time.Time
)

func shortcuts() []shortcut {
	lnkMu.Lock()
	defer lnkMu.Unlock()
	if lnks != nil && time.Since(lnkAt) < 60*time.Second {
		return lnks
	}
	seen := map[string]bool{}
	var out []shortcut
	for _, d := range startMenuDirs() {
		_ = filepath.WalkDir(d, func(p string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.EqualFold(filepath.Ext(p), ".lnk") {
				return nil
			}
			n := strings.TrimSuffix(de.Name(), filepath.Ext(p))
			if strings.Contains(strings.ToLower(n), "uninstall") || strings.Contains(strings.ToLower(n), "desinstalar") || seen[strings.ToLower(n)] {
				return nil
			}
			seen[strings.ToLower(n)] = true
			out = append(out, shortcut{n, p})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	lnks, lnkAt = out, time.Now()
	return out
}

func findShortcut(name string) (shortcut, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, s := range shortcuts() {
		if strings.ToLower(s.Name) == n {
			return s, true
		}
	}
	for _, s := range shortcuts() {
		if strings.Contains(strings.ToLower(s.Name), n) {
			return s, true
		}
	}
	return shortcut{}, false
}

func (*windows) OpenApp(name string) error {
	name = strings.TrimSpace(name)
	if s, ok := findShortcut(name); ok {
		return Detached("cmd", "/C", "start", "", s.Path)
	}
	if name = badChars.Replace(name); name == "" {
		return errors.New("falta el nombre de la app")
	}
	return Detached("cmd", "/C", "start", "", name)
}

// QuitApp asks nicely first: taskkill by executable, then WM_CLOSE to every visible window whose title matches. Store apps
// ignore taskkill without /F.
func (*windows) QuitApp(name string) error {
	name = badChars.Replace(strings.TrimSpace(name))
	if name == "" {
		return errors.New("falta el nombre de la app")
	}
	exe := name
	if !strings.HasSuffix(strings.ToLower(exe), ".exe") {
		exe += ".exe"
	}
	if _, err := Run(5*time.Second, "", "taskkill", "/IM", exe); err == nil {
		return nil
	}
	if closeWindowsTitled(name) > 0 {
		return nil
	}
	return fmt.Errorf("no se pudo cerrar %q (¿está abierta?)", name)
}

var (
	pEnumWindows     = user32.NewProc("EnumWindows")
	pGetWindowTextW  = user32.NewProc("GetWindowTextW")
	pIsWindowVisible = user32.NewProc("IsWindowVisible")
	pPostMessageW    = user32.NewProc("PostMessageW")
)

func closeWindowsTitled(sub string) int {
	sub = strings.ToLower(sub)
	n := 0
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		buf := make([]uint16, 256)
		l, _, _ := pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if l > 0 && strings.Contains(strings.ToLower(syscall.UTF16ToString(buf[:l])), sub) {
			pPostMessageW.Call(hwnd, 0x0010, 0, 0)
			n++
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	return n
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
	names := []string{base}
	for _, s := range shortcuts() {
		if strings.Contains(strings.ToLower(s.Name), strings.ToLower(base)) {
			names = append(names, s.Name)
			break
		}
	}
	return names
}

func (*windows) InstalledApps() []string {
	var out []string
	for _, s := range shortcuts() {
		out = append(out, s.Name)
	}
	return out
}

var vkMods = map[string]uint16{"ctrl": 0x11, "shift": 0x10, "alt": 0x12, "gui": 0x5B}

func keyEvent(vk uint16, up bool) {
	flags := uintptr(0)
	if up {
		flags = 2
	}
	switch vk {
	case 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x2D, 0x2E, 0x5B:
		flags |= 1
	}
	pKeybdEvent.Call(uintptr(vk), 0, flags, 0)
}

func sendCombo(mods []string, vk uint16) error {
	if vk == 0 {
		return errors.New("tecla sin equivalente en Windows")
	}
	for _, m := range mods {
		keyEvent(vkMods[m], false)
	}
	keyEvent(vk, false)
	time.Sleep(10 * time.Millisecond)
	keyEvent(vk, true)
	for i := len(mods) - 1; i >= 0; i-- {
		keyEvent(vkMods[mods[i]], true)
	}
	return nil
}

func (*windows) SoftHotkey(spec string) error {
	mods, key, err := splitHotkey(spec)
	if err != nil {
		return err
	}
	if u, ok := hid.KeyUsage(key); ok {
		return sendCombo(mods, vkOf(u))
	}
	if r := []rune(key); len(r) == 1 {
		v, _, _ := pVkKeyScanW.Call(uintptr(r[0]))
		if int16(v) == -1 {
			return fmt.Errorf("tecla desconocida: %s", key)
		}
		m := append([]string{}, mods...)
		if v&0x100 != 0 {
			m = append(m, "shift")
		}
		return sendCombo(m, uint16(v&0xFF))
	}
	return fmt.Errorf("tecla desconocida: %s", key)
}

func (*windows) SoftStroke(st hid.Stroke) error {
	var mods []string
	for _, m := range []struct {
		bit  byte
		name string
	}{{hid.ModCtrl, "ctrl"}, {hid.ModShift, "shift"}, {hid.ModAlt, "alt"}, {hid.ModGui, "gui"}} {
		if st.Mods&m.bit != 0 {
			mods = append(mods, m.name)
		}
	}
	return sendCombo(mods, vkOf(st.Usage))
}

func (*windows) Clipboard() string {
	if r, _, _ := pOpenClipboard.Call(0); r == 0 {
		return ""
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(13)
	if h == 0 {
		return ""
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(h)
	var u []uint16
	for i := 0; i < 1<<20; i++ {
		c := *(*uint16)(ptr(p + uintptr(i)*2))
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func setClipboard(text string) error {
	u := utf16.Encode([]rune(text + "\x00"))
	for i := 0; ; i++ {
		if r, _, _ := pOpenClipboard.Call(0); r != 0 {
			break
		}
		if i > 20 {
			return errors.New("no se pudo abrir el portapapeles")
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAlloc.Call(0x2, uintptr(len(u)*2))
	if h == 0 {
		return errors.New("sin memoria para el portapapeles")
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return errors.New("no se pudo reservar el portapapeles")
	}
	copy(unsafe.Slice((*uint16)(ptr(p)), len(u)), u)
	pGlobalUnlock.Call(h)
	if r, _, _ := pSetClipboardData.Call(13, h); r == 0 {
		return errors.New("no se pudo escribir en el portapapeles")
	}
	return nil
}

func (p *windows) Paste(text string) error {
	old := p.Clipboard()
	if err := setClipboard(text); err != nil {
		return err
	}
	err := sendCombo([]string{"ctrl"}, 'V')
	time.Sleep(300 * time.Millisecond)
	_ = setClipboard(old)
	return err
}

func (*windows) SoftMedia(cmd string, step int) (string, error) {
	vk := map[string]uint16{"playpause": 0xB3, "next": 0xB0, "prev": 0xB1, "volup": 0xAF, "voldown": 0xAE, "mute": 0xAD}[cmd]
	if vk == 0 {
		return "", fmt.Errorf("orden multimedia desconocida: %s", cmd)
	}
	n := 1
	if cmd == "volup" || cmd == "voldown" {
		if n = step / 2; n < 1 {
			n = 1
		}
	}
	for i := 0; i < n; i++ {
		keyEvent(vk, false)
		keyEvent(vk, true)
	}
	return "", nil
}

func (*windows) System(cmd string) error {
	switch cmd {
	case "lock":
		_, err := Run(5*time.Second, "", "rundll32.exe", "user32.dll,LockWorkStation")
		return err
	case "sleep":
		_, err := Run(5*time.Second, "", "rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0")
		return err
	case "sleepdisplay":
		pSendMessageW.Call(0xFFFF, 0x112, 0xF170, 2)
		return nil
	case "screensaver":
		pSendMessageW.Call(0xFFFF, 0x112, 0xF140, 0)
		return nil
	case "screenshot":
		return sendCombo([]string{"gui", "shift"}, 'S')
	case "darkmode":
		const key = `HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
		out, err := Run(3*time.Second, "", "reg", "query", key, "/v", "AppsUseLightTheme")
		if err != nil {
			return err
		}
		next := "0"
		if strings.Contains(out, "0x0") {
			next = "1"
		}
		for _, v := range []string{"AppsUseLightTheme", "SystemUsesLightTheme"} {
			if _, err := Run(3*time.Second, "", "reg", "add", key, "/v", v, "/t", "REG_DWORD", "/d", next, "/f"); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnsupported
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
		q := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
		script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms,System.Drawing;`+
			`$n=New-Object System.Windows.Forms.NotifyIcon;$n.Icon=[System.Drawing.SystemIcons]::Information;$n.Visible=$true;`+
			`$n.ShowBalloonTip(2500,'%s','%s',[System.Windows.Forms.ToolTipIcon]::None);Start-Sleep -Seconds 4;$n.Dispose()`, q(title), q(body))
		_ = Detached("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	}
	if sound {
		pMessageBeep.Call(0)
	}
}

func (*windows) SystemLayout() string {
	hwnd, _ := foreground()
	var tid uintptr
	if hwnd != 0 {
		tid, _, _ = pGetWindowThreadProcessID.Call(hwnd, 0)
	}
	hkl, _, _ := pGetKeyboardLayout.Call(tid)
	layoutID := uint16(hkl >> 16)
	if layoutID&0xF000 == 0xF000 {
		return ""
	}
	switch layoutID {
	case 0x040A:
		return "es-win"
	case 0x0409:
		return "us"
	}
	switch layoutID & 0x3FF {
	case 0x09, 0x0A, 0x16, 0x10, 0x13, 0x1D, 0x14, 0x06:
		return "qwerty"
	}
	return ""
}

func (*windows) AdaptStroke(st hid.Stroke, _ *hid.Layout) hid.Stroke { return st }

func (*windows) Info(l *hid.Layout) SystemInfo {
	return SystemInfo{TypingLayout: l.Name, ModRemap: []string{}}
}

func (*windows) ShellCommand(cmd string) (string, []string) { return "cmd", []string{"/C", cmd} }

// QuoteArg quotes for cmd.exe. Inside double quotes & | < > are inert, but quotes, % and ^ are not, so they are dropped.
func (*windows) QuoteArg(s string) string {
	s = strings.NewReplacer(`"`, "", "%", "", "^", "", "\r", " ", "\n", " ").Replace(s)
	return `"` + s + `"`
}

func (*windows) LogFile() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, "typedeck", "typedeck.log")
}

func (*windows) ConfigDir() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(base, "typedeck")
}

func (*windows) Doctor() []Check {
	return []Check{
		{have("taskkill"), "taskkill disponible (cerrar aplicaciones)", "falta taskkill: no se podrán cerrar aplicaciones"},
		{have("schtasks"), "schtasks disponible (arranque con la sesión)", "falta schtasks: typedeck install no funcionará"},
		{have("powershell.exe"), "PowerShell disponible (carteles de aviso)", "falta PowerShell: no habrá carteles de aviso"},
	}
}

const taskName = "Typedeck"

func (*windows) InstallService(exe string) (string, error) {
	if out, err := Run(15*time.Second, "", "schtasks", "/Create", "/TN", taskName, "/TR", `"`+exe+`"`, "/SC", "ONLOGON", "/RL", "LIMITED", "/F"); err != nil {
		return "", fmt.Errorf("schtasks: %s", out)
	}
	_, _ = Run(15*time.Second, "", "schtasks", "/Run", "/TN", taskName)
	return "Tarea programada «" + taskName + "» creada: arranca al iniciar sesión (y ya está en marcha)", nil
}

func (*windows) UninstallService() (string, error) {
	_, _ = Run(15*time.Second, "", "schtasks", "/End", "/TN", taskName)
	if out, err := Run(15*time.Second, "", "schtasks", "/Delete", "/TN", taskName, "/F"); err != nil && !strings.Contains(strings.ToLower(out), "no se") && !strings.Contains(strings.ToLower(out), "cannot find") {
		return "", fmt.Errorf("schtasks: %s", out)
	}
	_ = exec.Command("taskkill", "/IM", "typedeck.exe").Run()
	return "Tarea quitada (la configuración se conserva)", nil
}
