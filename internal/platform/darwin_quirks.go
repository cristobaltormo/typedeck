//go:build darwin

package platform

import (
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const leonardoKey = "com.apple.keyboard.modifiermapping.9025-32822-0"

var pairRe = regexp.MustCompile(`HIDKeyboardModifierMappingDst\s*=\s*(\d+);\s*HIDKeyboardModifierMappingSrc\s*=\s*(\d+);`)

func parseModifierMapping(out string) map[int]int {
	m := map[int]int{}
	for _, p := range pairRe.FindAllStringSubmatch(out, -1) {
		dst, _ := strconv.ParseInt(p[1], 10, 64)
		src, _ := strconv.ParseInt(p[2], 10, 64)
		m[int(src&0xFF)] = int(dst & 0xFF)
	}
	return m
}

func invertMods(mapping map[int]int, wanted byte) byte {
	if len(mapping) == 0 {
		return wanted
	}
	var out byte
	for bit := 0; bit < 4; bit++ {
		if wanted&(1<<bit) == 0 {
			continue
		}
		send := bit
		for src, dst := range mapping {
			if dst == 0xE0+bit && src >= 0xE0 && src <= 0xE3 {
				send = src - 0xE0
			}
		}
		out |= 1 << send
	}
	return out | wanted&0xF0
}

var (
	modMu  sync.Mutex
	modMap map[int]int
	modAt  time.Time
)

func currentModMapping() map[int]int {
	modMu.Lock()
	defer modMu.Unlock()
	if time.Since(modAt) > 30*time.Second {
		out, _ := Run(3*time.Second, "", "defaults", "-currentHost", "read", "-g", leonardoKey)
		modMap, modAt = parseModifierMapping(out), time.Now()
	}
	return modMap
}

var kbTypeRe = regexp.MustCompile(`"32822-9025-0"\s*=\s*(\d+)`)

var (
	kbMu   sync.Mutex
	kbType int
	kbAt   time.Time
)

// keyboardType is the type macOS stores for the board (40 ANSI, 41 ISO). With ANSI and an ISO layout macOS swaps the two keys
// that do not exist on ANSI (usages 0x35 and 0x64).
func keyboardType() int {
	kbMu.Lock()
	defer kbMu.Unlock()
	if time.Since(kbAt) > 30*time.Second {
		out, _ := Run(3*time.Second, "", "defaults", "read", "/Library/Preferences/com.apple.keyboardtype")
		kbType = 0
		if m := kbTypeRe.FindStringSubmatch(out); m != nil {
			kbType, _ = strconv.Atoi(m[1])
		}
		kbAt = time.Now()
	}
	return kbType
}

func (*darwin) AdaptStroke(s hid.Stroke, layout *hid.Layout) hid.Stroke {
	s.Mods = invertMods(currentModMapping(), s.Mods)
	if layout != nil && layout.Name == "es-iso" && keyboardType() == 40 {
		switch s.Usage {
		case 0x35:
			s.Usage = 0x64
		case 0x64:
			s.Usage = 0x35
		}
	}
	return s
}

var modName = map[int]string{0xE0: "Ctrl", 0xE1: "Shift", 0xE2: "Alt", 0xE3: "Cmd", 0xE4: "Ctrl der.", 0xE5: "Shift der.", 0xE6: "Alt der.", 0xE7: "Cmd der."}

func (*darwin) Info(l *hid.Layout) SystemInfo {
	info := SystemInfo{TypingLayout: l.Name, ModRemap: []string{}}
	switch keyboardType() {
	case 40:
		info.KeyboardType = "ansi"
	case 41:
		info.KeyboardType = "iso"
	case 42:
		info.KeyboardType = "jis"
	}
	info.KeysSwapped = l.Name == "es-iso" && keyboardType() == 40
	for src, dst := range currentModMapping() {
		if src != dst && modName[src] != "" && modName[dst] != "" {
			info.ModRemap = append(info.ModRemap, modName[src]+" -> "+modName[dst])
		}
	}
	sort.Strings(info.ModRemap)
	return info
}
