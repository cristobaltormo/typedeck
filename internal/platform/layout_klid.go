package platform

import (
	"fmt"
	"strconv"
	"strings"
)

var qwertyLanguages = map[uint64]bool{0x09: true, 0x0A: true, 0x16: true, 0x10: true, 0x13: true, 0x1D: true, 0x14: true, 0x06: true}

func klidFromHKL(hkl uint32, byLayoutID func(id, lang uint16) string) string {
	lang, dev := uint16(hkl), uint16(hkl>>16)
	if dev&0xF000 == 0xF000 {
		if byLayoutID == nil {
			return ""
		}
		return byLayoutID(dev&0x0FFF, lang)
	}
	if dev == 0 {
		dev = lang
	}
	return fmt.Sprintf("0000%04X", dev)
}

func layoutFromKLID(klid string) string {
	klid = strings.ToUpper(strings.TrimSpace(klid))
	if len(klid) != 8 || klid[:4] != "0000" {
		return ""
	}
	switch klid {
	case "0000040A":
		return "es-win"
	case "00000409":
		return "us"
	}
	id, err := strconv.ParseUint(klid[4:], 16, 16)
	if err != nil || !qwertyLanguages[id&0x3FF] {
		return ""
	}
	return "qwerty"
}
