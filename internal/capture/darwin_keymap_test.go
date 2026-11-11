package capture

import (
	"testing"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

func TestMacKeycodesRoundTrip(t *testing.T) {
	for _, layout := range []string{layoutANSI, layoutISO, layoutJIS} {
		for u, want := range macCode {
			c, ok := MacKeycode(u, layout)
			if !ok {
				t.Fatalf("%s: usage %02X has no key code", layout, u)
			}
			if u == 0x32 || u == 0x35 || u == 0x64 {
				continue
			}
			if c != want || MacUsage(c, layout) != u {
				t.Errorf("%s: usage %02X -> %d -> %02X", layout, u, c, MacUsage(c, layout))
			}
		}
	}
}

func TestMacKeycodesAreUnique(t *testing.T) {
	seen := map[int]byte{}
	for u, c := range macCode {
		if prev, dup := seen[c]; dup && !(u == 0x31 || u == 0x32) {
			t.Errorf("key code %d is used by %02X and %02X", c, prev, u)
		}
		seen[c] = u
	}
}

func TestMacKnownKeys(t *testing.T) {
	cases := []struct {
		usage byte
		code  int
	}{{0x04, 0}, {0x05, 11}, {0x1D, 6}, {0x1E, 18}, {0x27, 29}, {0x28, 36}, {0x2C, 49}, {0x39, 57}, {0x3A, 122}, {0x45, 111},
		{0x4F, 124}, {0x52, 126}, {0x59, 83}, {0x62, 82}, {0x68, 105}, {0xE0, 59}, {0xE3, 55}, {0xE7, 54}}
	for _, c := range cases {
		got, ok := MacKeycode(c.usage, layoutISO)
		if !ok || got != c.code {
			t.Errorf("usage %02X: got %d, want %d", c.usage, got, c.code)
		}
	}
}

func TestMacISOSectionAndGrave(t *testing.T) {
	cases := []struct {
		layout string
		usage  byte
		code   int
	}{
		{layoutISO, 0x35, 10}, {layoutISO, 0x64, 50},
		{layoutJIS, 0x35, 10}, {layoutJIS, 0x64, 50},
		{layoutANSI, 0x35, 50}, {layoutANSI, 0x64, 10},
	}
	for _, c := range cases {
		got, _ := MacKeycode(c.usage, c.layout)
		if got != c.code || MacUsage(c.code, c.layout) != c.usage {
			t.Errorf("%s %02X: code %d, back %02X", c.layout, c.usage, got, MacUsage(got, c.layout))
		}
	}
}

func TestMacUsageOfUnknownCode(t *testing.T) {
	for _, c := range []int{63, 66, 127, 200} {
		if u := MacUsage(c, layoutISO); u != 0 {
			t.Errorf("code %d should have no usage, got %02X", c, u)
		}
	}
}

func TestModsFromFlags(t *testing.T) {
	cases := []struct {
		flags uint64
		mods  byte
	}{
		{0, 0},
		{0x8 | flagCommand, hid.ModGui},
		{0x10 | flagCommand, 0x80},
		{0x1 | 0x2 | flagControl | flagShift, hid.ModCtrl | hid.ModShift},
		{0x20 | flagOption, hid.ModAlt},
		{0x40 | flagOption, hid.ModAltGr},
		{0x2000 | flagControl, 0x10},
		{0x4 | flagShift, 0x20},
		{flagShift, hid.ModShift},
		{flagCommand | flagOption, hid.ModGui | hid.ModAlt},
	}
	for _, c := range cases {
		if got := ModsFromFlags(c.flags); got != c.mods {
			t.Errorf("flags %x: got %02X, want %02X", c.flags, got, c.mods)
		}
	}
}

func TestFlagsFromModsRoundTrip(t *testing.T) {
	for m := 0; m < 256; m++ {
		if got := ModsFromFlags(flagsFromMods(byte(m))); got != byte(m) {
			t.Errorf("mods %02X came back as %02X", m, got)
		}
	}
}

func TestKeycodeMask(t *testing.T) {
	var s Snapshot
	for _, u := range []byte{0x04, 0x39, 0x35, 0x64, 0x32, 0x46} {
		s.Mask[u>>3] |= 1 << (u & 7)
	}
	iso := keycodeMask(s, layoutISO)
	for _, c := range []int{0, 57, 10, 50} {
		if iso[c>>3]>>(c&7)&1 == 0 {
			t.Errorf("code %d missing from the mask", c)
		}
	}
	if iso[42>>3]>>(42&7)&1 != 0 {
		t.Error("the 0x32 alias must not set a code")
	}
	var bits int
	for _, b := range iso {
		for ; b != 0; b &= b - 1 {
			bits++
		}
	}
	if bits != 4 {
		t.Errorf("expected 4 codes, got %d", bits)
	}
}

func TestConsumerKeyType(t *testing.T) {
	for usage, want := range map[uint16]int{0xE9: 0, 0xEA: 1, 0xE2: 7, 0xCD: 16, 0xB5: 17, 0xB6: 18} {
		if got, ok := ConsumerKeyType(usage); !ok || got != want {
			t.Errorf("usage %X: got %d %v", usage, got, ok)
		}
	}
	if _, ok := ConsumerKeyType(0x999); ok {
		t.Error("unknown media key accepted")
	}
}

func TestStrokePlanShortcut(t *testing.T) {
	down, up, err := strokePlan(hid.Stroke{Usage: 0x06, Mods: hid.ModGui}, layoutISO)
	if err != nil {
		t.Fatal(err)
	}
	cmd := flagsFromMods(hid.ModGui)
	wantDown := []keyStep{{true, 55, cmd}, {true, 8, cmd}}
	wantUp := []keyStep{{false, 8, cmd}, {false, 55, 0}}
	if !sameSteps(down, wantDown) || !sameSteps(up, wantUp) {
		t.Fatalf("down %v up %v", down, up)
	}
}

func TestStrokePlanTwoModifiersReleaseInReverse(t *testing.T) {
	down, up, _ := strokePlan(hid.Stroke{Usage: 0x16, Mods: hid.ModCtrl | hid.ModShift}, layoutISO)
	if len(down) != 3 || len(up) != 3 {
		t.Fatalf("down %v up %v", down, up)
	}
	if down[0].code != 59 || down[1].code != 56 || up[1].code != 56 || up[2].code != 59 {
		t.Errorf("order: down %v up %v", down, up)
	}
	if up[1].flags != flagsFromMods(hid.ModCtrl) || up[2].flags != 0 {
		t.Errorf("release flags: %v", up)
	}
}

func TestStrokePlanModifierAlone(t *testing.T) {
	down, up, err := strokePlan(hid.Stroke{Usage: 0xE1}, layoutISO)
	if err != nil || len(down) != 1 || len(up) != 1 {
		t.Fatalf("%v %v %v", down, up, err)
	}
	if down[0].code != 56 || down[0].flags&0x2 == 0 || up[0].flags != 0 {
		t.Errorf("down %v up %v", down, up)
	}
}

func TestStrokePlanUnknownKey(t *testing.T) {
	if _, _, err := strokePlan(hid.Stroke{Usage: 0x46}, layoutISO); err == nil {
		t.Fatal("PrintScreen has no macOS key")
	}
}

func sameSteps(a, b []keyStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
