//go:build windows

package platform

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cristobaltormo/typedeck/internal/winreg"
)

const keyboardLayoutsKey = `SYSTEM\CurrentControlSet\Control\Keyboard Layouts`

func (*windows) SystemLayout() string {
	return layoutFromKLID(klidFromHKL(uint32(foregroundLayout()), klidByLayoutID))
}

func klidByLayoutID(id, lang uint16) string {
	want := strings.ToUpper(strconv.FormatUint(uint64(id), 16))
	suffix := fmt.Sprintf("%04X", lang)
	for _, klid := range winreg.SubKeys(winreg.LocalMachine, keyboardLayoutsKey) {
		if len(klid) != 8 || !strings.EqualFold(klid[4:], suffix) {
			continue
		}
		v, ok := winreg.String(winreg.LocalMachine, keyboardLayoutsKey+`\`+klid, "Layout Id")
		if ok && strings.EqualFold(strings.TrimLeft(v, "0"), want) {
			return klid
		}
	}
	return ""
}
