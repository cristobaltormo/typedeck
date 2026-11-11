package board

import (
	"github.com/cristobaltormo/typedeck/internal/winreg"
)

const (
	serialCommKey = `HARDWARE\DEVICEMAP\SERIALCOMM`
	usbEnumKey    = `SYSTEM\CurrentControlSet\Enum\USB`
)

func presentPorts() []string {
	var out []string
	for _, v := range winreg.StringValues(winreg.LocalMachine, serialCommKey) {
		out = append(out, v.Data)
	}
	return out
}

func usbPortNames() map[string][]string {
	usb := map[string][]string{}
	for _, dev := range winreg.SubKeys(winreg.LocalMachine, usbEnumKey) {
		if !isBoardKey(dev) {
			continue
		}
		for _, inst := range winreg.SubKeys(winreg.LocalMachine, usbEnumKey+`\`+dev) {
			path := usbEnumKey + `\` + dev + `\` + inst + `\Device Parameters`
			if port, ok := winreg.String(winreg.LocalMachine, path, "PortName"); ok {
				usb[dev] = append(usb[dev], port)
			}
		}
	}
	return usb
}

func systemPorts() []string { return boardPorts(usbPortNames(), presentPorts()) }
