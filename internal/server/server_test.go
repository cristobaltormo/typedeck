package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/engine"
)

type noBoard struct{}

func (noBoard) Open() (io.ReadWriteCloser, string, error) { return nil, "", board.ErrNoBoard }

func newSrv(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	brd := board.New(noBoard{})
	paths := config.Paths{Dir: t.TempDir()}
	eng, err := engine.New(paths, brd)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(eng, brd, paths, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	return s, s.Handler()
}

func do(h http.Handler, method, path, host, token string, body any) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = host
	if token != "" {
		req.Header.Set("X-Token", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const good = "127.0.0.1:7788"

func TestApiNeedsTokenAndHost(t *testing.T) {
	s, h := newSrv(t)
	for _, c := range []struct {
		name, host, token string
		want              int
	}{
		{"without a token", good, "", 403},
		{"token malo", good, "x", 403},
		{"foreign host with a good token (DNS rebinding)", "evil.com", s.token, 403},
		{"host with a foreign port", "127.0.0.1:9999", s.token, 403},
		{"localhost vale", "localhost:7788", s.token, 200},
		{"correcto", good, s.token, 200},
	} {
		if got := do(h, "GET", "/api/config", c.host, c.token, nil).Code; got != c.want {
			t.Errorf("%s: %d, expected %d", c.name, got, c.want)
		}
	}
}

func TestWritesRejectedWithoutToken(t *testing.T) {
	_, h := newSrv(t)
	for _, p := range []string{"/api/config", "/api/test", "/api/hud", "/api/layer", "/api/board", "/api/backups/restore", "/api/learn"} {
		if got := do(h, "POST", p, good, "", map[string]any{}).Code; got != 403 {
			t.Errorf("POST %s without a token: %d", p, got)
		}
	}
}

func TestMethodsAreChecked(t *testing.T) {
	s, h := newSrv(t)
	if got := do(h, "POST", "/api/status", good, s.token, nil).Code; got != 405 && got != 404 {
		t.Errorf("POST to a GET route: %d", got)
	}
	if got := do(h, "GET", "/api/test", good, s.token, nil).Code; got != 405 && got != 404 {
		t.Errorf("GET to a POST route: %d", got)
	}
}

func TestConfigRoundTripAndValidation(t *testing.T) {
	s, h := newSrv(t)
	rec := do(h, "GET", "/api/config", good, s.token, nil)
	var c config.Config
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil || len(c.Layers) != 2 {
		t.Fatalf("config inicial: %v %s", err, rec.Body.String())
	}
	c.Layers[0].Name = "Nueva"
	if got := do(h, "POST", "/api/config", good, s.token, c).Code; got != 200 {
		t.Fatalf("guardar: %d", got)
	}
	var again config.Config
	_ = json.Unmarshal(do(h, "GET", "/api/config", good, s.token, nil).Body.Bytes(), &again)
	if again.Layers[0].Name != "Nueva" {
		t.Fatal("it was not saved")
	}
	bad := map[string]any{"version": 3, "layers": []any{}}
	if got := do(h, "POST", "/api/config", good, s.token, bad).Code; got != 400 {
		t.Fatalf("configuration without layers: %d", got)
	}
	evil := map[string]any{"version": 3, "layers": []any{map[string]any{"name": "x", "keys": map[string]any{"04": map[string]any{"tap": map[string]any{"type": "rm -rf"}}}}}}
	if got := do(h, "POST", "/api/config", good, s.token, evil).Code; got != 400 {
		t.Fatalf("made-up action type: %d", got)
	}
}

func TestStaticAssetsHeadersAndCaching(t *testing.T) {
	s, h := newSrv(t)
	idx := do(h, "GET", "/", good, "", nil)
	if idx.Code != 200 || !strings.Contains(idx.Body.String(), s.token) || strings.Contains(idx.Body.String(), "__TOKEN__") {
		t.Fatal("the index must carry this run's key")
	}
	if idx.Header().Get("Content-Security-Policy") == "" || idx.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers are missing")
	}
	if got := do(h, "GET", "/", "evil.com", "", nil).Code; got != 403 {
		t.Fatalf("index with a foreign host: %d", got)
	}
	req := httptest.NewRequest("GET", "/js/app.js", nil)
	req.Host = good
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("js without gzip: %d %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !strings.Contains(string(plain), "eventsLoop") {
		t.Fatal("the decompressed js is not the expected one")
	}
	etag := rec.Header().Get("ETag")
	req2 := httptest.NewRequest("GET", "/js/app.js", nil)
	req2.Host = good
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != 304 {
		t.Fatalf("ETag not honoured: %d", rec2.Code)
	}
	for _, p := range []string{"/../go.mod", "/js/../../go.mod", "/%2e%2e/go.mod"} {
		if got := do(h, "GET", p, good, "", nil).Code; got == 200 {
			t.Errorf("%s must not be served", p)
		}
	}
}

func TestEventsLongPollReturnsOnTimeout(t *testing.T) {
	s, h := newSrv(t)
	rec := do(h, "GET", "/api/events?since=999999&wait=0.2", good, s.token, nil)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	var out struct {
		Events []any `json:"events"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Events) != 0 {
		t.Fatal("there should be no events")
	}
}

func TestTestEndpointRunsSafeActionAndRejectsInvalid(t *testing.T) {
	s, h := newSrv(t)
	rec := do(h, "POST", "/api/test", good, s.token, map[string]any{"type": "wait", "ms": 5})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("wait: %d %s", rec.Code, rec.Body.String())
	}
	if got := do(h, "POST", "/api/test", good, s.token, map[string]any{"type": "nope"}).Code; got != 400 {
		t.Fatalf("invalid type: %d", got)
	}
}

func TestLayoutsPacksSetupAndCompat(t *testing.T) {
	s, h := newSrv(t)
	get := func(path string) *httptest.ResponseRecorder { return do(h, "GET", path, good, s.token, nil) }

	var ls []struct {
		ID      string
		Percent int
		Keys    []any
	}
	_ = json.Unmarshal(get("/api/layouts").Body.Bytes(), &ls)
	if len(ls) != 11 || ls[0].ID == "" || len(ls[0].Keys) == 0 {
		t.Fatalf("layouts: %d", len(ls))
	}

	var pk struct {
		Categories []any
		Packs      []struct{ ID string }
	}
	_ = json.Unmarshal(get("/api/packs").Body.Bytes(), &pk)
	if len(pk.Packs) < 25 || len(pk.Categories) < 8 {
		t.Fatalf("gallery: %d packs, %d categories", len(pk.Packs), len(pk.Categories))
	}
	rec := do(h, "POST", "/api/packs/resolve", good, s.token, map[string]string{"id": "zoom", "layout": "tkl-ansi", "lang": "en"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"region":"fn"`) || !strings.Contains(rec.Body.String(), `"name":"Zoom"`) {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}
	if do(h, "POST", "/api/packs/resolve", good, s.token, map[string]string{"id": "nope"}).Code != 404 {
		t.Fatal("a missing pack must give 404")
	}

	if !strings.Contains(get("/api/setup").Body.String(), "no_board") {
		t.Fatal("without a board it must warn")
	}
	var compat map[string]any
	if err := json.Unmarshal(get("/api/compat").Body.Bytes(), &compat); err != nil || compat["groups"] == nil {
		t.Fatalf("compat: %v", err)
	}
	if do(h, "POST", "/api/keyboard/layout", good, s.token, map[string]string{"layout": "no-existe"}).Code != 400 {
		t.Fatal("an unknown layout must give 400")
	}
	if do(h, "POST", "/api/keyboard/layout", good, s.token, map[string]string{"layout": "iso-full"}).Code != 200 {
		t.Fatal("the old name is accepted")
	}
	var cfg config.Config
	_ = json.Unmarshal(get("/api/config").Body.Bytes(), &cfg)
	if cfg.Keyboards["default"].Layout != "full-iso" {
		t.Fatalf("the layout was not saved: %+v", cfg.Keyboards)
	}
}

func TestLearnReportsSuggestionsAndAmbiguity(t *testing.T) {
	s, h := newSrv(t)
	rec := do(h, "GET", "/api/learn", good, s.token, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"suggestions"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestInspectPackCleansAndFlagsRiskyActions(t *testing.T) {
	s, h := newSrv(t)
	pack := map[string]any{"pack": map[string]any{
		"typedeck_macros": "1", "name": "demo",
		"layers": []any{map[string]any{"name": "Dev", "keys": map[string]any{
			"04": map[string]any{"tap": map[string]any{"type": "shell", "cmd": "rm -rf ~"}},
			"05": map[string]any{"tap": map[string]any{"type": "sequence", "steps": []any{map[string]any{"type": "http", "url": "https://x.test", "method": "POST"}, map[string]any{"type": "wait", "ms": 99999999}}}},
			"06": map[string]any{"tap": map[string]any{"type": "url", "url": "https://example.com"}},
		}}},
	}}
	rec := do(h, "POST", "/api/macros/inspect", good, s.token, pack)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out struct {
		Keys  int `json:"keys"`
		Risky []struct{ Type, Key string }
		Pack  struct {
			Layers []struct {
				Keys map[string]struct {
					Tap struct{ Steps []struct{ Ms int } }
				}
			}
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Keys != 3 || len(out.Risky) != 2 {
		t.Fatalf("wrong summary: %+v", out)
	}
	if out.Pack.Layers[0].Keys["05"].Tap.Steps[1].Ms != 60000 {
		t.Error("the huge wait was not clamped")
	}
	bad := map[string]any{"pack": map[string]any{"name": "x", "layers": []any{}}}
	if got := do(h, "POST", "/api/macros/inspect", good, s.token, bad).Code; got != 400 {
		t.Errorf("a foreign file was accepted: %d", got)
	}
}

func TestHistoryEndpointsReadAndClear(t *testing.T) {
	s, h := newSrv(t)
	cfg := config.Default()
	cfg.Settings.KeyHistory = true
	cfg.Settings.TypingLayout = "auto"
	s.eng.ApplyRestored(cfg)
	now := time.Now()
	s.eng.KeyLog().Press(0x04, now)
	s.eng.KeyLog().Release(0x04, now.Add(50*time.Millisecond))

	var out struct {
		Enabled bool
		Count   int
		Text    string
		Entries []struct{ K string }
	}
	rec := do(h, "GET", "/api/history?limit=10", good, s.token, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.Enabled || out.Count != 1 || out.Entries[0].K != "A" || strings.ToLower(out.Text) != "a" {
		t.Fatalf("historial: %s", rec.Body)
	}
	if got := do(h, "GET", "/api/history", good, "", nil).Code; got != 403 {
		t.Errorf("the history could be read without a token: %d", got)
	}
	if got := do(h, "POST", "/api/history/clear", good, s.token, nil).Code; got != 200 {
		t.Fatalf("borrar: %d", got)
	}
	rec = do(h, "GET", "/api/history", good, s.token, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Count != 0 {
		t.Errorf("after deleting %d remain", out.Count)
	}
}

func TestExamplesAreValidPacks(t *testing.T) {
	files, err := filepath.Glob("../../examples/*.json")
	if err != nil || len(files) < 3 {
		t.Fatalf("examples not found: %v %v", files, err)
	}
	s, h := newSrv(t)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var pack map[string]any
		if err := json.Unmarshal(raw, &pack); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		rec := do(h, "POST", "/api/macros/inspect", good, s.token, map[string]any{"pack": pack})
		if rec.Code != 200 {
			t.Errorf("%s: %d %s", f, rec.Code, rec.Body)
		}
	}
}

func TestHistoryTextFallsBackToUSWhenTheSystemLayoutIsUnknown(t *testing.T) {
	s, h := newSrv(t)
	cfg := config.Default()
	cfg.Settings.KeyHistory = true
	cfg.Settings.TypingLayout = "auto"
	s.eng.ApplyRestored(cfg)
	now := time.Now()
	s.eng.KeyLog().Press(0x04, now)
	s.eng.KeyLog().Release(0x04, now.Add(40*time.Millisecond))
	var out struct{ Text string }
	_ = json.Unmarshal(do(h, "GET", "/api/history", good, s.token, nil).Body.Bytes(), &out)
	if strings.ToLower(out.Text) != "a" {
		t.Fatalf("text %q", out.Text)
	}
}
