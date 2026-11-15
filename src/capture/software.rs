use std::sync::{Arc, Mutex, Weak};
use std::time::{Duration, SystemTime};

use super::{CaptureError, Check, Filter, HandlerSlot, Handlers, Info, Msg, SOFTWARE, Source, Status};
use crate::hid::Stroke;
use crate::sync::{Flag, Signal, lock};

pub const NOT_RUNNING: &str = "software capture is not running";
const RETRY_BASE: Duration = Duration::from_secs(4);
const RETRY_CEILING: Duration = Duration::from_secs(120);

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Latency {
    pub count: u64,
    pub avg_us: i64,
    pub max_us: i64,
}

pub trait Backend: Send + Sync {
    fn start(&self, f: &Arc<Filter>) -> Result<(), CaptureError>;
    fn alive(&self) -> bool;
    fn stop(&self);
    fn inject(&self, s: Stroke, hold_ms: u32) -> Result<(), String>;
    fn consumer(&self, usage: u16) -> Result<(), String>;
    fn type_text(&self, text: &str) -> Result<(), String>;
    fn latency(&self) -> Latency;
    fn checks(&self) -> Vec<Check>;
    fn warnings(&self) -> Vec<String> {
        vec![]
    }
    fn reinstall(&self) {}
}

#[derive(Default)]
struct State {
    wanted: bool,
    running: bool,
    since: Option<SystemTime>,
    last_err: String,
}

pub struct SoftwareSource {
    me: Weak<SoftwareSource>,
    be: Box<dyn Backend>,
    filter: Arc<Filter>,
    life: Mutex<()>,
    st: Mutex<State>,
    handlers: HandlerSlot,
    wake: Signal,
    stop: Flag,
    retry: Mutex<(Duration, Duration)>,
}

pub fn next_retry(cur: Duration, ceiling: Duration) -> Duration {
    (cur * 2).min(ceiling)
}

impl SoftwareSource {
    pub fn new(be: Box<dyn Backend>) -> Arc<SoftwareSource> {
        Arc::new_cyclic(|me| SoftwareSource {
            me: me.clone(),
            be,
            filter: Arc::new(Filter::new()),
            life: Mutex::new(()),
            st: Mutex::new(State::default()),
            handlers: HandlerSlot::default(),
            wake: Signal::new(),
            stop: Flag::new(),
            retry: Mutex::new((RETRY_BASE, RETRY_CEILING)),
        })
    }

    pub fn set_retry(&self, base: Duration, ceiling: Duration) {
        *lock(&self.retry) = (base, ceiling);
    }

    pub fn filter(&self) -> &Arc<Filter> {
        &self.filter
    }

    pub fn backend(&self) -> &dyn Backend {
        self.be.as_ref()
    }

    pub fn checks(&self) -> Vec<Check> {
        self.be.checks()
    }

    fn wanted(&self) -> bool {
        lock(&self.st).wanted
    }

    fn running(&self) -> bool {
        lock(&self.st).running
    }

    fn supervise(&self, wait: Duration) -> Duration {
        let (base, ceiling) = *lock(&self.retry);
        let _g = lock(&self.life);
        if self.wanted() && self.running() && !self.be.alive() {
            self.halt();
        }
        if !self.wanted() || self.running() || self.start().is_ok() {
            return base;
        }
        next_retry(wait, ceiling)
    }

    fn pump(&self) {
        let q = self.filter.queue().clone();
        while !self.stop.is_set() {
            match q.pop(Duration::from_millis(500)) {
                Some(Msg::Event(ev)) => self.handlers.event(ev),
                Some(Msg::Up(true)) => self.handlers.connected(&Info::system_keyboard()),
                Some(Msg::Up(false)) => self.handlers.disconnected(),
                None => {}
            }
        }
    }

    fn notify(&self, up: bool) {
        self.filter.queue().push(Msg::Up(up));
    }

    fn start(&self) -> Result<(), CaptureError> {
        if self.running() {
            return Ok(());
        }
        if let Err(e) = self.be.start(&self.filter) {
            lock(&self.st).last_err = e.code.to_string();
            return Err(e);
        }
        {
            let mut st = lock(&self.st);
            st.running = true;
            st.since = Some(SystemTime::now());
            st.last_err.clear();
        }
        if let Some(m) = self.handlers.mask() {
            self.filter.set_mask(m);
        }
        self.notify(true);
        Ok(())
    }

    fn halt(&self) {
        let was = std::mem::replace(&mut lock(&self.st).running, false);
        if !was {
            return;
        }
        self.be.stop();
        self.filter.release_all();
        self.notify(false);
    }
}

impl Source for SoftwareSource {
    fn kind(&self) -> &'static str {
        SOFTWARE
    }

    fn bind(&self, h: Handlers) {
        self.handlers.set(h);
    }

    fn run(&self) {
        if let Some(me) = self.me.upgrade() {
            crate::sync::spawn("software-events", move || me.pump());
        }
        let mut wait = lock(&self.retry).0;
        while !self.stop.is_set() {
            self.wake.wait(wait);
            if self.stop.is_set() {
                return;
            }
            wait = self.supervise(wait);
        }
    }

    fn close(&self) {
        if self.stop.is_set() {
            return;
        }
        self.stop.set();
        self.wake.notify();
        self.disable();
    }

    fn status(&self) -> Status {
        let st = lock(&self.st);
        if !st.running {
            return Status::default();
        }
        Status { connected: true, port: String::new(), info: Some(Info::system_keyboard()), since: st.since }
    }

    fn connected(&self) -> bool {
        self.running()
    }

    fn error(&self) -> String {
        let st = lock(&self.st);
        if st.running { String::new() } else { st.last_err.clone() }
    }

    fn sync_mask(&self, _force: bool) {
        if let Some(m) = self.handlers.mask() {
            self.filter.set_mask(m);
        }
    }

    fn keys(&self, on: bool) -> Result<(), String> {
        self.filter.set_report_all(on);
        Ok(())
    }

    fn pause(&self) {
        self.filter.pause();
    }

    fn resume(&self) {
        self.filter.resume();
        if self.running() {
            self.be.reinstall();
        }
    }

    fn tap_held(&self, s: Stroke, hold_ms: u32) -> Result<(), String> {
        if !self.connected() {
            return Err(NOT_RUNNING.into());
        }
        self.be.inject(s, hold_ms)
    }

    fn consumer(&self, usage: u16) -> Result<(), String> {
        if !self.connected() {
            return Err(NOT_RUNNING.into());
        }
        self.be.consumer(usage)
    }

    fn type_text(&self, text: &str) -> Result<(), String> {
        if !self.connected() {
            return Err(NOT_RUNNING.into());
        }
        self.be.type_text(text)
    }

    fn command(&self, name: &str, _arg: i64) -> Result<Vec<String>, String> {
        match name {
            "ping" => Ok(vec!["PONG".into()]),
            "stats" => {
                let l = self.be.latency();
                Ok(vec![format!(
                    "n={} avg_us={} max_us={} capture={} dropped={}",
                    l.count,
                    l.avg_us,
                    l.max_us,
                    u8::from(self.connected()),
                    self.filter.dropped()
                )])
            }
            _ => Err("command not available without a board".into()),
        }
    }

    fn warnings(&self) -> Vec<String> {
        if self.running() { self.be.warnings() } else { vec![] }
    }

    fn enable(&self) -> Result<(), String> {
        let _g = lock(&self.life);
        lock(&self.st).wanted = true;
        self.start().map_err(|e| e.msg)
    }

    fn disable(&self) {
        let _g = lock(&self.life);
        lock(&self.st).wanted = false;
        self.halt();
    }
}
