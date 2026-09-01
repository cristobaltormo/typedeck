package hid

import (
	"fmt"
	"strings"
)

const (
	ModCtrl  byte = 0x01
	ModShift byte = 0x02
	ModAlt   byte = 0x04
	ModGui   byte = 0x08
	ModAltGr byte = 0x40
)

type Stroke struct {
	Usage byte
	Mods  byte
}

var keyNames = map[string]byte{
	"return": 0x28, "enter": 0x28, "esc": 0x29, "escape": 0x29, "backspace": 0x2A, "delete": 0x2A, "tab": 0x2B,
	"space": 0x2C, "capslock": 0x39, "printscreen": 0x46, "scrolllock": 0x47, "pause": 0x48, "insert": 0x49,
	"home": 0x4A, "pageup": 0x4B, "forwarddelete": 0x4C, "end": 0x4D, "pagedown": 0x4E,
	"right": 0x4F, "left": 0x50, "down": 0x51, "up": 0x52, "numlock": 0x53,
	"minus": 0x2D, "equals": 0x2E, "comma": 0x36, "period": 0x37, "slash": 0x38,
}

func init() {
	for i := 0; i < 26; i++ {
		keyNames[string(rune('a'+i))] = byte(0x04 + i)
	}
	for i := 1; i <= 9; i++ {
		keyNames[string(rune('0'+i))] = byte(0x1D + i)
	}
	keyNames["0"] = 0x27
	for i := 1; i <= 12; i++ {
		keyNames[fmt.Sprintf("f%d", i)] = byte(0x3A + i - 1)
	}
	for i := 13; i <= 24; i++ {
		keyNames[fmt.Sprintf("f%d", i)] = byte(0x68 + i - 13)
	}
}

var modNames = map[string]byte{
	"ctrl": ModCtrl, "control": ModCtrl, "shift": ModShift, "alt": ModAlt, "option": ModAlt, "opt": ModAlt,
	"cmd": ModGui, "command": ModGui, "gui": ModGui, "win": ModGui,
}

func ParseHotkey(spec string, l *Layout) (Stroke, error) {
	var st Stroke
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return st, fmt.Errorf("atajo vacío")
	}
	for _, p := range parts[:len(parts)-1] {
		m, ok := modNames[strings.TrimSpace(p)]
		if !ok {
			return st, fmt.Errorf("modificador desconocido: %s", p)
		}
		st.Mods |= m
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	if u, ok := keyNames[key]; ok {
		st.Usage = u
		return st, nil
	}
	if key == "plus" {
		key = "+"
	}
	r := []rune(key)
	if len(r) == 1 {
		if strokes, ok := l.chars[r[0]]; ok && len(strokes) == 1 {
			st.Usage = strokes[0].Usage
			st.Mods |= strokes[0].Mods &^ ModShift
			return st, nil
		}
	}
	return st, fmt.Errorf("tecla desconocida: %s", key)
}

func KeyUsage(name string) (byte, bool) {
	u, ok := keyNames[strings.ToLower(name)]
	return u, ok
}

func UsageName(u byte) string {
	for n, v := range keyNames {
		if v == u && len(n) > 1 && n != "enter" && n != "escape" && n != "delete" {
			return n
		}
	}
	switch {
	case u >= 0x04 && u <= 0x1D:
		return strings.ToUpper(string(rune('a' + int(u) - 0x04)))
	case u >= 0x1E && u <= 0x26:
		return string(rune('1' + int(u) - 0x1E))
	case u == 0x27:
		return "0"
	}
	return fmt.Sprintf("0x%02X", u)
}
