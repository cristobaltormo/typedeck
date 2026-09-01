package setup

import (
	_ "embed"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/kbdb"
	"github.com/cristobaltormo/typedeck/internal/layouts"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

//go:embed compat.json
var Compat []byte

type LayoutChoice struct {
	ID        string `json:"id"`
	Percent   int    `json:"percent"`
	Standard  string `json:"standard"`
	Source    string `json:"source"`
	Confident bool   `json:"confident"`
}

func Resolve(cfg config.Config, info *board.Info, id kbdb.Identity, sys actions.SystemInfo) LayoutChoice {
	pick := func(layoutID, source string, confident bool) LayoutChoice {
		l, ok := layouts.Get(layoutID)
		if !ok {
			return LayoutChoice{}
		}
		return LayoutChoice{ID: l.ID, Percent: l.Percent, Standard: l.Standard, Source: source, Confident: confident}
	}
	if info != nil {
		if kb, ok := cfg.Keyboards[info.ID()]; ok && kb.Layout != "" {
			if c := pick(kb.Layout, "user", true); c.ID != "" {
				return c
			}
		}
	}
	if id.Layout != "" {
		src := "name"
		if id.Confidence == "verified" {
			src = "verified"
		}
		if c := pick(id.Layout, src, id.Confidence == "verified"); c.ID != "" {
			return c
		}
	}
	std := "ansi"
	if sys.KeyboardType == "iso" || (sys.KeyboardType == "" && (sys.TypingLayout == "es-iso" || sys.TypingLayout == "es-pc")) {
		std = "iso"
	}
	family := "full"
	if id.FormFactor != "" {
		family = id.FormFactor
	}
	return pick(family+"-"+std, "default", false)
}

type Check struct {
	Level string            `json:"level"`
	Code  string            `json:"code"`
	Args  map[string]string `json:"args,omitempty"`
}

type Host struct {
	OS           string   `json:"os"`
	Version      string   `json:"version"`
	Arch         string   `json:"arch"`
	TypingLayout string   `json:"typing_layout"`
	KeyboardType string   `json:"keyboard_type"`
	ModRemap     []string `json:"mod_remap"`
	KeysSwapped  bool     `json:"keys_swapped"`
}

type Setup struct {
	Board    *BoardInfo    `json:"board"`
	Keyboard *KeyboardInfo `json:"keyboard"`
	Host     Host          `json:"host"`
	Checks   []Check       `json:"checks"`
	Summary  string        `json:"summary"`
}

type BoardInfo struct {
	Connected bool       `json:"connected"`
	Port      string     `json:"port"`
	Firmware  string     `json:"firmware"`
	Sys       *board.Sys `json:"sys,omitempty"`
}

type KeyboardInfo struct {
	Present  bool          `json:"present"`
	Info     *board.Info   `json:"info,omitempty"`
	Identity kbdb.Identity `json:"identity"`
	Layout   LayoutChoice  `json:"layout"`
}

func hostInfo(sys actions.SystemInfo) Host {
	h := Host{OS: runtime.GOOS, Arch: runtime.GOARCH, TypingLayout: sys.TypingLayout, KeyboardType: sys.KeyboardType, ModRemap: sys.ModRemap, KeysSwapped: sys.KeysSwapped}
	h.Version = platform.Current.Version()
	return h
}

func Describe(cfg config.Config, brd *board.Board, env actions.Env) Setup {
	sys := actions.GetSystemInfo(env)
	su := Setup{Host: hostInfo(sys), Checks: []Check{}}
	connected, port, info, _ := brd.Status()
	su.Board = &BoardInfo{Connected: connected, Port: port}
	add := func(level, code string, kv ...string) {
		c := Check{Level: level, Code: code}
		if len(kv) > 0 {
			c.Args = map[string]string{}
			for i := 0; i+1 < len(kv); i += 2 {
				c.Args[kv[i]] = kv[i+1]
			}
		}
		su.Checks = append(su.Checks, c)
	}
	if !connected {
		add("error", "no_board")
		su.Summary = "Sin placa conectada"
		return su
	}
	var bs board.Sys
	if s, err := brd.Sys(); err == nil {
		bs = s
		su.Board.Sys = &bs
	}
	if info != nil {
		su.Board.Firmware = info.Firmware
	}
	if info == nil || !info.Present {
		add("warn", "no_keyboard")
		su.Summary = "Placa conectada, sin teclado en el shield"
		return su
	}
	id := kbdb.Identify(info.VID, info.PID, info.Mfr, info.Prod)
	ch := Resolve(cfg, info, id, sys)
	su.Keyboard = &KeyboardInfo{Present: true, Info: info, Identity: id, Layout: ch}

	add("ok", "board_ok")
	if bs.VccMV > 0 {
		switch {
		case bs.VccMV < 4500:
			add("error", "vcc_low", "mv", fmt.Sprint(bs.VccMV))
		case bs.VccMV < 4750 && info.PowerMA >= 300:
			add("warn", "vcc_marginal", "mv", fmt.Sprint(bs.VccMV), "ma", fmt.Sprint(info.PowerMA))
		}
	}
	if info.PowerMA > 500 {
		add("warn", "power_over", "ma", fmt.Sprint(info.PowerMA))
	}
	hasBoot, hasConsumer, hasOther := false, false, false
	for _, r := range info.Reports {
		if r.Dir != "input" {
			continue
		}
		switch r.Kind {
		case "keyboard":
			hasBoot = true
		case "consumer":
			hasConsumer = true
		case "mouse", "system", "nkro":
			hasOther = true
		}
	}
	if len(info.Reports) > 0 && !hasBoot {
		add("error", "no_boot_keyboard")
	} else if hasBoot {
		add("ok", "boot_keyboard")
	}
	if hasConsumer {
		add("ok", "media_forwarded")
	}
	if hasOther {
		add("info", "extras_not_forwarded")
	}
	add("info", "six_key_limit")
	if !ch.Confident {
		add("info", "layout_unconfirmed")
	}
	if id.Generic || id.VendorIsChip {
		add("info", "generic_keyboard")
	}
	switch sys.TypingLayout {
	case "us", "es-iso", "es-pc":
	default:
		add("warn", "typing_layout_unknown")
	}
	if len(sys.ModRemap) > 0 {
		add("info", "mod_remap", "map", strings.Join(sys.ModRemap, ", "))
	}
	if sys.KeysSwapped {
		add("info", "ansi_iso_swapped")
	}

	osName := map[string]string{"darwin": "macOS", "linux": "Linux", "windows": "Windows"}[su.Host.OS]
	if osName == "" {
		osName = su.Host.OS
	}
	su.Summary = fmt.Sprintf("%s + %s + %s en %s %s (%s)", boardName(bs), shieldName(bs), strings.TrimSpace(id.Display), osName, su.Host.Version, su.Host.Arch)
	if su.Board.Sys == nil {
		su.Summary = fmt.Sprintf("Placa + %s en %s %s", strings.TrimSpace(id.Display), osName, su.Host.Version)
	}
	return su
}

func boardName(s board.Sys) string {
	switch s.Board {
	case "leonardo":
		return fmt.Sprintf("Arduino Leonardo (ATmega32U4, %d MHz)", s.FCPU/1_000_000)
	case "micro":
		return fmt.Sprintf("Arduino Micro (ATmega32U4, %d MHz)", s.FCPU/1_000_000)
	}
	return "Placa ATmega32U4"
}

func shieldName(s board.Sys) string {
	if s.MAX3421Rev == "" {
		return "USB Host Shield"
	}
	rev := s.MAX3421Rev
	if n, err := strconv.ParseUint(rev, 16, 8); err == nil {
		rev = strconv.Itoa(int(n & 0x0F))
	}
	return "USB Host Shield (MAX3421E rev " + rev + ")"
}
