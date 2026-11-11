package config

func DefaultSettings() Settings {
	return Settings{Language: "en", Theme: "auto", Accent: "#2563eb", Density: "comfortable", KeySize: 88,
		HUD:    HUD{Enabled: true, Position: "bottom", Seconds: 1.2, OnAuto: true},
		HoldMS: 450, DoubleMS: 280, AutoLayer: true, VolumeStep: 6, Input: "auto", Capture: "board", TypingLayout: "auto", OBS: OBS{Host: "127.0.0.1", Port: 4455}, NotifyKeyboard: true, KeyHistoryDays: 30}
}

func Default() Config {
	return Config{
		Version:  Version,
		Settings: DefaultSettings(),
		Global: map[string]KeyDef{
			"53": {Tap: &Action{Type: "layer", To: "next"}, Label: "Capa", Icon: "layers"},
			"58": {Tap: &Action{Type: "url", URL: "{editor}"}, Label: "Editor", Icon: "sliders"},
		},
		Layers: []Layer{
			{Name: "Capa 1", Color: "#2563eb", Icon: "keyboard", AutoApps: []string{}, Keys: map[string]KeyDef{}},
			{Name: "Capa 2", Color: "#8b5cf6", Icon: "bolt", AutoApps: []string{}, Keys: map[string]KeyDef{}},
		},
		Keyboards: map[string]Keyboard{},
	}
}
