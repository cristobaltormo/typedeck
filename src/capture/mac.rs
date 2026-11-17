use std::collections::HashMap;
use std::io::{BufRead, BufReader, Write};
use std::path::PathBuf;
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::mpsc;
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;

use super::{Backend, CaptureError, Check, Filter, Latency, Snapshot};
use crate::hid::Stroke;
use crate::sync::{Flag, lock};

pub const HELPER_NAME: &str = "typedeck-capture";
const HELPER_START_TIMEOUT: Duration = Duration::from_secs(5);
const HELPER_CHECK_TIMEOUT: Duration = Duration::from_secs(5);
const LAYOUT_ANSI: &str = "ansi";
const LAYOUT_ISO: &str = "iso";
const CODE_ISO_SECTION: u8 = 10;
const CODE_GRAVE: u8 = 50;
const CODE_CAPS_LOCK: u8 = 57;
pub const CODE_RETURN: u8 = 36;
pub const CODE_TAB: u8 = 48;
const MAC_TAP_MS: u32 = 12;

struct Keymap {
    code: HashMap<u8, u8>,
    usage: HashMap<u8, u8>,
}

fn keymap() -> &'static Keymap {
    static K: OnceLock<Keymap> = OnceLock::new();
    K.get_or_init(|| {
        let mut m: HashMap<u8, u8> = HashMap::new();
        let runs: [(u8, &[u8]); 6] = [
            (0x04, &[0, 11, 8, 2, 14, 3, 5, 4, 34, 38, 40, 37, 46, 45, 31, 35, 12, 15, 1, 17, 32, 9, 13, 7, 16, 6]),
            (0x1E, &[18, 19, 20, 21, 23, 22, 26, 28, 25, 29]),
            (0x3A, &[122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111]),
            (0x68, &[105, 107, 113, 106, 64, 79, 80, 90]),
            (0x59, &[83, 84, 85, 86, 87, 88, 89, 91, 92, 82]),
            (0xE0, &[59, 56, 58, 55, 62, 60, 61, 54]),
        ];
        for (start, codes) in runs {
            for (i, c) in codes.iter().enumerate() {
                m.insert(start + i as u8, *c);
            }
        }
        for (u, c) in [
            (0x28, 36),
            (0x29, 53),
            (0x2A, 51),
            (0x2B, 48),
            (0x2C, 49),
            (0x2D, 27),
            (0x2E, 24),
            (0x2F, 33),
            (0x30, 30),
            (0x31, 42),
            (0x32, 42),
            (0x33, 41),
            (0x34, 39),
            (0x36, 43),
            (0x37, 47),
            (0x38, 44),
            (0x39, CODE_CAPS_LOCK),
            (0x49, 114),
            (0x4A, 115),
            (0x4B, 116),
            (0x4C, 117),
            (0x4D, 119),
            (0x4E, 121),
            (0x4F, 124),
            (0x50, 123),
            (0x51, 125),
            (0x52, 126),
            (0x53, 71),
            (0x54, 75),
            (0x55, 67),
            (0x56, 78),
            (0x57, 69),
            (0x58, 76),
            (0x63, 65),
            (0x65, 110),
            (0x67, 81),
            (0x85, 95),
            (0x87, 94),
            (0x89, 93),
            (0x90, 104),
            (0x91, 102),
        ] {
            m.insert(u, c);
        }
        let usage = m.iter().filter(|(u, _)| **u != 0x32).map(|(u, c)| (*c, *u)).collect();
        Keymap { code: m, usage }
    })
}

fn iso_like(layout: &str) -> bool {
    layout != LAYOUT_ANSI
}

pub fn mac_keycode(usage: u8, layout: &str) -> Option<u8> {
    match usage {
        0x35 => Some(if iso_like(layout) { CODE_ISO_SECTION } else { CODE_GRAVE }),
        0x64 => Some(if iso_like(layout) { CODE_GRAVE } else { CODE_ISO_SECTION }),
        _ => keymap().code.get(&usage).copied(),
    }
}

