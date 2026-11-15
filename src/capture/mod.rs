mod evdev_codes;
mod filter;
mod mac;
mod scancodes;
mod selector;
mod software;

#[cfg(target_os = "linux")]
mod linux;
#[cfg(windows)]
mod windows;

use std::sync::{Arc, Mutex};
use std::time::SystemTime;

use serde::Serialize;

use crate::hid::{Report, Stroke};
use crate::jsonx::{is_false, is_zero_i64};
use crate::sync::lock;

pub use evdev_codes::{consumer_evdev, evdev_from_usage, evdev_table, usage_from_evdev};
pub use filter::{Filter, Msg, Snapshot};
pub use mac::{HELPER_NAME, MacBackend, mac_keycode, mac_usage, mods_from_flags};
pub use scancodes::{ScanKey, consumer_vk, scan_from_usage, usage_from_scan};
pub use selector::Selector;
pub use software::{Backend, Latency, SoftwareSource};

pub const BOARD: &str = "board";
pub const SOFTWARE: &str = "software";
pub const SYSTEM_KEYBOARD_ID: &str = "system";
pub const NEEDS_BOARD: &str = "not available without a board";

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Event {
    pub kind: u8,
    pub usage: u8,
    pub mods: u8,
    pub arg: u8,
}

impl Event {
    pub fn new(kind: u8, usage: u8, mods: u8) -> Event {
        Event { kind, usage, mods, arg: 0 }
    }
}

fn is_empty(s: &str) -> bool {
    s.is_empty()
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
pub struct Info {
    pub firmware: String,
    pub present: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub system: bool,
    #[serde(skip_serializing_if = "is_empty")]
    pub vid: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub pid: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub bcd: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub usb: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub class: String,
    #[serde(skip_serializing_if = "is_zero_i64")]
    pub ep0: i64,
    #[serde(skip_serializing_if = "is_empty")]
    pub mfr: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub prod: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub serial: String,
    #[serde(skip_serializing_if = "is_zero_i64")]
    pub cfglen: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    pub ifaces: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    pub power_ma: i64,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub hid_len: Vec<i64>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub reports: Vec<Report>,
    #[serde(skip)]
    pub raw_rdesc: Vec<String>,
}

impl Info {
    pub fn id(&self) -> String {
        if self.system {
            return SYSTEM_KEYBOARD_ID.into();
        }
        if self.vid.is_empty() {
            return String::new();
        }
        format!("{}:{}", self.vid, self.pid).to_uppercase()
    }

    pub fn system_keyboard() -> Info {
        Info { present: true, system: true, prod: "System keyboard".into(), ..Default::default() }
    }
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct Sys {
    pub mcu: String,
    pub board: String,
    pub f_cpu: i64,
    pub fw: String,
    pub vcc_mv: i64,
    pub free_ram: i64,
    #[serde(rename = "max3421e_rev")]
    pub max3421_rev: String,
    pub uptime_s: i64,
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Check {
    pub ok: bool,
    pub code: String,
    pub message: String,
    pub problem: String,
}

impl Check {
    pub fn ok(code: &str, message: impl Into<String>) -> Check {
        Check { ok: true, code: code.into(), message: message.into(), problem: String::new() }
    }

    pub fn fail(code: &str, problem: impl Into<String>) -> Check {
        Check { ok: false, code: code.into(), message: String::new(), problem: problem.into() }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct CaptureError {
    pub code: &'static str,
    pub msg: String,
}

impl CaptureError {
    pub fn new(code: &'static str, msg: impl Into<String>) -> CaptureError {
        CaptureError { code, msg: msg.into() }
    }

    pub fn unsupported() -> CaptureError {
        CaptureError::new("unsupported", "software capture is not available on this system")
    }
}

impl std::fmt::Display for CaptureError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.msg)
    }
}

#[derive(Clone, Debug, Default)]
pub struct Status {
    pub connected: bool,
    pub port: String,
    pub info: Option<Info>,
    pub since: Option<SystemTime>,
}

pub type EventFn = Arc<dyn Fn(Event) + Send + Sync>;
pub type MaskFn = Arc<dyn Fn() -> [u8; 32] + Send + Sync>;
pub type ConnectedFn = Arc<dyn Fn(&Info) + Send + Sync>;
pub type DisconnectFn = Arc<dyn Fn() + Send + Sync>;

#[derive(Clone, Default)]
pub struct Handlers {
    pub on_event: Option<EventFn>,
    pub mask: Option<MaskFn>,
    pub on_connected: Option<ConnectedFn>,
    pub on_disconnect: Option<DisconnectFn>,
}

#[derive(Default)]
pub struct HandlerSlot(Mutex<Handlers>);

impl HandlerSlot {
    pub fn set(&self, h: Handlers) {
        *lock(&self.0) = h;
    }

    pub fn get(&self) -> Handlers {
        lock(&self.0).clone()
    }

    pub fn event(&self, ev: Event) {
        if let Some(f) = self.get().on_event {
            f(ev);
        }
    }

    pub fn mask(&self) -> Option<[u8; 32]> {
        self.get().mask.map(|f| f())
    }

    pub fn connected(&self, i: &Info) {
        if let Some(f) = self.get().on_connected {
            f(i);
        }
    }

    pub fn disconnected(&self) {
        if let Some(f) = self.get().on_disconnect {
            f();
        }
    }
}

pub type RawReply = (Vec<String>, Option<String>);

pub trait Source: Send + Sync {
    fn kind(&self) -> &'static str;
    fn bind(&self, h: Handlers);
    fn run(&self);
    fn close(&self);
    fn status(&self) -> Status;
    fn connected(&self) -> bool {
        self.status().connected
    }
    fn error(&self) -> String;
    fn sync_mask(&self, force: bool);
    fn keys(&self, on: bool) -> Result<(), String>;
    fn pause(&self);
    fn resume(&self);
    fn tap(&self, s: Stroke) -> Result<(), String> {
        self.tap_held(s, 0)
    }
    fn tap_held(&self, s: Stroke, hold_ms: u32) -> Result<(), String>;
    fn consumer(&self, usage: u16) -> Result<(), String>;
    fn command(&self, _name: &str, _arg: i64) -> Result<Vec<String>, String> {
        Err(NEEDS_BOARD.into())
    }
    fn raw(&self, _cmd: &str, _multi: bool, _timeout: std::time::Duration) -> RawReply {
        (vec![], Some(NEEDS_BOARD.into()))
    }
    fn sys(&self) -> Result<Sys, String> {
        Err(NEEDS_BOARD.into())
    }
    fn set_layer(&self, _index: usize) {}
    fn warnings(&self) -> Vec<String> {
        vec![]
    }
    fn type_text(&self, _text: &str) -> Result<(), String> {
        Err("not supported".into())
    }
    fn select(&self, _kind: &str) -> Result<(), String> {
        Err("this build cannot switch the capture mode".into())
    }
    fn active(&self) -> &'static str {
        BOARD
    }
    fn can_switch(&self) -> bool {
        false
    }
    fn enable(&self) -> Result<(), String> {
        Ok(())
    }
    fn disable(&self) {}
}

pub fn native_backend() -> Box<dyn Backend> {
    #[cfg(target_os = "linux")]
    return Box::new(linux::LinuxBackend::new());
    #[cfg(windows)]
    return Box::new(windows::WinBackend::new());
    #[cfg(target_os = "macos")]
    return Box::new(MacBackend::new(Box::new(mac::helper_path)));
}

pub fn new_software() -> Arc<SoftwareSource> {
    SoftwareSource::new(native_backend())
}

#[cfg(test)]
mod tests;
