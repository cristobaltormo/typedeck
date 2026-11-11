//go:build windows

package capture

import (
	"errors"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	injectTag = 0x5444434B
	probeTag  = 0x54445052
)

var live struct {
	filter    atomic.Pointer[Filter]
	lastEvent atomic.Int64
	probeSeen atomic.Uint64
	count     atomic.Uint64
	totalNS   atomic.Int64
	maxNS     atomic.Int64
}

var (
	keyProcOnce sync.Once
	keyProcPtr  uintptr
)

func keyProc(code, wparam, lparam uintptr) uintptr {
	if int32(code) == hcAction {
		k := *(**kbdllhook)(unsafe.Pointer(&lparam))
		if handleKey(wparam, k) {
			return 1
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, code, wparam, lparam)
	return r
}

func handleKey(msg uintptr, k *kbdllhook) bool {
	start := time.Now()
	live.lastEvent.Store(start.UnixNano())
	if k.extraInfo == probeTag {
		live.probeSeen.Add(1)
		return false
	}
	if k.flags&(llkhfInjected|llkhfLowerIL) != 0 {
		return false
	}
	f := live.filter.Load()
	if f == nil {
		return false
	}
	usage := UsageFromScan(k.scanCode, k.flags&llkhfExtended != 0, k.vkCode)
	swallow := f.Handle(usage, msg == wmKeyDown || msg == wmSysKeyDown)
	recordLatency(time.Since(start))
	return swallow
}

func recordLatency(d time.Duration) {
	ns := d.Nanoseconds()
	live.count.Add(1)
	live.totalNS.Add(ns)
	for {
		cur := live.maxNS.Load()
		if ns <= cur || live.maxNS.CompareAndSwap(cur, ns) {
			return
		}
	}
}

const (
	startTimeout  = 3 * time.Second
	cancelTimeout = 5 * time.Second
	wmQuit        = 0x0012
)

var errHookTimeout = errors.New("the keyboard hook did not start")

type hookThread struct {
	tid    atomic.Uint32
	hook   uintptr
	cancel atomic.Bool
	ready  chan error
	done   chan struct{}
}

func newHookThread() *hookThread {
	return &hookThread{ready: make(chan error, 1), done: make(chan struct{})}
}

func (t *hookThread) alive() bool {
	select {
	case <-t.done:
		return false
	default:
		return true
	}
}

func (t *hookThread) post(msg uintptr) {
	if tid := t.tid.Load(); tid != 0 {
		pPostThreadMessage.Call(uintptr(tid), msg, 0, 0)
	}
}

func (t *hookThread) run(f *Filter) {
	runtime.LockOSThread()
	defer close(t.done)
	tid, _, _ := pGetCurrentThreadID.Call()
	t.tid.Store(uint32(tid))
	if err := t.install(); err != nil {
		t.ready <- err
		return
	}
	defer f.Release()
	defer t.remove()
	if t.cancel.Load() {
		t.ready <- errHookTimeout
		return
	}
	live.lastEvent.Store(time.Now().UnixNano())
	t.ready <- nil
	t.messages()
}

func (t *hookThread) install() error {
	h, _, err := pSetWindowsHookEx.Call(whKeyboardLL, keyProcPtr, 0, 0)
	if h == 0 {
		return err
	}
	t.hook = h
	return nil
}

func (t *hookThread) remove() {
	if t.hook != 0 {
		pUnhookWindowsHook.Call(t.hook)
		t.hook = 0
	}
}

func (t *hookThread) messages() {
	var m winMsg
	for !t.cancel.Load() {
		r, _, _ := pGetMessage.Call(ptr(&m), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.hwnd == 0 && m.message == wmReinstall {
			if err := t.reinstall(); err != nil {
				log.Printf("capture: the keyboard hook could not be reinstalled: %v", err)
				return
			}
			continue
		}
		pTranslateMessage.Call(ptr(&m))
		pDispatchMessage.Call(ptr(&m))
	}
}

func (t *hookThread) reinstall() error {
	t.remove()
	if f := live.filter.Load(); f != nil {
		f.Release()
	}
	return t.install()
}

func (t *hookThread) awaitReady() error {
	select {
	case err := <-t.ready:
		return err
	case <-time.After(startTimeout):
	}
	t.cancel.Store(true)
	deadline := time.After(cancelTimeout)
	for {
		t.post(wmQuit)
		select {
		case <-t.done:
			return errHookTimeout
		case <-deadline:
			return errHookTimeout
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type winBackend struct {
	mu   sync.Mutex
	th   atomic.Pointer[hookThread]
	stop chan struct{}
}

func newBackend() Backend { return &winBackend{} }

func (b *winBackend) Start(f *Filter) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Alive() {
		return nil
	}
	keyProcOnce.Do(func() { keyProcPtr = syscall.NewCallback(keyProc) })
	live.filter.Store(f)
	th := newHookThread()
	go th.run(f)
	if err := th.awaitReady(); err != nil {
		live.filter.Store(nil)
		return err
	}
	b.th.Store(th)
	b.stop = make(chan struct{})
	go watchdog(th, b.stop)
	return nil
}

func (b *winBackend) Alive() bool {
	th := b.th.Load()
	return th != nil && th.alive()
}

func (b *winBackend) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	th := b.th.Swap(nil)
	if th == nil || !th.alive() {
		return
	}
	close(b.stop)
	live.filter.Store(nil)
	th.post(wmQuit)
	select {
	case <-th.done:
	case <-time.After(2 * time.Second):
	}
}

func (b *winBackend) Reinstall() {
	if th := b.th.Load(); th != nil {
		th.post(wmReinstall)
	}
}

func (b *winBackend) Latency() Latency {
	n := live.count.Load()
	l := Latency{Count: n, MaxUS: live.maxNS.Load() / 1000}
	if n > 0 {
		l.AvgUS = live.totalNS.Load() / int64(n) / 1000
	}
	return l
}

func watchdog(th *hookThread, stop <-chan struct{}) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-th.done:
			return
		case <-tick.C:
			if userIsActive() && hookIsSilent() && !probeAnswered() {
				th.post(wmReinstall)
			}
		}
	}
}

func userIsActive() bool {
	li := lastInput{size: uint32(unsafe.Sizeof(lastInput{}))}
	if r, _, _ := pGetLastInputInfo.Call(ptr(&li)); r == 0 {
		return false
	}
	now, _, _ := pGetTickCount.Call()
	return uint32(now)-li.time < 3000
}

func hookIsSilent() bool {
	return time.Since(time.Unix(0, live.lastEvent.Load())) > 10*time.Second
}

func probeAnswered() bool {
	before := live.probeSeen.Load()
	sendInputs([]input{probeInput(false), probeInput(true)})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if live.probeSeen.Load() >= before+2 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
