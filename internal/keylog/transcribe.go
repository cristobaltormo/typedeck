package keylog

import (
	"sort"
	"strings"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const (
	maxTranscript = 4000
	holdSlackMS   = 40
)

type span struct {
	from, to int64
	bit      byte
}

func Transcribe(entries []Entry, layout *hid.Layout) string {
	sorted := append([]Entry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].T < sorted[j].T })

	var held []span
	for _, e := range sorted {
		if e.U >= 0xE0 && e.U <= 0xE7 {
			held = append(held, span{e.T, e.T + int64(e.D) + holdSlackMS, 1 << (e.U - 0xE0)})
		}
	}
	macOption := layout != nil && layout.Name == "es-iso"

	var out []rune
	caps := false
	for _, e := range sorted {
		if e.U >= 0xE0 && e.U <= 0xE7 {
			continue
		}
		if e.U == 0x39 {
			caps = !caps
			continue
		}
		var bits byte
		for _, s := range held {
			if e.T >= s.from && e.T <= s.to {
				bits |= s.bit
			}
		}
		shift := bits&0x22 != 0
		ctrl := bits&0x11 != 0
		gui := bits&0x88 != 0
		altL := bits&0x04 != 0
		altGr := bits&0x40 != 0
		if e.U >= 0x04 && e.U <= 0x1D && caps {
			shift = !shift
		}
		if ctrl || gui || (altL && !macOption) {
			out = append(out, []rune(shortcut(e, ctrl, gui, altL, shift))...)
			continue
		}
		if e.U == 0x2A {
			if n := len(out); n > 0 {
				out = out[:n-1]
			}
			continue
		}
		var mods byte
		if shift {
			mods |= hid.ModShift
		}
		if altL {
			mods |= hid.ModAlt
		}
		if altGr {
			mods |= hid.ModAltGr
		}
		if layout != nil {
			if r, ok := layout.Rune(mods, e.U); ok {
				out = append(out, r)
			}
		}
	}
	if len(out) > maxTranscript {
		out = out[len(out)-maxTranscript:]
	}
	return string(out)
}

func shortcut(e Entry, ctrl, gui, alt, shift bool) string {
	var parts []string
	if ctrl {
		parts = append(parts, "Ctrl")
	}
	if alt {
		parts = append(parts, "Alt")
	}
	if shift {
		parts = append(parts, "Shift")
	}
	if gui {
		parts = append(parts, "Cmd")
	}
	return "[" + strings.Join(append(parts, Name(e.U)), "+") + "]"
}
