package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const CaptureConsentVersion = 1

type CaptureConsent struct {
	Version int       `json:"version"`
	At      time.Time `json:"at"`
}

func (p Paths) CaptureConsent() string { return filepath.Join(p.Dir, "capture-consent.json") }

func HasCaptureConsent(p Paths) bool {
	b, err := os.ReadFile(p.CaptureConsent())
	if err != nil {
		return false
	}
	var c CaptureConsent
	return json.Unmarshal(b, &c) == nil && c.Version == CaptureConsentVersion
}

func RecordCaptureConsent(p Paths) error {
	b, err := json.Marshal(CaptureConsent{Version: CaptureConsentVersion, At: time.Now().UTC().Truncate(time.Second)})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(p.CaptureConsent(), b, 0o600)
}