pub fn mac_usage(code: u8, layout: &str) -> u8 {
    match code {
        CODE_ISO_SECTION => {
            if iso_like(layout) {
                0x35
            } else {
                0x64
            }
        }
        CODE_GRAVE => {
            if iso_like(layout) {
                0x64
            } else {
                0x35
            }
        }
        _ => keymap().usage.get(&code).copied().unwrap_or(0),
    }
}

pub fn keycode_mask(s: &Snapshot, layout: &str) -> [u8; 16] {
    let mut out = [0u8; 16];
    for u in 0..=255u8 {
        if s.mask[usize::from(u >> 3)] >> (u & 7) & 1 == 0 {
            continue;
        }
        if let Some(c) = mac_keycode(u, layout)
            && c < 128
            && mac_usage(c, layout) == u
        {
            out[usize::from(c >> 3)] |= 1 << (c & 7);
        }
    }
    out
}

const MOD_DEVICE: [u64; 8] = [0x1, 0x2, 0x20, 0x8, 0x2000, 0x4, 0x40, 0x10];
const FLAG_SHIFT: u64 = 0x20000;
const FLAG_CONTROL: u64 = 0x40000;
const FLAG_OPTION: u64 = 0x80000;
const FLAG_COMMAND: u64 = 0x100000;
const MOD_GENERIC: [u64; 8] = [FLAG_CONTROL, FLAG_SHIFT, FLAG_OPTION, FLAG_COMMAND, FLAG_CONTROL, FLAG_SHIFT, FLAG_OPTION, FLAG_COMMAND];

pub fn mods_from_flags(flags: u64) -> u8 {
    let mut mods = 0u8;
    for (bit, f) in MOD_DEVICE.iter().enumerate() {
        if flags & f != 0 {
            mods |= 1 << bit;
        }
    }
    if mods != 0 {
        return mods;
    }
    for (bit, f) in MOD_GENERIC.iter().take(4).enumerate() {
        if flags & f != 0 {
            mods |= 1 << bit;
        }
    }
    mods
}

#[cfg(all(test, unix))]
pub fn flags_from_mods(mods: u8) -> u64 {
    (0..8).filter(|b| mods & (1 << b) != 0).fold(0, |acc, b| acc | MOD_DEVICE[b] | MOD_GENERIC[b])
}

