use serde_json::Value;

use super::{Action, Config, KeyDef, Keys, Settings, VERSION};

pub const MAX_LAYERS: usize = 9;
pub const MAX_STEPS: usize = 100;
const MAX_STR: usize = 4000;

const MEDIA_CMDS: &[&str] = &["playpause", "next", "prev", "volup", "voldown", "mute"];
const OBS_CMDS: &[&str] = &[
    "scene",
    "scene_next",
    "scene_prev",
    "stream",
    "stream_start",
    "stream_stop",
    "record",
    "record_pause",
    "mute",
    "replay_save",
    "virtualcam",
    "studio",
    "studio_transition",
];
const SYSTEM_CMDS: &[&str] = &["lock", "sleepdisplay", "sleep", "screensaver", "screenshot", "darkmode", "caffeinate"];
const COND_KINDS: &[&str] = &["app", "layer", "time", "os", "obs_stream", "obs_record", "clipboard"];
const ACTION_TYPES: &[&str] =
    &["app", "url", "shell", "ssh", "http", "hotkey", "text", "sequence", "media", "system", "timer", "layer", "hud", "wait", "open", "obs", "if", "history"];

pub fn is_hex_color(s: &str) -> bool {
    s.len() == 7 && s.starts_with('#') && s[1..].bytes().all(|b| b.is_ascii_hexdigit())
}

pub fn is_key_id(s: &str) -> bool {
    s.len() == 2 && s.bytes().all(|b| b.is_ascii_hexdigit())
}

pub fn is_backup_name(s: &str) -> bool {
    let Some(mid) = s.strip_prefix("config-").and_then(|r| r.strip_suffix(".json")) else {
        return false;
    };
    let b = mid.as_bytes();
    b.len() == 15 && b[8] == b'-' && b[..8].iter().chain(&b[9..]).all(u8::is_ascii_digit)
}

fn one_of(v: &str, def: &str, opts: &[&str]) -> String {
    if opts.contains(&v) { v.to_string() } else { def.to_string() }
}

fn clamp_int(v: i64, lo: i64, hi: i64, def: i64) -> i64 {
    if v == 0 { def } else { v.clamp(lo, hi) }
}

pub fn truncate_bytes(s: &str, max: usize) -> String {
    if s.len() <= max {
        return s.to_string();
    }
    let mut end = max;
    while !s.is_char_boundary(end) {
        end -= 1;
    }
    s[..end].to_string()
}

fn checked(s: &str, where_: &str, name: &str, max: usize) -> Result<String, String> {
    if s.len() > max { Err(format!("{where_}.{name}: too long")) } else { Ok(s.to_string()) }
}

fn clean_layer_target(to: &Option<Value>) -> Value {
    match to {
        Some(Value::String(s)) if s == "next" || s == "prev" => Value::String(s.clone()),
        Some(Value::String(s)) => s.parse::<i64>().map(Value::from).unwrap_or_else(|_| Value::from("next")),
        Some(Value::Number(n)) => Value::from(n.as_i64().unwrap_or_else(|| n.as_f64().map(|f| f.trunc() as i64).unwrap_or(0))),
        _ => Value::from("next"),
    }
}

