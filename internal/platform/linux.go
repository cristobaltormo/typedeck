//go:build linux

package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type linux struct{}

func newPlatform() Platform {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		if _, err := os.Stat("/tmp/.X11-unix/X0"); err == nil {
			_ = os.Setenv("DISPLAY", ":0")
		}
	}
	uid := os.Getuid()
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		if d := fmt.Sprintf("/run/user/%d", uid); dirExists(d) {
			_ = os.Setenv("XDG_RUNTIME_DIR", d)
		}
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		if b := fmt.Sprintf("/run/user/%d/bus", uid); fileExists(b) {
			_ = os.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+b)
		}
	}
	return &linux{}
}

func dirExists(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }
func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func (*linux) Name() string { return "linux" }

func (*linux) Version() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

func wayland() bool {
	if t := os.Getenv("XDG_SESSION_TYPE"); t != "" {
		return t == "wayland"
	}
	return os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("DISPLAY") == ""
}

func (*linux) OpenURL(u string) error {
	if !have("xdg-open") {
		return errors.New("xdg-open is missing (xdg-utils package)")
	}
	return Detached("xdg-open", u)
}

func (*linux) OpenApp(name string) error {
	if e, ok := findEntry(name); ok {
		switch {
		case have("gtk-launch"):
			if err := Launch("gtk-launch", e.ID); err == nil {
				return nil
			}
		case have("gio"):
			if err := Launch("gio", "launch", e.Path); err == nil {
				return nil
			}
		}
		if args := expandExec(e.Exec); len(args) > 0 {
			return Detached(args[0], args[1:]...)
		}
	}
	if p, err := exec.LookPath(strings.ToLower(strings.TrimSpace(name))); err == nil {
		return Detached(p)
	}
	return fmt.Errorf("application %q not found", name)
}

func (*linux) QuitApp(name string) error {
	base := strings.ToLower(strings.TrimSpace(name))
	if e, ok := findEntry(name); ok {
		base = execBase(e.Exec)
	}
	if base == "" {
		return errors.New("the app name is missing")
	}
	if len(base) > 15 {
		base = base[:15]
	}
	if _, err := Run(3*time.Second, "", "pkill", "-TERM", "-x", base); err != nil {
		return fmt.Errorf("could not close %q (is it open?)", name)
	}
	return nil
}

func (*linux) HideApp(name string) error {
	switch {
	case !wayland() && have("xdotool"):
		_, err := Run(3*time.Second, "", "xdotool", "getactivewindow", "windowminimize")
		return err
	case have("hyprctl"):
		_, err := Run(3*time.Second, "", "hyprctl", "dispatch", "movetoworkspacesilent", "special")
		return err
	}
	return ErrUnsupported
}

type frontInfo struct {
	id    string
	class string
	pid   int
	x11   bool
}

// front finds the focused window on Hyprland, Sway or X11 (EWMH first, then the input focus when no window manager sets
// _NET_ACTIVE_WINDOW). GNOME and KDE on Wayland expose nothing, so per-application layers are unavailable there.
func front() frontInfo {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && have("hyprctl") {
		out, err := Run(2*time.Second, "", "hyprctl", "activewindow", "-j")
		if err == nil {
			var w struct {
				Address string `json:"address"`
				Class   string `json:"class"`
				PID     int    `json:"pid"`
			}
			if json.Unmarshal([]byte(out), &w) == nil && w.Address != "" {
				return frontInfo{id: w.Address, class: w.Class, pid: w.PID}
			}
		}
		return frontInfo{}
	}
	if os.Getenv("SWAYSOCK") != "" && have("swaymsg") {
		out, err := Run(2*time.Second, "", "swaymsg", "-t", "get_tree")
		if err == nil {
			var root map[string]any
			if json.Unmarshal([]byte(out), &root) == nil {
				if n := focusedNode(root); n != nil {
					id, _ := n["id"].(float64)
					class, _ := n["app_id"].(string)
					if class == "" {
						if wp, ok := n["window_properties"].(map[string]any); ok {
							class, _ = wp["class"].(string)
						}
					}
					pid, _ := n["pid"].(float64)
					return frontInfo{id: strconv.Itoa(int(id)), class: class, pid: int(pid)}
				}
			}
		}
		return frontInfo{}
	}
	if have("xprop") && os.Getenv("DISPLAY") != "" {
		out, _ := Run(2*time.Second, "", "xprop", "-root", "_NET_ACTIVE_WINDOW")
		if i := strings.LastIndex(out, "0x"); i >= 0 {
			if id := strings.TrimSpace(out[i:]); id != "0x0" {
				return frontInfo{id: id, x11: true}
			}
		}
		if have("xdotool") {
			if out, err := Run(2*time.Second, "", "xdotool", "getwindowfocus"); err == nil && strings.TrimSpace(out) != "" {
				return frontInfo{id: strings.TrimSpace(out), x11: true}
			}
		}
	}
	return frontInfo{}
}

