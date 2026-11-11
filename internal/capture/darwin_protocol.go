package capture

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

type helperMsg struct {
	Kind   string
	Code   int
	Flags  uint64
	Text   string
	Counts [3]uint64
}

type noKeyError struct{ usage byte }

func (e *noKeyError) Error() string { return fmt.Sprintf("key %02X has no macOS equivalent", e.usage) }

func parseHelperLine(line string) (helperMsg, bool) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return helperMsg{}, false
	}
	m := helperMsg{Kind: f[0]}
	switch m.Kind {
	case "D", "U", "P", "R":
		return parseKeyMsg(m, f)
	case "READY", "KBTYPE", "TAPOFF":
		if len(f) > 1 {
			m.Text = f[1]
		}
		return m, true
	case "TAPON":
		return m, true
	case "ERR":
		m.Text = strings.TrimSpace(strings.TrimPrefix(line, "ERR"))
		return m, true
	case "LAT":
		return parseLatency(m, f)
	}
	return helperMsg{}, false
}

func parseKeyMsg(m helperMsg, f []string) (helperMsg, bool) {
	if len(f) < 2 {
		return helperMsg{}, false
	}
	code, err := strconv.Atoi(f[1])
	if err != nil || code < 0 || code > 127 {
		return helperMsg{}, false
	}
	m.Code = code
	if m.Kind == "D" {
		if len(f) < 3 {
			return helperMsg{}, false
		}
		flags, err := strconv.ParseUint(f[2], 16, 64)
		if err != nil {
			return helperMsg{}, false
		}
		m.Flags = flags
	}
	return m, true
}

func parseLatency(m helperMsg, f []string) (helperMsg, bool) {
	if len(f) != 4 {
		return helperMsg{}, false
	}
	for i := range m.Counts {
		n, err := strconv.ParseUint(f[i+1], 10, 64)
		if err != nil {
			return helperMsg{}, false
		}
		m.Counts[i] = n
	}
	return m, true
}

func configLine(s Snapshot, layout string) string {
	mask := keycodeMask(s, layout)
	return fmt.Sprintf("CFG %s %d %d", hex.EncodeToString(mask[:]), b2i(s.ReportAll), b2i(!s.Inactive))
}

func keyLine(st keyStep) string {
	dir := "u"
	if st.down {
		dir = "d"
	}
	return fmt.Sprintf("KEY %s %d %x", dir, st.code, st.flags)
}

func textLine(text string) string { return "TEXT " + hex.EncodeToString([]byte(text)) }

func mediaLine(keyType int) string { return fmt.Sprintf("MEDIA %d", keyType) }
