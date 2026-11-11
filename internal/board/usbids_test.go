package board

import (
	"io"
	"reflect"
	"testing"
)

func TestParseUSBKey(t *testing.T) {
	cases := map[string][2]string{
		"VID_2341&PID_8036":       {"2341", "8036"},
		"vid_1b4f&pid_9206&mi_00": {"1B4F", "9206"},
	}
	for in, want := range cases {
		vid, pid, ok := parseUSBKey(in)
		if !ok || vid != want[0] || pid != want[1] {
			t.Errorf("%s: %s %s %v", in, vid, pid, ok)
		}
	}
	for _, bad := range []string{"", "ROOT_HUB30", "VID_2341", "VID_2341-PID_8036"} {
		if _, _, ok := parseUSBKey(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}

func TestBoardPortsFiltersByVIDPID(t *testing.T) {
	usb := map[string][]string{
		"VID_2341&PID_8036&MI_00": {"COM7"},
		"VID_0403&PID_6001":       {"COM3"},
		"VID_1B4F&PID_9206":       {"COM9", "COM12"},
		"VID_2341&PID_8037":       {"COM5"},
	}
	got := boardPorts(usb, []string{"COM3", "COM7", "com9", "COM4"})
	if want := []string{"COM7", "COM9"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v, expected %v", got, want)
	}
	if got := boardPorts(usb, nil); len(got) != 0 {
		t.Fatalf("stale registry entries must not be used: %v", got)
	}
}

func TestCheckWritten(t *testing.T) {
	if err := checkWritten(3, 5, nil); err != io.ErrShortWrite {
		t.Fatalf("short write gave %v", err)
	}
	if err := checkWritten(5, 5, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkWritten(0, 5, io.EOF); err != io.EOF {
		t.Fatal("an existing error must be kept")
	}
}