func focusedNode(n map[string]any) map[string]any {
	if f, _ := n["focused"].(bool); f {
		return n
	}
	for _, k := range []string{"nodes", "floating_nodes"} {
		if kids, ok := n[k].([]any); ok {
			for _, c := range kids {
				if m, ok := c.(map[string]any); ok {
					if r := focusedNode(m); r != nil {
						return r
					}
				}
			}
		}
	}
	return nil
}

func (*linux) FrontID() string { return front().id }

func (*linux) FrontNames(id string) []string {
	f := front()
	if f.id == "" || (id != "" && f.id != id) {
		return nil
	}
	if f.class == "" && f.pid == 0 && f.x11 {
		out, _ := Run(2*time.Second, "", "xprop", "-id", f.id, "WM_CLASS", "_NET_WM_PID")
		for _, l := range strings.Split(out, "\n") {
			if _, v, ok := strings.Cut(l, "WM_CLASS(STRING) = "); ok {
				parts := strings.Split(v, ",")
				f.class = strings.Trim(strings.TrimSpace(parts[len(parts)-1]), `"`)
				if len(parts) > 1 {
					f.class = strings.Trim(strings.TrimSpace(parts[0]), `"`) + "\x00" + f.class
				}
			}
			if _, v, ok := strings.Cut(l, "_NET_WM_PID(CARDINAL) = "); ok {
				f.pid, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
	}
	var names []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		for _, x := range names {
			if strings.EqualFold(x, n) {
				return
			}
		}
		if n != "" {
			names = append(names, n)
		}
	}
	classes := strings.Split(f.class, "\x00")
	comm := ""
	if f.pid > 0 {
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", f.pid)); err == nil {
			comm = strings.TrimSpace(string(b))
		}
	}
	for _, c := range classes {
		if e, ok := entryFor(c, comm); ok {
			add(e.Name)
			break
		}
	}
	for _, c := range classes {
		add(c)
	}
	add(comm)
	return names
}

func (*linux) InstalledApps() []string {
	var out []string
	for _, e := range loadEntries() {
		out = append(out, e.Name)
	}
	return out
}

var xkeyNames = map[byte]string{
	0x28: "Return", 0x29: "Escape", 0x2A: "BackSpace", 0x2B: "Tab", 0x2C: "space", 0x2D: "minus", 0x2E: "equal", 0x2F: "bracketleft", 0x30: "bracketright",
	0x31: "backslash", 0x33: "semicolon", 0x34: "apostrophe", 0x35: "grave", 0x36: "comma", 0x37: "period", 0x38: "slash", 0x39: "Caps_Lock",
	0x46: "Print", 0x47: "Scroll_Lock", 0x48: "Pause", 0x49: "Insert", 0x4A: "Home", 0x4B: "Prior", 0x4C: "Delete", 0x4D: "End", 0x4E: "Next",
	0x4F: "Right", 0x50: "Left", 0x51: "Down", 0x52: "Up", 0x53: "Num_Lock",
}

func xkey(u byte) string {
	switch {
	case u >= 0x04 && u <= 0x1D:
		return string(rune('a' + int(u) - 0x04))
	case u >= 0x1E && u <= 0x26:
		return string(rune('1' + int(u) - 0x1E))
	case u == 0x27:
		return "0"
	case u >= 0x3A && u <= 0x45:
		return fmt.Sprintf("F%d", int(u)-0x3A+1)
	case u >= 0x68 && u <= 0x73:
		return fmt.Sprintf("F%d", int(u)-0x68+13)
	}
	return xkeyNames[u]
}

