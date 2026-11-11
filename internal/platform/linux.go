//go:build linux

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func (*linux) InstalledApps() []string {
	var out []string
	for _, e := range loadEntries() {
		out = append(out, e.Name)
	}
	return out
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

func (*linux) Info(l *hid.Layout) SystemInfo {
	return SystemInfo{TypingLayout: l.Name, ModRemap: []string{}}
}

func (*linux) Popup(PopupOptions) bool { return false }

func (*linux) ShellCommand(cmd string) (string, []string) { return "/bin/sh", []string{"-c", cmd} }

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
			"optional: the active window cannot be read, so per-application layers are unavailable (they need X11 with xprop, Sway or Hyprland; GNOME and KDE on Wayland do not allow it)"},
		{(!wayland() && have("xdotool")) || have("wtype"), "software keys available (xdotool or wtype)",
			"optional: without xdotool or wtype, software keys do not work; they are not needed with the board connected"},
	}
	acm, _ := filepath.Glob("/dev/ttyACM*")
	switch {
	case len(acm) == 0:
		checks = append(checks, Check{true, "no /dev/ttyACM*: the port permission cannot be checked (plug the board in)", ""})
	default:
		checks = append(checks, Check{len(candidatesACM()) > 0, "your user can open " + acm[0],
			"your user cannot open " + acm[0] + ": copy packaging/linux/70-typedeck.rules to /etc/udev/rules.d/ or add yourself to the dialout group (and sign in again)"})
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
Description=Typedeck: macros for your keyboard
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
