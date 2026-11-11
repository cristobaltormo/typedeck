package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/app"
	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hid"
	"github.com/cristobaltormo/typedeck/internal/kbdb"
	"github.com/cristobaltormo/typedeck/internal/layouts"
	"github.com/cristobaltormo/typedeck/internal/obs"
	"github.com/cristobaltormo/typedeck/internal/packs"
	"github.com/cristobaltormo/typedeck/internal/platform"
	"github.com/cristobaltormo/typedeck/internal/setup"
)

func (s *Server) postConfig(w http.ResponseWriter, r *http.Request) {
	var c config.Config
	c.Settings = config.DefaultSettings()
	if err := decode(r, &c); err != nil {
		fail(w, 400, "json: "+err.Error())
		return
	}
	c.Settings.Capture = s.eng.Config().Settings.Capture
	loaded, err := s.eng.SetConfig(c)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "config": loaded})
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	var b struct{ Name string }
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	c, err := config.RestoreBackup(s.paths, b.Name)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "config": s.eng.ApplyRestored(c)})
}

func (s *Server) test(w http.ResponseWriter, r *http.Request) {
	var a config.Action
	if err := decode(r, &a); err != nil {
		fail(w, 400, "json")
		return
	}
	clean, err := config.Validate(config.Config{Layers: []config.Layer{{Name: "t", Keys: map[string]config.KeyDef{"04": {Tap: &a}}}}})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.eng.Test(*clean.Layers[0].Keys["04"].Tap))
}

func (s *Server) hud(w http.ResponseWriter, r *http.Request) {
	var b struct{ Title, Subtitle string }
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	s.eng.HUD(clip(b.Title, 60), clip(b.Subtitle, 80), true)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) obsProbe(w http.ResponseWriter, r *http.Request) {
	var b config.OBS
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	if b == (config.OBS{}) {
		b = s.eng.Config().Settings.OBS
	}
	info, err := obs.Probe(obs.Config{Host: b.Host, Port: b.Port, Password: b.Password})
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "info": info})
}

func (s *Server) boardCmd(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Cmd string
		Arg int
	}
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	c, ok := s.src.(capture.Commander)
	if !ok {
		fail(w, 400, "command not available without a board")
		return
	}
	lines, err := c.Command(b.Cmd, b.Arg)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "reply": lines})
}

func (s *Server) setCapture(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Mode    string
		Consent bool
	}
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	if b.Mode == capture.Software && !b.Consent {
		fail(w, 400, "software capture needs explicit consent")
		return
	}
	if b.Mode != capture.Board && b.Mode != capture.Software {
		fail(w, 400, "unknown capture mode")
		return
	}
	if err := s.eng.SetCapture(b.Mode, b.Consent); err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "capture": b.Mode, "config": s.eng.Config()})
}

func (s *Server) learnSet(w http.ResponseWriter, r *http.Request) {
	var b struct{ On bool }
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	if b.On {
		if err := s.eng.StartLearn(); err != nil {
			fail(w, 400, err.Error())
			return
		}
	} else {
		s.eng.StopLearn()
	}
	writeJSON(w, 200, map[string]any{"ok": true, "learning": b.On})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	connected, port, info, since := s.src.Status()
	st := s.eng.Status()
	out := map[string]any{
		"connected": connected, "port": port, "layer": st.Layer, "manual_layer": st.ManualLyr, "last": st.Last,
		"last_ts": st.LastTS, "front": st.Front, "version": Version, "uptime": st.Uptime, "auto_active": st.AutoActive,
		"last_id": st.LastID, "learning": st.Learning, "firmware": "", "keyboard_present": false, "board_error": s.src.Error(), "capture": s.eng.CaptureMode(), "app_name": app.Name, "editing": s.eng.Editing(), "editors": s.hub.count(),
	}
	if connected {
		out["connected_since"] = float64(since.Unix())
	}
	if info != nil {
		out["firmware"] = info.Firmware
		out["keyboard_present"] = info.Present
	}
	writeJSON(w, 200, out)
}

