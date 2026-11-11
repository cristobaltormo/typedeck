//go:build linux

package capture

import (
	"errors"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

func TestIoctlNumbers(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"EVIOCGRAB", eviocgrab, 0x40044590},
		{"EVIOCGBIT(EV_KEY, 96)", eviocgbit(evKey, 96), 0x80604521},
		{"EVIOCGNAME(256)", eviocgname(256), 0x81004506},
		{"EVIOCGPHYS(256)", eviocgphys(256), 0x81004507},
		{"EVIOCGKEY(96)", eviocgkey(96), 0x80604518},
		{"UI_DEV_CREATE", uiDevCreate, 0x5501},
		{"UI_DEV_DESTROY", uiDevDestroy, 0x5502},
		{"UI_DEV_SETUP", uiDevSetup, 0x405C5503},
		{"UI_SET_EVBIT", uiSetEvbit, 0x40045564},
		{"UI_SET_KEYBIT", uiSetKeybit, 0x40045565},
		{"UI_SET_LEDBIT", uiSetLedbit, 0x40045569},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %#x, want %#x", c.name, c.got, c.want)
		}
	}
}

func TestStructSizes(t *testing.T) {
	if got := unsafe.Sizeof(uinputSetup{}); got != setupSize {
		t.Errorf("uinput_setup is %d bytes, the kernel expects %d", got, setupSize)
	}
	if eventSize != 24 && eventSize != 16 {
		t.Errorf("input_event is %d bytes", eventSize)
	}
}

func TestEventEncodingRoundTrips(t *testing.T) {
	in := []inputEvent{{Type: evKey, Code: 30, Value: 1}, {Type: evSyn, Code: synReport}}
	out := decodeEvents(encodeEvents(in))
	if len(out) != 2 || out[0].Code != 30 || out[0].Value != 1 || out[1].Type != evSyn {
		t.Errorf("round trip changed the events: %+v", out)
	}
}

func newTestKeyboard(mask ...byte) *keyboard {
	f := NewFilter()
	var m [32]byte
	for _, u := range mask {
		m[u>>3] |= 1 << (u & 7)
	}
	f.SetMask(m)
	return newKeyboard(&session{filter: f}, "/dev/input/event0", nil)
}

func key(code uint16, value int32) inputEvent {
	return inputEvent{Type: evKey, Code: code, Value: value}
}

func TestConsumeSwallowsPressRepeatAndRelease(t *testing.T) {
	k := newTestKeyboard(0x04)
	for i, ev := range []inputEvent{key(30, 1), key(30, 2), key(30, 2), key(30, 0)} {
		if !k.consume(ev) {
			t.Errorf("event %d of a captured key was forwarded", i)
		}
	}
	if len(k.swallowed) != 0 || len(k.forwarded) != 0 {
		t.Error("state left over after the release")
	}
}

func TestConsumeForwardsOtherKeysUntouched(t *testing.T) {
	k := newTestKeyboard(0x04)
	for i, ev := range []inputEvent{key(18, 1), key(18, 2), key(18, 0), key(113, 1), key(113, 0)} {
		if k.consume(ev) {
			t.Errorf("event %d of an ordinary key was swallowed", i)
		}
	}
}

func TestConsumeKeepsAPressedKeyConsistentWhenTheMaskChanges(t *testing.T) {
	k := newTestKeyboard()
	if k.consume(key(30, 1)) {
		t.Fatal("a key outside the mask was swallowed")
	}
	var m [32]byte
	m[0] = 0x10
	k.s.filter.SetMask(m)
	if k.consume(key(30, 2)) || k.consume(key(30, 0)) {
		t.Error("a key already forwarded as pressed must keep being forwarded until it is released")
	}
}

func TestAPausedFilterStillSwallowsTheReleaseOfAKeyItTook(t *testing.T) {
	k := newTestKeyboard(0x16)
	if !k.consume(key(31, 1)) {
		t.Fatal("the macro key was not swallowed")
	}
	k.s.filter.Pause()
	if !k.consume(key(31, 2)) || !k.consume(key(31, 0)) {
		t.Error("a swallowed key must keep being swallowed until its release")
	}
}

