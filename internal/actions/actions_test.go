package actions

import "testing"

func TestSubstituteAndDescribe(t *testing.T) {
	if got := Substitute("sin llaves", false); got != "sin llaves" {
		t.Fatal(got)
	}
	if got := Substitute("{date}", false); len(got) != 10 {
		t.Fatalf("fecha: %q", got)
	}
	if shellQuote("a'b") != `'a'\''b'` {
		t.Fatal(shellQuote("a'b"))
	}
}
