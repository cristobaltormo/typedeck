use std::sync::atomic::{AtomicBool, AtomicI64, AtomicU32, AtomicU64, Ordering};
use std::sync::{Arc, Mutex, mpsc};
use std::time::{Duration, Instant, SystemTime};

use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, LPARAM, LRESULT, WPARAM};
use windows_sys::Win32::Security::{GetTokenInformation, TOKEN_ELEVATION, TOKEN_QUERY, TokenElevation};
use windows_sys::Win32::System::RemoteDesktop::ProcessIdToSessionId;
use windows_sys::Win32::System::SystemInformation::GetTickCount;
use windows_sys::Win32::System::Threading::{GetCurrentProcess, GetCurrentProcessId, GetCurrentThreadId, OpenProcessToken};
use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
    GetLastInputInfo, INPUT, INPUT_0, INPUT_KEYBOARD, KEYBD_EVENT_FLAGS, KEYBDINPUT, KEYEVENTF_EXTENDEDKEY, KEYEVENTF_KEYUP, KEYEVENTF_SCANCODE,
    KEYEVENTF_UNICODE, LASTINPUTINFO, SendInput,
};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    CallNextHookEx, DispatchMessageW, GetMessageW, HC_ACTION, HHOOK, KBDLLHOOKSTRUCT, LLKHF_EXTENDED, LLKHF_INJECTED, LLKHF_LOWER_IL_INJECTED, MSG,
    PostThreadMessageW, SetWindowsHookExW, TranslateMessage, UnhookWindowsHookEx, WH_KEYBOARD_LL, WM_APP, WM_KEYDOWN, WM_QUIT, WM_SYSKEYDOWN,
};

use super::{Backend, CaptureError, Check, Filter, Latency, ScanKey, consumer_vk, scan_from_usage, usage_from_scan};
use crate::hid::Stroke;
use crate::sync::{Flag, lock};

const INJECT_TAG: usize = 0x5444434B;
const PROBE_TAG: usize = 0x54445052;
const WM_REINSTALL: u32 = WM_APP + 1;
const DEFAULT_TAP_MS: u32 = 12;

static FILTER: Mutex<Option<Arc<Filter>>> = Mutex::new(None);
static LAST_EVENT: AtomicI64 = AtomicI64::new(0);
static PROBE_SEEN: AtomicU64 = AtomicU64::new(0);
static COUNT: AtomicU64 = AtomicU64::new(0);
static TOTAL_NS: AtomicI64 = AtomicI64::new(0);
static MAX_NS: AtomicI64 = AtomicI64::new(0);
static INJECT: Mutex<()> = Mutex::new(());

fn now_nanos() -> i64 {
    crate::clock::unix_millis(SystemTime::now()) * 1_000_000
}

fn handle_key(msg: WPARAM, k: &KBDLLHOOKSTRUCT) -> bool {
    let start = Instant::now();
    LAST_EVENT.store(now_nanos(), Ordering::Relaxed);
    if k.dwExtraInfo == PROBE_TAG {
        PROBE_SEEN.fetch_add(1, Ordering::SeqCst);
        return false;
    }
    if k.flags & (LLKHF_INJECTED | LLKHF_LOWER_IL_INJECTED) != 0 {
        return false;
    }
    let Some(f) = lock(&FILTER).clone() else { return false };
    let usage = usage_from_scan(k.scanCode, k.flags & LLKHF_EXTENDED != 0, k.vkCode);
    let down = msg == WM_KEYDOWN as WPARAM || msg == WM_SYSKEYDOWN as WPARAM;
    let swallow = f.handle(usage, down);
    let ns = start.elapsed().as_nanos() as i64;
    COUNT.fetch_add(1, Ordering::Relaxed);
    TOTAL_NS.fetch_add(ns, Ordering::Relaxed);
    MAX_NS.fetch_max(ns, Ordering::Relaxed);
    swallow
}

unsafe extern "system" fn key_proc(code: i32, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
    if code == HC_ACTION as i32 && lparam != 0 {
        let k = unsafe { &*(lparam as *const KBDLLHOOKSTRUCT) };
        let swallow = crate::sync::catching(|| handle_key(wparam, k)).unwrap_or(false);
        if swallow {
            return 1;
        }
    }
    unsafe { CallNextHookEx(std::ptr::null_mut(), code, wparam, lparam) }
}

