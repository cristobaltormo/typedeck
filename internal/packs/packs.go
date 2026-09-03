package packs

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/layouts"
)

//go:embed packs.json
var raw []byte

type Text map[string]string

func (t Text) Get(lang string) string {
	if s := t[lang]; s != "" {
		return s
	}
	return t["es"]
}

type Macro struct {
	Key      string         `json:"key,omitempty"`
	Label    Text           `json:"label"`
	Icon     string         `json:"icon,omitempty"`
	Color    string         `json:"color,omitempty"`
	Tap      *config.Action `json:"tap,omitempty"`
	Hold     *config.Action `json:"hold,omitempty"`
	Double   *config.Action `json:"double,omitempty"`
	TapPC    *config.Action `json:"tap_pc,omitempty"`
	HoldPC   *config.Action `json:"hold_pc,omitempty"`
	DoublePC *config.Action `json:"double_pc,omitempty"`
}

type Pack struct {
	ID          string                   `json:"id"`
	Category    string                   `json:"category"`
	Icon        string                   `json:"icon"`
	Color       string                   `json:"color,omitempty"`
	Name        Text                     `json:"name"`
	Description Text                     `json:"description"`
	Notes       Text                     `json:"notes,omitempty"`
	Apps        []string                 `json:"apps,omitempty"`
	AutoApps    []string                 `json:"auto_apps,omitempty"`
	Tags        []string                 `json:"tags,omitempty"`
	Region      string                   `json:"region"`
	OS          []string                 `json:"os,omitempty"`
	Macros      []Macro                  `json:"macros"`
	Global      map[string]config.KeyDef `json:"global,omitempty"`
}

type Category struct {
	ID   string `json:"id"`
	Icon string `json:"icon"`
	Name Text   `json:"name"`
}

type file struct {
	Categories []Category `json:"categories"`
	Packs      []Pack     `json:"packs"`
}

var data = func() file {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		panic("packs.json invalido: " + err.Error())
	}
	return f
}()

func All() []Pack            { return data.Packs }
func Categories() []Category { return data.Categories }

func Get(id string) (Pack, bool) {
	for _, p := range data.Packs {
		if p.ID == id {
			return p, true
		}
	}
	return Pack{}, false
}

var regions = map[string][]byte{
	"numpad":  {0x5F, 0x60, 0x61, 0x5C, 0x5D, 0x5E, 0x59, 0x5A, 0x5B, 0x62, 0x63, 0x54, 0x55, 0x56, 0x57},
	"fn":      {0x3A, 0x3B, 0x3C, 0x3D, 0x3E, 0x3F, 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48},
	"numrow":  {0x1E, 0x1F, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x2D, 0x2E},
	"letters": {0x14, 0x1A, 0x08, 0x15, 0x17, 0x1C, 0x18, 0x0C, 0x12, 0x13, 0x04, 0x16, 0x07, 0x09, 0x0A, 0x0B, 0x0D, 0x0E, 0x0F},
}

type Resolved struct {
	Layer   config.Layer             `json:"layer"`
	Global  map[string]config.KeyDef `json:"global"`
	Region  string                   `json:"region"`
	Dropped int                      `json:"dropped"`
}

func (p Pack) ForOS(goos string) bool {
	if len(p.OS) == 0 {
		return true
	}
	for _, o := range p.OS {
		if o == goos {
			return true
		}
	}
	return false
}

func ForOS(goos string) []Pack {
	var out []Pack
	for _, p := range data.Packs {
		if p.ForOS(goos) {
			out = append(out, p)
		}
	}
	return out
}

func pcAction(a *config.Action) *config.Action {
	if a == nil {
		return nil
	}
	c := *a
	switch c.Type {
	case "hotkey":
		var parts []string
		seen := map[string]bool{}
		for _, t := range strings.Split(strings.ToLower(c.Keys), "+") {
			if t == "cmd" || t == "command" || t == "gui" {
				t = "ctrl"
			}
			if t == "ctrl" && seen[t] {
				continue
			}
			seen[t] = true
			parts = append(parts, t)
		}
		c.Keys = strings.Join(parts, "+")
	case "sequence":
		c.Steps = nil
		for i := range a.Steps {
			c.Steps = append(c.Steps, *pcAction(&a.Steps[i]))
		}
	}
	return &c
}

func Resolve(p Pack, layoutID, lang string, reserved map[string]bool) (Resolved, error) {
	return ResolveFor(runtime.GOOS, p, layoutID, lang, reserved)
}

func ResolveFor(goos string, p Pack, layoutID, lang string, reserved map[string]bool) (Resolved, error) {
	l, ok := layouts.Get(layoutID)
	if !ok {
		l, _ = layouts.Get("full-iso")
	}
	have := l.KeySet()
	slotsFor := func(region string) []byte {
		var out []byte
		for _, u := range regions[region] {
			if have[u] && !reserved[fmt.Sprintf("%02X", u)] {
				out = append(out, u)
			}
		}
		return out
	}
	order := []string{p.Region}
	for _, r := range []string{"numpad", "fn", "numrow", "letters"} {
		if r != p.Region {
			order = append(order, r)
		}
	}
	region, slots := "", []byte(nil)
	need := len(p.Macros)
	for _, r := range order {
		s := slotsFor(r)
		if len(s) >= need || (len(s) >= (need+1)/2 && len(s) > 0 && r == p.Region) {
			region, slots = r, s
			break
		}
		if region == "" || len(s) > len(slots) {
			region, slots = r, s
		}
	}
	res := Resolved{Region: region, Global: map[string]config.KeyDef{}}
	res.Layer = config.Layer{Name: p.Name.Get(lang), Color: p.Color, Icon: p.Icon, AutoApps: append([]string{}, p.AutoApps...), Keys: map[string]config.KeyDef{}}
	put := func(id string, m Macro) {
		tap, hold, double := m.Tap, m.Hold, m.Double
		if goos != "darwin" {
			pick := func(pc, mac *config.Action) *config.Action {
				if pc != nil {
					return pc
				}
				return pcAction(mac)
			}
			tap, hold, double = pick(m.TapPC, m.Tap), pick(m.HoldPC, m.Hold), pick(m.DoublePC, m.Double)
		}
		res.Layer.Keys[id] = config.KeyDef{Tap: tap, Hold: hold, Double: double, Label: m.Label.Get(lang), Icon: m.Icon, Color: m.Color}
	}
	free := append([]byte(nil), slots...)
	var pending []Macro
	for _, m := range p.Macros {
		if m.Key != "" {
			var u byte
			if _, err := fmt.Sscanf(m.Key, "%02X", &u); err == nil && have[u] && !reserved[m.Key] {
				put(m.Key, m)
				for i, f := range free {
					if f == u {
						free = append(free[:i], free[i+1:]...)
						break
					}
				}
				continue
			}
		}
		pending = append(pending, m)
	}
	for i, m := range pending {
		if i >= len(free) {
			res.Dropped++
			continue
		}
		put(fmt.Sprintf("%02X", free[i]), m)
	}
	for k, kd := range p.Global {
		res.Global[k] = kd
	}
	return res, nil
}

func SortedIDs() []string {
	var ids []string
	for _, p := range data.Packs {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)
	return ids
}
