package setup

import "testing"

func TestSoftwareFailureCodesOfLinux(t *testing.T) {
	cases := map[string]string{"no_uinput": "no_uinput", "uinput_access": "no_uinput_access", "input_access": "no_input_access"}
	for in, want := range cases {
		if got := softwareFailure(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