pub fn consumer_key_type(usage: u16) -> Option<u8> {
    Some(match usage {
        0xE9 => 0,
        0xEA => 1,
        0x6F => 2,
        0x70 => 3,
        0xE2 => 7,
        0xCD => 16,
        0xB5 => 17,
        0xB6 => 18,
        0xB3 => 19,
        0xB4 => 20,
        _ => return None,
    })
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct KeyStep {
    pub down: bool,
    pub code: u8,
    pub flags: u64,
}

pub fn inject_layout(typing: Option<&str>, keyboard: &str) -> String {
    if typing == Some("es-iso") { LAYOUT_ISO.into() } else { keyboard.into() }
}

pub fn stroke_plan(s: Stroke, layout: &str) -> Result<(Vec<KeyStep>, Vec<KeyStep>), String> {
    let mut acc = 0u64;
    let mut mods = Vec::new();
    for bit in 0..8u8 {
        if s.mods & (1 << bit) != 0 {
            acc |= MOD_DEVICE[usize::from(bit)] | MOD_GENERIC[usize::from(bit)];
            let code = keymap().code.get(&(0xE0 + bit)).copied().unwrap_or(0);
            mods.push(KeyStep { down: true, code, flags: acc });
        }
    }
    let mut down = mods.clone();
    let mut up = Vec::new();
    if s.usage != 0 {
        let c = mac_keycode(s.usage, layout).ok_or_else(|| format!("key {:02X} has no macOS equivalent", s.usage))?;
        let mut flags = acc;
        if (0xE0..=0xE7).contains(&s.usage) {
            let b = usize::from(s.usage - 0xE0);
            flags |= MOD_DEVICE[b] | MOD_GENERIC[b];
        }
        down.push(KeyStep { down: true, code: c, flags });
        up.push(KeyStep { down: false, code: c, flags: acc });
    }
    for i in (0..mods.len()).rev() {
        let rest = if i > 0 { mods[i - 1].flags } else { 0 };
        up.push(KeyStep { down: false, code: mods[i].code, flags: rest });
    }
    Ok((down, up))
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct HelperMsg {
    pub kind: String,
    pub code: u8,
    pub flags: u64,
    pub text: String,
    pub counts: [u64; 3],
}

pub fn parse_helper_line(line: &str) -> Option<HelperMsg> {
    let f: Vec<&str> = line.split_whitespace().collect();
    let kind = *f.first()?;
    let mut m = HelperMsg { kind: kind.to_string(), ..Default::default() };
    match kind {
        "D" | "U" | "P" | "R" => {
            let code: u8 = f.get(1)?.parse().ok().filter(|c| *c <= 127)?;
            m.code = code;
            if kind == "D" {
                m.flags = u64::from_str_radix(f.get(2)?, 16).ok()?;
            }
            Some(m)
        }
        "READY" | "KBTYPE" | "TAPOFF" => {
            m.text = f.get(1).map(|s| s.to_string()).unwrap_or_default();
            Some(m)
        }
        "TAPON" => Some(m),
        "ERR" => {
            m.text = line.trim_start().strip_prefix("ERR").unwrap_or("").trim().to_string();
            Some(m)
        }
        "LAT" => {
            if f.len() != 4 {
                return None;
            }
            for i in 0..3 {
                m.counts[i] = f[i + 1].parse().ok()?;
            }
            Some(m)
        }
        _ => None,
    }
}

pub fn config_line(s: &Snapshot, layout: &str) -> String {
    let mask = keycode_mask(s, layout);
    format!("CFG {} {} {}", crate::crypto::hex(&mask), u8::from(s.report_all), u8::from(!s.inactive))
}

pub fn key_line(st: KeyStep) -> String {
    format!("KEY {} {} {:x}", if st.down { "d" } else { "u" }, st.code, st.flags)
}

pub fn text_line(text: &str) -> String {
    format!("TEXT {}", crate::crypto::hex(text.as_bytes()))
}

pub fn media_line(key_type: u8) -> String {
    format!("MEDIA {key_type}")
}

pub fn parse_check(out: &str) -> HashMap<String, String> {
    out.lines().filter_map(|l| l.split_once(' ')).map(|(k, v)| (k.to_string(), v.trim().to_string())).collect()
}

#[cfg(target_os = "macos")]
pub fn helper_path() -> PathBuf {
    let exe = std::env::current_exe().ok().map(|e| std::fs::canonicalize(&e).unwrap_or(e));
    match exe.as_ref().and_then(|e| e.parent()) {
        Some(dir) => dir.join(HELPER_NAME),
        None => PathBuf::from(HELPER_NAME),
    }
}

fn permission_error(text: &str) -> CaptureError {
    let mut missing = Vec::new();
    if !text.contains("accessibility=1") {
        missing.push("Accessibility");
    }
    if !text.contains("input_monitoring=1") {
        missing.push("Input Monitoring");
    }
    CaptureError::new(
        "permission",
        format!("macOS has not allowed {HELPER_NAME} to read the keyboard: turn it on under {} in System Settings > Privacy & Security", missing.join(" and ")),
    )
}

fn missing_error(path: &std::path::Path) -> CaptureError {
    CaptureError::new("helper_missing", format!("{HELPER_NAME} was not found at {}: it must sit next to the typedeck binary", path.display()))
}

struct HelperProc {
    stdin: Arc<Mutex<Option<ChildStdin>>>,
    child: Arc<Mutex<Child>>,
    done: Arc<Flag>,
}

impl HelperProc {
    fn start(path: &std::path::Path, args: &[&str], on_line: Box<dyn Fn(&str) + Send>, on_exit: Box<dyn FnOnce() + Send>) -> std::io::Result<HelperProc> {
        let mut cmd = Command::new(path);
        cmd.args(args).stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::null());
        crate::platform::apply_env(&mut cmd);
        let mut child = cmd.spawn()?;
        let stdin = Arc::new(Mutex::new(child.stdin.take()));
        let stdout = child.stdout.take();
        let child = Arc::new(Mutex::new(child));
        let done = Arc::new(Flag::new());
        let (c2, d2, s2) = (child.clone(), done.clone(), stdin.clone());
        crate::sync::spawn("capture-helper", move || {
            if let Some(out) = stdout {
                let mut r = BufReader::new(out);
                let mut line = Vec::new();
                loop {
                    line.clear();
                    match r.read_until(b'\n', &mut line) {
                        Ok(0) | Err(_) => break,
                        Ok(_) => on_line(String::from_utf8_lossy(&line).trim_end_matches(['\r', '\n'])),
                    }
                }
            }
            lock(&s2).take();
            let deadline = std::time::Instant::now() + Duration::from_secs(2);
            while let Ok(None) = lock(&c2).try_wait() {
                if std::time::Instant::now() > deadline {
                    let _ = lock(&c2).kill();
                }
                std::thread::sleep(Duration::from_millis(20));
            }
            on_exit();
            d2.set();
        });
        Ok(HelperProc { stdin, child, done })
    }

    fn send(&self, lines: &[String]) -> Result<(), String> {
        let mut g = lock(&self.stdin);
        let Some(w) = g.as_mut() else {
            return Err("the capture helper is not running".into());
        };
        let mut buf = String::new();
        for l in lines {
            buf.push_str(l);
            buf.push('\n');
        }
        w.write_all(buf.as_bytes()).and_then(|_| w.flush()).map_err(|e| e.to_string())
    }

    fn alive(&self) -> bool {
        !self.done.is_set()
    }

    fn stop(&self) {
        let _ = self.send(&["QUIT".into()]);
        lock(&self.stdin).take();
        if !self.done.wait(Duration::from_secs(2)) {
            let _ = lock(&self.child).kill();
            self.done.wait(Duration::from_secs(2));
        }
    }
}

struct Running {
    proc: Arc<HelperProc>,
    filter: Arc<Filter>,
}

pub struct MacBackend {
    path: Box<dyn Fn() -> PathBuf + Send + Sync>,
    run: Mutex<Option<Running>>,
    layout: Arc<Mutex<String>>,
    prompted: AtomicBool,
    inject: Mutex<()>,
    lat: Arc<[AtomicU64; 3]>,
    tap_resets: Arc<AtomicU64>,
}

impl MacBackend {
    pub fn new(path: Box<dyn Fn() -> PathBuf + Send + Sync>) -> MacBackend {
        MacBackend {
            path,
            run: Mutex::new(None),
            layout: Arc::new(Mutex::new(LAYOUT_ISO.into())),
            prompted: AtomicBool::new(false),
            inject: Mutex::new(()),
            lat: Arc::new(Default::default()),
            tap_resets: Arc::new(AtomicU64::new(0)),
        }
    }

    fn current_layout(&self) -> String {
        lock(&self.layout).clone()
    }

    fn inject_layout(&self) -> String {
        inject_layout(cfg!(target_os = "macos").then(|| crate::platform::get().system_layout()).as_deref(), &self.current_layout())
    }

    fn send(&self, lines: &[String]) -> Result<(), String> {
        let proc = lock(&self.run).as_ref().map(|r| r.proc.clone());
        match proc {
            Some(p) if p.alive() => p.send(lines),
            _ => Err(CaptureError::unsupported().msg),
        }
    }

    fn run_check(path: &std::path::Path) -> Result<HashMap<String, String>, String> {
        let mut cmd = Command::new(path);
        cmd.arg("--check");
        let out = crate::platform::run_command(cmd, HELPER_CHECK_TIMEOUT, "")?;
        Ok(parse_check(&out))
    }
}

fn permission_check(name: &str, granted: bool, path: &std::path::Path) -> Check {
    if granted {
        return Check::ok("", format!("{name} is allowed for {HELPER_NAME}"));
    }
    Check::fail(
        "",
        format!(
            "{name} is not allowed: open System Settings > Privacy & Security > {name} and turn on {HELPER_NAME} (use + and choose {} if it is not listed); it is asked for when software capture is first turned on",
            path.display()
        ),
    )
}

impl Backend for MacBackend {
    fn start(&self, f: &Arc<Filter>) -> Result<(), CaptureError> {
        let mut run = lock(&self.run);
        if run.as_ref().is_some_and(|r| r.proc.alive()) {
            return Ok(());
        }
        let path = (self.path)();
        if !path.exists() {
            return Err(missing_error(&path));
        }
        let mut args = vec!["run"];
        if !self.prompted.swap(true, Ordering::SeqCst) {
            args.push("--prompt");
        }
        let (first_tx, first_rx) = mpsc::channel::<HelperMsg>();
        let first_tx = Mutex::new(Some(first_tx));
        let (filter, layout, lat, resets) = (f.clone(), self.layout.clone(), self.lat.clone(), self.tap_resets.clone());
        let resend = Arc::new(Mutex::new(None::<Arc<HelperProc>>));
        let resend_slot = resend.clone();
        let on_line = Box::new(move |line: &str| {
            let Some(m) = parse_helper_line(line) else { return };
            match m.kind.as_str() {
                "READY" | "ERR" => {
                    if let Some(tx) = lock(&first_tx).take() {
                        let _ = tx.send(m);
                    }
                }
                "D" | "U" | "P" | "R" => {
                    let u = mac_usage(m.code, &lock(&layout));
                    if u != 0 {
                        filter.observe(m.kind.as_bytes()[0], u, mods_from_flags(m.flags));
                    }
                }
                "KBTYPE" => {
                    *lock(&layout) = m.text.clone();
                    if let Some(p) = lock(&resend_slot).clone() {
                        let _ = p.send(&[config_line(&filter.snapshot(), &m.text)]);
                    }
                }
                "TAPOFF" => {
                    resets.fetch_add(1, Ordering::SeqCst);
                    filter.release_all();
                }
                "LAT" => {
                    for i in 0..3 {
                        lat[i].store(m.counts[i], Ordering::SeqCst);
                    }
                }
                _ => {}
            }
        });
        let exit_filter = f.clone();
        let proc = HelperProc::start(&path, &args, on_line, Box::new(move || exit_filter.release_all()))
            .map_err(|e| CaptureError::new("hook_failed", e.to_string()))?;
        let proc = Arc::new(proc);
        let first = match first_rx.recv_timeout(HELPER_START_TIMEOUT) {
            Ok(m) => m,
            Err(mpsc::RecvTimeoutError::Timeout) => {
                proc.stop();
                return Err(CaptureError::new("hook_failed", "the capture helper did not start"));
            }
            Err(mpsc::RecvTimeoutError::Disconnected) => {
                proc.stop();
                return Err(CaptureError::new("hook_failed", "the capture helper stopped before it was ready"));
            }
        };
        if first.kind != "READY" {
            proc.stop();
            if first.text.starts_with("permission") {
                return Err(permission_error(&first.text));
            }
            return Err(CaptureError::new("hook_failed", format!("capture helper: {}", first.text)));
        }
        *lock(&self.layout) = first.text.clone();
        *lock(&resend) = Some(proc.clone());
        let _ = proc.send(&[config_line(&f.snapshot(), &first.text)]);
        let (p2, f2, l2) = (proc.clone(), f.clone(), self.layout.clone());
        crate::sync::spawn("capture-config", move || {
            while p2.alive() {
                if f2.changed().wait(Duration::from_millis(500)) {
                    let _ = p2.send(&[config_line(&f2.snapshot(), &lock(&l2))]);
                }
            }
            lock(&resend).take();
        });
        *run = Some(Running { proc, filter: f.clone() });
        Ok(())
    }

    fn alive(&self) -> bool {
        lock(&self.run).as_ref().is_some_and(|r| r.proc.alive())
    }

    fn stop(&self) {
        let r = lock(&self.run).take();
        if let Some(r) = r {
            r.proc.stop();
            r.filter.release_all();
        }
    }

    fn inject(&self, s: Stroke, hold_ms: u32) -> Result<(), String> {
        let _g = lock(&self.inject);
        if s.usage == 0x39 && s.mods == 0 {
            return self.send(&["CAPS".into()]);
        }
        let (down, up) = stroke_plan(s, &self.inject_layout())?;
        if down.is_empty() {
            return Ok(());
        }
        self.send(&down.into_iter().map(key_line).collect::<Vec<_>>())?;
        std::thread::sleep(Duration::from_millis(u64::from(if hold_ms == 0 { MAC_TAP_MS } else { hold_ms })));
        self.send(&up.into_iter().map(key_line).collect::<Vec<_>>())
    }

    fn consumer(&self, usage: u16) -> Result<(), String> {
        let k = consumer_key_type(usage).ok_or_else(|| format!("media key {usage:X} has no macOS equivalent"))?;
        let _g = lock(&self.inject);
        self.send(&[media_line(k)])
    }

    fn type_text(&self, text: &str) -> Result<(), String> {
        let _g = lock(&self.inject);
        let mut lines = Vec::new();
        let mut run = String::new();
        for c in text.chars() {
            let code = match c {
                '\n' | '\r' => CODE_RETURN,
                '\t' => CODE_TAB,
                _ => {
                    run.push(c);
                    continue;
                }
            };
            if !run.is_empty() {
                lines.push(text_line(&std::mem::take(&mut run)));
            }
            lines.push(key_line(KeyStep { down: true, code, flags: 0 }));
            lines.push(key_line(KeyStep { down: false, code, flags: 0 }));
        }
        if !run.is_empty() {
            lines.push(text_line(&run));
        }
        if lines.is_empty() {
            return Ok(());
        }
        self.send(&lines)
    }

    fn latency(&self) -> Latency {
        Latency {
            count: self.lat[0].load(Ordering::SeqCst),
            avg_us: self.lat[1].load(Ordering::SeqCst) as i64,
            max_us: self.lat[2].load(Ordering::SeqCst) as i64,
        }
    }

    fn checks(&self) -> Vec<Check> {
        let path = (self.path)();
        if !path.exists() {
            return vec![Check::fail("", format!("{}; the macOS archive includes it, or build it with 'make macos-helpers'", missing_error(&path).msg))];
        }
        let state = match MacBackend::run_check(&path) {
            Ok(s) => s,
            Err(e) => return vec![Check::fail("", format!("{HELPER_NAME} does not run: {e}"))],
        };
        let mut checks = vec![Check::ok("", format!("{HELPER_NAME} found at {}", path.display()))];
        if state.get("tap").map(String::as_str) == Some("1") {
            checks.push(Check::ok("", format!("macOS lets {HELPER_NAME} read and take keys: the keyboard tap installs and removes cleanly")));
        } else {
            checks.push(permission_check("Accessibility", state.get("accessibility").map(String::as_str) == Some("1"), &path));
            checks.push(permission_check("Input Monitoring", state.get("input_monitoring").map(String::as_str) == Some("1"), &path));
            checks.push(Check::fail("", "the keyboard tap cannot be installed yet; it needs both permissions above"));
        }
        checks.push(Check::fail(
            "",
            "optional: keys typed into password fields are hidden from every keyboard tool by macOS (secure input), so macros do not run there",
        ));
        checks
    }
}

#[cfg(all(test, unix))]
mod tests;
