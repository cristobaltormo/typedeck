use std::collections::BTreeMap;
use std::io;
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd};
use std::path::Path;
use std::sync::atomic::{AtomicI64, AtomicU64, Ordering};
use std::sync::{Arc, Mutex, Weak};
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

use super::keyboard::Keyboard;
use super::sys::*;
use super::uinput::VirtualKeyboard;
use crate::capture::{CaptureError, Filter, Latency};
use crate::sync::{Flag, lock};

pub const INPUT_DIR: &str = "/dev/input";
const SETTLE_WAIT: Duration = Duration::from_millis(150);
const RETRY_WAIT: Duration = Duration::from_secs(3);
pub const ACCESS_HINT: &str =
    "add your user to the input group ('sudo usermod -aG input $USER') and sign in again; the package installs the udev rule that gives that group access";

pub fn no_read_access() -> String {
    format!("no permission to read the keyboards in /dev/input: {ACCESS_HINT}")
}

pub fn uinput_error(e: &io::Error) -> CaptureError {
    match e.raw_os_error() {
        Some(libc::ENOENT) => CaptureError::new("no_uinput", "the uinput module is not loaded: run 'sudo modprobe uinput'"),
        Some(libc::EACCES) | Some(libc::EPERM) => CaptureError::new("uinput_access", format!("no permission to open /dev/uinput: {ACCESS_HINT}")),
        _ => CaptureError::new("hook_failed", e.to_string()),
    }
}

#[derive(Default)]
pub struct LatencyStats {
    count: AtomicU64,
    total_ns: AtomicI64,
    max_ns: AtomicI64,
}

impl LatencyStats {
    pub fn record(&self, d: Duration) {
        let ns = d.as_nanos() as i64;
        self.count.fetch_add(1, Ordering::Relaxed);
        self.total_ns.fetch_add(ns, Ordering::Relaxed);
        self.max_ns.fetch_max(ns, Ordering::Relaxed);
    }

    pub fn snapshot(&self) -> Latency {
        let n = self.count.load(Ordering::Relaxed);
        let mut out = Latency { count: n, avg_us: 0, max_us: self.max_ns.load(Ordering::Relaxed) / 1000 };
        if n > 0 {
            out.avg_us = self.total_ns.load(Ordering::Relaxed) / n as i64 / 1000;
        }
        out
    }
}

pub fn input_paths() -> Vec<String> {
    let mut out: Vec<String> = std::fs::read_dir(INPUT_DIR)
        .map(|rd| {
            rd.flatten().filter_map(|e| e.file_name().into_string().ok()).filter(|n| n.starts_with("event")).map(|n| format!("{INPUT_DIR}/{n}")).collect()
        })
        .unwrap_or_default();
    out.sort();
    out
}

fn denied(e: &io::Error) -> bool {
    matches!(e.raw_os_error(), Some(libc::EACCES) | Some(libc::EPERM))
}

pub fn probe_keyboards() -> (usize, usize) {
    let (mut readable, mut denied_n) = (0, 0);
    for p in input_paths() {
        match open(Path::new(&p)) {
            Ok(fd) => {
                if is_keyboard(fd.as_raw_fd()) {
                    readable += 1;
                }
            }
            Err(e) if denied(&e) => denied_n += 1,
            Err(_) => {}
        }
    }
    (readable, denied_n)
}

struct Board {
    fd: Arc<OwnedFd>,
}

pub struct Session {
    me: Weak<Session>,
    filter: Arc<Filter>,
    virt: Option<VirtualKeyboard>,
    lat: Arc<LatencyStats>,
    stop: StopPipe,
    done: Flag,
    closed: Mutex<bool>,
    boards: Mutex<BTreeMap<String, Board>>,
    grab_fail: Mutex<BTreeMap<String, String>>,
    threads: Mutex<Vec<JoinHandle<()>>>,
}

pub type ProbeFn = fn() -> (usize, usize);
pub type MakeVirtualFn = fn(super::uinput::LedFn) -> io::Result<VirtualKeyboard>;

impl Session {
    pub fn start(filter: Arc<Filter>, lat: Arc<LatencyStats>) -> Result<Arc<Session>, CaptureError> {
        Session::start_with(filter, lat, probe_keyboards, VirtualKeyboard::new)
    }

    pub fn start_with(filter: Arc<Filter>, lat: Arc<LatencyStats>, probe: ProbeFn, make_virtual: MakeVirtualFn) -> Result<Arc<Session>, CaptureError> {
        let (readable, denied_n) = probe();
        if readable == 0 && denied_n > 0 {
            return Err(CaptureError::new("input_access", no_read_access()));
        }
        let stop = StopPipe::new().map_err(|e| CaptureError::new("hook_failed", e.to_string()))?;
        let slot: Arc<Mutex<Weak<Session>>> = Arc::new(Mutex::new(Weak::new()));
        let led_slot = slot.clone();
        let virt = make_virtual(Arc::new(move |code, value| {
            if let Some(s) = lock(&led_slot).upgrade() {
                s.set_led(code, value);
            }
        }))
        .map_err(|e| uinput_error(&e))?;
        let s = Arc::new_cyclic(|me| Session {
            me: me.clone(),
            filter,
            virt: Some(virt),
            lat,
            stop,
            done: Flag::new(),
            closed: Mutex::new(false),
            boards: Mutex::new(BTreeMap::new()),
            grab_fail: Mutex::new(BTreeMap::new()),
            threads: Mutex::new(Vec::new()),
        });
        *lock(&slot) = Arc::downgrade(&s);
        let (_, denied_now) = s.scan();
        let watch = match watch_input_dir() {
            Ok(w) => w,
            Err(e) => {
                s.close();
                return Err(CaptureError::new("hook_failed", e.to_string()));
            }
        };
        let me = s.clone();
        if let Some(h) = crate::sync::spawn("input-watch", move || me.watch(watch, denied_now > 0)) {
            lock(&s.threads).push(h);
        }
        Ok(s)
    }

