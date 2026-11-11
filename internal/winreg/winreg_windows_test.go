package winreg

import "testing"

const testKey = `Software\TypedeckTest`

func TestRoundTrip(t *testing.T) {
	defer DeleteValue(CurrentUser, testKey, "s")
	defer DeleteValue(CurrentUser, testKey, "d")
	if err := SetString(CurrentUser, testKey, "s", `"C:\a b\typedeck.exe" --background`); err != nil {
		t.Fatal(err)
	}
	if err := SetDWORD(CurrentUser, testKey, "d", 7); err != nil {
		t.Fatal(err)
	}
	if v, ok := String(CurrentUser, testKey, "s"); !ok || v != `"C:\a b\typedeck.exe" --background` {
		t.Fatalf("%q %v", v, ok)
	}
	if v, ok := DWORD(CurrentUser, testKey, "d"); !ok || v != 7 {
		t.Fatalf("%d %v", v, ok)
	}
	if len(StringValues(CurrentUser, testKey)) != 1 {
		t.Fatal("expected one string value")
	}
	if err := DeleteValue(CurrentUser, testKey, "s"); err != nil {
		t.Fatal(err)
	}
	if _, ok := String(CurrentUser, testKey, "s"); ok {
		t.Fatal("the value must be gone")
	}
	if err := DeleteValue(CurrentUser, testKey, "s"); err != nil {
		t.Fatal("deleting twice must be fine")
	}
}

func TestSubKeysOfMissingKey(t *testing.T) {
	if len(SubKeys(LocalMachine, `SOFTWARE\TypedeckDoesNotExist`)) != 0 {
		t.Fatal("expected nothing")
	}
}
