package capture

import (
	"strings"
	"testing"
)

func TestParseHelperLine(t *testing.T) {
	cases := []struct {
		line string
		want helperMsg
		ok   bool
	}{
		{"D 13 100108", helperMsg{Kind: "D", Code: 13, Flags: 0x100108}, true},
		{"U 13", helperMsg{Kind: "U", Code: 13}, true},
		{"P 0", helperMsg{Kind: "P", Code: 0}, true},
		{"R 126", helperMsg{Kind: "R", Code: 126}, true},
		{"READY iso", helperMsg{Kind: "READY", Text: "iso"}, true},
		{"KBTYPE ansi", helperMsg{Kind: "KBTYPE", Text: "ansi"}, true},
		{"TAPOFF timeout", helperMsg{Kind: "TAPOFF", Text: "timeout"}, true},
		{"TAPON", helperMsg{Kind: "TAPON"}, true},
		{"ERR permission accessibility=0 input_monitoring=1", helperMsg{Kind: "ERR", Text: "permission accessibility=0 input_monitoring=1"}, true},
		{"LAT 10 5 90", helperMsg{Kind: "LAT", Counts: [3]uint64{10, 5, 90}}, true},
		{"", helperMsg{}, false},
		{"D 13", helperMsg{}, false},
		{"D x 1", helperMsg{}, false},
		{"D 200 1", helperMsg{}, false},
		{"D 5 zz", helperMsg{}, false},
		{"LAT 1 2", helperMsg{}, false},
		{"NOPE 1", helperMsg{}, false},
	}
	for _, c := range cases {
		got, ok := parseHelperLine(c.line)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%q: got %+v %v, want %+v %v", c.line, got, ok, c.want, c.ok)
		}
	}
}

func TestConfigLine(t *testing.T) {
	var s Snapshot
	s.Mask[0x04>>3] |= 1 << (0x04 & 7)
	s.Mask[0x39>>3] |= 1 << (0x39 & 7)
	line := configLine(s, layoutISO)
	want := "CFG 01000000000000020000000000000000 0 1"
	if line != want {
		t.Fatalf("got %q want %q", line, want)
	}
	s.ReportAll, s.Inactive = true, true
	if got := configLine(s, layoutISO); !strings.HasSuffix(got, " 1 0") {
		t.Fatalf("flags: %q", got)
	}
}

func TestKeyAndTextLines(t *testing.T) {
	if got := keyLine(keyStep{true, 8, 0x100108}); got != "KEY d 8 100108" {
		t.Errorf("down: %q", got)
	}
	if got := keyLine(keyStep{false, 55, 0}); got != "KEY u 55 0" {
		t.Errorf("up: %q", got)
	}
	if got := textLine("ñ€"); got != "TEXT c3b1e282ac" {
		t.Errorf("text: %q", got)
	}
	if got := mediaLine(16); got != "MEDIA 16" {
		t.Errorf("media: %q", got)
	}
}

func TestParseCheck(t *testing.T) {
	st := parseCheck("accessibility 1\ninput_monitoring 0\ntap 0\nlayout iso\n")
	if st["accessibility"] != "1" || st["input_monitoring"] != "0" || st["tap"] != "0" || st["layout"] != "iso" {
		t.Fatalf("%v", st)
	}
}
