package actions

import (
	"runtime"
	"testing"
)

func TestSubstituteAndDescribe(t *testing.T) {
	if got := Substitute("sin llaves", false); got != "sin llaves" {
		t.Fatal(got)
	}
	if got := Substitute("{date}", false); len(got) != 10 {
		t.Fatalf("fecha: %q", got)
	}
	want := `'a'\''b'`
	if runtime.GOOS == "windows" {
		want = `"a'b"`
	}
	if got := shellQuote("a'b"); got != want {
		t.Fatal(got)
	}
}
