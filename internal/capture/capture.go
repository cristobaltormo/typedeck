package capture

import (
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const (
	Board    = "board"
	Software = "software"
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
	System    bool         `json:"system,omitempty"`
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

const SystemKeyboardID = "system"

func (i Info) ID() string {
	if i.System {
		return SystemKeyboardID
	}
	if i.VID == "" {
		return ""
	}
	return strings.ToUpper(i.VID + ":" + i.PID)
}

type Handlers struct {
	OnEvent      func(Event)
	Mask         func() [32]byte
	OnConnected  func(*Info)
	OnDisconnect func()
}

type Capturer interface {
	Kind() string
	Bind(Handlers)
	Run()
	Close()
	Status() (connected bool, port string, info *Info, since time.Time)
	Connected() bool
	Error() string
	SyncMask(force bool)
	Keys(on bool) error
	Pause()
	Resume()
}

type Injector interface {
	Tap(hid.Stroke) error
	TapHeld(s hid.Stroke, holdMS int) error
	Consumer(usage uint16) error
}

type Source interface {
	Capturer
	Injector
}

type Commander interface {
	Command(name string, arg int) ([]string, error)
}

type RawDoer interface {
	Do(cmd string, multi bool, timeout time.Duration) ([]string, error)
}

type SysReporter interface {
	Sys() (Sys, error)
}

type LayerSetter interface {
	SetLayer(index int)
}

type Warner interface {
	Warnings() []string
}

type Switcher interface {
	Select(kind string) error
	Active() string
}

type TextTyper interface {
	TypeText(text string) error
}

type Check struct {
	OK      bool
	Code    string
	Message string
	Problem string
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
