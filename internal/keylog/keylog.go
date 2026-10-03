package keylog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const (
	maxBytes   = 4 << 20
	flushAfter = 1500 * time.Millisecond
	pruneEvery = time.Hour
)

type Entry struct {
	T int64  `json:"t"`
	U byte   `json:"u"`
	K string `json:"k"`
	D int    `json:"d"`
}

type Log struct {
	mu        sync.Mutex
	path      string
	on        bool
	keepFor   time.Duration
	lastPrune time.Time
	down      map[byte]time.Time
	buf       []Entry
	timer     *time.Timer
}

func New(path string) *Log { return &Log{path: path, down: map[byte]time.Time{}} }

func (l *Log) SetRetention(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.keepFor = d
	l.pruneLocked(time.Now())
}

func (l *Log) Prune(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(now)
}

func (l *Log) pruneLocked(now time.Time) {
	l.lastPrune = now
	if l.keepFor <= 0 {
		return
	}
	cutoff := now.Add(-l.keepFor).UnixMilli()
	all := l.readAllLocked()
	keep := all[:0:0]
	for _, e := range all {
		if e.T >= cutoff {
			keep = append(keep, e)
		}
	}
	if len(keep) == len(all) {
		return
	}
	if len(keep) == 0 {
		_ = os.Remove(l.path)
		return
	}
	var b []byte
	for _, e := range keep {
		line, _ := json.Marshal(e)
		b = append(append(b, line...), '\n')
	}
	_ = os.WriteFile(l.path, b, 0o600)
}

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
	if now := time.Now(); now.Sub(l.lastPrune) > pruneEvery {
		l.pruneLocked(now)
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

func (l *Log) readAllLocked() []Entry {
	l.flushLocked()
	f, err := os.Open(l.path)
	if err != nil {
		return nil
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
	return all
}

func (l *Log) Recent(limit int) ([]Entry, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	all := l.readAllLocked()
	return newest(all, limit), len(all)
}

func newest(all []Entry, limit int) []Entry {
	out := make([]Entry, 0, min(limit, len(all)))
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, all[i])
	}
	return out
}

type Report struct {
	Entries []Entry
	Summary Summary
	Text    string
}

func (l *Log) Report(limit, textEntries int, now time.Time, layout *hid.Layout) Report {
	l.mu.Lock()
	all := l.readAllLocked()
	l.mu.Unlock()
	tail := all
	if len(tail) > textEntries {
		tail = tail[len(tail)-textEntries:]
	}
	return Report{Entries: newest(all, limit), Summary: summarize(all, now), Text: Transcribe(tail, layout)}
}

type KeyCount struct {
	K string `json:"k"`
	U byte   `json:"u"`
	N int    `json:"n"`
}

type Summary struct {
	Total int        `json:"total"`
	Today int        `json:"today"`
	AvgMS int        `json:"avg_ms"`
	Top   []KeyCount `json:"top"`
}

func (l *Log) Summary(now time.Time) Summary {
	l.mu.Lock()
	all := l.readAllLocked()
	l.mu.Unlock()
	return summarize(all, now)
}

func summarize(all []Entry, now time.Time) Summary {
	var sum Summary
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
	counts := map[byte]*KeyCount{}
	var totalMS int64
	for _, e := range all {
		sum.Total++
		totalMS += int64(e.D)
		if e.T >= midnight {
			sum.Today++
		}
		if c := counts[e.U]; c != nil {
			c.N++
		} else {
			counts[e.U] = &KeyCount{K: e.K, U: e.U, N: 1}
		}
	}
	if sum.Total > 0 {
		sum.AvgMS = int(totalMS / int64(sum.Total))
	}
	for _, c := range counts {
		sum.Top = append(sum.Top, *c)
	}
	sort.Slice(sum.Top, func(i, j int) bool {
		if sum.Top[i].N != sum.Top[j].N {
			return sum.Top[i].N > sum.Top[j].N
		}
		return sum.Top[i].U < sum.Top[j].U
	})
	if len(sum.Top) > 5 {
		sum.Top = sum.Top[:5]
	}
	return sum
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
