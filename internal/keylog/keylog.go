package keylog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const (
	maxBytes   = 4 << 20
	flushAfter = 1500 * time.Millisecond
)

type Entry struct {
	T int64  `json:"t"`
	U byte   `json:"u"`
	K string `json:"k"`
	D int    `json:"d"`
}

type Log struct {
	mu    sync.Mutex
	path  string
	on    bool
	down  map[byte]time.Time
	buf   []Entry
	timer *time.Timer
}

func New(path string) *Log { return &Log{path: path, down: map[byte]time.Time{}} }

func (l *Log) Enabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.on
}

func (l *Log) SetEnabled(on bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.on && !on {
		l.flushLocked()
	}
	l.on = on
	l.down = map[byte]time.Time{}
}

func (l *Log) Press(u byte, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.on {
		return
	}
	if _, held := l.down[u]; !held {
		l.down[u] = at
	}
}

func (l *Log) Release(u byte, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	start, held := l.down[u]
	if !l.on || !held {
		return
	}
	delete(l.down, u)
	l.buf = append(l.buf, Entry{T: start.UnixMilli(), U: u, K: Name(u), D: int(at.Sub(start).Milliseconds())})
	if l.timer == nil {
		l.timer = time.AfterFunc(flushAfter, func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.flushLocked()
		})
	}
}

func (l *Log) flushLocked() {
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	if len(l.buf) == 0 {
		return
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		l.buf = nil
		return
	}
	w := bufio.NewWriter(f)
	for _, e := range l.buf {
		b, _ := json.Marshal(e)
		w.Write(b)
		w.WriteByte('\n')
	}
	l.buf = nil
	w.Flush()
	st, _ := f.Stat()
	f.Close()
	if st != nil && st.Size() > maxBytes {
		l.trimLocked()
	}
}

func (l *Log) trimLocked() {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	cut := len(data) / 2
	for cut < len(data) && data[cut] != '\n' {
		cut++
	}
	_ = os.WriteFile(l.path, data[min(cut+1, len(data)):], 0o600)
}

func (l *Log) Recent(limit int) ([]Entry, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flushLocked()
	f, err := os.Open(l.path)
	if err != nil {
		return []Entry{}, 0
	}
	defer f.Close()
	var all []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			all = append(all, e)
		}
	}
	total := len(all)
	out := make([]Entry, 0, min(limit, total))
	for i := total - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, all[i])
	}
	return out, total
}

func (l *Log) Clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = nil
	l.down = map[byte]time.Time{}
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

var modNames = [8]string{"ctrl", "shift", "alt", "cmd", "ctrl", "shift", "alt", "cmd"}

func Name(u byte) string {
	switch {
	case u >= 0x04 && u <= 0x1D:
		return string(rune('A' + u - 0x04))
	case u >= 0x1E && u <= 0x26:
		return string(rune('1' + u - 0x1E))
	case u == 0x27:
		return "0"
	case u >= 0xE0 && u <= 0xE7:
		return modNames[u-0xE0]
	}
	if n := hid.UsageName(u); n != "" {
		return n
	}
	return fmt.Sprintf("0x%02X", u)
}
