package setup

import (
	_ "embed"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/capture"
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

func Identify(info *capture.Info) kbdb.Identity {
	if info.System {
		return kbdb.Identity{Display: "System keyboard", Confidence: "unknown", Source: "system"}
	}
	return kbdb.Identify(info.VID, info.PID, info.Mfr, info.Prod)
}

func Resolve(cfg config.Config, info *capture.Info, id kbdb.Identity, sys actions.SystemInfo) LayoutChoice {
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
	if sys.KeyboardType == "iso" || (sys.KeyboardType == "" && (sys.TypingLayout == "es-iso" || sys.TypingLayout == "es-pc" || sys.TypingLayout == "es-win")) {
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
	Capture   string       `json:"capture"`
	Connected bool         `json:"connected"`
	Port      string       `json:"port"`
	Firmware  string       `json:"firmware"`
	Sys       *capture.Sys `json:"sys,omitempty"`
}

type KeyboardInfo struct {
	Present  bool          `json:"present"`
	Info     *capture.Info `json:"info,omitempty"`
	Identity kbdb.Identity `json:"identity"`
	Layout   LayoutChoice  `json:"layout"`
}

func hostInfo(sys actions.SystemInfo) Host {
	h := Host{OS: runtime.GOOS, Arch: runtime.GOARCH, TypingLayout: sys.TypingLayout, KeyboardType: sys.KeyboardType, ModRemap: sys.ModRemap, KeysSwapped: sys.KeysSwapped}
	h.Version = platform.Current.Version()
	return h
}

func (su *Setup) add(level, code string, kv ...string) {
	c := Check{Level: level, Code: code}
	if len(kv) > 0 {
		c.Args = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			c.Args[kv[i]] = kv[i+1]
		}
	}
	su.Checks = append(su.Checks, c)
}

func Describe(cfg config.Config, src capture.Source, env actions.Env) Setup {
	sys := actions.GetSystemInfo(env)
	su := Setup{Host: hostInfo(sys), Checks: []Check{}}
	connected, port, info, _ := src.Status()
	su.Board = &BoardInfo{Capture: src.Kind(), Connected: connected, Port: port}
	if src.Kind() == capture.Software {
		return su.describeSoftware(cfg, sys, connected, info, src)
	}
	if !connected {
		su.add("error", "no_board")
		su.Summary = "No board connected"
		return su
	}
	var bs capture.Sys
	if r, ok := src.(capture.SysReporter); ok {
		if s, err := r.Sys(); err == nil {
			bs = s
			su.Board.Sys = &bs
		}
	}
	if info != nil {
		su.Board.Firmware = info.Firmware
	}
	if info == nil || !info.Present {
		su.add("warn", "no_keyboard")
		su.Summary = "Board connected, no keyboard on the shield"
		return su
	}
	id := Identify(info)
	ch := Resolve(cfg, info, id, sys)
	su.Keyboard = &KeyboardInfo{Present: true, Info: info, Identity: id, Layout: ch}

	su.add("ok", "board_ok")
	su.addPowerChecks(bs, info)
	su.addReportChecks(info)
	su.addHostChecks(sys, id, ch)
	su.Summary = su.summary(bs, id)
	return su
}

func (su *Setup) describeSoftware(cfg config.Config, sys actions.SystemInfo, connected bool, info *capture.Info, src capture.Source) Setup {
	if !connected || info == nil {
		su.add("error", softwareFailure(src.Error()))
		su.Summary = "Software capture is not running"
		return *su
	}
	id := Identify(info)
	ch := Resolve(cfg, info, id, sys)
	su.Keyboard = &KeyboardInfo{Present: true, Info: info, Identity: id, Layout: ch}
	su.add("ok", "software_ok")
	su.add("info", "software_limits")
	if w, ok := src.(capture.Warner); ok {
		for _, code := range w.Warnings() {
			su.add("warn", code)
		}
	}
	su.addHostChecks(sys, id, ch)
	su.Summary = fmt.Sprintf("System keyboard on %s %s (%s), software capture", osLabel(su.Host.OS), su.Host.Version, su.Host.Arch)
	return *su
}

func softwareFailure(code string) string {
	switch code {
	case "permission":
		return "no_permission"
	case "helper_missing":
		return "no_helper"
	case "no_uinput":
		return "no_uinput"
	case "uinput_access":
		return "no_uinput_access"
	case "input_access":
		return "no_input_access"
	}
	return "no_hook"
}

func osLabel(goos string) string {
	if n := map[string]string{"darwin": "macOS", "linux": "Linux", "windows": "Windows"}[goos]; n != "" {
		return n
	}
	return goos
}

func (su *Setup) addPowerChecks(bs capture.Sys, info *capture.Info) {
	if bs.VccMV > 0 {
		switch {
		case bs.VccMV < 4500:
			su.add("error", "vcc_low", "mv", fmt.Sprint(bs.VccMV))
		case bs.VccMV < 4750 && info.PowerMA >= 300:
			su.add("warn", "vcc_marginal", "mv", fmt.Sprint(bs.VccMV), "ma", fmt.Sprint(info.PowerMA))
		}
	}
	if info.PowerMA > 500 {
		su.add("warn", "power_over", "ma", fmt.Sprint(info.PowerMA))
	}
}

func (su *Setup) addReportChecks(info *capture.Info) {
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
		su.add("error", "no_boot_keyboard")
	} else if hasBoot {
		su.add("ok", "boot_keyboard")
	}
	if hasConsumer {
		su.add("ok", "media_forwarded")
	}
	if hasOther {
		su.add("info", "extras_not_forwarded")
	}
	su.add("info", "six_key_limit")
}

func (su *Setup) addHostChecks(sys actions.SystemInfo, id kbdb.Identity, ch LayoutChoice) {
	if !ch.Confident {
		su.add("info", "layout_unconfirmed")
	}
	if id.Generic || id.VendorIsChip {
		su.add("info", "generic_keyboard")
	}
	switch sys.TypingLayout {
	case "us", "es-iso", "es-pc", "es-win":
	default:
		su.add("warn", "typing_layout_unknown")
	}
	if len(sys.ModRemap) > 0 {
		su.add("info", "mod_remap", "map", strings.Join(sys.ModRemap, ", "))
	}
	if sys.KeysSwapped {
		su.add("info", "ansi_iso_swapped")
	}
}

func (su *Setup) summary(bs capture.Sys, id kbdb.Identity) string {
	osName := osLabel(su.Host.OS)
	if su.Board.Sys == nil {
		return fmt.Sprintf("Board + %s on %s %s", strings.TrimSpace(id.Display), osName, su.Host.Version)
	}
	return fmt.Sprintf("%s + %s + %s en %s %s (%s)", boardName(bs), shieldName(bs), strings.TrimSpace(id.Display), osName, su.Host.Version, su.Host.Arch)
}

func boardName(s capture.Sys) string {
	switch s.Board {
	case "leonardo":
		return fmt.Sprintf("Arduino Leonardo (ATmega32U4, %d MHz)", s.FCPU/1_000_000)
	case "micro":
		return fmt.Sprintf("Arduino Micro (ATmega32U4, %d MHz)", s.FCPU/1_000_000)
	}
	return "ATmega32U4 board"
}

func shieldName(s capture.Sys) string {
	if s.MAX3421Rev == "" {
		return "USB Host Shield"
	}
	rev := s.MAX3421Rev
	if n, err := strconv.ParseUint(rev, 16, 8); err == nil {
		rev = strconv.Itoa(int(n & 0x0F))
	}
	return "USB Host Shield (MAX3421E rev " + rev + ")"
}
