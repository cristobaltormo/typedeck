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
	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/engine"
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
	src    capture.Source
	paths  config.Paths
	token  string
	assets map[string]*asset
	log    func(string, ...any)
	hub    hub
}

func New(eng *engine.Engine, src capture.Source, paths config.Paths, logf func(string, ...any)) (*Server, error) {
	tok := make([]byte, 24)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	s := &Server{eng: eng, src: src, paths: paths, token: base64.RawURLEncoding.EncodeToString(tok), assets: map[string]*asset{}, log: logf}
	eng.OnPlainKey = func(u byte, down bool) {
		s.hub.broadcast(map[string]any{"t": "key", "k": fmt.Sprintf("%02X", u), "d": down})
	}
	go s.pumpEvents()
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

type router struct {
	s   *Server
	mux *http.ServeMux
}

func (rt router) handle(method, p string, h http.HandlerFunc) {
	rt.mux.HandleFunc(method+" "+p, func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if !rt.s.authed(r) {
			fail(w, 403, "token")
			return
		}
		h(w, r)
	})
}

func (rt router) get(p string, h http.HandlerFunc)  { rt.handle("GET", p, h) }
func (rt router) post(p string, h http.HandlerFunc) { rt.handle("POST", p, h) }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	rt := router{s: s, mux: mux}
	s.configRoutes(rt)
	s.boardRoutes(rt)
	s.libraryRoutes(rt)
	mux.HandleFunc("GET /api/ws", s.ws)
	mux.HandleFunc("/", s.static)
	return mux
}

func writeOK(w http.ResponseWriter) { writeJSON(w, 200, map[string]bool{"ok": true}) }

func (s *Server) configRoutes(rt router) {
	rt.get("/api/config", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.eng.Config()) })
	rt.post("/api/config", s.postConfig)
	rt.get("/api/backups", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, config.ListBackups(s.paths)) })
	rt.post("/api/backups/restore", s.restore)
	rt.post("/api/backups/create", s.createBackup)
	rt.post("/api/editing", s.setEditing)
}

func (s *Server) boardRoutes(rt router) {
	rt.get("/api/status", s.status)
	rt.get("/api/keyboard", s.keyboard)
	rt.get("/api/setup", s.setup)
	rt.post("/api/keyboard/layout", s.setLayout)
	rt.post("/api/board", s.boardCmd)
	rt.post("/api/capture", s.setCapture)
	rt.get("/api/learn", s.learnGet)
	rt.post("/api/learn", s.learnSet)
	rt.post("/api/layer", s.setLayer)
	if os.Getenv("TYPEDECK_DEV") == "1" {
		rt.post("/api/dev/raw", s.devRaw)
	}
}

func (s *Server) libraryRoutes(rt router) {
	rt.get("/api/apps", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, installedApps()) })
	rt.get("/api/layouts", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, layouts.All()) })
	rt.get("/api/compat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(setup.Compat)
	})
	rt.get("/api/packs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"categories": packs.Categories(), "packs": packs.ForOS(platform.Current.Name()), "os": platform.Current.Name()})
	})
	rt.post("/api/packs/resolve", s.resolvePack)
	rt.post("/api/macros/inspect", s.inspectPack)
	rt.get("/api/system", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.eng.System()) })
	rt.get("/api/events", s.events)
	rt.get("/api/stats", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.eng.Stats()) })
	rt.post("/api/stats/reset", s.resetStats)
	rt.get("/api/history", s.history)
	rt.post("/api/history/clear", s.clearHistory)
	rt.post("/api/test", s.test)
	rt.post("/api/hud", s.hud)
	rt.post("/api/obs/probe", s.obsProbe)
}

func (s *Server) resetStats(w http.ResponseWriter, r *http.Request) {
	s.eng.ResetStats()
	writeOK(w)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if err := config.Save(s.paths, s.eng.Config(), true, true); err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) setLayer(w http.ResponseWriter, r *http.Request) {
	var b struct{ Index int }
	if json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&b) != nil {
		fail(w, 400, "json")
		return
	}
	s.eng.Goto(b.Index)
	writeOK(w)
}

func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.KeyLog().Clear(); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) setEditing(w http.ResponseWriter, r *http.Request) {
	var b struct{ On bool }
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	s.eng.SetEditing(b.On || s.hub.editing())
	writeOK(w)
}

func (s *Server) devRaw(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Line  string
		Multi bool
	}
	if decode(r, &b) != nil {
		fail(w, 400, "json")
		return
	}
	d, ok := s.src.(capture.RawDoer)
	if !ok {
		writeJSON(w, 200, map[string]any{"reply": []string{}, "error": "not available without a board"})
		return
	}
	lines, err := d.Do(b.Line, b.Multi, 3*time.Second)
	writeJSON(w, 200, map[string]any{"reply": lines, "error": fmt.Sprint(err)})
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if !s.hostOK(r) {
		fail(w, 403, "host")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "method not allowed")
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

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func installedApps() []string {
	apps := platform.Current.InstalledApps()
	if apps == nil {
		return []string{}
	}
	return apps
}

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
