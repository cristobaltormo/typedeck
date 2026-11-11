package platform

import "testing"

func TestLayoutFromHKL(t *testing.T) {
	byID := func(id, lang uint16) string {
		if id == 0x0002 && lang == 0x0409 {
			return "00020409"
		}
		if id == 0x00AE && lang == 0x040A {
			return "0001040A"
		}
		return ""
	}
	cases := []struct {
		hkl  uint32
		want string
	}{
		{0x040A040A, "es-win"},
		{0x04090409, "us"},
		{0x08090809, "qwerty"},
		{0x080A080A, "qwerty"},
		{0x040A080A, "es-win"},
		{0xF0020409, ""},
		{0xF0AE040A, ""},
		{0x00000407, ""},
		{0x04070407, ""},
		{0x0C0A0C0A, "qwerty"},
	}
	for _, c := range cases {
		if got := layoutFromKLID(klidFromHKL(c.hkl, byID)); got != c.want {
			t.Errorf("HKL %08X: %q, expected %q", c.hkl, got, c.want)
		}
	}
}

func TestKLIDFromVariantHKL(t *testing.T) {
	byID := func(id, lang uint16) string { return "found" }
	if got := klidFromHKL(0xF0AE040A, byID); got != "found" {
		t.Errorf("variant layouts must be looked up by layout id, got %q", got)
	}
	if got := klidFromHKL(0x040A040A, nil); got != "0000040A" {
		t.Errorf("got %q", got)
	}
}
