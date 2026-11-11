package capture

import "testing"

func TestEveryTableEntryRoundTrips(t *testing.T) {
	for _, e := range scanTable {
		if got := UsageFromScan(uint32(e.code), e.ext, 0); got != e.usage && e.usage != 0x48 && e.usage != 0x53 {
			t.Errorf("scan %02X ext=%v: got usage %02X, want %02X", e.code, e.ext, got, e.usage)
		}
		k, ok := ScanFromUsage(e.usage)
		if !ok {
			t.Errorf("usage %02X has no scancode", e.usage)
			continue
		}
		if k.VK != 0 {
			continue
		}
		if back := UsageFromScan(uint32(k.Code), k.Ext, 0); back != e.usage && e.usage != 0x32 {
			t.Errorf("usage %02X -> scan %02X ext=%v -> usage %02X", e.usage, k.Code, k.Ext, back)
		}
	}
}

func TestNoTwoUsagesShareAScancode(t *testing.T) {
	seen := map[uint16]byte{}
	for _, e := range scanTable {
		id := scanID(e.code, e.ext)
		if other, dup := seen[id]; dup {
			t.Errorf("usages %02X and %02X share scancode %03X", other, e.usage, id)
		}
		seen[id] = e.usage
	}
}

func TestCoverageOfTheKeyboard(t *testing.T) {
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
		if _, ok := ScanFromUsage(u); !ok {
			t.Errorf("usage %02X is not mapped", u)
		}
	}
}

func TestNumpadAndNavigationAreTold(t *testing.T) {
	cases := []struct {
		code uint32
		ext  bool
		want byte
	}{
		{0x1C, false, 0x28}, {0x1C, true, 0x58}, {0x35, false, 0x38}, {0x35, true, 0x54},
		{0x47, false, 0x5F}, {0x47, true, 0x4A}, {0x53, false, 0x63}, {0x53, true, 0x4C},
		{0x1D, false, 0xE0}, {0x1D, true, 0xE4}, {0x38, false, 0xE2}, {0x38, true, 0xE6},
		{0x5B, true, 0xE3}, {0x5C, true, 0xE7}, {0x5D, true, 0x65}, {0x37, true, 0x46}, {0x37, false, 0x55},
	}
	for _, c := range cases {
		if got := UsageFromScan(c.code, c.ext, 0); got != c.want {
			t.Errorf("scan %02X ext=%v: got %02X want %02X", c.code, c.ext, got, c.want)
		}
	}
}

func TestIsoKeys(t *testing.T) {
	if got := UsageFromScan(0x56, false, 0); got != 0x64 {
		t.Errorf("ISO < > key: %02X", got)
	}
	if got := UsageFromScan(0x2B, false, 0); got != 0x31 {
		t.Errorf("key next to Enter: %02X", got)
	}
	for _, u := range []byte{0x31, 0x32} {
		if k, _ := ScanFromUsage(u); k.Code != 0x2B || k.Ext {
			t.Errorf("usage %02X injects scancode %02X", u, k.Code)
		}
	}
}

func TestPauseNumLockAndPrintScreenUseTheVirtualKey(t *testing.T) {
	if got := UsageFromScan(0x45, false, vkPause); got != 0x48 {
		t.Errorf("pause: %02X", got)
	}
	if got := UsageFromScan(0x45, true, vkNumlock); got != 0x53 {
		t.Errorf("num lock: %02X", got)
	}
	if got := UsageFromScan(0x54, false, vkSnapshot); got != 0x46 {
		t.Errorf("print screen with alt: %02X", got)
	}
	if got := UsageFromScan(0x46, true, 0x03); got != 0x48 {
		t.Errorf("ctrl+break: %02X", got)
	}
	if k, _ := ScanFromUsage(0x48); k.VK != vkPause {
		t.Errorf("pause is injected by virtual key, got %+v", k)
	}
}

func TestFakeKeysWindowsInventsAreIgnored(t *testing.T) {
	for _, c := range []struct {
		code uint32
		ext  bool
	}{{0x21D, false}, {0x22A, false}, {0x236, false}, {0x2A, true}, {0x36, true}} {
		if got := UsageFromScan(c.code, c.ext, 0); got != 0 {
			t.Errorf("scan %X ext=%v should be ignored, got %02X", c.code, c.ext, got)
		}
	}
}

func TestUnknownScancodes(t *testing.T) {
	if UsageFromScan(0x7F, false, 0) != 0 || UsageFromScan(0x00, false, 0) != 0 {
		t.Error("unknown scancodes must map to nothing")
	}
}

func TestMediaKeys(t *testing.T) {
	for _, u := range []uint16{0xCD, 0xB5, 0xB6, 0xE2, 0xE9, 0xEA} {
		if _, ok := ConsumerVK(u); !ok {
			t.Errorf("consumer %X not mapped", u)
		}
	}
	if _, ok := ConsumerVK(0x1); ok {
		t.Error("unknown consumer usage mapped")
	}
}

func TestConsumerKeysMatchOnWindowsAndLinux(t *testing.T) {
	for usage := range consumerEvdev {
		if _, ok := ConsumerVK(usage); !ok {
			t.Errorf("media usage %#x has a Linux code but no Windows virtual key", usage)
		}
	}
	for usage := range consumerVK {
		if _, ok := ConsumerEvdev(usage); !ok {
			t.Errorf("media usage %#x has a Windows virtual key but no Linux code", usage)
		}
	}
}