fn clean_action(a: &Action, where_: &str, depth: u32) -> Result<Action, String> {
    if !ACTION_TYPES.contains(&a.kind.as_str()) {
        return Err(format!("{where_}: invalid action type"));
    }
    let kind = if a.kind == "open" { "app" } else { a.kind.as_str() };
    let mut out = Action { kind: kind.to_string(), confirm: a.confirm, ..Default::default() };
    let s = |v: &str, name: &str, max: usize| checked(v, where_, name, max);
    match kind {
        "app" => {
            out.app = s(&a.app, "app", 200)?;
            out.mode = one_of(&a.mode, "open", &["open", "toggle", "quit"]);
        }
        "url" => out.url = s(&a.url, "url", 2000)?,
        "shell" => {
            out.cmd = s(&a.cmd, "cmd", MAX_STR)?;
            out.show_output = Some(a.show_output == Some(true));
        }
        "ssh" => {
            out.host = s(&a.host, "host", 200)?;
            out.cmd = s(&a.cmd, "cmd", MAX_STR)?;
            out.show_output = Some(a.show_output != Some(false));
        }
        "http" => {
            out.method = one_of(&a.method, "GET", &["GET", "POST", "PUT", "DELETE"]);
            out.url = s(&a.url, "url", 2000)?;
            out.body = s(&a.body, "body", MAX_STR)?;
        }
        "hotkey" => out.keys = s(&a.keys, "keys", 100)?,
        "text" => out.text = s(&a.text, "text", MAX_STR)?,
        "media" => out.cmd = one_of(&a.cmd, "playpause", MEDIA_CMDS),
        "system" => out.cmd = one_of(&a.cmd, "screensaver", SYSTEM_CMDS),
        "timer" => {
            out.minutes = if a.minutes.is_nan() { 0.1 } else { a.minutes.clamp(0.1, 600.0) };
            out.label = s(&a.label, "label", 60)?;
        }
        "layer" => {
            out.to = Some(clean_layer_target(&a.to));
            out.momentary = a.momentary;
            out.toggle = a.toggle && !a.momentary;
        }
        "obs" => {
            out.cmd = one_of(&a.cmd, "record", OBS_CMDS);
            out.target = s(&a.target, "target", 200)?;
        }
        "hud" => out.text = s(&a.text, "text", 80)?,
        "history" => out.cmd = one_of(&a.cmd, "toggle", &["on", "off", "toggle"]),
        "wait" => out.ms = a.ms.clamp(0, 60000),
        "sequence" => {
            if depth >= 1 {
                return Err(format!("{where_}: nested sequences are not allowed"));
            }
            out.steps = clean_steps(&a.steps, &format!("{where_}.steps"), depth + 1)?;
        }
        "if" => {
            if depth >= 2 {
                return Err(format!("{where_}: nested conditions are not allowed"));
            }
            if !COND_KINDS.contains(&a.cond.as_str()) {
                return Err(format!("{where_}: invalid condition"));
            }
            out.cond = a.cond.clone();
            out.not = a.not;
            out.target = s(&a.target, "target", 200)?;
            out.steps = clean_steps(&a.steps, &format!("{where_}.steps"), 2)?;
            out.otherwise = clean_steps(&a.otherwise, &format!("{where_}.else"), 2)?;
        }
        _ => {}
    }
    Ok(out)
}

fn clean_steps(steps: &[Action], where_: &str, depth: u32) -> Result<Vec<Action>, String> {
    if steps.len() > MAX_STEPS {
        return Err(format!("{where_}: at most {MAX_STEPS} steps"));
    }
    steps.iter().enumerate().map(|(i, s)| clean_action(s, &format!("{where_}[{i}]"), depth)).collect()
}

fn clean_keys(keys: &Keys, where_: &str) -> Result<Keys, String> {
    let mut out = Keys::new();
    for (id, kd) in keys {
        if !is_key_id(id) {
            return Err(format!("{where_}: unknown key {id}"));
        }
        let n = u8::from_str_radix(id, 16).unwrap_or(0);
        if !(0x04..=0xA4).contains(&n) {
            return Err(format!("{where_}: key {id} is not valid"));
        }
        let id = id.to_ascii_uppercase();
        let mut c = KeyDef::default();
        for (slot, src, name) in [(&mut c.tap, &kd.tap, "tap"), (&mut c.hold, &kd.hold, "hold"), (&mut c.double, &kd.double, "double")] {
            if let Some(a) = src {
                *slot = Some(clean_action(a, &format!("{where_}/{id}.{name}"), 0)?);
            }
        }
        if kd.label.len() > 40 || kd.icon.len() > 30 {
            return Err(format!("{where_}/{id}: text too long"));
        }
        c.label = kd.label.clone();
        c.icon = kd.icon.clone();
        if !kd.color.is_empty() {
            if !is_hex_color(&kd.color) {
                return Err(format!("{where_}/{id}: invalid color"));
            }
            c.color = kd.color.clone();
        }
        if !c.is_empty() {
            out.entry(id).or_insert(c);
        }
    }
    Ok(out)
}