var xmods = map[string]string{"gui": "super", "shift": "shift", "alt": "alt", "ctrl": "ctrl"}

var punctKeys = map[string]string{"+": "plus", "-": "minus", ".": "period", ",": "comma", "/": "slash", ";": "semicolon", "'": "apostrophe",
	"[": "bracketleft", "]": "bracketright", "\\": "backslash", "=": "equal", "`": "grave"}

func press(mods []string, key string) error {
	switch {
	case !wayland() && have("xdotool"):
		combo := append([]string{}, mods...)
		for i, m := range combo {
			combo[i] = xmods[m]
		}
		combo = append(combo, key)
		_, err := Run(3*time.Second, "", "xdotool", "key", "--clearmodifiers", strings.Join(combo, "+"))
		return err
	case have("wtype"):
		args := []string{}
		for _, m := range mods {
			args = append(args, "-M", map[string]string{"gui": "logo", "shift": "shift", "alt": "alt", "ctrl": "ctrl"}[m])
		}
		args = append(args, "-k", key)
		for i := len(mods) - 1; i >= 0; i-- {
			args = append(args, "-m", map[string]string{"gui": "logo", "shift": "shift", "alt": "alt", "ctrl": "ctrl"}[mods[i]])
		}
		_, err := Run(3*time.Second, "", "wtype", args...)
		return err
	}
	if wayland() {
		return errors.New("without the board, software keys need wtype (Wayland with wlroots) or X11; with the board connected they are not needed")
	}
	return errors.New("without the board, software keys need xdotool; with the board connected it is not needed")
}

func (*linux) SoftHotkey(spec string) error {
	mods, key, err := splitHotkey(spec)
	if err != nil {
		return err
	}
	if u, ok := hid.KeyUsage(key); ok {
		if k := xkey(u); k != "" {
			return press(mods, k)
		}
	}
	if k, ok := punctKeys[key]; ok {
		return press(mods, k)
	}
	if len([]rune(key)) == 1 {
		return press(mods, key)
	}
	return fmt.Errorf("unknown key: %s", key)
}

func (*linux) SoftStroke(st hid.Stroke) error {
	var mods []string
	for _, m := range []struct {
		bit  byte
		name string
	}{{hid.ModGui, "gui"}, {hid.ModShift, "shift"}, {hid.ModAlt, "alt"}, {hid.ModCtrl, "ctrl"}} {
		if st.Mods&m.bit != 0 {
			mods = append(mods, m.name)
		}
	}
	k := xkey(st.Usage)
	if k == "" {
		return fmt.Errorf("key without an equivalent: 0x%02X", st.Usage)
	}
	return press(mods, k)
}

func (*linux) Clipboard() string {
	for _, c := range [][]string{{"wl-paste", "-n"}, {"xclip", "-selection", "clipboard", "-o"}, {"xsel", "-ob"}} {
		if have(c[0]) && (c[0] == "wl-paste") == wayland() {
			out, err := exec.Command(c[0], c[1:]...).Output()
			if err == nil {
				return string(out)
			}
		}
	}
	return ""
}

func setClipboard(text string) error {
	for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "-ib"}} {
		if have(c[0]) && (c[0] == "wl-copy") == wayland() {
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}
	return errors.New("pasting text needs wl-clipboard (Wayland) or xclip (X11)")
}

func (p *linux) Paste(text string) error {
	old := p.Clipboard()
	if err := setClipboard(text); err != nil {
		return err
	}
	err := press([]string{"ctrl"}, "v")
	time.Sleep(300 * time.Millisecond)
	_ = setClipboard(old)
	return err
}

