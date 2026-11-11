package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuoteArgThroughCmd(t *testing.T) {
	in := `a&b|c<d>e^f%PATH%g!h (i) j`
	out, err := Run(5e9, "", "cmd", "/C", "echo "+quoteCmdArg(in))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"` + in + `"`; out != want {
		t.Fatalf("cmd printed %q, expected %q", out, want)
	}
}

func TestDecodeOutputOEM(t *testing.T) {
	if got := decodeOutput([]byte("plain ascii")); got != "plain ascii" {
		t.Fatalf("%q", got)
	}
	if got := decodeOutput([]byte("caf\u00e9")); got != "caf\u00e9" {
		t.Fatalf("valid UTF-8 must pass through, got %q", got)
	}
	if got := decodeOutput([]byte{0x63, 0x61, 0x66, 0x82}); got == "caf\x82" || len(got) < 4 {
		t.Fatalf("an OEM byte must be decoded, got %q", got)
	}
}

func TestCmdOutputAccents(t *testing.T) {
	out, err := Run(5e9, "", "cmd", "/C", "echo caf\u00e9")
	if err != nil {
		t.Fatal(err)
	}
	if out != "caf\u00e9" && out != "caf?" {
		t.Fatalf("accent lost: %q", out)
	}
}

func TestSingleInstance(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if !AcquireInstance() {
		t.Fatal("the first copy must get the lock")
	}
	if AcquireInstance() {
		t.Fatal("a second copy must be refused")
	}
	releaseInstance()
	if !AcquireInstance() {
		t.Fatal("the lock must be free after release")
	}
	releaseInstance()
}

func TestPrivateDirAndMigration(t *testing.T) {
	roaming, local := t.TempDir(), t.TempDir()
	t.Setenv("APPDATA", roaming)
	t.Setenv("LOCALAPPDATA", local)
	cfg := Current.ConfigDir()
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "keystrokes.jsonl"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	MigratePrivate(cfg, "keystrokes.jsonl")
	moved := filepath.Join(local, "typedeck", "keystrokes.jsonl")
	if b, err := os.ReadFile(moved); err != nil || string(b) != "x" {
		t.Fatalf("the history must move to the local folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "keystrokes.jsonl")); err == nil {
		t.Fatal("the roaming copy must be gone")
	}
	if got := PrivateDir(t.TempDir()); got == filepath.Join(local, "typedeck") {
		t.Fatal("a custom config folder must stay where it is")
	}
}

func TestShellParsesStoreApps(t *testing.T) {
	apps := parseStoreApps("Calculator|Microsoft.WindowsCalculator_8wekyb3d8bbwe!App\nNotepad++|C:\\Program Files\\n++\\n.exe\nUninstall X|a.b!c\n\n")
	if len(apps) != 1 || apps[0].Name != "Calculator" || apps[0].AUMID != "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App" {
		t.Fatalf("%+v", apps)
	}
}
