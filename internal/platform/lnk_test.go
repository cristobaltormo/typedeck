package platform

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func buildLnk(withList bool, ansi, unicode, suffix string) []byte {
	data := make([]byte, lnkHeaderSize)
	binary.LittleEndian.PutUint32(data, lnkHeaderSize)
	flags := uint32(lnkHasLinkInfo)
	if withList {
		flags |= lnkHasTargetList
	}
	binary.LittleEndian.PutUint32(data[0x14:], flags)
	if withList {
		data = append(data, 3, 0, 1, 2, 3)
	}
	header := 0x1C
	if unicode != "" {
		header = 0x24
	}
	info := make([]byte, header)
	binary.LittleEndian.PutUint32(info[4:], uint32(header))
	binary.LittleEndian.PutUint32(info[8:], lnkLocalBasePath)
	binary.LittleEndian.PutUint32(info[0x10:], uint32(len(info)))
	info = append(info, append([]byte(ansi), 0)...)
	binary.LittleEndian.PutUint32(info[0x18:], uint32(len(info)))
	info = append(info, append([]byte(suffix), 0)...)
	if unicode != "" {
		binary.LittleEndian.PutUint32(info[0x1C:], uint32(len(info)))
		for _, c := range append(utf16.Encode([]rune(unicode)), 0) {
			info = append(info, byte(c), byte(c>>8))
		}
	}
	binary.LittleEndian.PutUint32(info, uint32(len(info)))
	return append(data, info...)
}

func TestLnkTarget(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"ansi", buildLnk(true, `C:\Program Files\App\app.exe`, "", ""), `C:\Program Files\App\app.exe`},
		{"unicode wins", buildLnk(true, `C:\x.exe`, `C:\Programas\Caf\u00e9\caf.exe`, ""), `C:\Programas\Caf\u00e9\caf.exe`},
		{"no id list", buildLnk(false, `C:\a\b.exe`, "", ""), `C:\a\b.exe`},
		{"suffix", buildLnk(true, `C:\a`, "", `b.exe`), `C:\ab.exe`},
		{"garbage", []byte("not a shortcut"), ""},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		if got := lnkTarget(c.data); got != c.want {
			t.Errorf("%s: %q, expected %q", c.name, got, c.want)
		}
	}
}

func TestLnkTargetTruncated(t *testing.T) {
	full := buildLnk(true, `C:\a\b.exe`, "", "")
	for n := 0; n < len(full); n++ {
		lnkTarget(full[:n])
	}
}
