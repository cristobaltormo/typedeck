package capture

type evdevKey struct {
	usage byte
	code  uint16
}

var evdevTable = buildEvdevTable()

var (
	usageFromEvdev = map[uint16]byte{}
	evdevFromUsage = map[byte]uint16{}
)

func buildEvdevTable() []evdevKey {
	var t []evdevKey
	for i, c := range []uint16{30, 48, 46, 32, 18, 33, 34, 35, 23, 36, 37, 38, 50, 49, 24, 25, 16, 19, 31, 20, 22, 47, 17, 45, 21, 44} {
		t = append(t, evdevKey{byte(0x04 + i), c})
	}
	for i := 0; i < 10; i++ {
		t = append(t, evdevKey{byte(0x1E + i), uint16(2 + i)})
	}
	for i := 0; i < 10; i++ {
		t = append(t, evdevKey{byte(0x3A + i), uint16(59 + i)})
	}
	for i := 0; i < 12; i++ {
		t = append(t, evdevKey{byte(0x68 + i), uint16(183 + i)})
	}
	return append(t, evdevKeys...)
}

var evdevKeys = []evdevKey{
	{0x28, 28}, {0x29, 1}, {0x2A, 14}, {0x2B, 15}, {0x2C, 57}, {0x2D, 12}, {0x2E, 13}, {0x2F, 26}, {0x30, 27}, {0x31, 43},
	{0x33, 39}, {0x34, 40}, {0x35, 41}, {0x36, 51}, {0x37, 52}, {0x38, 53}, {0x39, 58}, {0x44, 87}, {0x45, 88},
	{0x46, 99}, {0x47, 70}, {0x48, 119}, {0x49, 110}, {0x4A, 102}, {0x4B, 104}, {0x4C, 111}, {0x4D, 107}, {0x4E, 109},
	{0x4F, 106}, {0x50, 105}, {0x51, 108}, {0x52, 103}, {0x53, 69}, {0x54, 98}, {0x55, 55}, {0x56, 74}, {0x57, 78},
	{0x58, 96}, {0x59, 79}, {0x5A, 80}, {0x5B, 81}, {0x5C, 75}, {0x5D, 76}, {0x5E, 77}, {0x5F, 71}, {0x60, 72},
	{0x61, 73}, {0x62, 82}, {0x63, 83}, {0x64, 86}, {0x65, 127}, {0x67, 117},
	{0x85, 121}, {0x87, 89}, {0x88, 93}, {0x89, 124}, {0x8A, 92}, {0x8B, 94}, {0x90, 122}, {0x91, 123},
	{0xE0, 29}, {0xE1, 42}, {0xE2, 56}, {0xE3, 125}, {0xE4, 97}, {0xE5, 54}, {0xE6, 100}, {0xE7, 126},
}

var consumerEvdev = map[uint16]uint16{
	0xCD: 164, 0xB5: 163, 0xB6: 165, 0xB7: 166, 0xE2: 113, 0xE9: 115, 0xEA: 114,
	0x18A: 155, 0x192: 140, 0x221: 217, 0x223: 172, 0x224: 158, 0x225: 159,
}

func init() {
	for _, e := range evdevTable {
		usageFromEvdev[e.code] = e.usage
		evdevFromUsage[e.usage] = e.code
	}
}

func UsageFromEvdev(code uint16) byte { return usageFromEvdev[code] }

func EvdevFromUsage(usage byte) (uint16, bool) {
	if usage == 0x32 {
		usage = 0x31
	}
	c, ok := evdevFromUsage[usage]
	return c, ok
}

func ConsumerEvdev(usage uint16) (uint16, bool) {
	c, ok := consumerEvdev[usage]
	return c, ok
}
