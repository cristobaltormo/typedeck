package board

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type Event struct {
	Kind  byte
	Usage byte
	Mods  byte
	Arg   byte
}

type Info struct {
	Firmware  string       `json:"firmware"`
	Present   bool         `json:"present"`
	VID       string       `json:"vid,omitempty"`
	PID       string       `json:"pid,omitempty"`
	BCD       string       `json:"bcd,omitempty"`
	USB       string       `json:"usb,omitempty"`
	Class     string       `json:"class,omitempty"`
	EP0       int          `json:"ep0,omitempty"`
	Mfr       string       `json:"mfr,omitempty"`
	Prod      string       `json:"prod,omitempty"`
	Serial    string       `json:"serial,omitempty"`
	CfgLen    int          `json:"cfglen,omitempty"`
	Ifaces    int          `json:"ifaces,omitempty"`
	PowerMA   int          `json:"power_ma,omitempty"`
	HIDLen    []int        `json:"hid_len,omitempty"`
	Reports   []hid.Report `json:"reports,omitempty"`
	RawRDesc  []string     `json:"-"`
	Connected time.Time    `json:"-"`
}

func (i Info) ID() string {
	if i.VID == "" {
		return ""
	}
	return strings.ToUpper(i.VID + ":" + i.PID)
}

type Connector interface {
	Open() (io.ReadWriteCloser, string, error)
}

var (
	ErrNoBoard          = errors.New("no se encuentra la placa")
	ErrOutdatedFirmware = errors.New("la placa tiene un firmware antiguo")
)

type Board struct {
	conn Connector

	OnEvent      func(Event)
	OnConnected  func(*Info)
	OnDisconnect func()
	Mask         func() [32]byte
	Log          func(format string, args ...any)

	mu      sync.Mutex
	rw      io.ReadWriteCloser
	port    string
	info    *Info
	since   time.Time
	wmu     sync.Mutex
	cmu     sync.Mutex
	replies chan string
	lastMsk string
	stop    chan struct{}
	lastErr string
}

func New(c Connector) *Board {
	return &Board{conn: c, replies: make(chan string, 64), stop: make(chan struct{}), Log: func(string, ...any) {}}
}

func (b *Board) Status() (connected bool, port string, info *Info, since time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.info != nil {
		cp := *b.info
		info = &cp
	}
	return b.rw != nil, b.port, info, b.since
}

func (b *Board) Close() {
	_ = b.write("MASK " + strings.Repeat("00", 32))
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
	b.mu.Lock()
	if b.rw != nil {
		_ = b.rw.Close()
	}
	b.mu.Unlock()
}

