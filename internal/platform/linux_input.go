//go:build linux

package platform

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

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

var (
	pasteKeysMu sync.Mutex
	pasteKeysFn func(shift bool) error
)

func SetPasteKeys(fn func(shift bool) error) {
	pasteKeysMu.Lock()
	pasteKeysFn = fn
	pasteKeysMu.Unlock()
}

func pasteKeys() error {
	pasteKeysMu.Lock()
	fn := pasteKeysFn
	pasteKeysMu.Unlock()
	shift := isTerminal(front().class)
	if fn != nil && fn(shift) == nil {
		return nil
	}
	if shift {
		return press([]string{"ctrl", "shift"}, "v")
	}
	return press([]string{"ctrl"}, "v")
}

func (p *linux) Paste(text string) error {
	old := p.Clipboard()
	if err := setClipboard(text); err != nil {
		return err
	}
	err := pasteKeys()
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
