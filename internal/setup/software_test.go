package setup

import (
	"testing"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
)

func TestSoftwareFailureCodes(t *testing.T) {
	cases := map[string]string{"permission": "no_permission", "helper_missing": "no_helper", "hook_failed": "no_hook", "": "no_hook"}
	for in, want := range cases {
		if got := softwareFailure(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

type warningSource struct {
	capture.Source
	warnings []string
}

func (w warningSource) Warnings() []string { return w.warnings }

func TestSoftwareSetupShowsBackendWarnings(t *testing.T) {
	var su Setup
	src := warningSource{warnings: []string{"grab_failed"}}
	out := su.describeSoftware(config.Default(), actions.SystemInfo{}, true, capture.SystemInfo(), src)
	found := false
	for _, c := range out.Checks {
		if c.Code == "grab_failed" && c.Level == "warn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("checks: %+v", out.Checks)
	}
}
