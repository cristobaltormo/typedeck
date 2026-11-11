package platform

import "testing"

func TestQuoteCmdArg(t *testing.T) {
	cases := map[string]string{
		"hello":            `^"hello^"`,
		"a b":              `^"a b^"`,
		"a&b|c":            `^"a^&b^|c^"`,
		`say "hi"`:         `^"say \^"hi\^"^"`,
		`dir\`:             `^"dir\\^"`,
		"100%":             `^"100%^^"`,
		"line1\nline2":     `^"line1 line2^"`,
		"caf\u00e9 \u00f1": "^\"caf\u00e9 \u00f1^\"",
		"(x) <y> ^z! 'q'":  `^"^(x^) ^<y^> ^^z^! 'q'^"`,
	}
	for in, want := range cases {
		if got := quoteCmdArg(in); got != want {
			t.Errorf("%q: got %s, expected %s", in, got, want)
		}
	}
}

func TestQuoteCmdArgKeepsEveryCharacter(t *testing.T) {
	in := `a&b|c<d>e^f"g%h`
	got := quoteCmdArg(in)
	for _, r := range "abcdefgh" {
		found := false
		for _, g := range got {
			found = found || g == r
		}
		if !found {
			t.Errorf("%c was lost in %s", r, got)
		}
	}
}
