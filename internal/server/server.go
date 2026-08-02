package server

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/app"
	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/engine"
	"github.com/cristobaltormo/typedeck/internal/kbdb"
	"github.com/cristobaltormo/typedeck/internal/layouts"
	"github.com/cristobaltormo/typedeck/internal/packs"
	"github.com/cristobaltormo/typedeck/internal/platform"
	"github.com/cristobaltormo/typedeck/internal/setup"
	"github.com/cristobaltormo/typedeck/web"
)

var Version = "dev"

var Port = app.DefaultPort

type asset struct {
	data  []byte
	gz    []byte
	ctype string
	etag  string
}

type Server struct {
	eng    *engine.Engine
	brd    *board.Board
	paths  config.Paths
	token  string
	assets map[string]*asset
	log    func(string, ...any)
}

func New(eng *engine.Engine, brd *board.Board, paths config.Paths, logf func(string, ...any)) (*Server, error) {
	tok := make([]byte, 24)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	s := &Server{eng: eng, brd: brd, paths: paths, token: base64.RawURLEncoding.EncodeToString(tok), assets: map[string]*asset{}, log: logf}
	return s, s.loadAssets()
}

func (s *Server) loadAssets() error {
	sub, err := fs.Sub(web.UI, "ui")
	if err != nil {
		return err
	}
	return fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		ct := mime.TypeByExtension(path.Ext(p))
		switch path.Ext(p) {
		case ".js":
			ct = "text/javascript; charset=utf-8"
		case ".woff2":
			ct = "font/woff2"
		case ".css":
			ct = "text/css; charset=utf-8"
		case ".html":
			ct = "text/html; charset=utf-8"
		}
		sum := sha256.Sum256(b)
		a := &asset{data: b, ctype: ct, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
		if strings.HasPrefix(ct, "text/") || strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".svg") {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			_, _ = zw.Write(b)
			_ = zw.Close()
			if buf.Len() < len(b) {
				a.gz = buf.Bytes()
			}
		}
		s.assets[p] = a
		return nil
	})
}

func (s *Server) Token() string { return s.token }

func (s *Server) hostOK(r *http.Request) bool {
	h := r.Host
	return h == fmt.Sprintf("127.0.0.1:%d", Port) || h == fmt.Sprintf("localhost:%d", Port)
}

func (s *Server) authed(r *http.Request) bool {
	return s.hostOK(r) && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Token")), []byte(s.token)) == 1
}

func securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"json"}`, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(method string, handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			securityHeaders(w)
			if r.Method != method {
				fail(w, 405, "método no permitido")
				return
			}
			if !s.authed(r) {
				fail(w, 403, "token")
				return
			}
			handler(w, r)
		}
	}
	get := func(p string, h http.HandlerFunc) { mux.HandleFunc("GET "+p, api("GET", h)) }
	post := func(p string, h http.HandlerFunc) { mux.HandleFunc("POST "+p, api("POST", h)) }

	get("/api/config", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.eng.Config()) })
	post("/api/config", s.postConfig)
	get("/api/apps", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, installedApps()) })
	get("/api/status", s.status)
	get("/api/keyboard", s.keyboard)
	get("/api/layouts", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, layouts.All()) })
	get("/api/setup", s.setup)
	get("/api/compat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(setup.Compat)
	})
	get("/api/packs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"categories": packs.Categories(), "packs": packs.All()})
	})
	post("/api/packs/resolve", s.resolvePack)
	post("/api/keyboard/layout", s.setLayout)
	get("/api/system", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.eng.System()) })
	get("/api/events", s.events)
	get("/api/stats", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.eng.Stats()) })
	post("/api/stats/reset", func(w http.ResponseWriter, r *http.Request) {
		s.eng.ResetStats()
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	get("/api/backups", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, config.ListBackups(s.paths)) })
	post("/api/backups/restore", s.restore)
	post("/api/backups/create", func(w http.ResponseWriter, r *http.Request) {
		if err := config.Save(s.paths, s.eng.Config(), true, true); err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	post("/api/test", s.test)
	post("/api/hud", s.hud)
	post("/api/layer", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Index int }
		if json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b) != nil {
			fail(w, 400, "json")
			return
		}
		s.eng.Goto(b.Index)
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	post("/api/board", s.boardCmd)
	post("/api/learn", s.learnSet)
	if os.Getenv("TYPEDECK_DEV") == "1" {
		post("/api/dev/raw", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Line  string
				Multi bool
			}
			if decode(r, &b) != nil {
				fail(w, 400, "json")
				return
			}
			lines, err := s.brd.Do(b.Line, b.Multi, 3*time.Second)
			writeJSON(w, 200, map[string]any{"reply": lines, "error": fmt.Sprint(err)})
		})
	}
	get("/api/learn", s.learnGet)
	mux.HandleFunc("/", s.static)
	return mux
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if !s.hostOK(r) {
		fail(w, 403, "host")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "método no permitido")
		return
	}
	p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	a, ok := s.assets[p]
	if !ok {
		fail(w, 404, "no")
		return
	}
	if p == "index.html" {
		w.Header().Set("Content-Type", a.ctype)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(bytes.ReplaceAll(a.data, []byte("__TOKEN__"), []byte(s.token)))
		return
	}
	w.Header().Set("ETag", a.etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == a.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", a.ctype)
	body := a.data
	if a.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		body = a.gz
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	return dec.Decode(v)
}

func (s *Server) postConfig(w http.ResponseWriter, r *http.Request) {
	var c config.Config
	c.Settings = config.DefaultSettings()
	if err := decode(r, &c); err != nil {
		fail(w, 400, "json: "+err.Error())
		return
	}
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
	s.eng.ApplyRestored(c)
	writeJSON(w, 200, map[string]any{"ok": true, "config": c})
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

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
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
	lines, err := s.brd.Command(b.Cmd, b.Arg)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "reply": lines})
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
	connected, port, info, since := s.brd.Status()
	st := s.eng.Status()
	out := map[string]any{
		"connected": connected, "port": port, "layer": st.Layer, "manual_layer": st.ManualLyr, "last": st.Last,
		"last_ts": st.LastTS, "front": st.Front, "version": Version, "uptime": st.Uptime, "auto_active": st.AutoActive,
		"last_id": st.LastID, "learning": st.Learning, "firmware": "", "keyboard_present": false, "board_error": s.brd.Error(), "app_name": app.Name,
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
	connected, port, info, since := s.brd.Status()
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
			id := kbdb.Identify(info.VID, info.PID, info.Mfr, info.Prod)
			out["identity"] = id
			out["layout"] = setup.Resolve(cfg, info, id, s.eng.System())
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, setup.Describe(s.eng.Config(), s.brd, s.eng))
}

func (s *Server) setLayout(w http.ResponseWriter, r *http.Request) {
	var b struct{ Layout string }
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	l, ok := layouts.Get(b.Layout)
	if !ok {
		fail(w, 400, "disposición desconocida")
		return
	}
	_, _, info, _ := s.brd.Status()
	key := "default"
	name := ""
	if info != nil && info.VID != "" {
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

func installedApps() []string { return platform.Current.InstalledApps() }

func Listen() (net.Listener, error) {
	var lastErr error
	for p := app.DefaultPort; p < app.DefaultPort+10; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			Port = p
			return ln, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func EditorURL() string { return fmt.Sprintf("http://127.0.0.1:%d/", Port) }

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
		fail(w, 404, "paquete desconocido")
		return
	}
	cfg := s.eng.Config()
	if b.Layout == "" {
		_, _, info, _ := s.brd.Status()
		id := kbdb.Identity{}
		if info != nil && info.Present {
			id = kbdb.Identify(info.VID, info.PID, info.Mfr, info.Prod)
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
	res, err := packs.Resolve(p, b.Layout, b.Lang, reserved)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}
