package keylog

import (
	"github.com/cristobaltormo/typedeck/internal/hid"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
		t.Fatalf("off must store nothing: %v", got)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("off must not create the file")
	}

	l.SetEnabled(true)
	l.Press(0x04, t0)
	l.Press(0x04, t0.Add(30*time.Millisecond))
	l.Release(0x04, t0.Add(120*time.Millisecond))
	l.Press(0xE1, t0.Add(200*time.Millisecond))
	l.Release(0xE1, t0.Add(260*time.Millisecond))
	got, n := l.Recent(10)
	if n != 2 || got[0].K != "shift" || got[1].K != "A" || got[1].D != 120 || got[1].T != t0.UnixMilli() {
		t.Fatalf("wrong log: %+v", got)
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
		t.Fatalf("expected 5, there are %d", n)
	}
	if err := l.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("the file is still there after deleting everything")
	}
	if _, n := l.Recent(100); n != 0 {
		t.Fatal("entries are left")
	}

	line := strings.Repeat("x", 99) + "\n"
	_ = os.WriteFile(p, []byte(strings.Repeat(line, 100)+`{"t":1,"u":4,"k":"A","d":9}`+"\n"), 0o600)
	l.mu.Lock()
	l.trimLocked()
	l.mu.Unlock()
	data, _ := os.ReadFile(p)
	if len(data) >= 100*100+30 || !strings.HasSuffix(string(data), `"d":9}`+"\n") {
		t.Fatalf("the trim did not keep the newest (%d bytes)", len(data))
	}
}

func TestNames(t *testing.T) {
	for u, want := range map[byte]string{0x04: "A", 0x1E: "1", 0x27: "0", 0x28: "return", 0xE3: "cmd", 0xFE: "0xFE"} {
		if got := Name(u); got != want {
			t.Errorf("%#x: %q, wanted %q", u, got, want)
		}
	}
}

func TestSummaryCountsTodayTopKeysAndAverage(t *testing.T) {
	l, _ := newLog(t)
	l.SetEnabled(true)
	now := time.Date(2026, 10, 3, 15, 0, 0, 0, time.Local)
	press := func(u byte, at time.Time, ms int) {
		l.Press(u, at)
		l.Release(u, at.Add(time.Duration(ms)*time.Millisecond))
	}
	press(0x04, now.Add(-48*time.Hour), 100)
	press(0x04, now.Add(-time.Hour), 80)
	press(0x04, now.Add(-30*time.Minute), 120)
	press(0x05, now.Add(-10*time.Minute), 60)
	sum := l.Summary(now)
	if sum.Total != 4 || sum.Today != 3 || sum.AvgMS != 90 {
		t.Fatalf("resumen: %+v", sum)
	}
	if len(sum.Top) != 2 || sum.Top[0].K != "A" || sum.Top[0].N != 3 {
		t.Fatalf("most used keys: %+v", sum.Top)
	}
	if empty := New(filepath.Join(t.TempDir(), "x")).Summary(now); empty.Total != 0 || empty.Top != nil {
		t.Fatalf("without a file the summary must be empty: %+v", empty)
	}
}

func TestTranscribeRebuildsWhatWasTyped(t *testing.T) {
	us := hid.LayoutByName("us")
	at := int64(1_000_000)
	var es1 []Entry
	add := func(u byte, dt int64, d int) { es1 = append(es1, Entry{T: at + dt, U: u, K: Name(u), D: d}) }

	add(0xE1, 0, 400)
	add(0x0B, 100, 50)
	add(0x08, 600, 50)
	add(0x0F, 700, 50)
	add(0x0F, 800, 50)
	add(0x12, 900, 50)
	add(0x2C, 1000, 50)
	add(0x1A, 1100, 50)
	add(0x12, 1200, 50)
	add(0x15, 1300, 50)
	add(0x2A, 1400, 50)
	add(0x0F, 1500, 50)
	add(0x28, 1600, 50)
	if got := Transcribe(es1, us); got != "Hello wol\n" {
		t.Fatalf("texto: %q", got)
	}

	var caps []Entry
	caps = append(caps, Entry{T: at, U: 0x39, D: 30}, Entry{T: at + 100, U: 0x04, D: 40}, Entry{T: at + 200, U: 0x39, D: 30}, Entry{T: at + 300, U: 0x04, D: 40})
	if got := Transcribe(caps, us); got != "Aa" {
		t.Fatalf("caps lock: %q", got)
	}

	combo := []Entry{{T: at, U: 0xE3, D: 300}, {T: at + 50, U: 0x06, D: 40}, {T: at + 500, U: 0x06, D: 40}}
	if got := Transcribe(combo, us); got != "[Cmd+C]c" {
		t.Fatalf("atajo: %q", got)
	}

	altgr := []Entry{{T: at, U: 0xE6, D: 300}, {T: at + 50, U: 0x1F, D: 40}}
	if got := Transcribe(altgr, hid.LayoutByName("es-pc")); got != "@" {
		t.Fatalf("AltGr: %q", got)
	}
}

func TestPruneDropsEntriesOlderThanTheRetention(t *testing.T) {
	l, p := newLog(t)
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)
	line := func(at time.Time, k string) string {
		return `{"t":` + strconv.FormatInt(at.UnixMilli(), 10) + `,"u":4,"k":"` + k + `","d":50}` + "\n"
	}
	_ = os.WriteFile(p, []byte(line(old, "OLD")+line(now.Add(-time.Hour), "NEW")), 0o600)
	l.SetRetention(7 * 24 * time.Hour)
	got, n := l.Recent(10)
	if n != 1 || got[0].K != "NEW" {
		t.Fatalf("after pruning: %+v", got)
	}
	l.SetRetention(time.Minute)
	if _, n := l.Recent(10); n != 0 {
		t.Fatalf("everything is older than a minute, %d remain", n)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("the file must be removed when nothing is left")
	}
}
