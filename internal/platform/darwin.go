//go:build darwin

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type darwin struct {
	layoutMu     sync.Mutex
	layoutCached string
	layoutAt     time.Time
}

func newPlatform() Platform { return &darwin{} }

func (*darwin) Name() string { return "darwin" }

func (*darwin) Version() string {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func osascript(script string) (string, error) {
	return Run(10*time.Second, "", "osascript", "-e", script)
}

func (*darwin) OpenURL(u string) error {
	_, err := Run(10*time.Second, "", "open", u)
	return err
}

var chromiumBrowsers = []string{"Google Chrome", "Brave Browser", "Microsoft Edge", "Vivaldi", "Chromium", "Opera"}

func focusScript(script string) (string, error) {
	return Run(4*time.Second, "", "osascript", "-e", script)
}

func browserRunning(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}

func chromiumFocus(app, url string) string {
	return fmt.Sprintf(`tell application %q
	set allURLs to URL of every tab of every window
	repeat with wi from 1 to count of allURLs
		set urls to item wi of allURLs
		repeat with ti from 1 to count of urls
			if (item ti of urls) starts with %q then
				set w to window wi
				set active tab index of w to ti
				set index of w to 1
				activate
				return "ok"
			end if
		end repeat
	end repeat
end tell
return ""`, app, url)
}

func safariFocus(url string) string {
	return fmt.Sprintf(`tell application "Safari"
	set allURLs to URL of every tab of every window
	repeat with wi from 1 to count of allURLs
		set urls to item wi of allURLs
		repeat with ti from 1 to count of urls
			if (item ti of urls) starts with %q then
				set w to window wi
				set current tab of w to tab ti of w
				set index of w to 1
				activate
				return "ok"
			end if
		end repeat
	end repeat
end tell
return ""`, url)
}

func (*darwin) FocusEditor(url string) bool {
	for _, b := range chromiumBrowsers {
		if browserRunning(b) {
			if out, err := focusScript(chromiumFocus(b, url)); err == nil && out == "ok" {
				return true
			}
		}
	}
	if browserRunning("Safari") {
		if out, err := focusScript(safariFocus(url)); err == nil && out == "ok" {
			return true
		}
	}
	return false
}

func (*darwin) OpenApp(name string) error {
	_, err := Run(10*time.Second, "", "open", "-a", name)
	return err
}

func (*darwin) QuitApp(name string) error {
	_, err := osascript(fmt.Sprintf("tell application %q to quit", name))
	return err
}

func (*darwin) HideApp(name string) error {
	_, err := osascript(fmt.Sprintf(`tell application "System Events" to set visible of process %q to false`, name))
	return err
}

var (
	nameRe = regexp.MustCompile(`^\s*"([^"]+)"`)
	pathRe = regexp.MustCompile(`bundle path="([^"]+)"`)
)

func (*darwin) FrontID() string {
	out, err := Run(3*time.Second, "", "lsappinfo", "front")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (p *darwin) FrontNames(asn string) []string {
	if asn == "" {
		asn = p.FrontID()
	}
	if asn == "" {
		return nil
	}
	out, _ := Run(3*time.Second, "", "lsappinfo", "info", "-only", "name", "-only", "bundlepath", asn)
	var names []string
	if m := nameRe.FindStringSubmatch(out); m != nil {
		names = append(names, m[1])
	}
	if m := pathRe.FindStringSubmatch(out); m != nil {
		base := strings.TrimSuffix(filepath.Base(m[1]), ".app")
		if len(names) == 0 || names[0] != base {
			names = append(names, base)
		}
	}
	return names
}

func (*darwin) InstalledApps() []string {
	seen := map[string]bool{}
	home, _ := os.UserHomeDir()
	for _, base := range []string{"/Applications", "/Applications/Utilities", "/System/Applications", "/System/Applications/Utilities", filepath.Join(home, "Applications")} {
		m, _ := filepath.Glob(filepath.Join(base, "*.app"))
		for _, p := range m {
			seen[strings.TrimSuffix(filepath.Base(p), ".app")] = true
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

var macMods = map[string]string{"gui": "command down", "shift": "shift down", "alt": "option down", "ctrl": "control down"}

func using(mods []string) string {
	var out []string
	for _, m := range mods {
		out = append(out, macMods[m])
	}
	if len(out) == 0 {
		return ""
	}
	return " using {" + strings.Join(out, ", ") + "}"
}

func (*darwin) SoftHotkey(spec string) error {
	mods, key, err := splitHotkey(spec)
	if err != nil {
		return err
	}
	if u, ok := hid.KeyUsage(key); ok && len([]rune(key)) > 1 {
		_, err := osascript(fmt.Sprintf(`tell application "System Events" to key code %d%s`, macKeyCode(u), using(mods)))
		return err
	}
	if len([]rune(key)) != 1 {
		return fmt.Errorf("tecla desconocida: %s", key)
	}
	_, err = osascript(fmt.Sprintf(`tell application "System Events" to keystroke %q%s`, key, using(mods)))
	return err
}

func (*darwin) SoftStroke(st hid.Stroke) error {
	var mods []string
	for _, m := range []struct {
		bit  byte
		name string
	}{{hid.ModGui, "gui"}, {hid.ModShift, "shift"}, {hid.ModAlt, "alt"}, {hid.ModCtrl, "ctrl"}} {
		if st.Mods&m.bit != 0 {
			mods = append(mods, m.name)
		}
	}
	_, err := osascript(fmt.Sprintf(`tell application "System Events" to key code %d%s`, macKeyCode(st.Usage), using(mods)))
	return err
}

func macKeyCode(u byte) int {
	letters := map[byte]int{0x04: 0, 0x16: 1, 0x07: 2, 0x09: 3, 0x08: 14, 0x0A: 5, 0x0B: 4, 0x0D: 38, 0x0C: 34, 0x0E: 40, 0x0F: 37,
		0x10: 46, 0x11: 45, 0x12: 31, 0x13: 35, 0x14: 12, 0x15: 15, 0x17: 17, 0x18: 32, 0x19: 9, 0x1A: 13, 0x1B: 7, 0x1C: 16,
		0x1D: 6, 0x05: 11, 0x06: 8, 0x1E: 18, 0x1F: 19, 0x20: 20, 0x21: 21, 0x22: 23, 0x23: 22, 0x24: 26, 0x25: 28, 0x26: 25,
		0x27: 29, 0x28: 36, 0x29: 53, 0x2A: 51, 0x2B: 48, 0x2C: 49, 0x4F: 124, 0x50: 123, 0x51: 125, 0x52: 126}
	if v, ok := letters[u]; ok {
		return v
	}
	if u >= 0x3A && u <= 0x45 {
		return []int{122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111}[u-0x3A]
	}
	return 0
}

func (*darwin) Clipboard() string {
	out, _ := exec.Command("pbpaste").Output()
	return string(out)
}

func (p *darwin) Paste(text string) error {
	old := p.Clipboard()
	if _, err := Run(3*time.Second, text, "pbcopy"); err != nil {
		return err
	}
	_, err := osascript(`tell application "System Events" to keystroke "v" using {command down}`)
	time.Sleep(300 * time.Millisecond)
	_, _ = Run(3*time.Second, old, "pbcopy")
	return err
}

func (*darwin) SoftMedia(cmd string, step int) (string, error) {
	switch cmd {
	case "volup", "voldown":
		sign := map[string]string{"volup": "+", "voldown": "-"}[cmd]
		if _, err := osascript(fmt.Sprintf("set volume output volume ((output volume of (get volume settings)) %s %d)", sign, step)); err != nil {
			return "", err
		}
		v, _ := osascript("output volume of (get volume settings)")
		return v + " %", nil
	case "mute":
		_, err := osascript("set volume output muted not (output muted of (get volume settings))")
		return "", err
	}
	verb := map[string]string{"playpause": "playpause", "next": "next track", "prev": "previous track"}[cmd]
	for _, p := range []string{"Spotify", "Music"} {
		if exec.Command("pgrep", "-x", p).Run() == nil {
			_, err := osascript(fmt.Sprintf(`tell application %q to %s`, p, verb))
			return p, err
		}
	}
	return "", errors.New("no hay Spotify ni Música abiertos")
}

func (*darwin) System(cmd string) error {
	switch cmd {
	case "sleepdisplay":
		_, err := Run(5*time.Second, "", "pmset", "displaysleepnow")
		return err
	case "sleep":
		_, err := Run(5*time.Second, "", "pmset", "sleepnow")
		return err
	case "screensaver":
		_, err := Run(5*time.Second, "", "open", "-a", "ScreenSaverEngine")
		return err
	case "screenshot":
		return Detached("screencapture", "-i", "-c")
	case "darkmode":
		_, err := osascript(`tell application "System Events" to tell appearance preferences to set dark mode to not dark mode`)
		return err
	case "lock":
		return (&darwin{}).SoftStroke(hid.Stroke{Usage: 0x14, Mods: hid.ModCtrl | hid.ModGui})
	}
	return ErrUnsupported
}

func (*darwin) LockStroke() (hid.Stroke, bool) {
	return hid.Stroke{Usage: 0x14, Mods: hid.ModCtrl | hid.ModGui}, true
}

func (*darwin) KeepAwake() (func(), error) {
	cmd := exec.Command("caffeinate", "-dis")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() {
		_ = cmd.Process.Kill()
		go func() { _ = cmd.Wait() }()
	}, nil
}

func (*darwin) Notify(title, body string, sound bool) {
	if title != "" || body != "" {
		if body == "" {
			body = title
		}
		_ = Detached("osascript", "-e", fmt.Sprintf("display notification %q with title %q", body, title))
	}
	if sound {
		_ = Detached("afplay", "/System/Library/Sounds/Tink.aiff")
	}
}

func (p *darwin) SystemLayout() string {
	p.layoutMu.Lock()
	defer p.layoutMu.Unlock()
	if time.Since(p.layoutAt) > 30*time.Second {
		out, _ := Run(3*time.Second, "", "defaults", "read", "com.apple.HIToolbox", "AppleCurrentKeyboardLayoutInputSourceID")
		p.layoutCached, p.layoutAt = hid.LayoutFromInputSource(strings.TrimSpace(out)), time.Now()
	}
	return p.layoutCached
}

func (*darwin) Popup(PopupOptions) bool { return false }

func (*darwin) ShellCommand(cmd string) (string, []string) { return "/bin/sh", []string{"-c", cmd} }

func (*darwin) QuoteArg(s string) string { return unixQuote(s) }

func (*darwin) LogFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "typedeck.log")
}

func (*darwin) ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "typedeck")
}

func (*darwin) Doctor() []Check {
	return []Check{
		{have("lsappinfo"), "lsappinfo disponible (capas por aplicación)", "falta lsappinfo: sin él no hay capas por aplicación"},
		{have("osascript"), "osascript disponible", "falta osascript: abrir y cerrar aplicaciones lo necesita"},
	}
}

const launchAgentLabel = "cc.cristobal.typedeck"

func (*darwin) InstallService(exe string) (string, error) {
	home, _ := os.UserHomeDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string></dict>
<key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, launchAgentLabel, exe, (&darwin{}).LogFile(), (&darwin{}).LogFile())
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(plist, []byte(body), 0o644); err != nil {
		return "", err
	}
	uid := fmt.Sprint(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+launchAgentLabel).Run()
	if out, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plist).CombinedOutput(); err != nil {
		return "", fmt.Errorf("launchctl: %s", strings.TrimSpace(string(out)))
	}
	return "Servicio instalado y en marcha: " + plist, nil
}

func (*darwin) UninstallService() (string, error) {
	home, _ := os.UserHomeDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	_ = exec.Command("launchctl", "bootout", "gui/"+fmt.Sprint(os.Getuid())+"/"+launchAgentLabel).Run()
	if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "Servicio quitado (la configuración se conserva)", nil
}
