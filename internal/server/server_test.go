package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
		{"sin token", good, "", 403},
		{"token malo", good, "x", 403},
		{"host ajeno con token bueno (DNS rebinding)", "evil.com", s.token, 403},
		{"host con puerto ajeno", "127.0.0.1:9999", s.token, 403},
		{"localhost vale", "localhost:7788", s.token, 200},
		{"correcto", good, s.token, 200},
	} {
		if got := do(h, "GET", "/api/config", c.host, c.token, nil).Code; got != c.want {
			t.Errorf("%s: %d, esperaba %d", c.name, got, c.want)
		}
	}
}

func TestWritesRejectedWithoutToken(t *testing.T) {
	_, h := newSrv(t)
	for _, p := range []string{"/api/config", "/api/test", "/api/hud", "/api/layer", "/api/board", "/api/backups/restore", "/api/learn"} {
		if got := do(h, "POST", p, good, "", map[string]any{}).Code; got != 403 {
			t.Errorf("POST %s sin token: %d", p, got)
		}
	}
}

func TestMethodsAreChecked(t *testing.T) {
	s, h := newSrv(t)
	if got := do(h, "POST", "/api/status", good, s.token, nil).Code; got != 405 && got != 404 {
		t.Errorf("POST a una ruta GET: %d", got)
	}
	if got := do(h, "GET", "/api/test", good, s.token, nil).Code; got != 405 && got != 404 {
		t.Errorf("GET a una ruta POST: %d", got)
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
		t.Fatal("no se guardó")
	}
	bad := map[string]any{"version": 3, "layers": []any{}}
	if got := do(h, "POST", "/api/config", good, s.token, bad).Code; got != 400 {
		t.Fatalf("configuración sin capas: %d", got)
	}
	evil := map[string]any{"version": 3, "layers": []any{map[string]any{"name": "x", "keys": map[string]any{"04": map[string]any{"tap": map[string]any{"type": "rm -rf"}}}}}}
	if got := do(h, "POST", "/api/config", good, s.token, evil).Code; got != 400 {
		t.Fatalf("tipo de acción inventado: %d", got)
	}
}

func TestStaticAssetsHeadersAndCaching(t *testing.T) {
	s, h := newSrv(t)
	idx := do(h, "GET", "/", good, "", nil)
	if idx.Code != 200 || !strings.Contains(idx.Body.String(), s.token) || strings.Contains(idx.Body.String(), "__TOKEN__") {
		t.Fatal("el índice debe llevar la clave de esta ejecución")
	}
	if idx.Header().Get("Content-Security-Policy") == "" || idx.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("faltan cabeceras de seguridad")
	}
	if got := do(h, "GET", "/", "evil.com", "", nil).Code; got != 403 {
		t.Fatalf("índice con host ajeno: %d", got)
	}
	req := httptest.NewRequest("GET", "/js/app.js", nil)
	req.Host = good
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("js sin gzip: %d %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !strings.Contains(string(plain), "eventsLoop") {
		t.Fatal("el js descomprimido no es el esperado")
	}
	etag := rec.Header().Get("ETag")
	req2 := httptest.NewRequest("GET", "/js/app.js", nil)
	req2.Host = good
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != 304 {
		t.Fatalf("ETag no respetado: %d", rec2.Code)
	}
	for _, p := range []string{"/../go.mod", "/js/../../go.mod", "/%2e%2e/go.mod"} {
		if got := do(h, "GET", p, good, "", nil).Code; got == 200 {
			t.Errorf("%s no debería servirse", p)
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
		t.Fatal("no debería haber eventos")
	}
}

func TestTestEndpointRunsSafeActionAndRejectsInvalid(t *testing.T) {
	s, h := newSrv(t)
	rec := do(h, "POST", "/api/test", good, s.token, map[string]any{"type": "wait", "ms": 5})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("wait: %d %s", rec.Code, rec.Body.String())
	}
	if got := do(h, "POST", "/api/test", good, s.token, map[string]any{"type": "nope"}).Code; got != 400 {
		t.Fatalf("tipo inválido: %d", got)
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
		t.Fatalf("galería: %d paquetes, %d categorías", len(pk.Packs), len(pk.Categories))
	}
	rec := do(h, "POST", "/api/packs/resolve", good, s.token, map[string]string{"id": "zoom", "layout": "tkl-ansi", "lang": "en"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"region":"fn"`) || !strings.Contains(rec.Body.String(), `"name":"Zoom"`) {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}
	if do(h, "POST", "/api/packs/resolve", good, s.token, map[string]string{"id": "nope"}).Code != 404 {
		t.Fatal("paquete inexistente debe dar 404")
	}

	if !strings.Contains(get("/api/setup").Body.String(), "no_board") {
		t.Fatal("sin placa debe avisar")
	}
	var compat map[string]any
	if err := json.Unmarshal(get("/api/compat").Body.Bytes(), &compat); err != nil || compat["groups"] == nil {
		t.Fatalf("compat: %v", err)
	}
	if do(h, "POST", "/api/keyboard/layout", good, s.token, map[string]string{"layout": "no-existe"}).Code != 400 {
		t.Fatal("disposición desconocida debe dar 400")
	}
	if do(h, "POST", "/api/keyboard/layout", good, s.token, map[string]string{"layout": "iso-full"}).Code != 200 {
		t.Fatal("el nombre antiguo se acepta")
	}
	var cfg config.Config
	_ = json.Unmarshal(get("/api/config").Body.Bytes(), &cfg)
	if cfg.Keyboards["default"].Layout != "full-iso" {
		t.Fatalf("no se guardó la disposición: %+v", cfg.Keyboards)
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
		t.Fatalf("resumen mal: %+v", out)
	}
	if out.Pack.Layers[0].Keys["05"].Tap.Steps[1].Ms != 60000 {
		t.Error("la espera enorme no se recortó")
	}
	bad := map[string]any{"pack": map[string]any{"name": "x", "layers": []any{}}}
	if got := do(h, "POST", "/api/macros/inspect", good, s.token, bad).Code; got != 400 {
		t.Errorf("un archivo ajeno se aceptó: %d", got)
	}
}

func TestHistoryEndpointsReadAndClear(t *testing.T) {
	s, h := newSrv(t)
	cfg := config.Default()
	cfg.Settings.KeyHistory = true
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
		t.Errorf("sin token se pudo leer el historial: %d", got)
	}
	if got := do(h, "POST", "/api/history/clear", good, s.token, nil).Code; got != 200 {
		t.Fatalf("borrar: %d", got)
	}
	rec = do(h, "GET", "/api/history", good, s.token, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Count != 0 {
		t.Errorf("tras borrar quedan %d", out.Count)
	}
}