func TestModifiersAreForwardedAndTracked(t *testing.T) {
	k := newTestKeyboard(0x04)
	if k.consume(key(29, 1)) {
		t.Fatal("ctrl was swallowed")
	}
	if !k.consume(key(30, 1)) {
		t.Fatal("the macro key was not swallowed with ctrl held")
	}
	ev := <-k.s.filter.Events()
	if ev.Kind != 'D' || ev.Usage != 0x04 || ev.Mods != 0x01 {
		t.Errorf("event %+v", ev)
	}
}

func TestDropScanRemovesTheScancodeOfASwallowedKey(t *testing.T) {
	out := dropScan([]inputEvent{{Type: evMsc, Code: mscScan, Value: 30}})
	if len(out) != 0 {
		t.Errorf("scancode left behind: %+v", out)
	}
	keep := dropScan([]inputEvent{{Type: evLed, Code: 1}})
	if len(keep) != 1 {
		t.Error("an unrelated event was dropped")
	}
}

func TestStrokeCodes(t *testing.T) {
	codes, err := strokeCodes(hid.Stroke{Usage: 0x09, Mods: hid.ModCtrl | hid.ModShift})
	if err != nil || len(codes) != 3 || codes[0] != 29 || codes[1] != 42 || codes[2] != 33 {
		t.Errorf("ctrl+shift+f: %v %v", codes, err)
	}
	if _, err := strokeCodes(hid.Stroke{Usage: 0xFE}); err == nil {
		t.Error("an unmapped usage should fail")
	}
}

func TestNoUinputDeviceIsCreatedWithoutReadAccess(t *testing.T) {
	probe, create := probeInputs, makeVirtual
	defer func() { probeInputs, makeVirtual = probe, create }()
	created := 0
	probeInputs = func() (int, int) { return 0, 2 }
	makeVirtual = func(func(uint16, int32)) (*virtualKeyboard, error) {
		created++
		return nil, errors.New("must not be called")
	}
	if _, err := startSession(NewFilter(), &latencyStats{}); err != errReadAccess {
		t.Fatalf("got %v", err)
	}
	if created != 0 {
		t.Fatalf("%d virtual keyboards created before the access check", created)
	}
}

func TestVirtualKeyboardDeclaresKeysAbove255ButNoButtons(t *testing.T) {
	declared := map[int]bool{}
	for _, c := range virtualKeys() {
		declared[c] = true
	}
	for _, want := range []int{1, 30, 255, 0x160, 0x1d0, 0x2bf} {
		if !declared[want] {
			t.Errorf("key %#x is not declared", want)
		}
	}
	for _, banned := range []int{0, 0x100, 0x110, 0x120, 0x130, 0x15f, 0x2c0, 0x2e7} {
		if declared[banned] {
			t.Errorf("code %#x would make the kernel treat the device as a mouse or joystick", banned)
		}
	}
}

func TestGrabFailuresAreReported(t *testing.T) {
	if c := grabCheck(nil); !c.OK {
		t.Fatalf("no failures must be ok: %+v", c)
	}
	c := grabCheck(map[string]string{"/dev/input/event3": "busy"})
	if c.OK || c.Code != "grab_failed" || !strings.Contains(c.Problem, "event3") {
		t.Fatalf("%+v", c)
	}
	s := &session{grabFail: map[string]string{}}
	s.grabFailed("/dev/input/event3", syscall.EBUSY)
	b := &linuxBackend{sess: s}
	if w := b.Warnings(); len(w) != 1 || w[0] != "grab_failed" {
		t.Fatalf("warnings %v", w)
	}
	s.grabbed("/dev/input/event3")
	if len(b.Warnings()) != 0 {
		t.Fatal("a keyboard taken later must clear the warning")
	}
}
