package capture

import "testing"

func TestEvdevTableHasNoDuplicates(t *testing.T) {
	usages, codes := map[byte]bool{}, map[uint16]bool{}
	for _, e := range evdevTable {
		if usages[e.usage] {
			t.Errorf("usage %02X is mapped twice", e.usage)
		}
		if codes[e.code] {
			t.Errorf("evdev code %d is mapped twice", e.code)
		}
		usages[e.usage], codes[e.code] = true, true
	}
}

func TestEvdevRoundTrip(t *testing.T) {
	for _, e := range evdevTable {
		code, ok := EvdevFromUsage(e.usage)
		if !ok || code != e.code {
			t.Errorf("usage %02X: got code %d ok=%v, want %d", e.usage, code, ok, e.code)
		}
		if back := UsageFromEvdev(e.code); back != e.usage {
			t.Errorf("code %d: got usage %02X, want %02X", e.code, back, e.usage)
		}
	}
}

func TestEvdevCoversTheKeyboard(t *testing.T) {
	var want []byte
	for u := 0x04; u <= 0x65; u++ {
		want = append(want, byte(u))
	}
	want = append(want, 0x67, 0x85, 0x87, 0x88, 0x89, 0x8A, 0x8B)
	for u := 0xE0; u <= 0xE7; u++ {
		want = append(want, byte(u))
	}
	for u := 0x68; u <= 0x73; u++ {
		want = append(want, byte(u))
	}
	for _, u := range want {
		if _, ok := EvdevFromUsage(u); !ok {
			t.Errorf("usage %02X is not mapped", u)
		}
	}
}

func TestEvdevKnownKeys(t *testing.T) {
	cases := []struct {
		code  uint16
		usage byte
	}{
		{30, 0x04}, {44, 0x1D}, {2, 0x1E}, {11, 0x27}, {28, 0x28}, {1, 0x29}, {57, 0x2C}, {43, 0x31}, {86, 0x64},
		{59, 0x3A}, {68, 0x43}, {87, 0x44}, {88, 0x45}, {183, 0x68}, {194, 0x73}, {99, 0x46}, {119, 0x48},
		{96, 0x58}, {82, 0x62}, {79, 0x59}, {29, 0xE0}, {42, 0xE1}, {56, 0xE2}, {125, 0xE3}, {97, 0xE4}, {100, 0xE6}, {126, 0xE7},
	}
	for _, c := range cases {
		if got := UsageFromEvdev(c.code); got != c.usage {
			t.Errorf("code %d: got %02X, want %02X", c.code, got, c.usage)
		}
	}
}

func TestEvdevUnknownCodesPassThrough(t *testing.T) {
	for _, code := range []uint16{0, 113, 163, 224, 0x110} {
		if got := UsageFromEvdev(code); got != 0 {
			t.Errorf("code %d should not map, got %02X", code, got)
		}
	}
}

func TestNonUSHashSharesTheBackslash(t *testing.T) {
	if got := UsageFromEvdev(43); got != 0x31 {
		t.Errorf("backslash key reports %02X", got)
	}
	if got, ok := EvdevFromUsage(0x32); !ok || got != 43 {
		t.Errorf("usage 32 must inject the backslash key, got %d ok=%v", got, ok)
	}
}

func TestConsumerEvdev(t *testing.T) {
	for usage, want := range map[uint16]uint16{0xCD: 164, 0xE2: 113, 0xE9: 115, 0xEA: 114, 0xB5: 163, 0xB6: 165} {
		if got, ok := ConsumerEvdev(usage); !ok || got != want {
			t.Errorf("consumer %X: got %d ok=%v, want %d", usage, got, ok, want)
		}
	}
	if _, ok := ConsumerEvdev(0xFFFF); ok {
		t.Error("an unknown consumer usage mapped")
	}
}
