//go:build windows

package platform

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

var (
	pMapVirtualKey = user32.NewProc("MapVirtualKeyW")
	pVkKeyScanExW  = user32.NewProc("VkKeyScanExW")
	pSendInput     = user32.NewProc("SendInput")
)

var vkMods = map[string]uint16{"ctrl": 0x11, "shift": 0x10, "alt": 0x12, "gui": 0x5B, "altgr": 0xA5}

type keyInput struct {
	Type  uint32
	_     uint32
	VK    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	_     uint32
	Extra uintptr
	_     [8]byte
}

func keyEvent(vk uint16, up bool) {
	flags := uintptr(0)
	if up {
		flags = 2
	}
	if isExtendedVK(vk) {
		flags |= 1
	}
	code := uintptr(vk & 0xFF)
	scan, _, _ := pMapVirtualKey.Call(code, 0)
	pKeybdEvent.Call(code, scan, flags, 0)
}

func sendCombo(mods []string, vk uint16) error {
	if vk == 0 {
		return errors.New("key without a Windows equivalent")
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

func foregroundLayout() uintptr {
	hwnd, _ := foreground()
	var tid uintptr
	if hwnd != 0 {
		tid, _, _ = pGetWindowThreadProcessID.Call(hwnd, 0)
	}
	hkl, _, _ := pGetKeyboardLayout.Call(tid)
	return hkl
}

func scanToCombo(v uint16) (mods []string, vk uint16) {
	if v&0x100 != 0 {
		mods = append(mods, "shift")
	}
	if v&0x200 != 0 {
		mods = append(mods, "ctrl")
	}
	if v&0x400 != 0 {
		mods = append(mods, "alt")
	}
	return mods, v & 0xFF
}

func (*windows) SoftHotkey(spec string) error {
	mods, key, err := splitHotkey(spec)
	if err != nil {
		return err
	}
	if u, ok := hid.KeyUsage(key); ok {
		return sendCombo(mods, vkOf(u))
	}
	r := []rune(key)
	if len(r) != 1 {
		return fmt.Errorf("unknown key: %s", key)
	}
	v, _, _ := pVkKeyScanExW.Call(uintptr(r[0]), foregroundLayout())
	if int16(v) == -1 {
		return fmt.Errorf("unknown key: %s", key)
	}
	extra, vk := scanToCombo(uint16(v))
	return sendCombo(mergeMods(mods, extra), vk)
}

func mergeMods(have, extra []string) []string {
	out := append([]string{}, have...)
	for _, m := range extra {
		if !containsString(out, m) {
			out = append(out, m)
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func strokeMods(m byte) []string {
	var mods []string
	for _, c := range []struct {
		bit  byte
		name string
	}{{hid.ModCtrl, "ctrl"}, {hid.ModShift, "shift"}, {hid.ModAlt, "alt"}, {hid.ModGui, "gui"}, {hid.ModAltGr, "altgr"}} {
		if m&c.bit != 0 {
			mods = append(mods, c.name)
		}
	}
	return mods
}

func (*windows) SoftStroke(st hid.Stroke) error {
	return sendCombo(strokeMods(st.Mods), vkOf(st.Usage))
}

func sendUnicodeUnit(unit uint16) {
	in := [2]keyInput{{Type: 1, Scan: unit, Flags: 0x4}, {Type: 1, Scan: unit, Flags: 0x4 | 0x2}}
	pSendInput.Call(2, uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func typeUnicode(text string) error {
	for _, r := range text {
		switch r {
		case '\r':
		case '\n':
			if err := sendCombo(nil, 0x0D); err != nil {
				return err
			}
		case '\t':
			if err := sendCombo(nil, 0x09); err != nil {
				return err
			}
		default:
			for _, unit := range utf16.Encode([]rune{r}) {
				sendUnicodeUnit(unit)
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

func (*windows) SoftMedia(cmd string, step int) (string, error) {
	vk := map[string]uint16{"playpause": 0xB3, "next": 0xB0, "prev": 0xB1, "volup": 0xAF, "voldown": 0xAE, "mute": 0xAD}[cmd]
	if vk == 0 {
		return "", fmt.Errorf("unknown media command: %s", cmd)
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
