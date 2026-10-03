package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/app"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

const Version = 3

type Action struct {
	Type       string   `json:"type"`
	App        string   `json:"app,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	URL        string   `json:"url,omitempty"`
	Cmd        string   `json:"cmd,omitempty"`
	ShowOutput *bool    `json:"show_output,omitempty"`
	Host       string   `json:"host,omitempty"`
	Method     string   `json:"method,omitempty"`
	Body       string   `json:"body,omitempty"`
	Keys       string   `json:"keys,omitempty"`
	Text       string   `json:"text,omitempty"`
	Minutes    float64  `json:"minutes,omitempty"`
	Label      string   `json:"label,omitempty"`
	To         any      `json:"to,omitempty"`
	Momentary  bool     `json:"momentary,omitempty"`
	Ms         int      `json:"ms,omitempty"`
	Target     string   `json:"target,omitempty"`
	Steps      []Action `json:"steps,omitempty"`
	Else       []Action `json:"else,omitempty"`
	Cond       string   `json:"cond,omitempty"`
	Not        bool     `json:"not,omitempty"`
	Confirm    bool     `json:"confirm,omitempty"`
}

type KeyDef struct {
	Tap    *Action `json:"tap,omitempty"`
	Hold   *Action `json:"hold,omitempty"`
	Double *Action `json:"double,omitempty"`
	Label  string  `json:"label,omitempty"`
	Icon   string  `json:"icon,omitempty"`
	Color  string  `json:"color,omitempty"`
}

func (k KeyDef) Empty() bool {
	return k.Tap == nil && k.Hold == nil && k.Double == nil && k.Label == "" && k.Icon == "" && k.Color == ""
}

func (k KeyDef) HasAction() bool { return k.Tap != nil || k.Hold != nil || k.Double != nil }

type Layer struct {
	Name     string            `json:"name"`
	Color    string            `json:"color,omitempty"`
	Icon     string            `json:"icon,omitempty"`
	AutoApps []string          `json:"auto_apps"`
	Keys     map[string]KeyDef `json:"keys"`
}

type HUD struct {
	Enabled  bool    `json:"enabled"`
	Position string  `json:"position"`
	Seconds  float64 `json:"seconds"`
	OnAction bool    `json:"on_action"`
	OnAuto   bool    `json:"on_auto"`
	Sound    bool    `json:"sound"`
}

type Settings struct {
	Language     string `json:"language"`
	Theme        string `json:"theme"`
	Accent       string `json:"accent"`
	Density      string `json:"density"`
	KeySize      int    `json:"keysize"`
	HUD          HUD    `json:"hud"`
	HoldMS       int    `json:"hold_ms"`
	DoubleMS     int    `json:"double_ms"`
	AutoLayer    bool   `json:"auto_layer"`
	VolumeStep   int    `json:"volume_step"`
	Input        string `json:"input"`
	TypingLayout string `json:"typing_layout"`
	OBS          OBS    `json:"obs"`

	NotifyKeyboard bool `json:"notify_keyboard"`
	KeyHistory     bool `json:"key_history"`
}

type OBS struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
}

type Keyboard struct {
	Layout string `json:"layout"`
	Name   string `json:"name,omitempty"`
}

type Config struct {
	Version   int                 `json:"version"`
	Settings  Settings            `json:"settings"`
	Global    map[string]KeyDef   `json:"global"`
	Layers    []Layer             `json:"layers"`
	Keyboards map[string]Keyboard `json:"keyboards"`
}

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	keyID    = regexp.MustCompile(`^[0-9A-Fa-f]{2}$`)
	backupRe = regexp.MustCompile(`^config-\d{8}-\d{6}\.json$`)
)

const (
	MaxLayers = 9
	maxStr    = 4000
)

type Paths struct{ Dir string }

func (p Paths) Config() string     { return filepath.Join(p.Dir, "config.json") }
func (p Paths) Backups() string    { return filepath.Join(p.Dir, "backups") }
func (p Paths) Stats() string      { return filepath.Join(p.Dir, "stats.json") }
func (p Paths) Keystrokes() string { return filepath.Join(p.Dir, "keystrokes.jsonl") }

func DefaultPaths() Paths {
	p := Paths{Dir: platform.Current.ConfigDir()}
	parent := filepath.Dir(p.Dir)
	if _, err := os.Stat(p.Dir); errors.Is(err, os.ErrNotExist) {
		for _, old := range app.OldSlugs {
			dir := filepath.Join(parent, old)
			if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
				_ = copyDir(dir, p.Dir)
				break
			}
		}
	}
	return p
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	})
}

func DefaultSettings() Settings {
	return Settings{Language: "en", Theme: "auto", Accent: "#2563eb", Density: "comfortable", KeySize: 88,
		HUD:    HUD{Enabled: true, Position: "bottom", Seconds: 1.2, OnAuto: true},
		HoldMS: 450, DoubleMS: 280, AutoLayer: true, VolumeStep: 6, Input: "auto", TypingLayout: "auto", OBS: OBS{Host: "127.0.0.1", Port: 4455}, NotifyKeyboard: true}
}

func Default() Config {
	return Config{
		Version:  Version,
		Settings: DefaultSettings(),
		Global: map[string]KeyDef{
			"53": {Tap: &Action{Type: "layer", To: "next"}, Label: "Capa", Icon: "layers"},
			"58": {Tap: &Action{Type: "url", URL: "{editor}"}, Label: "Editor", Icon: "sliders"},
		},
		Layers: []Layer{
			{Name: "Capa 1", Color: "#2563eb", Icon: "keyboard", AutoApps: []string{}, Keys: map[string]KeyDef{}},
			{Name: "Capa 2", Color: "#8b5cf6", Icon: "bolt", AutoApps: []string{}, Keys: map[string]KeyDef{}},
		},
		Keyboards: map[string]Keyboard{},
	}
}

var v2Names = map[string]string{"NUMLOCK": "53", "KP/": "54", "KP*": "55", "KP-": "56", "KP+": "57", "KPENTER": "58",
	"KP1": "59", "KP2": "5A", "KP3": "5B", "KP4": "5C", "KP5": "5D", "KP6": "5E", "KP7": "5F", "KP8": "60", "KP9": "61",
	"KP0": "62", "KP.": "63"}

func Parse(data []byte) (Config, error) {
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		return Config{}, fmt.Errorf("json invalido: %w", err)
	}
	ver, _ := probe["version"].(float64)
	switch {
	case ver >= 3:
		c := Config{Settings: DefaultSettings()}
		if err := json.Unmarshal(data, &c); err != nil {
			return Config{}, err
		}
		return Validate(c)
	case ver == 2:
		return migrateV2(probe)
	default:
		return migrateV1(probe)
	}
}

func migrateV1(m map[string]any) (Config, error) {
	conv := func(keys any) map[string]any {
		out := map[string]any{}
		km, _ := keys.(map[string]any)
		for k, v := range km {
			a, ok := v.(map[string]any)
			if !ok || a["type"] == nil {
				continue
			}
			a = cloneMap(a)
			kd := map[string]any{}
			if l, ok := a["label"].(string); ok && l != "" {
				kd["label"] = l
			}
			delete(a, "label")
			kd["tap"] = a
			out[k] = kd
		}
		return out
	}
	layers := []any{}
	if ls, ok := m["layers"].([]any); ok {
		for _, l := range ls {
			lm, _ := l.(map[string]any)
			layers = append(layers, map[string]any{"name": lm["name"], "keys": conv(lm["keys"])})
		}
	}
	v2 := map[string]any{"version": 2.0, "global": conv(m["global"]), "layers": layers, "settings": map[string]any{}}
	if st, ok := m["settings"].(map[string]any); ok && st["hud_on_action"] == true {
		v2["settings"] = map[string]any{"hud": map[string]any{"on_action": true}}
	}
	return migrateV2(v2)
}

func migrateV2(m map[string]any) (Config, error) {
	rekey := func(keys any) map[string]any {
		out := map[string]any{}
		km, _ := keys.(map[string]any)
		for k, v := range km {
			if id, ok := v2Names[k]; ok {
				out[id] = v
			} else if keyID.MatchString(k) {
				out[strings.ToUpper(k)] = v
			}
		}
		return out
	}
	m = cloneMap(m)
	m["global"] = rekey(m["global"])
	if ls, ok := m["layers"].([]any); ok {
		for i, l := range ls {
			if lm, ok := l.(map[string]any); ok {
				lm = cloneMap(lm)
				lm["keys"] = rekey(lm["keys"])
				ls[i] = lm
			}
		}
	}
	m["version"] = float64(Version)
	b, _ := json.Marshal(m)
	c := Config{Settings: DefaultSettings()}
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	if _, ok := c.Global["53"]; !ok {
		if c.Global == nil {
			c.Global = map[string]KeyDef{}
		}
		c.Global["53"] = KeyDef{Tap: &Action{Type: "layer", To: "next"}, Label: "Capa", Icon: "layers"}
	}
	return Validate(c)
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

var (
	mediaCmds  = set("playpause", "next", "prev", "volup", "voldown", "mute")
	obsCmds    = set("scene", "scene_next", "scene_prev", "stream", "stream_start", "stream_stop", "record", "record_pause", "mute", "replay_save", "virtualcam", "studio", "studio_transition")
	systemCmds = set("lock", "sleepdisplay", "sleep", "screensaver", "screenshot", "darkmode", "caffeinate")
	condKinds  = set("app", "layer", "time", "os", "obs_stream", "obs_record", "clipboard")
	actionSet  = set("app", "url", "shell", "ssh", "http", "hotkey", "text", "sequence", "media", "system", "timer", "layer", "hud", "wait", "open", "obs", "if")
)

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func str(s, name string, max int) (string, error) {
	if len(s) > max {
		return "", fmt.Errorf("%s: demasiado largo", name)
	}
	return s, nil
}

func bptr(b bool) *bool { return &b }

func cleanAction(a Action, where string, depth int) (Action, error) {
	if !actionSet[a.Type] {
		return a, fmt.Errorf("%s: tipo de acción inválido", where)
	}
	if a.Type == "open" {
		a.Type = "app"
	}
	out := Action{Type: a.Type, Confirm: a.Confirm}
	var err error
	field := func(v string, name string, max int) string {
		if err != nil {
			return ""
		}
		var s string
		s, err = str(v, where+"."+name, max)
		return s
	}
	switch a.Type {
	case "app":
		out.App = field(a.App, "app", 200)
		out.Mode = map[bool]string{true: a.Mode, false: "open"}[a.Mode == "open" || a.Mode == "toggle" || a.Mode == "quit"]
	case "url":
		out.URL = field(a.URL, "url", 2000)
	case "shell":
		out.Cmd = field(a.Cmd, "cmd", maxStr)
		out.ShowOutput = bptr(a.ShowOutput != nil && *a.ShowOutput)
	case "ssh":
		out.Host = field(a.Host, "host", 200)
		out.Cmd = field(a.Cmd, "cmd", maxStr)
		out.ShowOutput = bptr(a.ShowOutput == nil || *a.ShowOutput)
	case "http":
		out.Method = map[bool]string{true: a.Method, false: "GET"}[a.Method == "GET" || a.Method == "POST" || a.Method == "PUT" || a.Method == "DELETE"]
		out.URL = field(a.URL, "url", 2000)
		out.Body = field(a.Body, "body", maxStr)
	case "hotkey":
		out.Keys = field(a.Keys, "keys", 100)
	case "text":
		out.Text = field(a.Text, "text", maxStr)
	case "media":
		out.Cmd = map[bool]string{true: a.Cmd, false: "playpause"}[mediaCmds[a.Cmd]]
	case "system":
		out.Cmd = map[bool]string{true: a.Cmd, false: "screensaver"}[systemCmds[a.Cmd]]
	case "timer":
		out.Minutes = a.Minutes
		if out.Minutes < 0.1 {
			out.Minutes = 0.1
		}
		if out.Minutes > 600 {
			out.Minutes = 600
		}
		out.Label = field(a.Label, "label", 60)
	case "layer":
		switch v := a.To.(type) {
		case string:
			if v == "next" || v == "prev" {
				out.To = v
			} else if n, e := strconv.Atoi(v); e == nil {
				out.To = n
			} else {
				out.To = "next"
			}
		case float64:
			out.To = int(v)
		case int:
			out.To = v
		default:
			out.To = "next"
		}
		out.Momentary = a.Momentary
	case "obs":
		out.Cmd = map[bool]string{true: a.Cmd, false: "record"}[obsCmds[a.Cmd]]
		out.Target = field(a.Target, "target", 200)
	case "hud":
		out.Text = field(a.Text, "text", 80)
	case "wait":
		out.Ms = a.Ms
		if out.Ms < 0 {
			out.Ms = 0
		}
		if out.Ms > 60000 {
			out.Ms = 60000
		}
	case "sequence":
		if depth >= 1 {
			return a, fmt.Errorf("%s: secuencias anidadas no permitidas", where)
		}
		out.Steps, err = cleanSteps(a.Steps, where+".steps", depth+1)
	case "if":
		if depth >= 2 {
			return a, fmt.Errorf("%s: condiciones anidadas no permitidas", where)
		}
		if !condKinds[a.Cond] {
			return a, fmt.Errorf("%s: condición inválida", where)
		}
		out.Cond, out.Not = a.Cond, a.Not
		out.Target = field(a.Target, "target", 200)
		if err == nil {
			out.Steps, err = cleanSteps(a.Steps, where+".steps", 2)
		}
		if err == nil {
			out.Else, err = cleanSteps(a.Else, where+".else", 2)
		}
	}
	return out, err
}

const MaxSteps = 100

func cleanSteps(in []Action, where string, depth int) ([]Action, error) {
	if len(in) > MaxSteps {
		return nil, fmt.Errorf("%s: máximo %d pasos", where, MaxSteps)
	}
	out := make([]Action, 0, len(in))
	for i, s := range in {
		c, err := cleanAction(s, fmt.Sprintf("%s[%d]", where, i), depth)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func cleanKeys(keys map[string]KeyDef, where string) (map[string]KeyDef, error) {
	out := map[string]KeyDef{}
	for id, kd := range keys {
		if !keyID.MatchString(id) {
			return nil, fmt.Errorf("%s: tecla desconocida %s", where, id)
		}
		n, _ := strconv.ParseUint(id, 16, 8)
		if n < 0x04 || n > 0xA4 {
			return nil, fmt.Errorf("%s: la tecla %s no es válida", where, id)
		}
		id = strings.ToUpper(id)
		var c KeyDef
		for i, p := range []**Action{&c.Tap, &c.Hold, &c.Double} {
			src := []*Action{kd.Tap, kd.Hold, kd.Double}[i]
			if src == nil {
				continue
			}
			a, err := cleanAction(*src, fmt.Sprintf("%s/%s.%s", where, id, []string{"tap", "hold", "double"}[i]), 0)
			if err != nil {
				return nil, err
			}
			*p = &a
		}
		if len(kd.Label) > 40 || len(kd.Icon) > 30 {
			return nil, fmt.Errorf("%s/%s: texto demasiado largo", where, id)
		}
		c.Label, c.Icon = kd.Label, kd.Icon
		if kd.Color != "" {
			if !hexColor.MatchString(kd.Color) {
				return nil, fmt.Errorf("%s/%s: color inválido", where, id)
			}
			c.Color = kd.Color
		}
		if !c.Empty() {
			out[id] = c
		}
	}
	return out, nil
}

func clampInt(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func oneOf(v string, def string, opts ...string) string {
	for _, o := range opts {
		if v == o {
			return v
		}
	}
	return def
}

func Validate(c Config) (Config, error) {
	if len(c.Layers) == 0 {
		return c, errors.New("falta al menos una capa")
	}
	if len(c.Layers) > MaxLayers {
		return c, fmt.Errorf("máximo %d capas", MaxLayers)
	}
	d := DefaultSettings()
	s := c.Settings
	s.Language = oneOf(s.Language, d.Language, "es", "en")
	s.Theme = oneOf(s.Theme, d.Theme, "auto", "dark", "light")
	s.Density = oneOf(s.Density, d.Density, "comfortable", "compact")
	if !hexColor.MatchString(s.Accent) {
		s.Accent = d.Accent
	}
	s.KeySize = clampInt(s.KeySize, 64, 120, d.KeySize)
	s.HoldMS = clampInt(s.HoldMS, 200, 1500, d.HoldMS)
	s.DoubleMS = clampInt(s.DoubleMS, 120, 800, d.DoubleMS)
	s.VolumeStep = clampInt(s.VolumeStep, 1, 25, d.VolumeStep)
	s.Input = oneOf(s.Input, d.Input, "auto", "hardware", "software")
	s.TypingLayout = oneOf(s.TypingLayout, d.TypingLayout, "auto", "us", "es-iso", "es-pc", "es-win")
	s.OBS.Host = strings.TrimSpace(s.OBS.Host)
	if s.OBS.Host == "" || len(s.OBS.Host) > 200 || strings.ContainsAny(s.OBS.Host, " /\\") {
		s.OBS.Host = "127.0.0.1"
	}
	s.OBS.Port = clampInt(s.OBS.Port, 1, 65535, 4455)
	if len(s.OBS.Password) > 200 {
		s.OBS.Password = s.OBS.Password[:200]
	}
	s.HUD.Position = oneOf(s.HUD.Position, d.HUD.Position, "bottom", "top", "center")
	if s.HUD.Seconds < 0.4 {
		s.HUD.Seconds = d.HUD.Seconds
	}
	if s.HUD.Seconds > 6 {
		s.HUD.Seconds = 6
	}
	c.Settings = s
	c.Version = Version

	var err error
	if c.Global, err = cleanKeys(c.Global, "global"); err != nil {
		return c, err
	}
	for i := range c.Layers {
		l := c.Layers[i]
		l.Name = strings.TrimSpace(l.Name)
		if l.Name == "" {
			return c, fmt.Errorf("capa %d: falta el nombre", i)
		}
		if len(l.Name) > 30 {
			l.Name = l.Name[:30]
		}
		if l.Color != "" && !hexColor.MatchString(l.Color) {
			return c, fmt.Errorf("capa %d: color inválido", i)
		}
		if len(l.AutoApps) > 20 {
			return c, fmt.Errorf("capa %d: demasiadas apps", i)
		}
		apps := []string{}
		for _, a := range l.AutoApps {
			if a = strings.TrimSpace(a); a != "" && len(a) < 100 {
				apps = append(apps, a)
			}
		}
		l.AutoApps = apps
		if l.Keys, err = cleanKeys(l.Keys, fmt.Sprintf("capa %d", i)); err != nil {
			return c, err
		}
		c.Layers[i] = l
	}
	if c.Keyboards == nil {
		c.Keyboards = map[string]Keyboard{}
	}
	for k, kb := range c.Keyboards {
		if len(k) > 20 || len(kb.Layout) > 30 || len(kb.Name) > 80 {
			delete(c.Keyboards, k)
		}
	}
	return c, nil
}

func Load(p Paths) (Config, error) {
	c, _, err := LoadWithNote(p)
	return c, err
}

func LoadWithNote(p Paths) (Config, string, error) {
	b, err := os.ReadFile(p.Config())
	if errors.Is(err, os.ErrNotExist) {
		c := Default()
		return c, "", Save(p, c, false, false)
	}
	if err != nil {
		return Config{}, "", err
	}
	c, perr := Parse(b)
	if perr == nil {
		return c, "", nil
	}
	broken := filepath.Join(p.Dir, "config.broken-"+time.Now().Format("20060102-150405")+".json")
	_ = os.Rename(p.Config(), broken)
	files, _ := filepath.Glob(filepath.Join(p.Backups(), "config-*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if rc, err := Parse(raw); err == nil {
			if err := Save(p, rc, false, false); err == nil {
				return rc, fmt.Sprintf("la configuración estaba dañada (%v): se recuperó la copia %s y el original quedó en %s", perr, filepath.Base(f), filepath.Base(broken)), nil
			}
		}
	}
	d := Default()
	return d, fmt.Sprintf("la configuración estaba dañada (%v) y no había copias válidas: se empezó de cero; el original quedó en %s", perr, filepath.Base(broken)), Save(p, d, false, false)
}

func Save(p Paths, c Config, backup, force bool) error {
	c, err := Validate(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return err
	}
	if backup {
		if old, err := os.ReadFile(p.Config()); err == nil {
			_ = os.MkdirAll(p.Backups(), 0o755)
			files, _ := filepath.Glob(filepath.Join(p.Backups(), "config-*.json"))
			sort.Strings(files)
			recent := false
			if len(files) > 0 {
				if st, err := os.Stat(files[len(files)-1]); err == nil && time.Since(st.ModTime()) < 2*time.Minute {
					recent = true
				}
			}
			if force || !recent {
				name := filepath.Join(p.Backups(), "config-"+time.Now().Format("20060102-150405")+".json")
				_ = os.WriteFile(name, old, 0o600)
				files = append(files, name)
				sort.Strings(files)
				for len(files) > 40 {
					_ = os.Remove(files[0])
					files = files[1:]
				}
			}
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.Config() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.Config())
}

type BackupInfo struct {
	Name   string  `json:"name"`
	TS     float64 `json:"ts"`
	Layers int     `json:"layers"`
	Keys   int     `json:"keys"`
}

func ListBackups(p Paths) []BackupInfo {
	files, _ := filepath.Glob(filepath.Join(p.Backups(), "config-*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	out := []BackupInfo{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		c, err := Parse(b)
		if err != nil {
			continue
		}
		st, _ := os.Stat(f)
		keys := len(c.Global)
		for _, l := range c.Layers {
			keys += len(l.Keys)
		}
		out = append(out, BackupInfo{Name: filepath.Base(f), TS: float64(st.ModTime().Unix()), Layers: len(c.Layers), Keys: keys})
	}
	return out
}

func RestoreBackup(p Paths, name string) (Config, error) {
	if !backupRe.MatchString(name) {
		return Config{}, errors.New("copia desconocida")
	}
	b, err := os.ReadFile(filepath.Join(p.Backups(), name))
	if err != nil {
		return Config{}, err
	}
	c, err := Parse(b)
	if err != nil {
		return c, err
	}
	return c, Save(p, c, true, true)
}