func (*linux) SoftMedia(cmd string, step int) (string, error) {
	switch cmd {
	case "volup", "voldown":
		sign := map[string]string{"volup": "+", "voldown": "-"}[cmd]
		switch {
		case have("wpctl"):
			_, err := Run(3*time.Second, "", "wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", fmt.Sprintf("%d%%%s", step, sign))
			return "", err
		case have("pactl"):
			_, err := Run(3*time.Second, "", "pactl", "set-sink-volume", "@DEFAULT_SINK@", fmt.Sprintf("%s%d%%", sign, step))
			return "", err
		case have("amixer"):
			_, err := Run(3*time.Second, "", "amixer", "-q", "set", "Master", fmt.Sprintf("%d%%%s", step, sign))
			return "", err
		}
		return "", errors.New("wpctl, pactl or amixer is missing")
	case "mute":
		switch {
		case have("wpctl"):
			_, err := Run(3*time.Second, "", "wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle")
			return "", err
		case have("pactl"):
			_, err := Run(3*time.Second, "", "pactl", "set-sink-mute", "@DEFAULT_SINK@", "toggle")
			return "", err
		case have("amixer"):
			_, err := Run(3*time.Second, "", "amixer", "-q", "set", "Master", "toggle")
			return "", err
		}
		return "", errors.New("wpctl, pactl or amixer is missing")
	}
	verb := map[string]string{"playpause": "play-pause", "next": "next", "prev": "previous"}[cmd]
	if verb == "" {
		return "", fmt.Errorf("unknown media command: %s", cmd)
	}
	if !have("playerctl") {
		return "", errors.New("playerctl is missing to control the player (not needed with the board connected)")
	}
	_, err := Run(3*time.Second, "", "playerctl", verb)
	return "", err
}

func firstOf(cmds ...[]string) error {
	for _, c := range cmds {
		if have(c[0]) {
			_, err := Run(5*time.Second, "", c[0], c[1:]...)
			return err
		}
	}
	return ErrUnsupported
}