fn clean_settings(s: &Settings) -> Settings {
    let d = Settings::default();
    let mut s = s.clone();
    s.language = one_of(&s.language, &d.language, &["es", "en"]);
    s.theme = one_of(&s.theme, &d.theme, &["auto", "dark", "light"]);
    s.density = one_of(&s.density, &d.density, &["comfortable", "compact"]);
    if !is_hex_color(&s.accent) {
        s.accent = d.accent.clone();
    }
    s.keysize = clamp_int(s.keysize, 64, 120, d.keysize);
    s.hold_ms = clamp_int(s.hold_ms, 200, 1500, d.hold_ms);
    s.double_ms = clamp_int(s.double_ms, 120, 800, d.double_ms);
    s.volume_step = clamp_int(s.volume_step, 1, 25, d.volume_step);
    s.key_history_days = clamp_int(s.key_history_days, 1, 365, d.key_history_days);
    s.capture = one_of(&s.capture, &d.capture, &["board", "software"]);
    s.input = one_of(&s.input, &d.input, &["auto", "hardware", "software"]);
    s.typing_layout = one_of(&s.typing_layout, &d.typing_layout, &["auto", "us", "es-iso", "es-pc", "es-win"]);
    s.obs.host = s.obs.host.trim().to_string();
    if s.obs.host.is_empty() || s.obs.host.len() > 200 || s.obs.host.contains([' ', '/', '\\']) {
        s.obs.host = "127.0.0.1".into();
    }
    s.obs.port = clamp_int(s.obs.port, 1, 65535, 4455);
    s.obs.password = truncate_bytes(&s.obs.password, 200);
    s.hud.position = one_of(&s.hud.position, &d.hud.position, &["bottom", "top", "center"]);
    if s.hud.seconds.is_nan() || s.hud.seconds < 0.4 {
        s.hud.seconds = d.hud.seconds;
    }
    if s.hud.seconds > 6.0 {
        s.hud.seconds = 6.0;
    }
    s
}

pub fn validate(c: &Config) -> Result<Config, String> {
    if c.layers.is_empty() {
        return Err("at least one layer is required".into());
    }
    if c.layers.len() > MAX_LAYERS {
        return Err(format!("at most {MAX_LAYERS} layers"));
    }
    let mut out = Config { version: VERSION, settings: clean_settings(&c.settings), ..Default::default() };
    out.global = clean_keys(&c.global, "global")?;
    for (i, l) in c.layers.iter().enumerate() {
        let mut l = l.clone();
        l.name = l.name.trim().to_string();
        if l.name.is_empty() {
            return Err(format!("layer {i}: the name is missing"));
        }
        l.name = truncate_bytes(&l.name, 30);
        if !l.color.is_empty() && !is_hex_color(&l.color) {
            return Err(format!("layer {i}: invalid color"));
        }
        if l.auto_apps.len() > 20 {
            return Err(format!("layer {i}: too many apps"));
        }
        l.auto_apps = l.auto_apps.iter().map(|a| a.trim().to_string()).filter(|a| !a.is_empty() && a.len() < 100).collect();
        l.keys = clean_keys(&l.keys, &format!("layer {i}"))?;
        out.layers.push(l);
    }
    out.keyboards =
        c.keyboards.iter().filter(|(k, kb)| k.len() <= 20 && kb.layout.len() <= 30 && kb.name.len() <= 80).map(|(k, v)| (k.clone(), v.clone())).collect();
    Ok(out)
}
