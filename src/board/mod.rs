mod protocol;
mod serial;

use std::io;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, Weak};
use std::time::{Duration, Instant, SystemTime};

pub use protocol::{board_ports, is_board_key, key_values, parse_event, parse_info, parse_sys, parse_usb_key};
pub use serial::{SerialConnector, open_port};

use crate::capture::{BOARD, HandlerSlot, Handlers, Info, RawReply, Source, Status, Sys};
use crate::hid::Stroke;
use crate::queue::Queue;
use crate::sync::{Flag, lock};

pub const NO_BOARD: &str = "board not found";
pub const OUTDATED: &str = "the board has outdated firmware";
const ZERO_MASK: &str = "0000000000000000000000000000000000000000000000000000000000000000";

pub trait Port: Send + Sync {
    fn read(&self, buf: &mut [u8]) -> io::Result<usize>;
    fn write_all(&self, b: &[u8]) -> io::Result<()>;
    fn close(&self);
}

pub enum OpenError {
    NotFound,
    Outdated(String),
}

pub trait Connector: Send + Sync {
    fn open(&self) -> Result<(Arc<dyn Port>, String), OpenError>;
}

#[derive(Default)]
struct State {
    port: Option<Arc<dyn Port>>,
    name: String,
    info: Option<Info>,
    since: Option<SystemTime>,
    last_mask: String,
    last_err: String,
    generation: u64,
}

pub struct Board {
    me: Weak<Board>,
    conn: Box<dyn Connector>,
    handlers: HandlerSlot,
    st: Mutex<State>,
    wmu: Mutex<()>,
    cmu: Mutex<()>,
    notify: Mutex<()>,
    replies: Queue<String>,
    stop: Flag,
    paused: AtomicBool,
    log: Box<dyn Fn(&str) + Send + Sync>,
}

impl Board {
    pub fn new(conn: Box<dyn Connector>) -> Arc<Board> {
        Board::with_log(conn, Box::new(|_| {}))
    }

    pub fn with_log(conn: Box<dyn Connector>, log: Box<dyn Fn(&str) + Send + Sync>) -> Arc<Board> {
        Arc::new_cyclic(|me| Board {
            me: me.clone(),
            conn,
            handlers: HandlerSlot::default(),
            st: Mutex::new(State::default()),
            wmu: Mutex::new(()),
            cmu: Mutex::new(()),
            notify: Mutex::new(()),
            replies: Queue::new(64),
            stop: Flag::new(),
            paused: AtomicBool::new(false),
            log,
        })
    }

    fn write(&self, line: &str) -> Result<(), String> {
        let port = lock(&self.st).port.clone().ok_or_else(|| NO_BOARD.to_string())?;
        let _g = lock(&self.wmu);
        port.write_all(format!("{line}\n").as_bytes()).map_err(|e| e.to_string())
    }

    fn serve(&self, port: Arc<dyn Port>, name: String) {
        let generation = {
            let mut st = lock(&self.st);
            let generation = st.generation + 1;
            *st = State { port: Some(port.clone()), name: name.clone(), since: Some(SystemTime::now()), generation, ..Default::default() };
            generation
        };
        (self.log)(&format!("connected to {name}"));
        let done = Arc::new(Flag::new());
        if let Some(me) = self.me.upgrade() {
            let (p, d) = (port.clone(), done.clone());
            let reader = me.clone();
            crate::sync::spawn("board-read", move || {
                reader.read_loop(p.as_ref());
                d.set();
            });
            crate::sync::spawn("board-handshake", move || me.handshake(generation));
        }
        while !self.stop.is_set() {
            if done.wait(Duration::from_secs(1)) || self.stop.is_set() {
                break;
            }
            if self.write("HB").is_err() {
                break;
            }
        }
        port.close();
        let _n = lock(&self.notify);
        {
            let mut st = lock(&self.st);
            st.port = None;
            st.name.clear();
            st.generation += 1;
        }
        (self.log)("disconnected");
        self.handlers.disconnected();
    }

