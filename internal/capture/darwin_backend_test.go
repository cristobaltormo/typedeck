package capture

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const fakeHelper = `#!/bin/sh
if [ "$1" = "--check" ]; then
  printf 'accessibility 1\ninput_monitoring 0\ntap 0\nlayout iso\n'
  exit 0
fi
echo "$@" > "$LOG.args"
case "$MODE" in
perm) echo "ERR permission accessibility=1 input_monitoring=0"; exit 3;;
silent) exit 1;;
crash) echo "READY iso"; sleep 0.3; echo "D 13 108"; sleep 0.2; exit 1;;
esac
echo "READY iso"
if [ -n "$EVENTS" ]; then (sleep 0.3; for e in $EVENTS; do echo "$e" | tr '_' ' '; done) & fi
while read -r line; do
  echo "$line" >> "$LOG"
  [ "$line" = "QUIT" ] && exit 0
done
echo eof >> "$LOG"
`

type fakeMac struct {
	path string
	log  string
}

func newFakeMac(t *testing.T, mode string) *fakeMac {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake helper is a shell script")
	}
	dir := t.TempDir()
	f := &fakeMac{path: filepath.Join(dir, HelperName), log: filepath.Join(dir, "log")}
	if err := os.WriteFile(f.path, []byte(fakeHelper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOG", f.log)
	t.Setenv("MODE", mode)
	return f
}

func (f *fakeMac) received() []string {
	b, _ := os.ReadFile(f.log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func (f *fakeMac) backend() Backend { return NewMacBackend(func() string { return f.path }) }

func TestMacBackendStartsAndConfigures(t *testing.T) {
	f := newFakeMac(t, "")
	be := f.backend()
	flt := NewFilter()
	if err := be.Start(flt); err != nil {
		t.Fatal(err)
	}
	defer be.Stop()
	if !be.Alive() {
		t.Fatal("not alive after Start")
	}
	var m [32]byte
	m[0x04>>3] |= 1 << (0x04 & 7)
	flt.SetMask(m)
	waitFor(t, "mask line", func() bool {
		for _, l := range f.received() {
			if strings.HasPrefix(l, "CFG 01000000") {
				return true
			}
		}
		return false
	})
	args, _ := os.ReadFile(f.log + ".args")
	if strings.TrimSpace(string(args)) != "run --prompt" {
		t.Fatalf("first start must ask for permission, args %q", args)
	}
}

func TestMacBackendAsksForPermissionOnce(t *testing.T) {
	f := newFakeMac(t, "")
	be := f.backend()
	flt := NewFilter()
	if err := be.Start(flt); err != nil {
		t.Fatal(err)
	}
	be.Stop()
	if err := be.Start(flt); err != nil {
		t.Fatal(err)
	}
	defer be.Stop()
	args, _ := os.ReadFile(f.log + ".args")
	if strings.TrimSpace(string(args)) != "run" {
		t.Fatalf("second start must not prompt again, args %q", args)
	}
}

func TestMacBackendRelaysKeysThroughTheFilter(t *testing.T) {
	f := newFakeMac(t, "")
	t.Setenv("EVENTS", "D_13_108 U_13 P_0 R_0")
	be := f.backend()
	flt := NewFilter()
	if err := be.Start(flt); err != nil {
		t.Fatal(err)
	}
	defer be.Stop()
	var got []Event
	deadline := time.After(2 * time.Second)
	for len(got) < 3 {
		select {
		case ev := <-flt.Events():
			got = append(got, ev)
		case <-deadline:
			t.Fatalf("events so far: %+v", got)
		}
	}
	want := []Event{{Kind: 'D', Usage: 0x1A, Mods: hid.ModGui}, {Kind: 'U', Usage: 0x1A}, {Kind: 'P', Usage: 0x04}}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("event %d: got %+v want %+v", i, got[i], w)
		}
	}
}

func TestMacBackendReleasesHeldKeysWhenHelperDies(t *testing.T) {
	f := newFakeMac(t, "crash")
	be := f.backend()
	flt := NewFilter()
	if err := be.Start(flt); err != nil {
		t.Fatal(err)
	}
	var kinds []byte
	deadline := time.After(3 * time.Second)
	for len(kinds) < 2 {
		select {
		case ev := <-flt.Events():
			kinds = append(kinds, ev.Kind)
		case <-deadline:
			t.Fatalf("got %q", kinds)
		}
	}
	if string(kinds) != "DU" {
		t.Fatalf("a key held when the helper died must be released, got %q", kinds)
	}
	waitFor(t, "death noticed", func() bool { return !be.Alive() })
}

func TestMacBackendStopSendsQuit(t *testing.T) {
	f := newFakeMac(t, "")
	be := f.backend()
	if err := be.Start(NewFilter()); err != nil {
		t.Fatal(err)
	}
	be.Stop()
	if be.Alive() {
		t.Fatal("alive after Stop")
	}
	var quit bool
	for _, l := range f.received() {
		quit = quit || l == "QUIT"
	}
	if !quit {
		t.Fatalf("the helper was not asked to quit: %v", f.received())
	}
}

func TestMacBackendPermissionError(t *testing.T) {
	f := newFakeMac(t, "perm")
	err := f.backend().Start(NewFilter())
	var coded interface{ Code() string }
	if !errors.As(err, &coded) || coded.Code() != "permission" {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "Input Monitoring") || strings.Contains(err.Error(), "Accessibility") {
		t.Fatalf("the message must name only what is missing: %v", err)
	}
}

func TestMacBackendMissingHelper(t *testing.T) {
	be := NewMacBackend(func() string { return filepath.Join(t.TempDir(), HelperName) })
	err := be.Start(NewFilter())
	if !errors.Is(err, ErrHelperMissing) {
		t.Fatalf("got %v", err)
	}
	checks := be.Checks()
	if len(checks) != 1 || checks[0].OK || !strings.Contains(checks[0].Problem, "next to the typedeck binary") {
		t.Fatalf("%+v", checks)
	}
}

func TestMacBackendHelperThatSaysNothing(t *testing.T) {
	f := newFakeMac(t, "silent")
	if err := f.backend().Start(NewFilter()); err == nil {
		t.Fatal("a helper that exits without a word must fail the start")
	}
}

func TestMacBackendInjection(t *testing.T) {
	f := newFakeMac(t, "")
	be := f.backend()
	if err := be.Start(NewFilter()); err != nil {
		t.Fatal(err)
	}
	if err := be.Inject(hid.Stroke{Usage: 0x06, Mods: hid.ModGui}, 1); err != nil {
		t.Fatal(err)
	}
	if err := be.Inject(hid.Stroke{Usage: 0x39}, 0); err != nil {
		t.Fatal(err)
	}
	if err := be.Consumer(0xE9); err != nil {
		t.Fatal(err)
	}
	if err := be.TypeText("a\nñ"); err != nil {
		t.Fatal(err)
	}
	if err := be.Consumer(0x999); err == nil {
		t.Fatal("unknown media key accepted")
	}
	be.Stop()
	got := strings.Join(f.received()[1:], "|")
	for _, want := range []string{"KEY d 55 100008|KEY d 8 100008|KEY u 8 100008|KEY u 55 0", "CAPS", "MEDIA 0", "TEXT 61|KEY d 36 0|KEY u 36 0|TEXT c3b1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestMacBackendInjectWithoutHelper(t *testing.T) {
	f := newFakeMac(t, "")
	if err := f.backend().Inject(hid.Stroke{Usage: 0x04}, 0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}

func TestMacBackendChecks(t *testing.T) {
	f := newFakeMac(t, "")
	checks := f.backend().Checks()
	var bad []string
	for _, c := range checks {
		if !c.OK {
			bad = append(bad, c.Problem)
		}
	}
	joined := strings.Join(bad, "\n")
	if !strings.Contains(joined, "Input Monitoring is not allowed") || strings.Contains(joined, "Accessibility is not allowed") {
		t.Fatalf("problems: %q", joined)
	}
	if !strings.Contains(joined, "System Settings > Privacy & Security") || !strings.Contains(joined, f.path) {
		t.Fatalf("the check must say where to grant it: %q", joined)
	}
}

func TestMacBackendChecksWhenEverythingWorks(t *testing.T) {
	f := newFakeMac(t, "")
	script := strings.Replace(fakeHelper, "tap 0", "tap 1", 1)
	if err := os.WriteFile(f.path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.backend().Checks() {
		if !c.OK && !strings.HasPrefix(c.Problem, "optional: ") {
			t.Errorf("unexpected problem: %s", c.Problem)
		}
	}
}

func TestSoftwareSourceKeepsPermissionCode(t *testing.T) {
	f := newFakeMac(t, "perm")
	s := NewSoftwareWith(f.backend())
	defer s.Close()
	if err := s.Enable(); err == nil {
		t.Fatal("expected an error")
	}
	if s.Error() != "permission" {
		t.Fatalf("got %q", s.Error())
	}
}