struct HookThread {
    tid: AtomicU32,
    cancel: AtomicBool,
    done: Flag,
}

impl HookThread {
    fn post(&self, msg: u32) {
        let tid = self.tid.load(Ordering::SeqCst);
        if tid != 0 {
            unsafe {
                PostThreadMessageW(tid, msg, 0, 0);
            }
        }
    }

    fn alive(&self) -> bool {
        !self.done.is_set()
    }
}

struct Hook(HHOOK);

impl Hook {
    fn install() -> Result<Hook, String> {
        let h = unsafe { SetWindowsHookExW(WH_KEYBOARD_LL, Some(key_proc), std::ptr::null_mut(), 0) };
        if h.is_null() {
            return Err(std::io::Error::last_os_error().to_string());
        }
        Ok(Hook(h))
    }
}

impl Drop for Hook {
    fn drop(&mut self) {
        unsafe {
            UnhookWindowsHookEx(self.0);
        }
    }
}

fn hook_loop(th: Arc<HookThread>, f: Arc<Filter>, ready: mpsc::Sender<Result<(), String>>) {
    th.tid.store(unsafe { GetCurrentThreadId() }, Ordering::SeqCst);
    let mut hook = match Hook::install() {
        Ok(h) => h,
        Err(e) => {
            let _ = ready.send(Err(e));
            th.done.set();
            return;
        }
    };
    if th.cancel.load(Ordering::SeqCst) {
        drop(hook);
        f.release_all();
        th.done.set();
        return;
    }
    LAST_EVENT.store(now_nanos(), Ordering::Relaxed);
    let _ = ready.send(Ok(()));
    let mut m: MSG = unsafe { std::mem::zeroed() };
    while !th.cancel.load(Ordering::SeqCst) {
        let r = unsafe { GetMessageW(&mut m, std::ptr::null_mut(), 0, 0) };
        if r <= 0 {
            break;
        }
        if m.hwnd.is_null() && m.message == WM_REINSTALL {
            drop(hook);
            f.release_all();
            hook = match Hook::install() {
                Ok(h) => h,
                Err(e) => {
                    crate::log!("capture: the keyboard hook could not be reinstalled: {e}");
                    f.release_all();
                    th.done.set();
                    return;
                }
            };
            continue;
        }
        unsafe {
            TranslateMessage(&m);
            DispatchMessageW(&m);
        }
    }
    drop(hook);
    f.release_all();
    th.done.set();
}

fn start_hook(f: &Arc<Filter>) -> Result<Arc<HookThread>, String> {
    let th = Arc::new(HookThread { tid: AtomicU32::new(0), cancel: AtomicBool::new(false), done: Flag::new() });
    let (tx, rx) = mpsc::channel();
    let (t2, f2) = (th.clone(), f.clone());
    std::thread::Builder::new().name("keyboard-hook".into()).spawn(move || hook_loop(t2, f2, tx)).map_err(|e| e.to_string())?;
    match rx.recv_timeout(Duration::from_secs(3)) {
        Ok(Ok(())) => Ok(th),
        Ok(Err(e)) => Err(e),
        Err(_) => {
            th.cancel.store(true, Ordering::SeqCst);
            let deadline = Instant::now() + Duration::from_secs(5);
            while Instant::now() < deadline && !th.done.wait(Duration::from_millis(50)) {
                th.post(WM_QUIT);
            }
            Err("the keyboard hook did not start".into())
        }
    }
}

fn send_inputs(inputs: &[INPUT]) -> Result<(), String> {
    if inputs.is_empty() {
        return Ok(());
    }
    let n = unsafe { SendInput(inputs.len() as u32, inputs.as_ptr(), std::mem::size_of::<INPUT>() as i32) };
    if n as usize != inputs.len() {
        return Err(format!("the system blocked the keystrokes ({})", std::io::Error::last_os_error()));
    }
    Ok(())
}