    fn read_loop(&self, port: &dyn Port) {
        let mut buf = [0u8; 4096];
        let mut line: Vec<u8> = Vec::new();
        loop {
            let n = match port.read(&mut buf) {
                Ok(0) | Err(_) => return,
                Ok(n) => n,
            };
            for &b in &buf[..n] {
                if b != b'\n' {
                    if line.len() < 64 * 1024 {
                        line.push(b);
                    }
                    continue;
                }
                let text = String::from_utf8_lossy(&line).trim().to_string();
                line.clear();
                if text.is_empty() {
                    continue;
                }
                match parse_event(&text) {
                    Some(ev) => {
                        if ev.kind == b'K'
                            && ev.arg == 1
                            && let Some(me) = self.me.upgrade()
                        {
                            crate::sync::spawn("board-info", move || me.refresh_info());
                        }
                        self.handlers.event(ev);
                    }
                    None => self.replies.push_dropping_oldest(text),
                }
            }
        }
    }

    pub fn do_cmd(&self, cmd: &str, multi: bool, timeout: Duration) -> RawReply {
        let _g = lock(&self.cmu);
        self.replies.clear();
        if let Err(e) = self.write(cmd) {
            return (vec![], Some(e));
        }
        let mut out = Vec::new();
        let deadline = Instant::now() + timeout;
        loop {
            if self.stop.is_set() {
                return (out, Some(NO_BOARD.into()));
            }
            let now = Instant::now();
            if now >= deadline {
                if multi && !out.is_empty() {
                    return (out, Some("timed out waiting for END".into()));
                }
                let first = cmd.split_whitespace().next().unwrap_or("");
                return (out, Some(format!("the board did not answer \"{first}\"")));
            }
            let Some(l) = self.replies.pop((deadline - now).min(Duration::from_millis(100))) else { continue };
            if !multi {
                return (vec![l], None);
            }
            if l == "END" {
                return (out, None);
            }
            out.push(l);
        }
    }

    fn checked(&self, cmd: &str) -> Result<Vec<String>, String> {
        match self.do_cmd(cmd, false, Duration::from_secs(1)) {
            (_, Some(e)) => Err(e),
            (lines, None) => Ok(lines),
        }
    }

    fn current(&self, generation: u64) -> bool {
        let st = lock(&self.st);
        st.generation == generation && st.port.is_some()
    }

    fn handshake(&self, generation: u64) {
        let (lines, err) = self.do_cmd("WHO", false, Duration::from_secs(1));
        if !self.current(generation) {
            return;
        }
        let Some(fw) = lines.first().and_then(|l| l.strip_prefix(crate::app::FIRMWARE_ID)).filter(|_| err.is_none()) else {
            (self.log)(&format!("the board does not speak the expected protocol: {lines:?} {}", err.unwrap_or_default()));
            return;
        };
        {
            let mut st = lock(&self.st);
            if st.generation != generation {
                return;
            }
            st.info = Some(Info { firmware: fw.trim().to_string(), ..Default::default() });
        }
        self.refresh_info();
        self.sync_mask(true);
        let _n = lock(&self.notify);
        let info = {
            let st = lock(&self.st);
            if st.generation != generation || st.port.is_none() {
                return;
            }
            st.info.clone()
        };
        if let Some(info) = info {
            self.handlers.connected(&info);
        }
    }

    fn refresh_info(&self) {
        let (lines, err) = self.do_cmd("INFO", true, Duration::from_secs(2));
        if err.is_some() && lines.is_empty() {
            return;
        }
        let mut info = parse_info(&lines);
        info.firmware = lock(&self.st).info.as_ref().map(|i| i.firmware.clone()).unwrap_or_default();
        for (i, n) in info.hid_len.clone().into_iter().enumerate() {
            if n <= 0 {
                continue;
            }
            let (rd, err) = self.do_cmd(&format!("RDESC {i} {n}"), true, Duration::from_secs(3));
            if err.is_some() && rd.is_empty() {
                continue;
            }
            let hex = rd.join(" ");
            if let Ok(reps) = crate::hid::parse_report_descriptor_hex(&hex) {
                for mut r in reps {
                    r.detail = format!("interface {i}: {}", r.detail);
                    info.reports.push(r);
                }
            }
            info.raw_rdesc.push(hex);
        }
        lock(&self.st).info = Some(info);
    }

    pub fn watch(&self, on: bool) -> Result<(), String> {
        self.checked(&format!("WATCH {}", u8::from(on))).map(|_| ())
    }
}