func (s *Server) keyboard(w http.ResponseWriter, r *http.Request) {
	connected, port, info, since := s.src.Status()
	out := map[string]any{"connected": connected, "port": port}
	if connected {
		out["connected_since"] = float64(since.Unix())
	}
	if info != nil {
		out["info"] = info
		cfg := s.eng.Config()
		if prefs, ok := cfg.Keyboards[info.ID()]; ok {
			out["prefs"] = prefs
		}
		if info.Present {
			id := setup.Identify(info)
			out["identity"] = id
			out["layout"] = setup.Resolve(cfg, info, id, s.eng.System())
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, setup.Describe(s.eng.Config(), s.src, s.eng))
}

func (s *Server) setLayout(w http.ResponseWriter, r *http.Request) {
	var b struct{ Layout string }
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	l, ok := layouts.Get(b.Layout)
	if !ok {
		fail(w, 400, "unknown layout")
		return
	}
	_, _, info, _ := s.src.Status()
	key := "default"
	name := ""
	if info != nil && info.ID() != "" {
		key, name = info.ID(), strings.TrimSpace(info.Mfr+" "+info.Prod)
	}
	cfg := s.eng.Config()
	if cfg.Keyboards == nil {
		cfg.Keyboards = map[string]config.Keyboard{}
	}
	cfg.Keyboards[key] = config.Keyboard{Layout: l.ID, Name: name}
	loaded, err := s.eng.SetConfig(cfg)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "config": loaded})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	wait, _ := strconv.ParseFloat(r.URL.Query().Get("wait"), 64)
	if wait > 30 {
		wait = 30
	}
	evs, last := s.eng.EventsSince(since, time.Duration(wait*float64(time.Second)), r.Context().Done())
	writeJSON(w, 200, map[string]any{"events": evs, "last_id": last})
}

func (s *Server) learnGet(w http.ResponseWriter, r *http.Request) {
	seen := s.eng.Seen()
	set := map[byte]bool{}
	for k := range seen {
		var u byte
		if _, err := fmt.Sscanf(k, "%02X", &u); err == nil {
			set[u] = true
		}
	}
	sugs := layouts.Match(set)
	if len(sugs) > 4 {
		sugs = sugs[:4]
	}
	amb, hints := layouts.Ambiguity(sugs)
	writeJSON(w, 200, map[string]any{"learning": s.eng.Status().Learning, "seen": seen, "suggestions": sugs, "ambiguous": amb, "hint_keys": hints})
}

func (s *Server) resolvePack(w http.ResponseWriter, r *http.Request) {
	var b struct{ ID, Layout, Lang string }
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	p, ok := packs.Get(b.ID)
	if !ok {
		fail(w, 404, "unknown pack")
		return
	}
	cfg := s.eng.Config()
	if b.Layout == "" {
		_, _, info, _ := s.src.Status()
		id := kbdb.Identity{}
		if info != nil && info.Present {
			id = setup.Identify(info)
		}
		b.Layout = setup.Resolve(cfg, info, id, s.eng.System()).ID
	}
	if b.Lang == "" {
		b.Lang = cfg.Settings.Language
	}
	reserved := map[string]bool{}
	for k := range cfg.Global {
		reserved[k] = true
	}
	res, err := packs.ResolveFor(platform.Current.Name(), p, b.Layout, b.Lang, reserved)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	layout := actions.TypingLayout(s.eng)
	if layout.Name == "none" {
		layout = hid.LayoutByName("us")
	}
	rep := s.eng.KeyLog().Report(limit, 2500, time.Now(), layout)
	writeJSON(w, 200, map[string]any{"enabled": s.eng.KeyLog().Enabled(), "count": rep.Summary.Total, "entries": rep.Entries, "summary": rep.Summary, "text": rep.Text})
}
