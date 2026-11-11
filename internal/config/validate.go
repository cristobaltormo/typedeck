package config

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	keyID    = regexp.MustCompile(`^[0-9A-Fa-f]{2}$`)
	backupRe = regexp.MustCompile(`^config-\d{8}-\d{6}\.json$`)
)

const (
	MaxLayers = 9
	maxStr    = 4000
)

var (
	mediaCmds  = set("playpause", "next", "prev", "volup", "voldown", "mute")
	obsCmds    = set("scene", "scene_next", "scene_prev", "stream", "stream_start", "stream_stop", "record", "record_pause", "mute", "replay_save", "virtualcam", "studio", "studio_transition")
	systemCmds = set("lock", "sleepdisplay", "sleep", "screensaver", "screenshot", "darkmode", "caffeinate")
	condKinds  = set("app", "layer", "time", "os", "obs_stream", "obs_record", "clipboard")
	actionSet  = set("app", "url", "shell", "ssh", "http", "hotkey", "text", "sequence", "media", "system", "timer", "layer", "hud", "wait", "open", "obs", "if", "history")
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
		return "", fmt.Errorf("%s: too long", name)
	}
	return s, nil
}

func bptr(b bool) *bool { return &b }

type fieldCleaner struct {
	where string
	err   error
}

func (c *fieldCleaner) str(v, name string, max int) string {
	if c.err != nil {
		return ""
	}
	var s string
	s, c.err = str(v, c.where+"."+name, max)
	return s
}

func clamp[T int | float64](v, lo, hi T) T {
	return min(max(v, lo), hi)
}

func inSet(v, def string, options map[string]bool) string {
	if options[v] {
		return v
	}
	return def
}

func cleanLayerTarget(to any) any {
	switch v := to.(type) {
	case string:
		if v == "next" || v == "prev" {
			return v
		}
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	case float64:
		return int(v)
	case int:
		return v
	}
	return "next"
}

func cleanAction(a Action, where string, depth int) (Action, error) {
	if !actionSet[a.Type] {
		return a, fmt.Errorf("%s: invalid action type", where)
	}
	if a.Type == "open" {
		a.Type = "app"
	}
	out := Action{Type: a.Type, Confirm: a.Confirm}
	c := &fieldCleaner{where: where}
	switch a.Type {
	case "app":
		out.App = c.str(a.App, "app", 200)
		out.Mode = oneOf(a.Mode, "open", "open", "toggle", "quit")
	case "url":
		out.URL = c.str(a.URL, "url", 2000)
	case "shell":
		out.Cmd = c.str(a.Cmd, "cmd", maxStr)
		out.ShowOutput = bptr(a.ShowOutput != nil && *a.ShowOutput)
	case "ssh":
		out.Host = c.str(a.Host, "host", 200)
		out.Cmd = c.str(a.Cmd, "cmd", maxStr)
		out.ShowOutput = bptr(a.ShowOutput == nil || *a.ShowOutput)
	case "http":
		out.Method = oneOf(a.Method, "GET", "GET", "POST", "PUT", "DELETE")
		out.URL = c.str(a.URL, "url", 2000)
		out.Body = c.str(a.Body, "body", maxStr)
	case "hotkey":
		out.Keys = c.str(a.Keys, "keys", 100)
	case "text":
		out.Text = c.str(a.Text, "text", maxStr)
	case "media":
		out.Cmd = inSet(a.Cmd, "playpause", mediaCmds)
	case "system":
		out.Cmd = inSet(a.Cmd, "screensaver", systemCmds)
	case "timer":
		out.Minutes = clamp(a.Minutes, 0.1, 600)
		out.Label = c.str(a.Label, "label", 60)
	case "layer":
		out.To = cleanLayerTarget(a.To)
		out.Momentary = a.Momentary
		out.Toggle = a.Toggle && !a.Momentary
	case "obs":
		out.Cmd = inSet(a.Cmd, "record", obsCmds)
		out.Target = c.str(a.Target, "target", 200)
	case "hud":
		out.Text = c.str(a.Text, "text", 80)
	case "history":
		out.Cmd = oneOf(a.Cmd, "toggle", "on", "off", "toggle")
	case "wait":
		out.Ms = clamp(a.Ms, 0, 60000)
	case "sequence":
		return cleanSequence(a, out, where, depth)
	case "if":
		return cleanCondition(a, out, where, depth)
	}
	return out, c.err
}

func cleanSequence(a, out Action, where string, depth int) (Action, error) {
	if depth >= 1 {
		return a, fmt.Errorf("%s: nested sequences are not allowed", where)
	}
	var err error
	out.Steps, err = cleanSteps(a.Steps, where+".steps", depth+1)
	return out, err
}

func cleanCondition(a, out Action, where string, depth int) (Action, error) {
	if depth >= 2 {
		return a, fmt.Errorf("%s: nested conditions are not allowed", where)
	}
	if !condKinds[a.Cond] {
		return a, fmt.Errorf("%s: invalid condition", where)
	}
	out.Cond, out.Not = a.Cond, a.Not
	c := &fieldCleaner{where: where}
	out.Target = c.str(a.Target, "target", 200)
	err := c.err
	if err == nil {
		out.Steps, err = cleanSteps(a.Steps, where+".steps", 2)
	}
	if err == nil {
		out.Else, err = cleanSteps(a.Else, where+".else", 2)
	}
	return out, err
}

const MaxSteps = 100

func cleanSteps(in []Action, where string, depth int) ([]Action, error) {
	if len(in) > MaxSteps {
		return nil, fmt.Errorf("%s: at most %d steps", where, MaxSteps)
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
			return nil, fmt.Errorf("%s: unknown key %s", where, id)
		}
		n, _ := strconv.ParseUint(id, 16, 8)
		if n < 0x04 || n > 0xA4 {
			return nil, fmt.Errorf("%s: key %s is not valid", where, id)
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
			return nil, fmt.Errorf("%s/%s: text too long", where, id)
		}
		c.Label, c.Icon = kd.Label, kd.Icon
		if kd.Color != "" {
			if !hexColor.MatchString(kd.Color) {
				return nil, fmt.Errorf("%s/%s: invalid color", where, id)
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
		return c, errors.New("at least one layer is required")
	}
	if len(c.Layers) > MaxLayers {
		return c, fmt.Errorf("at most %d layers", MaxLayers)
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
	s.KeyHistoryDays = clampInt(s.KeyHistoryDays, 1, 365, d.KeyHistoryDays)
	s.Capture = oneOf(s.Capture, d.Capture, "board", "software")
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
			return c, fmt.Errorf("layer %d: the name is missing", i)
		}
		if len(l.Name) > 30 {
			l.Name = l.Name[:30]
		}
		if l.Color != "" && !hexColor.MatchString(l.Color) {
			return c, fmt.Errorf("layer %d: invalid color", i)
		}
		if len(l.AutoApps) > 20 {
			return c, fmt.Errorf("layer %d: too many apps", i)
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
