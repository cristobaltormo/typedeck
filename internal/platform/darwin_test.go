//go:build darwin

package platform

import (
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const realMapping = `(
        {
        HIDKeyboardModifierMappingDst = 30064771299;
        HIDKeyboardModifierMappingSrc = 30064771298;
    },
        {
        HIDKeyboardModifierMappingDst = 30064771302;
        HIDKeyboardModifierMappingSrc = 30064771303;
    },
        {
        HIDKeyboardModifierMappingDst = 30064771298;
        HIDKeyboardModifierMappingSrc = 30064771299;
    },
        {
        HIDKeyboardModifierMappingDst = 30064771303;
        HIDKeyboardModifierMappingSrc = 30064771302;
    }
)`

func TestModifierCompensation(t *testing.T) {
	m := parseModifierMapping(realMapping)
	if len(m) != 4 || m[0xE2] != 0xE3 || m[0xE3] != 0xE2 {
		t.Fatalf("mapeo mal leido: %v", m)
	}
	cases := []struct{ in, want byte }{
		{hid.ModGui, hid.ModAlt},
		{hid.ModAlt, hid.ModGui},
		{hid.ModGui | hid.ModShift, hid.ModAlt | hid.ModShift},
		{hid.ModCtrl, hid.ModCtrl},
		{0, 0},
	}
	for _, c := range cases {
		if got := invertMods(m, c.in); got != c.want {
			t.Errorf("mods %02X -> %02X, esperaba %02X", c.in, got, c.want)
		}
	}
	if got := invertMods(nil, hid.ModGui); got != hid.ModGui {
		t.Errorf("sin remapeo debe dejarlo igual: %02X", got)
	}
}

func TestAdaptSwapsISOKeysOnlyForANSIType(t *testing.T) {
	es := hid.LayoutByName("es-iso")
	kbMu.Lock()
	kbType, kbAt = 40, time.Now()
	kbMu.Unlock()
	modMu.Lock()
	modMap, modAt = nil, time.Now()
	modMu.Unlock()
	if got := (&darwin{}).AdaptStroke(hid.Stroke{Usage: 0x64}, es); got.Usage != 0x35 {
		t.Errorf("0x64 con ANSI+es-iso debe cruzarse: %02X", got.Usage)
	}
	if got := (&darwin{}).AdaptStroke(hid.Stroke{Usage: 0x35}, es); got.Usage != 0x64 {
		t.Errorf("0x35 con ANSI+es-iso debe cruzarse: %02X", got.Usage)
	}
	if got := (&darwin{}).AdaptStroke(hid.Stroke{Usage: 0x64}, hid.LayoutByName("us")); got.Usage != 0x64 {
		t.Errorf("con US no se cruza: %02X", got.Usage)
	}
	kbMu.Lock()
	kbType = 41
	kbMu.Unlock()
	if got := (&darwin{}).AdaptStroke(hid.Stroke{Usage: 0x64}, es); got.Usage != 0x64 {
		t.Errorf("con teclado ISO no se cruza: %02X", got.Usage)
	}
}
