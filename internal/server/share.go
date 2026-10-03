package server

import (
	"net/http"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/config"
)

type sharedPack struct {
	Format string                   `json:"typedeck_macros"`
	Name   string                   `json:"name"`
	Author string                   `json:"author,omitempty"`
	Layers []config.Layer           `json:"layers"`
	Global map[string]config.KeyDef `json:"global,omitempty"`
}

type riskyAction struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Key  string `json:"key"`
	At   string `json:"at"`
}

var riskyTypes = map[string]bool{"shell": true, "ssh": true, "http": true}

func collectRisky(a *config.Action, key, at string, out *[]riskyAction) {
	if a == nil {
		return
	}
	if riskyTypes[a.Type] {
		*out = append(*out, riskyAction{Type: a.Type, Text: actions.Describe(a), Key: key, At: at})
	}
	for i := range a.Steps {
		collectRisky(&a.Steps[i], key, at, out)
	}
	for i := range a.Else {
		collectRisky(&a.Else[i], key, at, out)
	}
}

func (s *Server) inspectPack(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pack sharedPack `json:"pack"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "json")
		return
	}
	p := in.Pack
	if p.Format == "" || len(p.Layers) == 0 && len(p.Global) == 0 {
		fail(w, 400, "not a Typedeck macro file")
		return
	}
	if len(p.Layers) > config.MaxLayers {
		fail(w, 400, "too many layers")
		return
	}
	for i := range p.Layers {
		if p.Layers[i].Keys == nil {
			p.Layers[i].Keys = map[string]config.KeyDef{}
		}
		if p.Layers[i].AutoApps == nil {
			p.Layers[i].AutoApps = []string{}
		}
	}
	if p.Global == nil {
		p.Global = map[string]config.KeyDef{}
	}
	clean, err := config.Validate(config.Config{Layers: p.Layers, Global: p.Global})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	p.Layers, p.Global = clean.Layers, clean.Global
	keys := len(p.Global)
	risky := []riskyAction{}
	scan := func(m map[string]config.KeyDef, at string) {
		for id, kd := range m {
			for _, a := range []*config.Action{kd.Tap, kd.Hold, kd.Double} {
				collectRisky(a, id, at, &risky)
			}
		}
	}
	scan(p.Global, "global")
	for _, l := range p.Layers {
		keys += len(l.Keys)
		scan(l.Keys, l.Name)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "pack": p, "keys": keys, "risky": risky})
}