func (*linux) System(cmd string) error {
	switch cmd {
	case "lock":
		return firstOf([]string{"loginctl", "lock-session"}, []string{"xdg-screensaver", "lock"})
	case "sleep":
		return firstOf([]string{"systemctl", "suspend"})
	case "sleepdisplay":
		switch {
		case os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "":
			return firstOf([]string{"hyprctl", "dispatch", "dpms", "off"})
		case os.Getenv("SWAYSOCK") != "":
			return firstOf([]string{"swaymsg", "output * dpms off"})
		case !wayland():
			return firstOf([]string{"xset", "dpms", "force", "off"})
		}
		return fmt.Errorf("turning the display off is not available on this Wayland desktop: %w", ErrUnsupported)
	case "screensaver":
		return firstOf([]string{"xdg-screensaver", "activate"}, []string{"loginctl", "lock-session"})
	case "screenshot":
		for _, c := range [][]string{{"gnome-screenshot", "-a", "-c"}, {"spectacle", "-r", "-b", "-c"}, {"flameshot", "gui"}, {"xfce4-screenshooter", "-r", "-c"}} {
			if have(c[0]) {
				return Detached(c[0], c[1:]...)
			}
		}
		if have("grim") && have("slurp") && have("wl-copy") {
			return Detached("sh", "-c", `grim -g "$(slurp)" - | wl-copy`)
		}
		return fmt.Errorf("a screenshot tool is missing (gnome-screenshot, spectacle, flameshot or grim+slurp): %w", ErrUnsupported)
	case "darkmode":
		if !have("gsettings") {
			return fmt.Errorf("switching the theme is only available on GNOME: %w", ErrUnsupported)
		}
		cur, err := Run(3*time.Second, "", "gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
		if err != nil {
			return err
		}
		next := "prefer-dark"
		if strings.Contains(cur, "prefer-dark") {
			next = "default"
		}
		_, err = Run(3*time.Second, "", "gsettings", "set", "org.gnome.desktop.interface", "color-scheme", next)
		return err
	}
	return ErrUnsupported
}

func (*linux) LockStroke() (hid.Stroke, bool) { return hid.Stroke{}, false }

func (*linux) KeepAwake() (func(), error) {
	if !have("systemd-inhibit") {
		return nil, ErrUnsupported
	}
	cmd := exec.Command("systemd-inhibit", "--what=idle:sleep", "--who=Typedeck", "--why=Mantener despierto", "--mode=block", "sleep", "infinity")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() {
		_ = cmd.Process.Kill()
		go func() { _ = cmd.Wait() }()
	}, nil
}

func (*linux) Notify(title, body string, sound bool) {
	if title != "" || body != "" {
		if have("notify-send") {
			_ = Detached("notify-send", "-a", "Typedeck", "-t", "2500", title, body)
		}
	}
	if sound {
		switch {
		case have("canberra-gtk-play"):
			_ = Detached("canberra-gtk-play", "-i", "message")
		case have("paplay") && fileExists("/usr/share/sounds/freedesktop/stereo/message.oga"):
			_ = Detached("paplay", "/usr/share/sounds/freedesktop/stereo/message.oga")
		}
	}
}

func xkbToHID(layout, variant string) string {
	layout, variant = strings.ToLower(strings.TrimSpace(layout)), strings.ToLower(strings.TrimSpace(variant))
	for _, v := range []string{"dvorak", "colemak", "workman", "norman", "dvp", "neo"} {
		if strings.HasPrefix(variant, v) {
			return ""
		}
	}
	switch {
	case layout == "us" && variant == "":
		return "us"
	case layout == "es" && (variant == "" || variant == "winkeys" || variant == "nodeadkeys"):
		return "es-pc"
	case layout == "us" || layout == "gb" || layout == "es" || layout == "pt" || layout == "br" || layout == "it" || layout == "latam" || layout == "ie" || layout == "ca":
		return "qwerty"
	}
	return ""
}

func (*linux) SystemLayout() string {
	if have("gsettings") {
		if out, err := Run(2*time.Second, "", "gsettings", "get", "org.gnome.desktop.input-sources", "mru-sources"); err == nil {
			if i := strings.Index(out, "'xkb', '"); i >= 0 {
				v := out[i+len("'xkb', '"):]
				if j := strings.Index(v, "'"); j > 0 {
					l, variant, _ := strings.Cut(v[:j], "+")
					return xkbToHID(l, variant)
				}
			}
		}
	}
	if have("setxkbmap") && os.Getenv("DISPLAY") != "" {
		out, _ := Run(2*time.Second, "", "setxkbmap", "-query")
		var l, v string
		for _, line := range strings.Split(out, "\n") {
			if k, val, ok := strings.Cut(line, ":"); ok {
				switch strings.TrimSpace(k) {
				case "layout":
					l, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				case "variant":
					v, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				}
			}
		}
		if l != "" {
			return xkbToHID(l, v)
		}
	}
	if have("localectl") {
		out, _ := Run(2*time.Second, "", "localectl", "status")
		var l, v string
		for _, line := range strings.Split(out, "\n") {
			if k, val, ok := strings.Cut(line, ":"); ok {
				switch strings.TrimSpace(k) {
				case "X11 Layout":
					l, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				case "X11 Variant":
					v, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				}
			}
		}
		if l != "" {
			return xkbToHID(l, v)
		}
	}
	return ""
}

func (*linux) AdaptStroke(st hid.Stroke, _ *hid.Layout) hid.Stroke { return st }

func (*linux) Info(l *hid.Layout) SystemInfo {
	return SystemInfo{TypingLayout: l.Name, ModRemap: []string{}}
}

func (*linux) Popup(PopupOptions) bool { return false }

func (*linux) ShellCommand(cmd string) (string, []string) { return "/bin/sh", []string{"-c", cmd} }

var browserClassRe = regexp.MustCompile(`(?i)firefox|navigator|chromium|chrome|brave|vivaldi|opera|librewolf|epiphany`)

func windowIsBrowser(id string) bool {
	if !have("xprop") {
		return true
	}
	class, err := Run(2*time.Second, "", "xprop", "-id", id, "WM_CLASS")
	return err == nil && browserClassRe.MatchString(class)
}

func (*linux) FocusEditor(string) bool {
	switch {
	case os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && have("hyprctl"):
		out, err := Run(2*time.Second, "", "hyprctl", "dispatch", "focuswindow", "title:Typedeck")
		return err == nil && strings.Contains(out, "ok")
	case os.Getenv("SWAYSOCK") != "" && have("swaymsg"):
		out, err := Run(2*time.Second, "", "swaymsg", `[title="Typedeck" app_id="(?i)(firefox|chromium|chrome|brave|vivaldi|opera|librewolf|epiphany)"] focus`)
		return err == nil && strings.Contains(out, `"success": true`)
	case !wayland() && have("xdotool"):
		ids, err := Run(3*time.Second, "", "xdotool", "search", "--onlyvisible", "--name", "Typedeck")
		if err != nil {
			return false
		}
		for _, id := range strings.Fields(ids) {
			if windowIsBrowser(id) {
				_, err = Run(3*time.Second, "", "xdotool", "windowactivate", id)
				return err == nil
			}
		}
		return false
	case !wayland() && have("wmctrl"):
		_, err := Run(3*time.Second, "", "wmctrl", "-a", "Typedeck")
		return err == nil
	}
	return false
}

func (*linux) QuoteArg(s string) string { return unixQuote(s) }

func (*linux) LogFile() string { return "" }

func (*linux) ConfigDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "typedeck")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "typedeck")
}