impl Source for Board {
    fn kind(&self) -> &'static str {
        BOARD
    }

    fn bind(&self, h: Handlers) {
        self.handlers.set(h);
    }

    fn run(&self) {
        while !self.stop.is_set() {
            match self.conn.open() {
                Ok((port, name)) => {
                    self.serve(port, name);
                    if self.stop.wait(Duration::from_secs(2)) {
                        return;
                    }
                }
                Err(e) => {
                    {
                        let mut st = lock(&self.st);
                        match e {
                            OpenError::Outdated(name) => {
                                st.last_err = "outdated_firmware".into();
                                st.name = name;
                            }
                            OpenError::NotFound => st.last_err = "not_found".into(),
                        }
                    }
                    if self.stop.wait(Duration::from_secs(4)) {
                        return;
                    }
                }
            }
        }
    }

    fn close(&self) {
        let _ = self.write(&format!("MASK {ZERO_MASK}"));
        self.stop.set();
        if let Some(p) = lock(&self.st).port.clone() {
            p.close();
        }
    }

    fn status(&self) -> Status {
        let st = lock(&self.st);
        Status { connected: st.port.is_some(), port: st.name.clone(), info: st.info.clone(), since: st.since }
    }

    fn error(&self) -> String {
        let st = lock(&self.st);
        if st.port.is_some() { String::new() } else { st.last_err.clone() }
    }

    fn sync_mask(&self, force: bool) {
        if self.paused.load(Ordering::SeqCst) {
            return;
        }
        let Some(m) = self.handlers.mask() else { return };
        let hex = crate::crypto::hex_upper(&m);
        if !force && lock(&self.st).last_mask == hex {
            return;
        }
        let (lines, err) = self.do_cmd(&format!("MASK {hex}"), false, Duration::from_secs(1));
        if err.is_none() && lines.len() == 1 && lines[0] == "OK" {
            lock(&self.st).last_mask = hex;
        }
    }

    fn keys(&self, on: bool) -> Result<(), String> {
        self.checked(&format!("KEYS {}", u8::from(on))).map(|_| ())
    }

    fn pause(&self) {
        if self.paused.swap(true, Ordering::SeqCst) {
            return;
        }
        if self.write(&format!("MASK {ZERO_MASK}")).is_ok() {
            lock(&self.st).last_mask = ZERO_MASK.into();
        }
    }

    fn resume(&self) {
        if self.paused.swap(false, Ordering::SeqCst) {
            self.sync_mask(true);
        }
    }

    fn tap_held(&self, s: Stroke, hold_ms: u32) -> Result<(), String> {
        let mut line = format!("KEY {:02X} {:02X}", s.mods, s.usage);
        if hold_ms > 0 {
            line.push_str(&format!(" {hold_ms}"));
        }
        self.checked(&line).map(|_| ())
    }

    fn consumer(&self, usage: u16) -> Result<(), String> {
        self.checked(&format!("CONS {usage:X}")).map(|_| ())
    }

    fn command(&self, name: &str, arg: i64) -> Result<Vec<String>, String> {
        let line = match name {
            "ping" => "PING".to_string(),
            "who" => "WHO".into(),
            "stats" => "STATS".into(),
            "bus" => "BUS".into(),
            "bootlog" => "BOOTLOG".into(),
            "reboot" => "REBOOT".into(),
            "dark" => format!("DARK {}", arg.clamp(0, 1)),
            "vbus" => format!("VBUS {}", arg.clamp(0, 1)),
            "layer" => format!("LAYER {}", arg.clamp(0, 8)),
            "leds" => format!("LEDS {}", arg.clamp(0, 7)),
            _ => return Err("command not allowed".into()),
        };
        self.checked(&line)
    }

    fn raw(&self, cmd: &str, multi: bool, timeout: Duration) -> RawReply {
        self.do_cmd(cmd, multi, timeout)
    }

    fn sys(&self) -> Result<Sys, String> {
        let lines = self.checked("SYS")?;
        Ok(lines.first().map(|l| parse_sys(l)).unwrap_or_default())
    }

    fn set_layer(&self, index: usize) {
        let _ = self.command("layer", index as i64);
    }
}

#[cfg(test)]
mod tests;
