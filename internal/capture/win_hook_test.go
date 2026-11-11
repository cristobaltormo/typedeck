//go:build windows

package capture

import (
	"syscall"
	"testing"
	"time"
)

func TestCancelledHookThreadEndsAndLeavesNoHook(t *testing.T) {
	keyProcOnce.Do(func() { keyProcPtr = syscall.NewCallback(keyProc) })
	th := newHookThread()
	th.cancel.Store(true)
	go th.run(NewFilter())
	if err := th.awaitReady(); err == nil {
		t.Fatal("a cancelled thread must not report ready")
	}
	select {
	case <-th.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the cancelled thread did not end")
	}
	if th.hook != 0 {
		t.Fatal("the hook of a cancelled thread must be removed")
	}
}

func TestStartAndStopRepeatedlyKeepsOneHookAtATime(t *testing.T) {
	b := &winBackend{}
	for i := 0; i < 20; i++ {
		if err := b.Start(NewFilter()); err != nil {
			t.Skipf("the hook cannot be installed here: %v", err)
		}
		if !b.Alive() {
			t.Fatalf("round %d: not alive after Start", i)
		}
		b.Stop()
		if b.Alive() {
			t.Fatalf("round %d: still alive after Stop", i)
		}
	}
}

func TestReinstallKeepsTheBackendAlive(t *testing.T) {
	b := &winBackend{}
	if err := b.Start(NewFilter()); err != nil {
		t.Skipf("the hook cannot be installed here: %v", err)
	}
	defer b.Stop()
	for i := 0; i < 5; i++ {
		b.Reinstall()
		time.Sleep(50 * time.Millisecond)
	}
	if !b.Alive() {
		t.Fatal("a reinstall that works must not end the session")
	}
}