func (*linux) Doctor() []Check {
	sess := "X11"
	if wayland() {
		sess = "Wayland"
	}
	checks := []Check{
		{true, "graphical session: " + sess, ""},
		{have("xdg-open"), "xdg-open available (links and files)", "xdg-open is missing: install xdg-utils to open links"},
		{have("notify-send"), "notify-send available (popups)", "notify-send is missing: install libnotify-bin to see the popups"},
		{front().id != "", "the front application can be read (per-application layers)",
			"the active window cannot be read: per-application layers need X11 with xprop, Sway or Hyprland (GNOME and KDE on Wayland do not allow it)"},
		{(!wayland() && have("xdotool")) || have("wtype"), "software keys available (xdotool or wtype)",
			"without xdotool or wtype, software keys do not work; they are not needed with the board connected"},
	}
	acm, _ := filepath.Glob("/dev/ttyACM*")
	switch {
	case len(acm) == 0:
		checks = append(checks, Check{true, "no /dev/ttyACM*: the port permission cannot be checked (plug the board in)", ""})
	default:
		checks = append(checks, Check{len(candidatesACM()) > 0, "your user can open " + acm[0],
			"your user cannot open " + acm[0] + ": copy packaging/linux/99-typedeck.rules to /etc/udev/rules.d/ or add yourself to the dialout group (and sign in again)"})
	}
	return checks
}

func candidatesACM() []string {
	m, _ := filepath.Glob("/dev/ttyACM*")
	var ok []string
	for _, p := range m {
		if f, err := os.OpenFile(p, os.O_RDWR, 0); err == nil {
			f.Close()
			ok = append(ok, p)
		}
	}
	return ok
}

const unitName = "typedeck.service"

func unitPath() string {
	return filepath.Join(filepath.Dir((&linux{}).ConfigDir()), "systemd", "user", unitName)
}

func (*linux) InstallService(exe string) (string, error) {
	if !have("systemctl") {
		return "", errors.New("systemd is needed; without it, start typedeck from your desktop session startup")
	}
	unit := fmt.Sprintf(`[Unit]
Description=Typedeck: macros para tu teclado
After=graphical-session.target

[Service]
ExecStart=%s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, exe)
	p := unitPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(unit), 0o644); err != nil {
		return "", err
	}
	for _, c := range [][]string{{"daemon-reload"}, {"enable", "--now", unitName}} {
		if out, err := exec.Command("systemctl", append([]string{"--user"}, c...)...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("systemctl --user %s: %s", c[0], strings.TrimSpace(string(out)))
		}
	}
	return "Service installed and running: " + p + "\nRegistro: journalctl --user -u typedeck", nil
}

func (*linux) UninstallService() (string, error) {
	if have("systemctl") {
		_ = exec.Command("systemctl", "--user", "disable", "--now", unitName).Run()
	}
	if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if have("systemctl") {
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	return "Service removed (the configuration is kept)", nil
}