    pub fn filter(&self) -> &Filter {
        &self.filter
    }

    pub fn virt(&self) -> io::Result<&VirtualKeyboard> {
        self.virt.as_ref().ok_or_else(|| io::Error::from(io::ErrorKind::NotConnected))
    }

    pub fn latency(&self) -> &LatencyStats {
        &self.lat
    }

    pub fn stop_fd(&self) -> i32 {
        self.stop.fd()
    }

    pub fn alive(&self) -> bool {
        !self.done.is_set()
    }

    fn scan(&self) -> (usize, usize) {
        let (mut opened, mut denied_n) = (0, 0);
        for path in input_paths() {
            if lock(&self.boards).contains_key(&path) {
                opened += 1;
                continue;
            }
            match open(Path::new(&path)) {
                Err(e) if denied(&e) => denied_n += 1,
                Err(_) => {}
                Ok(fd) if !is_keyboard(fd.as_raw_fd()) => {}
                Ok(fd) => {
                    self.add(path, fd);
                    opened += 1;
                }
            }
        }
        (opened, denied_n)
    }

    fn add(&self, path: String, fd: OwnedFd) {
        if *lock(&self.closed) {
            return;
        }
        let fd = Arc::new(fd);
        lock(&self.boards).insert(path.clone(), Board { fd: fd.clone() });
        let Some(me) = self.me.upgrade() else { return };
        let kb = Keyboard::new(path.clone(), fd.clone());
        let h = crate::sync::spawn("keyboard", move || {
            kb.run(&me);
            let mut boards = lock(&me.boards);
            if boards.get(&path).is_some_and(|b| Arc::ptr_eq(&b.fd, &fd)) {
                boards.remove(&path);
            }
        });
        if let Some(h) = h {
            let mut threads = lock(&self.threads);
            threads.retain(|t| !t.is_finished());
            threads.push(h);
        }
    }

    pub fn grab_failed(&self, path: &str, e: &io::Error) {
        lock(&self.grab_fail).insert(path.to_string(), e.to_string());
    }

    pub fn grabbed(&self, path: &str) {
        lock(&self.grab_fail).remove(path);
    }

    pub fn grab_failures(&self) -> BTreeMap<String, String> {
        lock(&self.grab_fail).clone()
    }

    fn set_led(&self, code: u16, value: i32) {
        let fds: Vec<Arc<OwnedFd>> = lock(&self.boards).values().map(|b| b.fd.clone()).collect();
        for fd in fds {
            let _ = write_events(fd.as_raw_fd(), &[InputEvent::new(EV_LED, code, value), InputEvent::syn()]);
        }
    }

    fn watch(&self, w: OwnedFd, retry: bool) {
        let mut deadline = retry.then(|| Instant::now() + RETRY_WAIT);
        let mut buf = [0u8; 4096];
        loop {
            let timeout = deadline.map(|d| d.saturating_duration_since(Instant::now()).as_millis() as i32).unwrap_or(-1);
            let Some(ready) = wait_readable(&[w.as_raw_fd(), self.stop.fd()], timeout) else { break };
            if ready[1] {
                break;
            }
            if ready[0] {
                while unsafe { libc::read(w.as_raw_fd(), buf.as_mut_ptr().cast(), buf.len()) } > 0 {}
                deadline = Some(Instant::now() + SETTLE_WAIT);
                continue;
            }
            if deadline.is_some_and(|d| Instant::now() >= d) {
                let (_, denied_n) = self.scan();
                deadline = (denied_n > 0).then(|| Instant::now() + RETRY_WAIT);
            }
        }
        self.done.set();
    }

    pub fn close(&self) {
        {
            let mut closed = lock(&self.closed);
            if *closed {
                return;
            }
            *closed = true;
        }
        self.stop.fire();
        let threads = std::mem::take(&mut *lock(&self.threads));
        for t in threads {
            let _ = t.join();
        }
        self.done.set();
    }

    #[cfg(test)]
    pub fn for_test(filter: Arc<Filter>) -> Arc<Session> {
        Arc::new_cyclic(|me| Session {
            me: me.clone(),
            filter,
            virt: None,
            lat: Arc::new(LatencyStats::default()),
            stop: StopPipe::new().expect("pipe"),
            done: Flag::new(),
            closed: Mutex::new(false),
            boards: Mutex::new(BTreeMap::new()),
            grab_fail: Mutex::new(BTreeMap::new()),
            threads: Mutex::new(Vec::new()),
        })
    }
}

fn watch_input_dir() -> io::Result<OwnedFd> {
    let fd = unsafe { libc::inotify_init1(libc::IN_NONBLOCK | libc::IN_CLOEXEC) };
    if fd < 0 {
        return Err(io::Error::last_os_error());
    }
    let fd = unsafe { OwnedFd::from_raw_fd(fd) };
    let dir = std::ffi::CString::new(INPUT_DIR).map_err(|_| io::Error::from(io::ErrorKind::InvalidInput))?;
    if unsafe { libc::inotify_add_watch(fd.as_raw_fd(), dir.as_ptr(), libc::IN_CREATE | libc::IN_DELETE | libc::IN_ATTRIB) } < 0 {
        return Err(io::Error::last_os_error());
    }
    Ok(fd)
}
