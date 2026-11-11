package config

const Version = 3

type Action struct {
	Type       string   `json:"type"`
	App        string   `json:"app,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	URL        string   `json:"url,omitempty"`
	Cmd        string   `json:"cmd,omitempty"`
	ShowOutput *bool    `json:"show_output,omitempty"`
	Host       string   `json:"host,omitempty"`
	Method     string   `json:"method,omitempty"`
	Body       string   `json:"body,omitempty"`
	Keys       string   `json:"keys,omitempty"`
	Text       string   `json:"text,omitempty"`
	Minutes    float64  `json:"minutes,omitempty"`
	Label      string   `json:"label,omitempty"`
	To         any      `json:"to,omitempty"`
	Momentary  bool     `json:"momentary,omitempty"`
	Toggle     bool     `json:"toggle,omitempty"`
	Ms         int      `json:"ms,omitempty"`
	Target     string   `json:"target,omitempty"`
	Steps      []Action `json:"steps,omitempty"`
	Else       []Action `json:"else,omitempty"`
	Cond       string   `json:"cond,omitempty"`
	Not        bool     `json:"not,omitempty"`
	Confirm    bool     `json:"confirm,omitempty"`
}

type KeyDef struct {
	Tap    *Action `json:"tap,omitempty"`
	Hold   *Action `json:"hold,omitempty"`
	Double *Action `json:"double,omitempty"`
	Label  string  `json:"label,omitempty"`
	Icon   string  `json:"icon,omitempty"`
	Color  string  `json:"color,omitempty"`
}

func (k KeyDef) Empty() bool {
	return k.Tap == nil && k.Hold == nil && k.Double == nil && k.Label == "" && k.Icon == "" && k.Color == ""
}

func (k KeyDef) HasAction() bool { return k.Tap != nil || k.Hold != nil || k.Double != nil }

type Layer struct {
	Name     string            `json:"name"`
	Color    string            `json:"color,omitempty"`
	Icon     string            `json:"icon,omitempty"`
	AutoApps []string          `json:"auto_apps"`
	Keys     map[string]KeyDef `json:"keys"`
}

type HUD struct {
	Enabled  bool    `json:"enabled"`
	Position string  `json:"position"`
	Seconds  float64 `json:"seconds"`
	OnAction bool    `json:"on_action"`
	OnAuto   bool    `json:"on_auto"`
	Sound    bool    `json:"sound"`
}

type Settings struct {
	Language     string `json:"language"`
	Theme        string `json:"theme"`
	Accent       string `json:"accent"`
	Density      string `json:"density"`
	KeySize      int    `json:"keysize"`
	HUD          HUD    `json:"hud"`
	HoldMS       int    `json:"hold_ms"`
	DoubleMS     int    `json:"double_ms"`
	AutoLayer    bool   `json:"auto_layer"`
	VolumeStep   int    `json:"volume_step"`
	Input        string `json:"input"`
	Capture      string `json:"capture"`
	TypingLayout string `json:"typing_layout"`
	OBS          OBS    `json:"obs"`

	NotifyKeyboard bool `json:"notify_keyboard"`
	KeyHistory     bool `json:"key_history"`
	KeyHistoryDays int  `json:"key_history_days"`
}

type OBS struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
}

type Keyboard struct {
	Layout string `json:"layout"`
	Name   string `json:"name,omitempty"`
}

type Config struct {
	Version   int                 `json:"version"`
	Settings  Settings            `json:"settings"`
	Global    map[string]KeyDef   `json:"global"`
	Layers    []Layer             `json:"layers"`
	Keyboards map[string]Keyboard `json:"keyboards"`
}
