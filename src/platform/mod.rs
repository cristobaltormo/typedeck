mod cmdquote;
mod klid;
mod lnk;
mod macquirks;
mod proc;
mod taskxml;
mod vk;

#[cfg(target_os = "linux")]
mod linux;
#[cfg(target_os = "macos")]
mod macos;
#[cfg(windows)]
mod windows;

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::{Mutex, OnceLock};

use serde::Serialize;

pub use cmdquote::quote_cmd_arg;
pub use klid::{klid_from_hkl, layout_from_klid};
pub use lnk::lnk_target;
pub use macquirks::{invert_mods, parse_modifier_mapping};
pub use proc::{detached, launch, run, run_command, run_out};
pub use taskxml::{task_xml, utf16_with_bom};
pub use vk::{is_extended_vk, vk_of};

#[cfg(target_os = "linux")]
pub use linux::set_paste_keys;
#[cfg(windows)]
pub use windows::{acquire_instance, attach_console, on_console_close, reg, show_error, start_shell, started_by_user};

use crate::hid::{Layout, Stroke};

pub const UNSUPPORTED: &str = "not available on this system";

#[derive(Clone, Debug, Default, PartialEq)]
pub struct PopupOptions {
    pub title: String,
    pub subtitle: String,
    pub seconds: f64,
    pub position: String,
    pub accent: String,
    pub dots: usize,
    pub active: usize,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct SystemInfo {
    pub os: String,
    pub typing_layout: String,
    pub keyboard_type: String,
    pub mod_remap: Vec<String>,
    pub keys_swapped: bool,
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct DoctorCheck {
    pub ok: bool,
    pub message: String,
    pub problem: String,
}

impl DoctorCheck {
    pub fn new(ok: bool, message: impl Into<String>, problem: impl Into<String>) -> DoctorCheck {
        DoctorCheck { ok, message: message.into(), problem: problem.into() }
    }
}

pub type StopFn = Box<dyn FnOnce() + Send>;

pub trait Platform: Send + Sync {
    fn name(&self) -> &'static str;
    fn version(&self) -> String;
    fn open_url(&self, url: &str) -> Result<(), String>;
    fn focus_editor(&self, url: &str) -> bool;
    fn open_app(&self, name: &str) -> Result<(), String>;
    fn quit_app(&self, name: &str) -> Result<(), String>;
    fn hide_app(&self, name: &str) -> Result<(), String>;
    fn front_id(&self) -> String;
    fn front_names(&self, id: &str) -> Vec<String>;
    fn installed_apps(&self) -> Vec<String>;
    fn soft_hotkey(&self, spec: &str) -> Result<(), String>;
    fn soft_stroke(&self, st: Stroke) -> Result<(), String>;
    fn paste(&self, text: &str) -> Result<(), String>;
    fn clipboard(&self) -> String;
    fn soft_media(&self, cmd: &str, step: i64) -> Result<String, String>;
    fn system(&self, cmd: &str) -> Result<(), String>;
    fn lock_stroke(&self) -> Option<Stroke>;
    fn keep_awake(&self) -> Result<StopFn, String>;
    fn notify(&self, title: &str, body: &str, sound: bool);
    fn popup(&self, o: &PopupOptions) -> bool;
    fn system_layout(&self) -> String;
    fn adapt_stroke(&self, st: Stroke, layout: &Layout) -> Stroke;
    fn info(&self, layout: &Layout) -> SystemInfo;
    fn shell(&self, cmd: &str) -> Command;
    fn quote_arg(&self, s: &str) -> String;
    fn log_file(&self) -> Option<PathBuf>;
    fn config_dir(&self) -> PathBuf;
    fn doctor(&self) -> Vec<DoctorCheck>;
    fn install_service(&self, exe: &Path) -> Result<String, String>;
    fn uninstall_service(&self) -> Result<String, String>;
}

pub fn get() -> &'static dyn Platform {
    static P: OnceLock<Box<dyn Platform>> = OnceLock::new();
    P.get_or_init(|| {
        #[cfg(target_os = "linux")]
        return Box::new(linux::Linux::new());
        #[cfg(target_os = "macos")]
        return Box::new(macos::Mac::new());
        #[cfg(windows)]
        return Box::new(windows::Windows::new());
    })
    .as_ref()
}

fn overlay() -> &'static Mutex<BTreeMap<String, String>> {
    static O: OnceLock<Mutex<BTreeMap<String, String>>> = OnceLock::new();
    O.get_or_init(|| Mutex::new(BTreeMap::new()))
}

pub fn env_var(name: &str) -> Option<String> {
    if let Some(v) = crate::sync::lock(overlay()).get(name) {
        return Some(v.clone());
    }
    std::env::var(name).ok().filter(|v| !v.is_empty())
}

pub fn set_env(name: &str, value: &str) {
    crate::sync::lock(overlay()).insert(name.to_string(), value.to_string());
}

pub fn apply_env(cmd: &mut Command) {
    for (k, v) in crate::sync::lock(overlay()).iter() {
        cmd.env(k, v);
    }
}

