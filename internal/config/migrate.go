package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

var v2Names = map[string]string{"NUMLOCK": "53", "KP/": "54", "KP*": "55", "KP-": "56", "KP+": "57", "KPENTER": "58",
	"KP1": "59", "KP2": "5A", "KP3": "5B", "KP4": "5C", "KP5": "5D", "KP6": "5E", "KP7": "5F", "KP8": "60", "KP9": "61",
	"KP0": "62", "KP.": "63"}

func Parse(data []byte) (Config, error) {
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		return Config{}, fmt.Errorf("invalid json: %w", err)
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
