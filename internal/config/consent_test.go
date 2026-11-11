package config

import (
	"os"
	"testing"
)

func TestConsentIsOnlyKnownOnceRecorded(t *testing.T) {
	p := Paths{Dir: t.TempDir()}
	if HasCaptureConsent(p) {
		t.Fatal("a fresh installation has no consent")
	}
	if err := RecordCaptureConsent(p); err != nil {
		t.Fatal(err)
	}
	if !HasCaptureConsent(p) {
		t.Fatal("recorded consent must be found")
	}
}

func TestConsentOfAnOlderTextOrGarbageDoesNotCount(t *testing.T) {
	p := Paths{Dir: t.TempDir()}
	for _, body := range []string{`{"version":0,"at":"2026-01-01T00:00:00Z"}`, `nope`, ``} {
		_ = os.WriteFile(p.CaptureConsent(), []byte(body), 0o600)
		if HasCaptureConsent(p) {
			t.Fatalf("%q must not count", body)
		}
	}
}

func TestConsentIsNotPartOfTheConfiguration(t *testing.T) {
	p := Paths{Dir: t.TempDir()}
	c := Default()
	c.Settings.Capture = "software"
	if err := Save(p, c, false, false); err != nil {
		t.Fatal(err)
	}
	if HasCaptureConsent(p) {
		t.Fatal("saving a configuration that says software must not record consent")
	}
}
