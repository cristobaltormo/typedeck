package capture

import "github.com/cristobaltormo/typedeck/internal/hid"

const (
	layoutANSI = "ansi"
	layoutISO  = "iso"
	layoutJIS  = "jis"
)

const (
	codeISOSection = 10
	codeGrave      = 50
	codeCapsLock   = 57
	codeReturn     = 36
	codeTab        = 48
)

var macCodes = buildMacCodes()

var (
	macUsage = map[int]byte{}
	macCode  = map[byte]int{}
)

func buildMacCodes() map[byte]int {
	m := map[byte]int{}
	for i, c := range []int{0, 11, 8, 2, 14, 3, 5, 4, 34, 38, 40, 37, 46, 45, 31, 35, 12, 15, 1, 17, 32, 9, 13, 7, 16, 6} {
		m[byte(0x04+i)] = c
	}
	for i, c := range []int{18, 19, 20, 21, 23, 22, 26, 28, 25, 29} {
		m[byte(0x1E+i)] = c
	}
	for i, c := range []int{122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111} {
		m[byte(0x3A+i)] = c
	}
	for i, c := range []int{105, 107, 113, 106, 64, 79, 80, 90} {
		m[byte(0x68+i)] = c
	}
	for i, c := range []int{83, 84, 85, 86, 87, 88, 89, 91, 92, 82} {
		m[byte(0x59+i)] = c
	}
	for i, c := range []int{59, 56, 58, 55, 62, 60, 61, 54} {
		m[byte(0xE0+i)] = c
	}
	for u, c := range map[byte]int{
		0x28: 36, 0x29: 53, 0x2A: 51, 0x2B: 48, 0x2C: 49, 0x2D: 27, 0x2E: 24, 0x2F: 33, 0x30: 30, 0x31: 42, 0x32: 42,
		0x33: 41, 0x34: 39, 0x36: 43, 0x37: 47, 0x38: 44, 0x39: codeCapsLock,
		0x49: 114, 0x4A: 115, 0x4B: 116, 0x4C: 117, 0x4D: 119, 0x4E: 121, 0x4F: 124, 0x50: 123, 0x51: 125, 0x52: 126,
		0x53: 71, 0x54: 75, 0x55: 67, 0x56: 78, 0x57: 69, 0x58: 76, 0x63: 65, 0x65: 110, 0x67: 81,
		0x85: 95, 0x87: 94, 0x89: 93, 0x90: 104, 0x91: 102,
	} {
		m[u] = c
	}
	return m
}

func init() {
	for u, c := range macCodes {
		macCode[u] = c
		if u != 0x32 {
			macUsage[c] = u
		}
	}
}

func isoLike(layout string) bool { return layout != layoutANSI }

func MacKeycode(usage byte, layout string) (int, bool) {
	switch usage {
	case 0x35:
		if isoLike(layout) {
			return codeISOSection, true
		}
		return codeGrave, true
	case 0x64:
		if isoLike(layout) {
			return codeGrave, true
		}
		return codeISOSection, true
	}
	c, ok := macCode[usage]
	return c, ok
}

func MacUsage(code int, layout string) byte {
	switch code {
	case codeISOSection:
		if isoLike(layout) {
			return 0x35
		}
		return 0x64
	case codeGrave:
		if isoLike(layout) {
			return 0x64
		}
		return 0x35
	}
	return macUsage[code]
}

func keycodeMask(s Snapshot, layout string) [16]byte {
	var out [16]byte
	for u := 0; u < 256; u++ {
		if s.Mask[u>>3]>>(u&7)&1 == 0 {
			continue
		}
		c, ok := MacKeycode(byte(u), layout)
		if ok && c < 128 && MacUsage(c, layout) == byte(u) {
			out[c>>3] |= 1 << (c & 7)
		}
	}
	return out
}

var modDevice = [8]uint64{0x1, 0x2, 0x20, 0x8, 0x2000, 0x4, 0x40, 0x10}

const (
	flagShift   = 0x20000
	flagControl = 0x40000
	flagOption  = 0x80000
	flagCommand = 0x100000
)

var modGeneric = [8]uint64{flagControl, flagShift, flagOption, flagCommand, flagControl, flagShift, flagOption, flagCommand}

func ModsFromFlags(flags uint64) byte {
	var mods byte
	for bit := 0; bit < 8; bit++ {
		if flags&modDevice[bit] != 0 {
			mods |= 1 << bit
		}
	}
	if mods != 0 {
		return mods
	}
	for bit := 0; bit < 4; bit++ {
		if flags&modGeneric[bit] != 0 {
			mods |= 1 << bit
		}
	}
	return mods
}

func flagsFromMods(mods byte) uint64 {
	var flags uint64
	for bit := 0; bit < 8; bit++ {
		if mods&(1<<bit) != 0 {
			flags |= modDevice[bit] | modGeneric[bit]
		}
	}
	return flags
}

var consumerKeyType = map[uint16]int{
	0xE9: 0, 0xEA: 1, 0x6F: 2, 0x70: 3, 0xE2: 7, 0xCD: 16, 0xB5: 17, 0xB6: 18, 0xB3: 19, 0xB4: 20,
}

func ConsumerKeyType(usage uint16) (int, bool) {
	k, ok := consumerKeyType[usage]
	return k, ok
}

type keyStep struct {
	down  bool
	code  int
	flags uint64
}

func strokePlan(s hid.Stroke, layout string) (down, up []keyStep, err error) {
	var acc uint64
	var mods []keyStep
	for bit := 0; bit < 8; bit++ {
		if s.Mods&(1<<bit) != 0 {
			acc |= modDevice[bit] | modGeneric[bit]
			mods = append(mods, keyStep{true, macCodes[byte(0xE0+bit)], acc})
		}
	}
	down = mods
	if s.Usage != 0 {
		c, ok := MacKeycode(s.Usage, layout)
		if !ok {
			return nil, nil, &noKeyError{usage: s.Usage}
		}
		flags := acc
		if s.Usage >= 0xE0 && s.Usage <= 0xE7 {
			flags |= modDevice[s.Usage-0xE0] | modGeneric[s.Usage-0xE0]
		}
		down = append(down, keyStep{true, c, flags})
		up = append(up, keyStep{false, c, acc})
	}
	for i := len(mods) - 1; i >= 0; i-- {
		var rest uint64
		if i > 0 {
			rest = mods[i-1].flags
		}
		up = append(up, keyStep{false, mods[i].code, rest})
	}
	return down, up, nil
}
