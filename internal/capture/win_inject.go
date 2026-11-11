//go:build windows

package capture

import (
	"fmt"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

const defaultTapMS = 12

var injectMu sync.Mutex

func sendInputs(in []input) error {
	if len(in) == 0 {
		return nil
	}
	n, _, err := pSendInput.Call(uintptr(len(in)), ptr(&in[0]), unsafeSizeofInput)
	if int(n) != len(in) {
		return fmt.Errorf("the system blocked the keystrokes (%v)", err)
	}
	return nil
}

func scanInput(k ScanKey, up bool) input {
	ki := keybdInput{vk: k.VK, scan: k.Code, extra: injectTag}
	if k.Code != 0 {
		ki.flags = keyeventfScan
	}
	if k.Ext {
		ki.flags |= keyeventfExt
	}
	if up {
		ki.flags |= keyeventfUp
	}
	return input{kind: inputKeyboard, ki: ki}
}

func probeInput(up bool) input {
	in := input{kind: inputKeyboard, ki: keybdInput{vk: 0xE8, extra: probeTag}}
	if up {
		in.ki.flags = keyeventfUp
	}
	return in
}

func strokeKeys(s hid.Stroke) ([]ScanKey, error) {
	var keys []ScanKey
	for bit := byte(0); bit < 8; bit++ {
		if s.Mods&(1<<bit) != 0 {
			k, _ := ScanFromUsage(0xE0 + bit)
			keys = append(keys, k)
		}
	}
	if s.Usage != 0 {
		k, ok := ScanFromUsage(s.Usage)
		if !ok {
			return nil, fmt.Errorf("key %02X has no Windows equivalent", s.Usage)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (b *winBackend) Inject(s hid.Stroke, holdMS int) error {
	keys, err := strokeKeys(s)
	if err != nil || len(keys) == 0 {
		return err
	}
	injectMu.Lock()
	defer injectMu.Unlock()
	down := make([]input, len(keys))
	up := make([]input, len(keys))
	for i, k := range keys {
		down[i] = scanInput(k, false)
		up[len(keys)-1-i] = scanInput(k, true)
	}
	err = sendInputs(down)
	hold := holdMS
	if hold <= 0 {
		hold = defaultTapMS
	}
	time.Sleep(time.Duration(hold) * time.Millisecond)
	if rel := sendInputs(up); err == nil {
		err = rel
	}
	return err
}

func (b *winBackend) Consumer(usage uint16) error {
	vk, ok := ConsumerVK(usage)
	if !ok {
		return fmt.Errorf("media key %X has no Windows equivalent", usage)
	}
	injectMu.Lock()
	defer injectMu.Unlock()
	key := ScanKey{VK: vk, Ext: true}
	return sendInputs([]input{scanInput(key, false), scanInput(key, true)})
}

func (b *winBackend) TypeText(text string) error {
	injectMu.Lock()
	defer injectMu.Unlock()
	var batch []input
	flush := func() error {
		err := sendInputs(batch)
		batch = batch[:0]
		return err
	}
	for _, r := range text {
		batch = append(batch, textInputs(r)...)
		if len(batch) >= 128 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func textInputs(r rune) []input {
	switch r {
	case '\n', '\r':
		return keyPair(0x28)
	case '\t':
		return keyPair(0x2B)
	}
	var out []input
	for _, u := range utf16.Encode([]rune{r}) {
		down := input{kind: inputKeyboard, ki: keybdInput{scan: u, flags: keyeventfUni, extra: injectTag}}
		up := down
		up.ki.flags |= keyeventfUp
		out = append(out, down, up)
	}
	return out
}

func keyPair(usage byte) []input {
	k, _ := ScanFromUsage(usage)
	return []input{scanInput(k, false), scanInput(k, true)}
}