fn key_input(vk: u16, scan: u16, flags: KEYBD_EVENT_FLAGS, extra: usize) -> INPUT {
    INPUT { r#type: INPUT_KEYBOARD, Anonymous: INPUT_0 { ki: KEYBDINPUT { wVk: vk, wScan: scan, dwFlags: flags, time: 0, dwExtraInfo: extra } } }
}

fn scan_input(k: ScanKey, up: bool) -> INPUT {
    let mut flags = if k.code != 0 { KEYEVENTF_SCANCODE } else { 0 };
    if k.ext {
        flags |= KEYEVENTF_EXTENDEDKEY;
    }
    if up {
        flags |= KEYEVENTF_KEYUP;
    }
    key_input(k.vk, k.code, flags, INJECT_TAG)
}

fn probe_input(up: bool) -> INPUT {
    key_input(0xE8, 0, if up { KEYEVENTF_KEYUP } else { 0 }, PROBE_TAG)
}

pub fn stroke_keys(s: Stroke) -> Result<Vec<ScanKey>, String> {
    let mut keys = Vec::new();
    for bit in 0..8u8 {
        if s.mods & (1 << bit) != 0
            && let Some(k) = scan_from_usage(0xE0 + bit)
        {
            keys.push(k);
        }
    }
    if s.usage != 0 {
        keys.push(scan_from_usage(s.usage).ok_or_else(|| format!("key {:02X} has no Windows equivalent", s.usage))?);
    }
    Ok(keys)
}

fn text_inputs(c: char) -> Vec<INPUT> {
    match c {
        '\n' | '\r' => key_pair(0x28),
        '\t' => key_pair(0x2B),
        _ => {
            let mut buf = [0u16; 2];
            c.encode_utf16(&mut buf)
                .iter()
                .flat_map(|u| [key_input(0, *u, KEYEVENTF_UNICODE, INJECT_TAG), key_input(0, *u, KEYEVENTF_UNICODE | KEYEVENTF_KEYUP, INJECT_TAG)])
                .collect()
        }
    }
}

fn key_pair(usage: u8) -> Vec<INPUT> {
    let k = scan_from_usage(usage).unwrap_or_default();
    vec![scan_input(k, false), scan_input(k, true)]
}

fn user_is_active() -> bool {
    let mut li = LASTINPUTINFO { cbSize: std::mem::size_of::<LASTINPUTINFO>() as u32, dwTime: 0 };
    if unsafe { GetLastInputInfo(&mut li) } == 0 {
        return false;
    }
    unsafe { GetTickCount() }.wrapping_sub(li.dwTime) < 3000
}

fn hook_is_silent() -> bool {
    now_nanos() - LAST_EVENT.load(Ordering::Relaxed) > 10_000_000_000
}

fn probe_answered() -> bool {
    let before = PROBE_SEEN.load(Ordering::SeqCst);
    let _ = send_inputs(&[probe_input(false), probe_input(true)]);
    crate::sync::wait_until(Duration::from_millis(500), || PROBE_SEEN.load(Ordering::SeqCst) >= before + 2)
}

#[derive(Default)]
pub struct WinBackend {
    th: Mutex<Option<Arc<HookThread>>>,
    stop: Mutex<Option<Arc<Flag>>>,
}

impl WinBackend {
    pub fn new() -> WinBackend {
        WinBackend::default()
    }
}

impl Backend for WinBackend {
    fn start(&self, f: &Arc<Filter>) -> Result<(), CaptureError> {
        let mut g = lock(&self.th);
        if g.as_ref().is_some_and(|t| t.alive()) {
            return Ok(());
        }
        *lock(&FILTER) = Some(f.clone());
        let th = match start_hook(f) {
            Ok(t) => t,
            Err(e) => {
                *lock(&FILTER) = None;
                return Err(CaptureError::new("hook_failed", e));
            }
        };
        let stop = Arc::new(Flag::new());
        let (t2, s2) = (th.clone(), stop.clone());
        crate::sync::spawn("hook-watchdog", move || {
            while !s2.wait(Duration::from_secs(5)) && t2.alive() {
                if user_is_active() && hook_is_silent() && !probe_answered() {
                    t2.post(WM_REINSTALL);
                }
            }
        });
        *g = Some(th);
        *lock(&self.stop) = Some(stop);
        Ok(())
    }

    fn alive(&self) -> bool {
        lock(&self.th).as_ref().is_some_and(|t| t.alive())
    }

    fn stop(&self) {
        let th = lock(&self.th).take();
        if let Some(s) = lock(&self.stop).take() {
            s.set();
        }
        let Some(th) = th.filter(|t| t.alive()) else { return };
        *lock(&FILTER) = None;
        th.cancel.store(true, Ordering::SeqCst);
        th.post(WM_QUIT);
        th.done.wait(Duration::from_secs(2));
    }

    fn reinstall(&self) {
        if let Some(th) = lock(&self.th).as_ref() {
            th.post(WM_REINSTALL);
        }
    }

    fn inject(&self, s: Stroke, hold_ms: u32) -> Result<(), String> {
        let keys = stroke_keys(s)?;
        if keys.is_empty() {
            return Ok(());
        }
        let _g = lock(&INJECT);
        let down: Vec<INPUT> = keys.iter().map(|k| scan_input(*k, false)).collect();
        let up: Vec<INPUT> = keys.iter().rev().map(|k| scan_input(*k, true)).collect();
        let r = send_inputs(&down);
        std::thread::sleep(Duration::from_millis(u64::from(if hold_ms == 0 { DEFAULT_TAP_MS } else { hold_ms })));
        let rel = send_inputs(&up);
        r.and(rel)
    }

    fn consumer(&self, usage: u16) -> Result<(), String> {
        let vk = consumer_vk(usage).ok_or_else(|| format!("media key {usage:X} has no Windows equivalent"))?;
        let _g = lock(&INJECT);
        let k = ScanKey { code: 0, ext: true, vk };
        send_inputs(&[scan_input(k, false), scan_input(k, true)])
    }

    fn type_text(&self, text: &str) -> Result<(), String> {
        let _g = lock(&INJECT);
        let mut batch = Vec::new();
        for c in text.chars() {
            batch.extend(text_inputs(c));
            if batch.len() >= 128 {
                send_inputs(&batch)?;
                batch.clear();
            }
        }
        send_inputs(&batch)
    }

    fn latency(&self) -> Latency {
        let n = COUNT.load(Ordering::Relaxed);
        Latency { count: n, avg_us: if n > 0 { TOTAL_NS.load(Ordering::Relaxed) / n as i64 / 1000 } else { 0 }, max_us: MAX_NS.load(Ordering::Relaxed) / 1000 }
    }

    fn checks(&self) -> Vec<Check> {
        vec![session_check(), self.hook_check(), elevation_check()]
    }
}

impl WinBackend {
    fn hook_check(&self) -> Check {
        if self.alive() {
            return Check::ok("", "the keyboard hook is installed and running");
        }
        let f = Arc::new(Filter::new());
        match start_hook(&f) {
            Ok(th) => {
                th.cancel.store(true, Ordering::SeqCst);
                th.post(WM_QUIT);
                th.done.wait(Duration::from_secs(2));
                Check::ok("", "the keyboard hook installs and removes cleanly")
            }
            Err(e) => Check::fail("", format!("the keyboard hook cannot be installed: {e}")),
        }
    }
}

fn session_check() -> Check {
    let mut id = 0u32;
    unsafe {
        ProcessIdToSessionId(GetCurrentProcessId(), &mut id);
    }
    if id == 0 {
        return Check::fail("", "running in session 0 (a service): it has no keyboard; start it from your own session");
    }
    Check::ok("", format!("interactive session {id}"))
}

fn is_elevated() -> bool {
    let mut token: HANDLE = std::ptr::null_mut();
    if unsafe { OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) } == 0 {
        return false;
    }
    let mut el = TOKEN_ELEVATION { TokenIsElevated: 0 };
    let mut size = 0u32;
    let ok = unsafe {
        GetTokenInformation(token, TokenElevation, (&mut el as *mut TOKEN_ELEVATION).cast(), std::mem::size_of::<TOKEN_ELEVATION>() as u32, &mut size)
    } != 0;
    unsafe {
        CloseHandle(token);
    }
    ok && el.TokenIsElevated != 0
}

fn elevation_check() -> Check {
    if is_elevated() {
        return Check::ok("", "running elevated: administrator windows are visible too");
    }
    Check::fail("", "optional: not elevated, so keys typed in administrator windows (and the UAC prompt) are not seen; that is a Windows rule")
}
