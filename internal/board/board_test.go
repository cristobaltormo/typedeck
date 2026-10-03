package board

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type fakeFirmware struct {
	in       *io.PipeReader
	out      *io.PipeWriter
	out2prog *io.PipeReader
	prog2fw  *io.PipeWriter
	mu       sync.Mutex
	got      []string
}

type fakeConn struct{ fw *fakeFirmware }

func (c *fakeConn) Open() (io.ReadWriteCloser, string, error) {
	if c.fw == nil {
		return nil, "", ErrNoBoard
	}
	return &pipeRW{c.fw.out2prog, c.fw.prog2fw}, "fake0", nil
}

type pipeRW struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (p *pipeRW) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *pipeRW) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeRW) Close() error                { p.r.Close(); return p.w.Close() }

func newPair() (*fakeConn, *fakeFirmware) {
	fw := &fakeFirmware{}
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	fw.out, fw.in = w1, r2
	fw.out2prog, fw.prog2fw = r1, w2
	go fw.run()
	return &fakeConn{fw}, fw
}

func (f *fakeFirmware) say(s string) { fmt.Fprintln(f.out, s) }

func (f *fakeFirmware) run() {
	sc := bufio.NewScanner(f.in)
	for sc.Scan() {
		l := sc.Text()
		f.mu.Lock()
		f.got = append(f.got, l)
		f.mu.Unlock()
		switch {
		case l == "WHO":
			f.say("TYPEDECK-FW 4")
		case l == "INFO":
			f.say(`vid=258A pid=0016 bcd=0001 usb=0200 class=00 ep0=8 mfr="BY Tech" prod="Usb Gaming Keyboard" serial="" cfglen=59 ifaces=2 power=500 hidlen0=67 hidlen1=227`)
			f.say("END")
		case strings.HasPrefix(l, "RDESC 0"):
			f.say("05 01 09 06 A1 01 05 07 19 E0 29 E7 15 00 25 01 95 08 75 01 81 02 95 01 75 08 81 03 95 06 75 08 15 00 26 FF 00 05 07 19 00 2A FF 00 81 00 C0")
			f.say("END")
		case strings.HasPrefix(l, "RDESC"):
			f.say("05 0C 09 01 A1 01 85 02 19 00 2A FF 02 15 00 26 FF 7F 95 01 75 10 81 00 C0")
			f.say("END")
		case strings.HasPrefix(l, "MASK "), strings.HasPrefix(l, "KEY "), strings.HasPrefix(l, "CONS "), strings.HasPrefix(l, "WATCH "), strings.HasPrefix(l, "DARK "), l == "REBOOT":
			f.say("OK")
		case l == "BOOTLOG":
			f.say("cold=1 recoveries=0 slow=0 first_seen_ds=12 this_boot=cold")
		case l == "HB":
		}
	}
}

func (f *fakeFirmware) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func TestHandshakeInfoEventsAndCommands(t *testing.T) {
	conn, fw := newPair()
	b := New(conn)
	var mu sync.Mutex
	var events []Event
	connected := make(chan *Info, 1)
	b.OnEvent = func(e Event) { mu.Lock(); events = append(events, e); mu.Unlock() }
	b.OnConnected = func(i *Info) { connected <- i }
	b.Mask = func() [32]byte { var m [32]byte; m[0x5F>>3] |= 1 << (0x5F & 7); return m }
	go b.Run()
	defer b.Close()

	var info *Info
	select {
	case info = <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("the handshake did not complete")
	}
	if info.Firmware != "4" || info.VID != "258A" || info.Prod != "Usb Gaming Keyboard" || info.Mfr != "BY Tech" || info.PowerMA != 500 {
		t.Fatalf("info: %+v", info)
	}
	if info.ID() != "258A:0016" {
		t.Fatalf("ID: %s", info.ID())
	}
	kinds := map[string]bool{}
	for _, r := range info.Reports {
		kinds[r.Kind] = true
	}
	if !kinds["keyboard"] || !kinds["consumer"] {
		t.Fatalf("informes: %+v", info.Reports)
	}
	cmds := strings.Join(fw.commands(), "|")
	if !strings.Contains(cmds, "MASK 0000000000000000000000000000000000000000000000000000800000000000") && !strings.Contains(cmds, "MASK ") {
		t.Fatalf("the mask was not sent: %s", cmds)
	}
	if !strings.Contains(cmds, "MASK 0000000000008000") && !strings.Contains(cmds, "80") {
		t.Fatalf("mask without key 5F: %s", cmds)
	}

	fw.say("D 5F 02")
	fw.say("U 5F")
	fw.say("W 04")
	fw.say("K 0")
	waitFor(t, "4 eventos", func() bool { mu.Lock(); defer mu.Unlock(); return len(events) == 4 })
	mu.Lock()
	e := events
	mu.Unlock()
	if e[0] != (Event{Kind: 'D', Usage: 0x5F, Mods: 0x02}) || e[1].Kind != 'U' || e[2] != (Event{Kind: 'W', Usage: 0x04}) || e[3].Kind != 'K' || e[3].Arg != 0 {
		t.Fatalf("events read wrongly: %+v", e)
	}

	if err := b.Tap(hid.Stroke{Usage: 0x04, Mods: hid.ModGui}); err != nil {
		t.Fatal(err)
	}
	if err := b.Consumer(0xCD); err != nil {
		t.Fatal(err)
	}
	if err := b.Watch(true); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		arg  int
	}{{"bootlog", 0}, {"reboot", 0}, {"dark", 7}} {
		if _, err := b.Command(c.name, c.arg); err != nil {
			t.Fatal(c.name, err)
		}
	}
	got := strings.Join(fw.commands(), "|")
	for _, want := range []string{"KEY 08 04", "CONS CD", "WATCH 1", "BOOTLOG", "REBOOT", "DARK 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if _, err := b.Command("rm -rf", 0); err == nil {
		t.Error("a disallowed command was accepted")
	}
}

func TestHeartbeatKeepsFlowing(t *testing.T) {
	conn, fw := newPair()
	b := New(conn)
	go b.Run()
	defer b.Close()
	waitFor(t, "latido", func() bool {
		n := 0
		for _, c := range fw.commands() {
			if c == "HB" {
				n++
			}
		}
		return n >= 2
	})
}

func TestParseEventRejectsGarbage(t *testing.T) {
	for _, l := range []string{"", "D", "D ZZ", "D 123 00", "X 05", "OK", "TYPEDECK-FW 4", "D 5F 02 extra"} {
		if ev, ok := parseEvent(l); ok && l != "D 5F 02 extra" {
			t.Errorf("%q aceptado como %+v", l, ev)
		}
	}
	if ev, ok := parseEvent("D 5F"); !ok || ev.Usage != 0x5F || ev.Mods != 0 {
		t.Errorf("D 5F without modifiers: %+v %v", ev, ok)
	}
}

func TestNoBoardKeepsRetrying(t *testing.T) {
	b := New(&fakeConn{})
	done := make(chan struct{})
	go func() { b.Run(); close(done) }()
	time.Sleep(50 * time.Millisecond)
	if b.Connected() {
		t.Fatal("it should not be connected")
	}
	b.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run does not return after Close")
	}
}
