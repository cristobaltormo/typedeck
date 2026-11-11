package kbdb

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
)

//go:embed db.json
var raw []byte

type Model struct {
	VID    string `json:"vid"`
	PID    string `json:"pid"`
	Mfr    string `json:"mfr,omitempty"`
	Prod   string `json:"prod,omitempty"`
	Brand  string `json:"brand"`
	Model  string `json:"model"`
	Layout string `json:"layout"`
}

type db struct {
	Vendors     map[string]string `json:"vendors"`
	ChipVendors map[string]string `json:"chip_vendors"`
	Models      []Model           `json:"models"`
}

var data = func() db {
	var d db
	_ = json.Unmarshal(raw, &d)
	return d
}()

type Identity struct {
	Brand        string   `json:"brand"`
	Model        string   `json:"model"`
	Display      string   `json:"display"`
	Vendor       string   `json:"vendor,omitempty"`
	VendorIsChip bool     `json:"vendor_is_chip,omitempty"`
	Generic      bool     `json:"generic_name,omitempty"`
	FormFactor   string   `json:"form_factor,omitempty"`
	Standard     string   `json:"standard,omitempty"`
	Layout       string   `json:"layout,omitempty"`
	Confidence   string   `json:"confidence"`
	Source       string   `json:"source"`
	Notes        []string `json:"notes,omitempty"`
}

var (
	genericRe = regexp.MustCompile(`(?i)^(usb|hid|gaming|standard|generic|keyboard|teclado|device|composite|\s|-|_)+$`)
	percentRe = regexp.MustCompile(`(?i)(?:^|[^0-9a-z])(60|65|75|80|96|100)\s?%`)
	tklRe     = regexp.MustCompile(`(?i)\b(tkl|tenkeyless|ten-key-less)\b`)
	isoRe     = regexp.MustCompile(`(?i)\b(iso)\b`)
	ansiRe    = regexp.MustCompile(`(?i)\b(ansi)\b`)
	fullRe    = regexp.MustCompile(`(?i)\b(full[- ]?size|fullsize)\b`)
)

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

func Identify(vid, pid, mfr, prod string) Identity {
	vid, pid = strings.ToLower(vid), strings.ToLower(pid)
	mfr, prod = clean(mfr), clean(prod)
	id := Identity{Confidence: "unknown", Source: "ninguna"}
	setVendor(&id, vid)
	if !setVerifiedModel(&id, vid, pid, mfr, prod) {
		setReportedName(&id, mfr, prod)
	}
	if prod != "" && genericRe.MatchString(prod) {
		id.Generic = true
		id.Notes = append(id.Notes, "generic_name")
	}
	if id.VendorIsChip {
		id.Notes = append(id.Notes, "chip_vendor")
	}
	text := mfr + " " + prod
	id.FormFactor = formFactor(text)
	id.Standard = standard(text)
	if id.Layout == "" && id.FormFactor != "" && id.Standard != "" {
		id.Layout = id.FormFactor + "-" + id.Standard
	}
	id.Display = displayName(id)
	return id
}

func setVendor(id *Identity, vid string) {
	if v, ok := data.Vendors[vid]; ok {
		id.Vendor = v
	} else if v, ok := data.ChipVendors[vid]; ok {
		id.Vendor, id.VendorIsChip = v, true
	}
}

func setVerifiedModel(id *Identity, vid, pid, mfr, prod string) bool {
	for _, m := range data.Models {
		if strings.EqualFold(m.VID, vid) && strings.EqualFold(m.PID, pid) &&
			(m.Mfr == "" || strings.EqualFold(m.Mfr, mfr)) && (m.Prod == "" || strings.EqualFold(m.Prod, prod)) {
			id.Brand, id.Model, id.Layout = m.Brand, m.Model, m.Layout
			id.Confidence, id.Source = "verified", "verified model database"
			return true
		}
	}
	return false
}

func setReportedName(id *Identity, mfr, prod string) {
	id.Brand = mfr
	id.Model = prod
	if prod != "" || mfr != "" {
		id.Confidence, id.Source = "name", "name reported by the keyboard"
	}
	if id.Brand == "" && id.Vendor != "" && !id.VendorIsChip {
		id.Brand, id.Confidence, id.Source = id.Vendor, "vendor", "vendor of the USB identifier"
	}
	if id.Brand == "" && id.VendorIsChip {
		id.Brand = id.Vendor
	}
}

func formFactor(text string) string {
	switch {
	case tklRe.MatchString(text):
		return "tkl"
	case percentRe.MatchString(text):
		switch percentRe.FindStringSubmatch(text)[1] {
		case "60":
			return "60"
		case "65":
			return "65"
		case "75":
			return "75"
		case "80":
			return "tkl"
		case "100":
			return "full"
		}
	case fullRe.MatchString(text):
		return "full"
	}
	return ""
}

func standard(text string) string {
	switch {
	case isoRe.MatchString(text):
		return "iso"
	case ansiRe.MatchString(text):
		return "ansi"
	}
	return ""
}

func displayName(id Identity) string {
	switch {
	case id.Brand != "" && id.Model != "":
		return id.Brand + " " + id.Model
	case id.Model != "":
		return id.Model
	}
	return id.Brand
}
