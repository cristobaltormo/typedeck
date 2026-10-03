package platform

import "testing"

func TestVirtualKeys(t *testing.T) {
	cases := map[byte]uint16{0x04: 'A', 0x1D: 'Z', 0x1E: '1', 0x27: '0', 0x3A: 0x70, 0x45: 0x7B, 0x68: 0x7C, 0x28: 0x0D, 0x29: 0x1B, 0x2C: 0x20, 0x4F: 0x27, 0x52: 0x26}
	for u, want := range cases {
		if got := vkOf(u); got != want {
			t.Errorf("usage %02X -> %02X, expected %02X", u, got, want)
		}
	}
	if vkOf(0xFF) != 0 {
		t.Error("an unknown usage must give no key")
	}
}
