mod http;

use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};

use serde::Serialize;
use serde_json::Value;

use crate::capture::{SOFTWARE, Source};
use crate::config::{Action, Settings};
use crate::hid::{self, Layout};
use crate::obs;
use crate::platform::{self, SystemInfo};

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct ActResult {
    pub ok: bool,
    pub output: String,
    pub ms: i64,
}

pub trait Env: Send + Sync {
    fn settings(&self) -> Settings;
    fn hud(&self, title: &str, subtitle: &str, force: bool);
    fn goto(&self, target: &Value);
    fn toggle_layer(&self, target: &Value);
    fn toggle_caffeinate(&self) -> bool;
    fn start_timer(&self, minutes: f64, label: &str);
    fn hardware(&self) -> Option<Arc<dyn Source>>;
    fn layer(&self) -> usize;
    fn layer_name(&self) -> String;
    fn set_key_history(&self, mode: &str) -> bool;
}

type UrlFn = Box<dyn Fn() -> String + Send + Sync>;
type FocusFn = Box<dyn Fn() -> bool + Send + Sync>;

static EDITOR_URL: RwLock<Option<UrlFn>> = RwLock::new(None);
static EDITOR_FOCUS: RwLock<Option<FocusFn>> = RwLock::new(None);

pub fn set_editor(url: UrlFn, focus: FocusFn) {
    *EDITOR_URL.write().unwrap_or_else(|e| e.into_inner()) = Some(url);
    *EDITOR_FOCUS.write().unwrap_or_else(|e| e.into_inner()) = Some(focus);
}

pub fn editor_url() -> String {
    EDITOR_URL.read().unwrap_or_else(|e| e.into_inner()).as_ref().map(|f| f()).unwrap_or_else(|| format!("http://127.0.0.1:{}/", crate::app::DEFAULT_PORT))
}

fn truncate(s: &str, max: usize) -> String {
    crate::config::truncate_bytes(s, max)
}

pub fn execute(a: &Action, env: &dyn Env, capture: bool) -> ActResult {
    let t0 = Instant::now();
    let r = dispatch(a, env, capture);
    let ms = t0.elapsed().as_millis() as i64;
    match r {
        Ok(out) => ActResult { ok: true, output: truncate(&out, 600), ms },
        Err(e) => ActResult { ok: false, output: truncate(&e, 300), ms },
    }
}

fn hardware(env: &dyn Env) -> Option<Arc<dyn Source>> {
    if env.settings().input == "software" {
        return None;
    }
    env.hardware().filter(|h| h.connected())
}

fn dispatch(a: &Action, env: &dyn Env, capture: bool) -> Result<String, String> {
    match a.kind.as_str() {
        "app" => app_action(a).map(|_| String::new()),
        "url" => open_url(a),
        "shell" => shell_action(a, capture),
        "ssh" => ssh_action(a, capture),
        "http" => http::request(&a.method, &substitute(&a.url, false), &a.body, |s| substitute(s, false)),
        "hotkey" => hotkey(&a.keys, env).map(|_| String::new()),
        "obs" => {
            let c = env.settings().obs;
            obs::run(&obs::Config { host: c.host, port: c.port, password: c.password }, &a.cmd, &substitute(&a.target, false))
        }
        "text" => type_text(&substitute(&a.text, false), env).map(|_| String::new()),
        "media" => media(&a.cmd, env),
        "system" => system(&a.cmd, env),
        "timer" => {
            env.start_timer(a.minutes, &a.label);
            Ok(format!("{} min", fmt_minutes(a.minutes)))
        }
        "layer" => {
            let to = a.to.clone().unwrap_or(Value::from("next"));
            if a.toggle {
                env.toggle_layer(&to)
            } else {
                env.goto(&to)
            }
            Ok(String::new())
        }
        "history" => Ok(if env.set_key_history(&a.cmd) { "on" } else { "off" }.into()),
        "hud" => {
            env.hud(&substitute(&a.text, false), "", true);
            Ok(String::new())
        }
        "wait" => {
            std::thread::sleep(Duration::from_millis(a.ms.max(0) as u64));
            Ok(String::new())
        }
        "sequence" => run_steps(&a.steps, env),
        "if" => {
            if condition(a, env)? {
                run_steps(&a.steps, env)
            } else {
                run_steps(&a.otherwise, env)
            }
        }
        other => Err(format!("unknown action: {other}")),
    }
}

pub fn fmt_minutes(m: f64) -> String {
    let s = format!("{m}");
    s.strip_suffix(".0").map(String::from).unwrap_or(s)
}

fn is_editor_url(u: &str) -> bool {
    u.trim_end_matches('/') == editor_url().trim_end_matches('/')
}

