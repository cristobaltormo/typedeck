use std::path::Path;
use std::sync::Mutex;
use std::time::{Duration, Instant};

use crate::platform::{env_var, set_env};
use crate::sync::lock;

static CHECKED: Mutex<Option<Instant>> = Mutex::new(None);

pub fn runtime_dir() -> String {
    format!("/run/user/{}", unsafe { libc::getuid() })
}

pub fn discover_session_in(dir: &Path) -> Vec<(String, String)> {
    let mut found = Vec::new();
    let mut names: Vec<String> = std::fs::read_dir(dir).map(|rd| rd.flatten().filter_map(|e| e.file_name().into_string().ok()).collect()).unwrap_or_default();
    names.sort();
    if env_var("SWAYSOCK").is_none()
        && let Some(n) = names.iter().find(|n| n.starts_with("sway-ipc.") && n.ends_with(".sock"))
    {
        found.push(("SWAYSOCK".into(), dir.join(n).display().to_string()));
    }
    if env_var("HYPRLAND_INSTANCE_SIGNATURE").is_none() {
        let mut hypr: Vec<String> = std::fs::read_dir(dir.join("hypr"))
            .map(|rd| rd.flatten().filter(|e| e.path().is_dir()).filter_map(|e| e.file_name().into_string().ok()).collect())
            .unwrap_or_default();
        hypr.sort();
        if let Some(h) = hypr.first() {
            found.push(("HYPRLAND_INSTANCE_SIGNATURE".into(), h.clone()));
        }
    }
    if env_var("WAYLAND_DISPLAY").is_none()
        && let Some(n) = names.iter().find(|n| n.strip_prefix("wayland-").is_some_and(|r| r.starts_with(|c: char| c.is_ascii_digit())) && !n.contains('.'))
    {
        found.push(("WAYLAND_DISPLAY".into(), n.clone()));
    }
    found
}

pub fn discover_session() {
    {
        let mut c = lock(&CHECKED);
        if c.is_some_and(|t| t.elapsed() < Duration::from_secs(3)) {
            return;
        }
        *c = Some(Instant::now());
    }
    for (k, v) in discover_session_in(Path::new(&runtime_dir())) {
        set_env(&k, &v);
    }
    if env_var("DISPLAY").is_none() && env_var("WAYLAND_DISPLAY").is_none() && Path::new("/tmp/.X11-unix/X0").exists() {
        set_env("DISPLAY", ":0");
    }
}

pub fn wayland() -> bool {
    discover_session();
    if let Some(t) = env_var("XDG_SESSION_TYPE") {
        return t == "wayland";
    }
    env_var("WAYLAND_DISPLAY").is_some() && env_var("DISPLAY").is_none()
}