pub fn command(name: &str) -> Command {
    let mut c = Command::new(name);
    apply_env(&mut c);
    proc::hide_window(&mut c);
    c
}

pub fn look_path(name: &str) -> Option<PathBuf> {
    if name.contains(std::path::MAIN_SEPARATOR) || (cfg!(windows) && name.contains('/')) {
        let p = PathBuf::from(name);
        return is_executable(&p).then_some(p);
    }
    let path = env_var("PATH")?;
    let exts: Vec<String> = if cfg!(windows) {
        let pathext = env_var("PATHEXT").unwrap_or_else(|| ".COM;.EXE;.BAT;.CMD".into());
        let mut v: Vec<String> = pathext.split(';').filter(|e| !e.is_empty()).map(|e| e.to_lowercase()).collect();
        if Path::new(name).extension().is_some() {
            v.insert(0, String::new());
        }
        v
    } else {
        vec![String::new()]
    };
    for dir in std::env::split_paths(&path) {
        for ext in &exts {
            let p = dir.join(format!("{name}{ext}"));
            if is_executable(&p) {
                return Some(p);
            }
        }
    }
    None
}

#[cfg(unix)]
fn is_executable(p: &Path) -> bool {
    use std::os::unix::fs::PermissionsExt;
    std::fs::metadata(p).is_ok_and(|m| m.is_file() && m.permissions().mode() & 0o111 != 0)
}

#[cfg(not(unix))]
fn is_executable(p: &Path) -> bool {
    p.is_file()
}

pub fn have(name: &str) -> bool {
    look_path(name).is_some()
}

pub fn unix_quote(s: &str) -> String {
    format!("'{}'", s.replace('\'', r"'\''"))
}

pub fn split_hotkey(spec: &str) -> Result<(Vec<&'static str>, String), String> {
    let spec = spec.trim().to_lowercase();
    let parts: Vec<&str> = spec.split('+').collect();
    let (last, mods) = parts.split_last().ok_or("empty shortcut")?;
    if last.is_empty() {
        return Err("empty shortcut".into());
    }
    let mut out = Vec::new();
    for p in mods {
        out.push(match p.trim() {
            "cmd" | "command" | "gui" | "win" | "super" => "gui",
            "shift" => "shift",
            "alt" | "option" | "opt" => "alt",
            "ctrl" | "control" => "ctrl",
            other => return Err(format!("unknown modifier: {other}")),
        });
    }
    let mut key = last.trim().to_string();
    if key == "plus" {
        key = "+".into();
    }
    Ok((out, key))
}

pub fn private_dir(config_dir: &Path) -> PathBuf {
    #[cfg(windows)]
    return windows::private_dir(config_dir);
    #[cfg(not(windows))]
    return config_dir.to_path_buf();
}

pub fn migrate_private(config_dir: &Path, name: &str) {
    #[cfg(windows)]
    windows::migrate_private(config_dir, name);
    #[cfg(not(windows))]
    let _ = (config_dir, name);
}

#[cfg(not(windows))]
pub fn acquire_instance() -> bool {
    true
}

#[cfg(not(windows))]
pub fn started_by_user() -> bool {
    false
}

#[cfg(not(windows))]
pub fn show_error(_title: &str, _text: &str) {}

#[cfg(not(windows))]
pub fn attach_console() {}

#[derive(Default)]
pub struct ShellHooks {
    pub status: Option<Box<dyn Fn() -> String + Send + Sync>>,
    pub open_editor: Option<Box<dyn Fn() + Send + Sync>>,
    pub shutdown: Option<Box<dyn Fn() + Send + Sync>>,
    pub release: Option<Box<dyn Fn() + Send + Sync>>,
    pub reclaim: Option<Box<dyn Fn() + Send + Sync>>,
}

#[cfg(not(windows))]
pub fn start_shell(_hooks: ShellHooks) {}

#[cfg(unix)]
pub fn cloexec_pipe() -> std::io::Result<[std::os::fd::OwnedFd; 2]> {
    use std::os::fd::FromRawFd;
    let mut p = [0i32; 2];
    #[cfg(target_os = "linux")]
    let r = unsafe { libc::pipe2(p.as_mut_ptr(), libc::O_CLOEXEC) };
    #[cfg(not(target_os = "linux"))]
    let r = unsafe {
        let r = libc::pipe(p.as_mut_ptr());
        if r == 0 {
            libc::fcntl(p[0], libc::F_SETFD, libc::FD_CLOEXEC);
            libc::fcntl(p[1], libc::F_SETFD, libc::FD_CLOEXEC);
        }
        r
    };
    if r != 0 {
        return Err(std::io::Error::last_os_error());
    }
    Ok(unsafe { [std::os::fd::OwnedFd::from_raw_fd(p[0]), std::os::fd::OwnedFd::from_raw_fd(p[1])] })
}

pub fn applescript_quote(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

#[cfg(test)]
mod tests;