fn open_url(a: &Action) -> Result<String, String> {
    let u = substitute(&a.url, false);
    if is_editor_url(&u)
        && let Some(focus) = EDITOR_FOCUS.read().unwrap_or_else(|e| e.into_inner()).as_ref()
    {
        if focus() {
            return Ok("focused".into());
        }
        return platform::get().open_url(&u).map(|_| "opened".into());
    }
    platform::get().open_url(&u).map(|_| String::new())
}

fn shell_action(a: &Action, capture: bool) -> Result<String, String> {
    let cmd = substitute(&a.cmd, true);
    if cmd.trim().is_empty() {
        return Err("empty command".into());
    }
    let c = platform::get().shell(&cmd);
    if capture || a.show_output == Some(true) {
        return platform::run_command(c, Duration::from_secs(20), "");
    }
    platform::detached(c).map(|_| String::new())
}

fn ssh_action(a: &Action, capture: bool) -> Result<String, String> {
    if a.host.trim().is_empty() || a.cmd.trim().is_empty() {
        return Err("the server or the command is missing".into());
    }
    let mut c = platform::command("ssh");
    c.args(["-o", "BatchMode=yes", "-o", "ConnectTimeout=5", &a.host, &substitute(&a.cmd, true)]);
    if capture || a.show_output != Some(false) {
        return platform::run_command(c, Duration::from_secs(25), "");
    }
    platform::detached(c).map(|_| String::new())
}

fn run_steps(steps: &[Action], env: &dyn Env) -> Result<String, String> {
    let mut last = String::new();
    for s in steps {
        let r = execute(s, env, true);
        if !r.ok {
            return Err(r.output);
        }
        if !r.output.is_empty() {
            last = r.output;
        }
    }
    Ok(last)
}

fn condition(a: &Action, env: &dyn Env) -> Result<bool, String> {
    let v = substitute(&a.target, false).trim().to_string();
    let ok = match a.cond.as_str() {
        "app" => front_names("").iter().any(|n| n.to_lowercase().contains(&v.to_lowercase())),
        "layer" => match v.parse::<i64>() {
            Ok(n) => n == env.layer() as i64 + 1,
            Err(_) => v.eq_ignore_ascii_case(&env.layer_name()),
        },
        "time" => in_time_range(&v, crate::clock::now_local().minutes_of_day()),
        "os" => v.eq_ignore_ascii_case(platform::get().name()),
        "obs_stream" | "obs_record" => {
            let c = env.settings().obs;
            obs::active(&obs::Config { host: c.host, port: c.port, password: c.password }, a.cond.trim_start_matches("obs_"))?
        }
        "clipboard" => !v.is_empty() && platform::get().clipboard().to_lowercase().contains(&v.to_lowercase()),
        other => return Err(format!("unknown condition: {other}")),
    };
    Ok(ok != a.not)
}

fn parse_hm(s: &str) -> Option<u32> {
    let (h, m) = s.trim().split_once(':')?;
    let digits = |x: &str| x.bytes().all(|b| b.is_ascii_digit());
    if !(1..=2).contains(&h.len()) || m.len() != 2 || !digits(h) || !digits(m) {
        return None;
    }
    let (h, m): (u32, u32) = (h.parse().ok()?, m.parse().ok()?);
    (h < 24 && m < 60).then_some(h * 60 + m)
}

pub fn in_time_range(spec: &str, now_minutes: u32) -> bool {
    let Some((from, to)) = spec.split_once('-') else { return false };
    let (Some(a), Some(b)) = (parse_hm(from), parse_hm(to)) else { return false };
    if a <= b { now_minutes >= a && now_minutes < b } else { now_minutes >= a || now_minutes < b }
}

fn app_action(a: &Action) -> Result<(), String> {
    if a.app.trim().is_empty() {
        return Err("the app name is missing".into());
    }
    let p = platform::get();
    match a.mode.as_str() {
        "quit" => return p.quit_app(&a.app),
        "toggle" if front_names("").iter().any(|n| n.eq_ignore_ascii_case(&a.app)) && p.hide_app(&a.app).is_ok() => {
            return Ok(());
        }
        _ => {}
    }
    p.open_app(&a.app)
}

pub fn front_id() -> String {
    platform::get().front_id()
}

pub fn front_names(id: &str) -> Vec<String> {
    platform::get().front_names(id)
}

pub fn typing_layout(env: &dyn Env) -> &'static Layout {
    let s = env.settings().typing_layout;
    if matches!(s.as_str(), "us" | "es-iso" | "es-pc" | "es-win") {
        return hid::layout_by_name(&s);
    }
    hid::layout_by_name(&platform::get().system_layout())
}

fn adapt(env: &dyn Env, st: hid::Stroke, l: &Layout) -> hid::Stroke {
    if env.settings().capture == SOFTWARE {
        return st;
    }
    platform::get().adapt_stroke(st, l)
}

fn hotkey(spec: &str, env: &dyn Env) -> Result<(), String> {
    let l = typing_layout(env);
    if let Some(hw) = hardware(env)
        && l.safe_for_keys()
    {
        let st = hid::parse_hotkey(spec, l)?;
        if hw.tap(adapt(env, st, l)).is_ok() {
            return Ok(());
        }
    }
    platform::get().soft_hotkey(spec)
}

