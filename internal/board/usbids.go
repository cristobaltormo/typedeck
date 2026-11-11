package board

import (
	"io"
	"sort"
	"strings"
)

var boardIDs = map[string]bool{
	"2341:8036": true, "2341:0036": true, "2341:8037": true,
	"1B4F:9206": true, "1B4F:9204": true,
}

func parseUSBKey(name string) (vid, pid string, ok bool) {
	name = strings.ToUpper(name)
	if !strings.HasPrefix(name, "VID_") || len(name) < len("VID_0000&PID_0000") || name[8:13] != "&PID_" {
		return "", "", false
	}
	return name[4:8], name[13:17], true
}

func isBoardKey(name string) bool {
	vid, pid, ok := parseUSBKey(name)
	return ok && boardIDs[vid+":"+pid]
}

func boardPorts(usb map[string][]string, present []string) []string {
	live := map[string]bool{}
	for _, p := range present {
		live[strings.ToUpper(p)] = true
	}
	var out []string
	seen := map[string]bool{}
	for key, ports := range usb {
		if !isBoardKey(key) {
			continue
		}
		for _, p := range ports {
			if live[strings.ToUpper(p)] && !seen[strings.ToUpper(p)] {
				seen[strings.ToUpper(p)] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

func checkWritten(n, want int, err error) error {
	if err == nil && n < want {
		return io.ErrShortWrite
	}
	return err
}