func (b *Board) Run() {
	for {
		select {
		case <-b.stop:
			return
		default:
		}
		rw, port, err := b.conn.Open()
		if err != nil {
			b.mu.Lock()
			b.lastErr = "not_found"
			if errors.Is(err, ErrOutdatedFirmware) {
				b.lastErr = "outdated_firmware"
				b.port = port
			}
			b.mu.Unlock()
			select {
			case <-b.stop:
				return
			case <-time.After(4 * time.Second):
			}
			continue
		}
		b.serve(rw, port)
		select {
		case <-b.stop:
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (b *Board) serve(rw io.ReadWriteCloser, port string) {
	b.mu.Lock()
	b.rw, b.port, b.since, b.info, b.lastMsk, b.lastErr = rw, port, time.Now(), nil, "", ""
	b.mu.Unlock()
	b.Log("conectado a %s", port)
	done := make(chan struct{})
	go b.readLoop(rw, done)
	go b.handshake()
	hb := time.NewTicker(time.Second)
	defer hb.Stop()
loop:
	for {
		select {
		case <-done:
			break loop
		case <-b.stop:
			break loop
		case <-hb.C:
			if err := b.write("HB"); err != nil {
				break loop
			}
		}
	}
	_ = rw.Close()
	b.mu.Lock()
	b.rw, b.port = nil, ""
	b.mu.Unlock()
	b.Log("desconectada")
	if b.OnDisconnect != nil {
		b.OnDisconnect()
	}
}

func (b *Board) write(line string) error {
	b.mu.Lock()
	rw := b.rw
	b.mu.Unlock()
	if rw == nil {
		return ErrNoBoard
	}
	b.wmu.Lock()
	defer b.wmu.Unlock()
	_, err := io.WriteString(rw, line+"\n")
	return err
}

func (b *Board) readLoop(rw io.Reader, done chan struct{}) {
	defer close(done)
	sc := bufio.NewScanner(rw)
	sc.Buffer(make([]byte, 4096), 64*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if ev, ok := parseEvent(line); ok {
			if ev.Kind == 'K' && ev.Arg == 1 {
				go b.refreshInfo()
			}
			if b.OnEvent != nil {
				b.OnEvent(ev)
			}
			continue
		}
		select {
		case b.replies <- line:
		default:
			select {
			case <-b.replies:
			default:
			}
			b.replies <- line
		}
	}
}

func parseEvent(line string) (Event, bool) {
	f := strings.Fields(line)
	if len(f) < 2 || len(f[0]) != 1 {
		return Event{}, false
	}
	switch f[0][0] {
	case 'D', 'U', 'W', 'P', 'R':
		u, err := strconv.ParseUint(f[1], 16, 8)
		if err != nil || len(f[1]) > 2 {
			return Event{}, false
		}
		ev := Event{Kind: f[0][0], Usage: byte(u)}
		if len(f) > 2 {
			if m, err := strconv.ParseUint(f[2], 16, 8); err == nil {
				ev.Mods = byte(m)
			}
		}
		return ev, true
	case 'K':
		if f[1] == "1" || f[1] == "0" {
			return Event{Kind: 'K', Arg: f[1][0] - '0'}, true
		}
	}
	return Event{}, false
}

func (b *Board) Do(cmd string, multi bool, timeout time.Duration) ([]string, error) {
	b.cmu.Lock()
	defer b.cmu.Unlock()
	for {
		select {
		case <-b.replies:
			continue
		default:
		}
		break
	}
	if err := b.write(cmd); err != nil {
		return nil, err
	}
	var out []string
	deadline := time.After(timeout)
	for {
		select {
		case l := <-b.replies:
			if !multi {
				return []string{l}, nil
			}
			if l == "END" {
				return out, nil
			}
			out = append(out, l)
		case <-deadline:
			if multi && len(out) > 0 {
				return out, errors.New("tiempo agotado esperando END")
			}
			return out, fmt.Errorf("la placa no respondió a %q", strings.Fields(cmd)[0])
		case <-b.stop:
			return out, ErrNoBoard
		}
	}
}

var kvRe = regexp.MustCompile(`(\w+)=("[^"]*"|\S+)`)

func parseInfo(lines []string) Info {
	in := Info{Present: true}
	if len(lines) == 0 {
		return Info{}
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "NODEVICE") || strings.HasPrefix(l, "ERR") {
			return Info{}
		}
		for _, m := range kvRe.FindAllStringSubmatch(l, -1) {
			k, v := m[1], strings.Trim(m[2], `"`)
			n, _ := strconv.Atoi(v)
			switch {
			case k == "vid":
				in.VID = strings.ToUpper(v)
			case k == "pid":
				in.PID = strings.ToUpper(v)
			case k == "bcd":
				in.BCD = v
			case k == "usb":
				in.USB = v
			case k == "class":
				in.Class = v
			case k == "ep0":
				in.EP0 = n
			case k == "mfr":
				in.Mfr = v
			case k == "prod":
				in.Prod = v
			case k == "serial":
				in.Serial = v
			case k == "cfglen":
				in.CfgLen = n
			case k == "ifaces":
				in.Ifaces = n
			case k == "power":
				in.PowerMA = n
			case strings.HasPrefix(k, "hidlen"):
				idx, _ := strconv.Atoi(strings.TrimPrefix(k, "hidlen"))
				for len(in.HIDLen) <= idx {
					in.HIDLen = append(in.HIDLen, 0)
				}
				in.HIDLen[idx] = n
			}
		}
	}
	return in
}

func (b *Board) handshake() {
	lines, err := b.Do("WHO", false, time.Second)
	if err != nil || len(lines) == 0 || !strings.HasPrefix(lines[0], "TYPEDECK-FW") {
		b.Log("la placa no habla el protocolo esperado: %v %v", lines, err)
		return
	}
	fw := strings.TrimSpace(strings.TrimPrefix(lines[0], "TYPEDECK-FW"))
	b.mu.Lock()
	b.info = &Info{Firmware: fw}
	b.mu.Unlock()
	b.refreshInfo()
	b.SyncMask(true)
	if b.OnConnected != nil {
		b.mu.Lock()
		cp := *b.info
		b.mu.Unlock()
		b.OnConnected(&cp)
	}
}

func (b *Board) refreshInfo() {
	lines, err := b.Do("INFO", true, 2*time.Second)
	if err != nil && len(lines) == 0 {
		return
	}
	in := parseInfo(lines)
	b.mu.Lock()
	fw := ""
	if b.info != nil {
		fw = b.info.Firmware
	}
	b.mu.Unlock()
	in.Firmware = fw
	in.Connected = time.Now()
	for i, n := range in.HIDLen {
		if n <= 0 {
			continue
		}
		rd, err := b.Do(fmt.Sprintf("RDESC %d %d", i, n), true, 3*time.Second)
		if err != nil && len(rd) == 0 {
			continue
		}
		hexs := strings.Join(rd, " ")
		in.RawRDesc = append(in.RawRDesc, hexs)
		if reps, err := hid.ParseReportDescriptorHex(hexs); err == nil {
			for _, r := range reps {
				r.Detail = fmt.Sprintf("interfaz %d: %s", i, r.Detail)
				in.Reports = append(in.Reports, r)
			}
		}
	}
	b.mu.Lock()
	b.info = &in
	b.mu.Unlock()
}

func (b *Board) SyncMask(force bool) {
	if b.Mask == nil {
		return
	}
	m := b.Mask()
	var sb strings.Builder
	for _, x := range m {
		fmt.Fprintf(&sb, "%02X", x)
	}
	hexs := sb.String()
	b.mu.Lock()
	same := b.lastMsk == hexs
	b.mu.Unlock()
	if same && !force {
		return
	}
	lines, err := b.Do("MASK "+hexs, false, time.Second)
	if err == nil && len(lines) == 1 && lines[0] == "OK" {
		b.mu.Lock()
		b.lastMsk = hexs
		b.mu.Unlock()
	}
}

func (b *Board) Tap(s hid.Stroke) error {
	_, err := b.Do(fmt.Sprintf("KEY %02X %02X", s.Mods, s.Usage), false, time.Second)
	return err
}

func (b *Board) Consumer(usage uint16) error {
	_, err := b.Do(fmt.Sprintf("CONS %X", usage), false, time.Second)
	return err
}

func (b *Board) Keys(on bool) error {
	n := 0
	if on {
		n = 1
	}
	_, err := b.Do(fmt.Sprintf("KEYS %d", n), false, time.Second)
	return err
}

func (b *Board) Watch(on bool) error {
	n := 0
	if on {
		n = 1
	}
	_, err := b.Do(fmt.Sprintf("WATCH %d", n), false, time.Second)
	return err
}

func (b *Board) Command(name string, arg int) ([]string, error) {
	switch name {
	case "ping":
		return b.Do("PING", false, time.Second)
	case "who":
		return b.Do("WHO", false, time.Second)
	case "stats":
		return b.Do("STATS", false, time.Second)
	case "bus":
		return b.Do("BUS", false, time.Second)
	case "bootlog":
		return b.Do("BOOTLOG", false, time.Second)
	case "reboot":
		return b.Do("REBOOT", false, time.Second)
	case "dark":
		return b.Do(fmt.Sprintf("DARK %d", max(0, min(arg, 1))), false, time.Second)
	case "vbus":
		return b.Do(fmt.Sprintf("VBUS %d", max(0, min(arg, 1))), false, time.Second)
	case "layer":
		return b.Do(fmt.Sprintf("LAYER %d", max(0, min(arg, 8))), false, time.Second)
	case "leds":
		return b.Do(fmt.Sprintf("LEDS %d", max(0, min(arg, 7))), false, time.Second)
	}
	return nil, errors.New("comando no permitido")
}

func (b *Board) Connected() bool {
	c, _, _, _ := b.Status()
	return c
}

type Sys struct {
	MCU        string `json:"mcu"`
	Board      string `json:"board"`
	FCPU       int    `json:"f_cpu"`
	FW         string `json:"fw"`
	VccMV      int    `json:"vcc_mv"`
	FreeRAM    int    `json:"free_ram"`
	MAX3421Rev string `json:"max3421e_rev"`
	UptimeS    int    `json:"uptime_s"`
}

func (b *Board) Sys() (Sys, error) {
	lines, err := b.Do("SYS", false, time.Second)
	if err != nil || len(lines) == 0 {
		return Sys{}, err
	}
	var s Sys
	for _, m := range kvRe.FindAllStringSubmatch(lines[0], -1) {
		v := strings.Trim(m[2], `"`)
		n, _ := strconv.Atoi(v)
		switch m[1] {
		case "mcu":
			s.MCU = v
		case "board":
			s.Board = v
		case "f_cpu":
			s.FCPU = n
		case "fw":
			s.FW = v
		case "vcc_mv":
			s.VccMV = n
		case "free_ram":
			s.FreeRAM = n
		case "max3421e_rev":
			s.MAX3421Rev = v
		case "uptime_s":
			s.UptimeS = n
		}
	}
	return s, nil
}

func (b *Board) Error() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rw != nil {
		return ""
	}
	return b.lastErr
}
