package kbdb

import "testing"

func TestIdentifyYourKeyboard(t *testing.T) {
	id := Identify("258A", "0016", "BY Tech", "Usb Gaming Keyboard")
	if id.Brand != "BY Tech" || id.Model != "Usb Gaming Keyboard" || id.Display != "BY Tech Usb Gaming Keyboard" {
		t.Fatalf("identidad: %+v", id)
	}
	if id.Vendor != "SINO WEALTH" || !id.VendorIsChip || !id.Generic {
		t.Fatalf("chip vendor and generic name: %+v", id)
	}
	if id.Confidence != "verified" || id.Layout != "full-iso" {
		t.Fatalf("model verified with the key wizard: %+v", id)
	}
}

func TestVendorOnlyBrand(t *testing.T) {
	id := Identify("046d", "c31c", "", "")
	if id.Brand != "Logitech" || id.Confidence != "vendor" {
		t.Fatalf("%+v", id)
	}
	if id := Identify("ffff", "0001", "", ""); id.Confidence != "unknown" || id.Display != "" {
		t.Fatalf("desconocido: %+v", id)
	}
}

func TestFormFactorFromName(t *testing.T) {
	cases := map[string]string{
		"Keychron K8 TKL": "tkl", "Ducky One 2 Mini 60%": "60", "GMMK Pro 75%": "75", "Royal Kludge RK68 65 %": "65",
		"Das Keyboard 4 Professional": "", "Full Size Gaming Keyboard": "full", "Redragon 80% Mechanical": "tkl",
	}
	for name, want := range cases {
		if got := Identify("1234", "0001", "", name).FormFactor; got != want {
			t.Errorf("%q -> %q, expected %q", name, got, want)
		}
	}
}

func TestLayoutNeedsFormAndStandard(t *testing.T) {
	if l := Identify("1234", "1", "", "Foo TKL ISO").Layout; l != "tkl-iso" {
		t.Errorf("tkl-iso: %q", l)
	}
	if l := Identify("1234", "1", "", "Foo TKL").Layout; l != "" {
		t.Errorf("without ISO/ANSI it is not decided: %q", l)
	}
}

func TestVerifiedModelWins(t *testing.T) {
	AddVerified(Model{VID: "1234", PID: "00aa", Brand: "Acme", Model: "K1", Layout: "75-ansi"})
	id := Identify("1234", "00AA", "x", "y")
	if id.Brand != "Acme" || id.Layout != "75-ansi" || id.Confidence != "verified" {
		t.Fatalf("%+v", id)
	}
}

func TestSharedControllerIDsAreNotAutoVerified(t *testing.T) {
	id := Identify("258A", "0016", "Otra Marca", "Mini 60% Keyboard")
	if id.Confidence == "verified" || id.Layout != "" && id.FormFactor != "60" {
		t.Fatalf("must not inherit the model of another keyboard: %+v", id)
	}
	if id.FormFactor != "60" {
		t.Fatalf("the name says 60%%: %+v", id)
	}
}
