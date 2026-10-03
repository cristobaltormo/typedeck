package keylog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newLog(t *testing.T) (*Log, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "keystrokes.jsonl")
	return New(p), p
}

func TestRecordsKeyAndDurationOnlyWhenEnabled(t *testing.T) {
	l, p := newLog(t)
	t0 := time.Now()
	l.Press(0x04, t0)
	l.Release(0x04, t0.Add(80*time.Millisecond))
	if got, n := l.Recent(10); n != 0 || len(got) != 0 {
		t.Fatalf("apagado no debe guardar nada: %v", got)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("apagado no debe crear el archivo")
	}

	l.SetEnabled(true)
	l.Press(0x04, t0)
	l.Press(0x04, t0.Add(30*time.Millisecond))
	l.Release(0x04, t0.Add(120*time.Millisecond))
	l.Press(0xE1, t0.Add(200*time.Millisecond))
	l.Release(0xE1, t0.Add(260*time.Millisecond))
	got, n := l.Recent(10)
	if n != 2 || got[0].K != "shift" || got[1].K != "A" || got[1].D != 120 || got[1].T != t0.UnixMilli() {
		t.Fatalf("registro mal: %+v", got)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
			t.Fatalf("permisos %v", st.Mode().Perm())
		}
	}
}

func TestClearDeletesEverythingAndTrimKeepsTheNewest(t *testing.T) {
	l, p := newLog(t)
	l.SetEnabled(true)
	now := time.Now()
	for i := 0; i < 5; i++ {
		l.Press(0x05, now)
		l.Release(0x05, now.Add(time.Millisecond))
	}
	if _, n := l.Recent(100); n != 5 {
		t.Fatalf("esperaba 5, hay %d", n)
	}
	if err := l.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("el archivo sigue ahí tras borrar todo")
	}
	if _, n := l.Recent(100); n != 0 {
		t.Fatal("quedan entradas")
	}

	line := strings.Repeat("x", 99) + "\n"
	_ = os.WriteFile(p, []byte(strings.Repeat(line, 100)+`{"t":1,"u":4,"k":"A","d":9}`+"\n"), 0o600)
	l.mu.Lock()
	l.trimLocked()
	l.mu.Unlock()
	data, _ := os.ReadFile(p)
	if len(data) >= 100*100+30 || !strings.HasSuffix(string(data), `"d":9}`+"\n") {
		t.Fatalf("el recorte no conservó lo último (%d bytes)", len(data))
	}
}

func TestNames(t *testing.T) {
	for u, want := range map[byte]string{0x04: "A", 0x1E: "1", 0x27: "0", 0x28: "return", 0xE3: "cmd", 0xFE: "0xFE"} {
		if got := Name(u); got != want {
			t.Errorf("%#x: %q, quería %q", u, got, want)
		}
	}
}
