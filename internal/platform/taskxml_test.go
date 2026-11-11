package platform

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

func TestTaskXMLHasNoLimits(t *testing.T) {
	x := taskXML(`PC\ana & co`, `C:\Program Files\Typedeck\typedeck.exe`)
	var doc struct {
		Settings struct {
			Limit     string `xml:"ExecutionTimeLimit"`
			Batt      string `xml:"DisallowStartIfOnBatteries"`
			StopBatt  string `xml:"StopIfGoingOnBatteries"`
			Instances string `xml:"MultipleInstancesPolicy"`
			Restart   struct {
				Count int `xml:"Count"`
			} `xml:"RestartOnFailure"`
		} `xml:"Settings"`
		Principal struct {
			User  string `xml:"UserId"`
			Level string `xml:"RunLevel"`
		} `xml:"Principals>Principal"`
		Trigger struct {
			User string `xml:"UserId"`
		} `xml:"Triggers>LogonTrigger"`
		Exec struct {
			Command string `xml:"Command"`
			Args    string `xml:"Arguments"`
		} `xml:"Actions>Exec"`
	}
	if err := xml.NewDecoder(strings.NewReader(strings.Replace(x, "UTF-16", "UTF-8", 1))).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	s := doc.Settings
	if s.Limit != "PT0S" || s.Batt != "false" || s.StopBatt != "false" || s.Instances != "IgnoreNew" || s.Restart.Count != 3 {
		t.Errorf("settings: %+v", s)
	}
	if doc.Principal.User != `PC\ana & co` || doc.Trigger.User != doc.Principal.User || doc.Principal.Level != "LeastPrivilege" {
		t.Errorf("principal: %+v %+v", doc.Principal, doc.Trigger)
	}
	if doc.Exec.Command != `C:\Program Files\Typedeck\typedeck.exe` || doc.Exec.Args != "--background" {
		t.Errorf("action: %+v", doc.Exec)
	}
}

func TestUTF16WithBOM(t *testing.T) {
	b := utf16WithBOM("a\u00e9")
	if !bytes.Equal(b, []byte{0xFF, 0xFE, 'a', 0, 0xE9, 0}) {
		t.Fatalf("% x", b)
	}
}
