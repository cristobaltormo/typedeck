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
	if v, ok := data.Vendors[vid]; ok {
		id.Vendor = v
	} else if v, ok := data.ChipVendors[vid]; ok {
		id.Vendor, id.VendorIsChip = v, true
	}

	for _, m := range data.Models {
		if strings.EqualFold(m.VID, vid) && strings.EqualFold(m.PID, pid) &&
			(m.Mfr == "" || strings.EqualFold(m.Mfr, mfr)) && (m.Prod == "" || strings.EqualFold(m.Prod, prod)) {
			id.Brand, id.Model, id.Layout = m.Brand, m.Model, m.Layout
			id.Confidence, id.Source = "verified", "base de modelos verificados"
			break
		}
	}

	if id.Brand == "" {
		id.Brand = mfr
		id.Model = prod
		if prod != "" || mfr != "" {
			id.Confidence, id.Source = "name", "nombre que da el propio teclado"
		}
		if id.Brand == "" && id.Vendor != "" && !id.VendorIsChip {
			id.Brand, id.Confidence, id.Source = id.Vendor, "vendor", "fabricante del identificador USB"
		}
		if id.Brand == "" && id.VendorIsChip {
			id.Brand = id.Vendor
		}
	}
	if prod == "" || genericRe.MatchString(prod) {
		id.Generic = prod != ""
		if id.Generic {
			id.Notes = append(id.Notes, "generic_name")
		}
	}
	if id.VendorIsChip {
		id.Notes = append(id.Notes, "chip_vendor")
	}

	text := mfr + " " + prod
	switch {
	case tklRe.MatchString(text):
		id.FormFactor = "tkl"
	case percentRe.MatchString(text):
		switch percentRe.FindStringSubmatch(text)[1] {
		case "60":
			id.FormFactor = "60"
		case "65":
			id.FormFactor = "65"
		case "75":
			id.FormFactor = "75"
		case "80":
			id.FormFactor = "tkl"
		case "100":
			id.FormFactor = "full"
		}
	case fullRe.MatchString(text):
		id.FormFactor = "full"
	}
	switch {
	case isoRe.MatchString(text):
		id.Standard = "iso"
	case ansiRe.MatchString(text):
		id.Standard = "ansi"
	}
	if id.Layout == "" && id.FormFactor != "" && id.Standard != "" {
		id.Layout = id.FormFactor + "-" + id.Standard
	}
	switch {
	case id.Brand != "" && id.Model != "":
		id.Display = id.Brand + " " + id.Model
	case id.Model != "":
		id.Display = id.Model
	default:
		id.Display = id.Brand
	}
	return id
}

func AddVerified(m Model) { data.Models = append(data.Models, m) }