fn type_text(text: &str, env: &dyn Env) -> Result<(), String> {
    let l = typing_layout(env);
    let hw = hardware(env);
    if let Some(hw) = &hw
        && l.typeable(text)
    {
        let all = text.chars().all(|c| l.strokes(c).unwrap_or(&[]).iter().all(|s| hw.tap(adapt(env, *s, l)).is_ok()));
        if all {
            return Ok(());
        }
    }
    if let Some(hw) = &hw
        && hw.type_text(text).is_ok()
    {
        return Ok(());
    }
    platform::get().paste(text)
}

fn media(cmd: &str, env: &dyn Env) -> Result<String, String> {
    let usage = match cmd {
        "playpause" => Some(0xCD),
        "next" => Some(0xB5),
        "prev" => Some(0xB6),
        "mute" => Some(0xE2),
        "volup" => Some(0xE9),
        "voldown" => Some(0xEA),
        _ => None,
    };
    if let (Some(hw), Some(u)) = (hardware(env), usage)
        && hw.consumer(u).is_ok()
    {
        return Ok(String::new());
    }
    platform::get().soft_media(cmd, env.settings().volume_step)
}

fn system(cmd: &str, env: &dyn Env) -> Result<String, String> {
    let p = platform::get();
    match cmd {
        "caffeinate" => return Ok(if env.toggle_caffeinate() { "on" } else { "off" }.into()),
        "lock" => {
            if let Some(st) = p.lock_stroke() {
                let l = typing_layout(env);
                if let Some(hw) = hardware(env)
                    && l.safe_for_keys()
                    && hw.tap(adapt(env, st, l)).is_ok()
                {
                    return Ok(String::new());
                }
                return p.soft_stroke(st).map(|_| String::new());
            }
        }
        "sleepdisplay" | "sleep" | "screensaver" | "screenshot" | "darkmode" => {}
        other => return Err(format!("unknown system command: {other}")),
    }
    p.system(cmd).map(|_| String::new())
}

pub fn substitute(text: &str, quote: bool) -> String {
    substitute_with(text, quote, || platform::get().clipboard())
}

fn substitute_with(text: &str, quote: bool, clipboard: impl FnOnce() -> String) -> String {
    if !text.contains('{') {
        return text.to_string();
    }
    let now = crate::clock::now_local();
    let mut clipboard = Some(clipboard);
    let mut clip: Option<String> = None;
    let mut out = String::with_capacity(text.len());
    let mut rest = text;
    while let Some(i) = rest.find('{') {
        out.push_str(&rest[..i]);
        rest = &rest[i..];
        let token =
            ["{datetime}", "{date}", "{fecha}", "{time}", "{hora}", "{editor}", "{clipboard}", "{portapapeles}"].into_iter().find(|t| rest.starts_with(t));
        let Some(token) = token else {
            out.push('{');
            rest = &rest[1..];
            continue;
        };
        match token {
            "{datetime}" => out.push_str(&format!("{} {}", now.date(), now.hm())),
            "{date}" | "{fecha}" => out.push_str(&now.date()),
            "{time}" | "{hora}" => out.push_str(&now.hm()),
            "{editor}" => out.push_str(&editor_url()),
            _ => {
                let c = clip.get_or_insert_with(|| {
                    let c = truncate(&clipboard.take().map(|f| f()).unwrap_or_default(), 2000);
                    if quote { platform::get().quote_arg(&c) } else { c }
                });
                out.push_str(c);
            }
        }
        rest = &rest[token.len()..];
    }
    out.push_str(rest);
    out
}

pub fn describe(a: &Action) -> String {
    let cut = |s: &str, n: usize| s.chars().take(n).collect::<String>();
    match a.kind.as_str() {
        "app" => a.app.clone(),
        "url" | "http" => cut(a.url.trim_start_matches("https://").trim_start_matches("http://"), 40),
        "shell" => cut(&a.cmd, 40),
        "ssh" => cut(&format!("{}: {}", a.host, a.cmd), 40),
        "hotkey" => a.keys.clone(),
        "text" => cut(&a.text, 30),
        "media" | "system" | "history" => a.cmd.clone(),
        "obs" => format!("OBS {} {}", a.cmd, a.target).trim().to_string(),
        "timer" => format!("{} min", fmt_minutes(a.minutes)),
        "sequence" => format!("{} steps", a.steps.len()),
        "if" => format!("{} {}", a.cond, cut(&a.target, 24)).trim().to_string(),
        other => other.to_string(),
    }
}

pub fn system_info(env: &dyn Env) -> SystemInfo {
    let mut info = platform::get().info(typing_layout(env));
    info.os = platform::get().name().into();
    info
}

#[cfg(test)]
mod tests;
