mod keyboard;
mod session;
mod sys;
mod uinput;

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use session::{LatencyStats, Session, no_read_access, probe_keyboards, uinput_error};

use crate::capture::{Backend, CaptureError, Check, Filter, Latency, consumer_evdev, evdev_from_usage};
use crate::hid::{MOD_CTRL, MOD_SHIFT, Stroke};
use crate::platform;
use crate::sync::lock;

const DEFAULT_TAP_MS: u32 = 12;

pub struct LinuxBackend {
    sess: Mutex<Option<Arc<Session>>>,
    lat: Arc<LatencyStats>,
    inject: Arc<Mutex<()>>,
}

impl Default for LinuxBackend {
    fn default() -> Self {
        Self::new()
    }
}

impl LinuxBackend {
    pub fn new() -> LinuxBackend {
        LinuxBackend { sess: Mutex::new(None), lat: Arc::new(LatencyStats::default()), inject: Arc::new(Mutex::new(())) }
    }

    fn current(&self) -> Option<Arc<Session>> {
        lock(&self.sess).clone()
    }
}

pub fn stroke_codes(st: Stroke) -> Result<Vec<u16>, String> {
    let mut codes = Vec::new();
    for bit in 0..8u8 {
        if st.mods & (1 << bit) != 0
            && let Some(c) = evdev_from_usage(0xE0 + bit)
        {
            codes.push(c);
        }
    }
    if st.usage != 0 {
        codes.push(evdev_from_usage(st.usage).ok_or_else(|| format!("key {:02X} has no Linux equivalent", st.usage))?);
    }
    Ok(codes)
}

fn press_codes(s: &Session, lockm: &Mutex<()>, codes: &[u16], hold_ms: u32) -> Result<(), String> {
    let _g = lock(lockm);
    let err = s.virt().and_then(|v| v.keys(codes, 1)).err();
    std::thread::sleep(Duration::from_millis(u64::from(if hold_ms == 0 { DEFAULT_TAP_MS } else { hold_ms })));
    let mut rev = codes.to_vec();
    rev.reverse();
    let rel = s.virt().and_then(|v| v.keys(&rev, 0)).err();
    match err.or(rel) {
        Some(e) => Err(e.to_string()),
        None => Ok(()),
    }
}

impl Backend for LinuxBackend {
    fn start(&self, f: &Arc<Filter>) -> Result<(), CaptureError> {
        let mut g = lock(&self.sess);
        if g.as_ref().is_some_and(|s| s.alive()) {
            return Ok(());
        }
        if let Some(old) = g.take() {
            old.close();
        }
        let s = Session::start(f.clone(), self.lat.clone())?;
        *g = Some(s.clone());
        // Ctrl+Z in the terminal that started typedeck would freeze it while it holds the keyboard.
        sys::ignore_tstp(true);
        let inject = self.inject.clone();
        let weak = Arc::downgrade(&s);
        platform::set_paste_keys(Some(Box::new(move |shift: bool| {
            let s = weak.upgrade().ok_or_else(|| "software capture is not running".to_string())?;
            let mods = MOD_CTRL | if shift { MOD_SHIFT } else { 0 };
            press_codes(&s, &inject, &stroke_codes(Stroke::new(0x19, mods))?, 0)
        })));
        Ok(())
    }

    fn alive(&self) -> bool {
        self.current().is_some_and(|s| s.alive())
    }

    fn stop(&self) {
        let s = lock(&self.sess).take();
        platform::set_paste_keys(None);
        sys::ignore_tstp(false);
        if let Some(s) = s {
            s.close();
        }
    }

    fn inject(&self, s: Stroke, hold_ms: u32) -> Result<(), String> {
        let codes = stroke_codes(s)?;
        if codes.is_empty() {
            return Ok(());
        }
        let sess = self.current().ok_or_else(|| crate::capture::software::NOT_RUNNING.to_string())?;
        press_codes(&sess, &self.inject, &codes, hold_ms)
    }

    fn consumer(&self, usage: u16) -> Result<(), String> {
        let code = consumer_evdev(usage).ok_or_else(|| format!("media key {usage:X} has no Linux equivalent"))?;
        let sess = self.current().ok_or_else(|| crate::capture::software::NOT_RUNNING.to_string())?;
        let _g = lock(&self.inject);
        let a = sess.virt().and_then(|v| v.keys(&[code], 1)).err();
        let b = sess.virt().and_then(|v| v.keys(&[code], 0)).err();
        a.or(b).map_or(Ok(()), |e| Err(e.to_string()))
    }

    fn type_text(&self, text: &str) -> Result<(), String> {
        platform::get().paste(text)
    }

    fn latency(&self) -> Latency {
        self.lat.snapshot()
    }

    fn checks(&self) -> Vec<Check> {
        let mut checks = vec![uinput_check(), keyboards_check()];
        if let Some(s) = self.current() {
            checks.push(grab_check(&s.grab_failures()));
        }
        checks.push(clipboard_check());
        checks
    }

    fn warnings(&self) -> Vec<String> {
        match self.current() {
            Some(s) if !s.grab_failures().is_empty() => vec!["grab_failed".into()],
            _ => vec![],
        }
    }
}

pub fn grab_check(failed: &BTreeMap<String, String>) -> Check {
    if failed.is_empty() {
        return Check::ok("grab", "every keyboard was taken for capture");
    }
    let paths: Vec<&str> = failed.keys().map(String::as_str).collect();
    Check::fail("grab_failed", format!("could not take {} for capture (another program may hold it): keys typed there are not captured", paths.join(", ")))
}

fn uinput_check() -> Check {
    match uinput::open_uinput() {
        Ok(_) => Check::ok("uinput", "/dev/uinput can be opened, so keys can be passed on and sent"),
        Err(e) => Check::fail("uinput", uinput_error(&e).msg),
    }
}

fn keyboards_check() -> Check {
    match probe_keyboards() {
        (r, _) if r > 0 => Check::ok("keyboards", format!("{r} keyboard(s) readable in /dev/input")),
        (_, d) if d > 0 => Check::fail("input_access", no_read_access()),
        _ => Check::fail("no_keyboard", "no keyboard found in /dev/input"),
    }
}

fn clipboard_check() -> Check {
    let (tool, other) = if platform::env_var("XDG_SESSION_TYPE").as_deref() == Some("wayland") { ("wl-copy", "") } else { ("xclip", "xsel") };
    if [tool, other].iter().any(|n| !n.is_empty() && platform::have(n)) {
        return Check::ok("clipboard", "clipboard tool found, so text the layout cannot type can be pasted");
    }
    Check::fail("clipboard", format!("optional: {tool} is missing, so text with characters the keyboard layout cannot type cannot be pasted"))
}

#[cfg(test)]
mod tests;
