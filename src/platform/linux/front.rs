use std::time::Duration;

use serde_json::Value;

use super::desktop::{entries, entry_for_in};
use super::session::{discover_session, wayland};
use crate::platform::{env_var, have, run, run_out};

#[derive(Clone, Debug, Default)]
pub struct Front {
    pub id: String,
    pub class: String,
    pub pid: i64,
    pub x11: bool,
}

const T: Duration = Duration::from_secs(2);

pub fn focused_node(n: &Value) -> Option<&Value> {
    if n.get("focused").and_then(Value::as_bool) == Some(true) {
        return Some(n);
    }
    for k in ["nodes", "floating_nodes"] {
        if let Some(kids) = n.get(k).and_then(Value::as_array) {
            for c in kids {
                if let Some(r) = focused_node(c) {
                    return Some(r);
                }
            }
        }
    }
    None
}

pub fn front() -> Front {
    discover_session();
    if env_var("HYPRLAND_INSTANCE_SIGNATURE").is_some() && have("hyprctl") {
        if let Ok(out) = run(T, "", "hyprctl", &["activewindow", "-j"])
            && let Ok(w) = serde_json::from_str::<Value>(&out)
            && let Some(addr) = w.get("address").and_then(Value::as_str).filter(|a| !a.is_empty())
        {
            return Front {
                id: addr.into(),
                class: w.get("class").and_then(Value::as_str).unwrap_or("").into(),
                pid: w.get("pid").and_then(Value::as_i64).unwrap_or(0),
                x11: false,
            };
        }
        return Front::default();
    }
    if env_var("SWAYSOCK").is_some() && have("swaymsg") {
        if let Ok(out) = run(T, "", "swaymsg", &["-t", "get_tree"])
            && let Ok(root) = serde_json::from_str::<Value>(&out)
            && let Some(n) = focused_node(&root)
        {
            let mut class = n.get("app_id").and_then(Value::as_str).unwrap_or("").to_string();
            if class.is_empty() {
                class = n.pointer("/window_properties/class").and_then(Value::as_str).unwrap_or("").to_string();
            }
            let id = n.get("id").and_then(Value::as_f64).unwrap_or(0.0) as i64;
            return Front { id: id.to_string(), class, pid: n.get("pid").and_then(Value::as_f64).unwrap_or(0.0) as i64, x11: false };
        }
        return Front::default();
    }
    if have("xprop") && env_var("DISPLAY").is_some() {
        let out = run_out(T, "xprop", &["-root", "_NET_ACTIVE_WINDOW"]);
        if let Some(i) = out.rfind("0x") {
            let id = out[i..].trim();
            if id != "0x0" {
                return Front { id: id.into(), x11: true, ..Default::default() };
            }
        }
        if have("xdotool")
            && let Ok(out) = run(T, "", "xdotool", &["getwindowfocus"])
            && !out.trim().is_empty()
        {
            return Front { id: out.trim().into(), x11: true, ..Default::default() };
        }
    }
    Front::default()
}

pub fn front_names(id: &str) -> Vec<String> {
    let mut f = front();
    if f.id.is_empty() || (!id.is_empty() && f.id != id) {
        return vec![];
    }
    if f.class.is_empty() && f.pid == 0 && f.x11 {
        let out = run_out(T, "xprop", &["-id", &f.id, "WM_CLASS", "_NET_WM_PID"]);
        for l in out.lines() {
            if let Some((_, v)) = l.split_once("WM_CLASS(STRING) = ") {
                let parts: Vec<&str> = v.split(',').map(|p| p.trim().trim_matches('"')).collect();
                f.class = match parts.as_slice() {
                    [one] => one.to_string(),
                    [first, .., last] => format!("{first}\0{last}"),
                    [] => String::new(),
                };
            }
            if let Some((_, v)) = l.split_once("_NET_WM_PID(CARDINAL) = ") {
                f.pid = v.trim().parse().unwrap_or(0);
            }
        }
    }
    let mut names: Vec<String> = Vec::new();
    let mut add = |n: &str| {
        let n = n.trim();
        if !n.is_empty() && !names.iter().any(|x| x.eq_ignore_ascii_case(n)) {
            names.push(n.to_string());
        }
    };
    let classes: Vec<&str> = f.class.split('\0').collect();
    let comm =
        if f.pid > 0 { std::fs::read_to_string(format!("/proc/{}/comm", f.pid)).map(|s| s.trim().to_string()).unwrap_or_default() } else { String::new() };
    let list = entries();
    for c in &classes {
        if let Some(e) = entry_for_in(&list, c, &comm) {
            add(&e.name);
            break;
        }
    }
    for c in &classes {
        add(c);
    }
    add(&comm);
    names
}

fn is_browser_class(s: &str) -> bool {
    let s = s.to_lowercase();
    ["firefox", "navigator", "chromium", "chrome", "brave", "vivaldi", "opera", "librewolf", "epiphany"].iter().any(|b| s.contains(b))
}

pub fn focus_editor() -> bool {
    if env_var("HYPRLAND_INSTANCE_SIGNATURE").is_some() && have("hyprctl") {
        return run(T, "", "hyprctl", &["dispatch", "focuswindow", "title:Typedeck"]).is_ok_and(|o| o.contains("ok"));
    }
    if env_var("SWAYSOCK").is_some() && have("swaymsg") {
        return run(T, "", "swaymsg", &[r#"[title="Typedeck" app_id="(?i)(firefox|chromium|chrome|brave|vivaldi|opera|librewolf|epiphany)"] focus"#])
            .is_ok_and(|o| o.contains(r#""success": true"#));
    }
    if !wayland() && have("xdotool") {
        let Ok(ids) = run(Duration::from_secs(3), "", "xdotool", &["search", "--onlyvisible", "--name", "Typedeck"]) else { return false };
        for id in ids.split_whitespace() {
            let browser = !have("xprop") || run(T, "", "xprop", &["-id", id, "WM_CLASS"]).is_ok_and(|c| is_browser_class(&c));
            if browser {
                return run(Duration::from_secs(3), "", "xdotool", &["windowactivate", id]).is_ok();
            }
        }
        return false;
    }
    if !wayland() && have("wmctrl") {
        return run(Duration::from_secs(3), "", "wmctrl", &["-a", "Typedeck"]).is_ok();
    }
    false
}

const TERMINALS: &[&str] =
    &["foot", "kitty", "alacritty", "xterm", "urxvt", "rxvt", "konsole", "terminator", "tilix", "wezterm", "ghostty", "st-256color", "lxterminal", "terminal"];

pub fn is_terminal(class: &str) -> bool {
    let c = class.to_lowercase();
    TERMINALS.iter().any(|t| c.contains(t))
}
