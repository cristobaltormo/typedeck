package platform

import (
	"encoding/binary"
	"unicode/utf16"
)

const (
	lnkHeaderSize     = 0x4C
	lnkHasTargetList  = 0x1
	lnkHasLinkInfo    = 0x2
	lnkLocalBasePath  = 0x1
	lnkInfoUnicodeMin = 0x24
)

func lnkTarget(data []byte) string {
	if len(data) < lnkHeaderSize || binary.LittleEndian.Uint32(data) != lnkHeaderSize {
		return ""
	}
	flags := binary.LittleEndian.Uint32(data[0x14:])
	off := lnkHeaderSize
	if flags&lnkHasTargetList != 0 {
		if off+2 > len(data) {
			return ""
		}
		off += 2 + int(binary.LittleEndian.Uint16(data[off:]))
	}
	if flags&lnkHasLinkInfo == 0 || off+0x1C > len(data) {
		return ""
	}
	info := data[off:]
	if binary.LittleEndian.Uint32(info[8:])&lnkLocalBasePath == 0 {
		return ""
	}
	if binary.LittleEndian.Uint32(info[4:]) >= lnkInfoUnicodeMin && len(info) >= 0x24 {
		base := lnkUTF16(info, binary.LittleEndian.Uint32(info[0x1C:]))
		suffix := lnkUTF16(info, binary.LittleEndian.Uint32(info[0x20:]))
		if base != "" {
			return base + suffix
		}
	}
	return lnkANSI(info, binary.LittleEndian.Uint32(info[0x10:])) + lnkANSI(info, binary.LittleEndian.Uint32(info[0x18:]))
}

func lnkANSI(info []byte, off uint32) string {
	if off == 0 || int(off) >= len(info) {
		return ""
	}
	end := int(off)
	for end < len(info) && info[end] != 0 {
		end++
	}
	return string(info[off:end])
}

func lnkUTF16(info []byte, off uint32) string {
	if off == 0 || int(off)+2 > len(info) {
		return ""
	}
	var u []uint16
	for i := int(off); i+2 <= len(info); i += 2 {
		c := binary.LittleEndian.Uint16(info[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
