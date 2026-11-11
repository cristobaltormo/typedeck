//go:build darwin

package capture

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func realHelper(t *testing.T) string {
	t.Helper()
	p := os.Getenv("TYPEDECK_CAPTURE_HELPER")
	if p == "" {
		t.Skip("set TYPEDECK_CAPTURE_HELPER to the compiled typedeck-capture to run these")
	}
	return p
}

func TestRealHelperCheckReportsEveryField(t *testing.T) {
	st, err := runHelperCheck(realHelper(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"accessibility", "input_monitoring", "tap", "layout"} {
		if st[k] == "" {
			t.Errorf("missing %q in %v", k, st)
		}
	}
	switch st["layout"] {
	case layoutANSI, layoutISO, layoutJIS:
	default:
		t.Errorf("unexpected layout %q", st["layout"])
	}
}

func TestRealHelperSaysWhatIsMissingOrIsReady(t *testing.T) {
	cmd := exec.Command(realHelper(t), "run")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "ERR permission accessibility="):
	case strings.HasPrefix(line, "READY "):
		in.Close()
	default:
		t.Fatalf("unexpected first line %q", line)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("the helper did not exit: with no permission it must stop, with a closed stdin it must stop")
	}
}

func TestRealHelperRejectsUnknownMode(t *testing.T) {
	if err := exec.Command(realHelper(t), "nope").Run(); err == nil {
		t.Fatal("an unknown mode must fail")
	}
}

func TestRealHelperThroughTheBackend(t *testing.T) {
	path := realHelper(t)
	be := NewMacBackend(func() string { return path }).(*macBackend)
	be.prompted = true
	err := be.Start(NewFilter())
	if err == nil {
		be.Stop()
		if be.Alive() {
			t.Fatal("alive after Stop")
		}
		return
	}
	var perm *permissionError
	if !errors.As(err, &perm) {
		t.Fatalf("without permission the error must say so, got %v", err)
	}
	if !strings.Contains(err.Error(), "System Settings") {
		t.Fatalf("the message must say where to grant it: %v", err)
	}
	if be.Alive() {
		t.Fatal("alive after a failed start")
	}
}
