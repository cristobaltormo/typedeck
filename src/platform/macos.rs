use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use super::macquirks::{invert_mods, parse_modifier_mapping};
use super::{
    DoctorCheck, Platform, PopupOptions, StopFn, SystemInfo, UNSUPPORTED, applescript_quote as q, command, detached, env_var, have, run, run_out, split_hotkey,
    unix_quote,
};
use crate::hid::{self, Layout, Stroke};
use crate::sync::lock;

const LEONARDO_KEY: &str = "com.apple.keyboard.modifiermapping.9025-32822-0";
const LABEL: &str = "io.github.cristobaltormo.typedeck";
const LEGACY_LABEL: &str = "cc.cristobal.typedeck";
const CHROMIUM: &[&str] = &["Google Chrome", "Brave Browser", "Microsoft Edge", "Vivaldi", "Chromium", "Opera"];

pub struct Mac {
    layout: Mutex<Option<(Instant, String)>>,
    mods: Mutex<Option<(Instant, BTreeMap<u8, u8>)>>,
    kb_type: Mutex<Option<(Instant, i64)>>,
}

impl Mac {
    pub fn new() -> Mac {
        Mac { layout: Mutex::new(None), mods: Mutex::new(None), kb_type: Mutex::new(None) }
    }

    fn mod_mapping(&self) -> BTreeMap<u8, u8> {
        let mut g = lock(&self.mods);
        if let Some((t, m)) = g.as_ref()
            && t.elapsed() < Duration::from_secs(30)
        {
            return m.clone();
        }
        let m = parse_modifier_mapping(&run_out(Duration::from_secs(3), "defaults", &["-currentHost", "read", "-g", LEONARDO_KEY]));
        *g = Some((Instant::now(), m.clone()));
        m
    }

    fn keyboard_type(&self) -> i64 {
        let mut g = lock(&self.kb_type);
        if let Some((t, k)) = g.as_ref()
            && t.elapsed() < Duration::from_secs(30)
        {
            return *k;
        }
        let out = run_out(Duration::from_secs(3), "defaults", &["read", "/Library/Preferences/com.apple.keyboardtype"]);
        let k = out
            .split("\"32822-9025-0\"")
            .nth(1)
            .and_then(|r| r.trim_start().strip_prefix('='))
            .map(|r| r.trim_start().chars().take_while(char::is_ascii_digit).collect::<String>())
            .and_then(|d| d.parse().ok())
            .unwrap_or(0);
        *g = Some((Instant::now(), k));
        k
    }
}

fn mod_name(u: u8) -> &'static str {
    match u {
        0xE0 => "Ctrl",
        0xE1 => "Shift",
        0xE2 => "Alt",
        0xE3 => "Cmd",
        0xE4 => "Right Ctrl",
        0xE5 => "Right Shift",
        0xE6 => "Right Alt",
        0xE7 => "Right Cmd",
        _ => "",
    }
}

fn osascript(script: &str) -> Result<String, String> {
    run(Duration::from_secs(10), "", "osascript", &["-e", script])
}

fn using(mods: &[&str]) -> String {
    let out: Vec<&str> = mods
        .iter()
        .map(|m| match *m {
            "gui" => "command down",
            "shift" => "shift down",
            "alt" => "option down",
            _ => "control down",
        })
        .collect();
    if out.is_empty() { String::new() } else { format!(" using {{{}}}", out.join(", ")) }
}

fn mac_key_code(u: u8) -> Result<u8, String> {
    crate::capture::mac_keycode(u, "iso").ok_or_else(|| format!("key without an equivalent: 0x{u:02X}"))
}

fn running(name: &str) -> bool {
    run(Duration::from_secs(3), "", "pgrep", &["-x", name]).is_ok()
}

fn tab_focus_script(app: &str, url: &str, chromium: bool) -> String {
    let select = if chromium { "set active tab index of w to ti" } else { "set current tab of w to tab ti of w" };
    format!(
        "tell application {}\n\tset allURLs to URL of every tab of every window\n\trepeat with wi from 1 to count of allURLs\n\t\tset urls to item wi of allURLs\n\t\trepeat with ti from 1 to count of urls\n\t\t\tif (item ti of urls) starts with {} then\n\t\t\t\tset w to window wi\n\t\t\t\t{select}\n\t\t\t\tset index of w to 1\n\t\t\t\tactivate\n\t\t\t\treturn \"ok\"\n\t\t\tend if\n\t\tend repeat\n\tend repeat\nend tell\nreturn \"\"",
        q(app),
        q(url)
    )
}

fn home() -> PathBuf {
    PathBuf::from(env_var("HOME").unwrap_or_default())
}

fn uid() -> String {
    unsafe { libc::getuid() }.to_string()
}

fn launchctl(args: &[&str]) -> Result<String, String> {
    run(Duration::from_secs(15), "", "launchctl", args)
}

fn remove_legacy_agent() {
    let _ = launchctl(&["bootout", &format!("gui/{}/{LEGACY_LABEL}", uid())]);
    let _ = std::fs::remove_file(home().join("Library/LaunchAgents").join(format!("{LEGACY_LABEL}.plist")));
}

fn xml(s: &str) -> String {
    s.replace('&', "&amp;").replace('<', "&lt;").replace('>', "&gt;")
}

impl Platform for Mac {
    fn name(&self) -> &'static str {
        "darwin"
    }

    fn version(&self) -> String {
        run(Duration::from_secs(5), "", "sw_vers", &["-productVersion"]).unwrap_or_default()
    }

    fn open_url(&self, url: &str) -> Result<(), String> {
        run(Duration::from_secs(10), "", "open", &[url]).map(|_| ())
    }

    fn focus_editor(&self, url: &str) -> bool {
        let t = Duration::from_secs(4);
        for b in CHROMIUM {
            if running(b) && run(t, "", "osascript", &["-e", &tab_focus_script(b, url, true)]).is_ok_and(|o| o == "ok") {
                return true;
            }
        }
        running("Safari") && run(t, "", "osascript", &["-e", &tab_focus_script("Safari", url, false)]).is_ok_and(|o| o == "ok")
    }

    fn open_app(&self, name: &str) -> Result<(), String> {
        run(Duration::from_secs(10), "", "open", &["-a", name]).map(|_| ())
    }

    fn quit_app(&self, name: &str) -> Result<(), String> {
        osascript(&format!("tell application {} to quit", q(name))).map(|_| ())
    }

    fn hide_app(&self, name: &str) -> Result<(), String> {
        osascript(&format!("tell application \"System Events\" to set visible of process {} to false", q(name))).map(|_| ())
    }

    fn front_id(&self) -> String {
        run(Duration::from_secs(3), "", "lsappinfo", &["front"]).unwrap_or_default()
    }

    fn front_names(&self, id: &str) -> Vec<String> {
        let id = if id.is_empty() { self.front_id() } else { id.to_string() };
        if id.is_empty() {
            return vec![];
        }
        let out = run_out(Duration::from_secs(3), "lsappinfo", &["info", "-only", "name", "-only", "bundlepath", &id]);
        let mut names = Vec::new();
        if let Some(n) = out.trim_start().strip_prefix('"').and_then(|r| r.split('"').next()).filter(|n| !n.is_empty()) {
            names.push(n.to_string());
        }
        if let Some(p) = out.split("bundle path=\"").nth(1).and_then(|r| r.split('"').next()) {
            let base = Path::new(p).file_name().map(|n| n.to_string_lossy().trim_end_matches(".app").to_string()).unwrap_or_default();
            if !base.is_empty() && names.first() != Some(&base) {
                names.push(base);
            }
        }
        names
    }

    fn installed_apps(&self) -> Vec<String> {
        let mut seen = std::collections::BTreeSet::new();
        for base in [
            PathBuf::from("/Applications"),
            PathBuf::from("/Applications/Utilities"),
            PathBuf::from("/System/Applications"),
            PathBuf::from("/System/Applications/Utilities"),
            home().join("Applications"),
        ] {
            for e in std::fs::read_dir(base).into_iter().flatten().flatten() {
                let n = e.file_name().to_string_lossy().into_owned();
                if let Some(app) = n.strip_suffix(".app") {
                    seen.insert(app.to_string());
                }
            }
        }
        let mut out: Vec<String> = seen.into_iter().collect();
        out.sort_by_key(|s| s.to_lowercase());
        out
    }

    fn soft_hotkey(&self, spec: &str) -> Result<(), String> {
        let (mods, key) = split_hotkey(spec)?;
        if key.chars().count() > 1
            && let Some(u) = hid::key_usage(&key)
        {
            return osascript(&format!("tell application \"System Events\" to key code {}{}", mac_key_code(u)?, using(&mods))).map(|_| ());
        }
        if key.chars().count() != 1 {
            return Err(format!("unknown key: {key}"));
        }
        osascript(&format!("tell application \"System Events\" to keystroke {}{}", q(&key), using(&mods))).map(|_| ())
    }

    fn soft_stroke(&self, st: Stroke) -> Result<(), String> {
        let mods: Vec<&str> = [(hid::MOD_GUI, "gui"), (hid::MOD_SHIFT, "shift"), (hid::MOD_ALT, "alt"), (hid::MOD_CTRL, "ctrl")]
            .iter()
            .filter(|(b, _)| st.mods & b != 0)
            .map(|(_, n)| *n)
            .collect();
        osascript(&format!("tell application \"System Events\" to key code {}{}", mac_key_code(st.usage)?, using(&mods))).map(|_| ())
    }

    fn paste(&self, text: &str) -> Result<(), String> {
        let old = self.clipboard();
        run(Duration::from_secs(3), text, "pbcopy", &[])?;
        let r = osascript("tell application \"System Events\" to keystroke \"v\" using {command down}");
        std::thread::sleep(Duration::from_millis(300));
        let _ = run(Duration::from_secs(3), &old, "pbcopy", &[]);
        r.map(|_| ())
    }

    fn clipboard(&self) -> String {
        let mut c = command("pbpaste");
        c.stderr(Stdio::null());
        super::run_command(c, Duration::from_secs(3), "").unwrap_or_default()
    }

    fn soft_media(&self, cmd: &str, step: i64) -> Result<String, String> {
        match cmd {
            "volup" | "voldown" => {
                let sign = if cmd == "volup" { "+" } else { "-" };
                osascript(&format!("set volume output volume ((output volume of (get volume settings)) {sign} {step})"))?;
                Ok(format!("{} %", osascript("output volume of (get volume settings)").unwrap_or_default()))
            }
            "mute" => osascript("set volume output muted not (output muted of (get volume settings))").map(|_| String::new()),
            _ => {
                let verb = match cmd {
                    "playpause" => "playpause",
                    "next" => "next track",
                    "prev" => "previous track",
                    _ => return Err(format!("unknown media command: {cmd}")),
                };
                for p in ["Spotify", "Music"] {
                    if running(p) {
                        return osascript(&format!("tell application {} to {verb}", q(p))).map(|_| p.to_string());
                    }
                }
                Err("neither Spotify nor Music is open".into())
            }
        }
    }

    fn system(&self, cmd: &str) -> Result<(), String> {
        let t = Duration::from_secs(5);
        match cmd {
            "sleepdisplay" => run(t, "", "pmset", &["displaysleepnow"]).map(|_| ()),
            "sleep" => run(t, "", "pmset", &["sleepnow"]).map(|_| ()),
            "screensaver" => run(t, "", "open", &["-a", "ScreenSaverEngine"]).map(|_| ()),
            "screenshot" => {
                let mut c = command("screencapture");
                c.args(["-i", "-c"]);
                detached(c)
            }
            "darkmode" => osascript("tell application \"System Events\" to tell appearance preferences to set dark mode to not dark mode").map(|_| ()),
            "lock" => self.soft_stroke(Stroke::new(0x14, hid::MOD_CTRL | hid::MOD_GUI)),
            _ => Err(UNSUPPORTED.into()),
        }
    }

    fn lock_stroke(&self) -> Option<Stroke> {
        Some(Stroke::new(0x14, hid::MOD_CTRL | hid::MOD_GUI))
    }

    fn keep_awake(&self) -> Result<StopFn, String> {
        let mut c = command("caffeinate");
        c.arg("-dis").stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null());
        let mut child = c.spawn().map_err(|e| e.to_string())?;
        Ok(Box::new(move || {
            let _ = child.kill();
            super::proc::reap(child);
        }))
    }

    fn notify(&self, title: &str, body: &str, sound: bool) {
        if !title.is_empty() || !body.is_empty() {
            let body = if body.is_empty() { title } else { body };
            let mut c = command("osascript");
            c.args(["-e", &format!("display notification {} with title {}", q(body), q(title))]);
            let _ = detached(c);
        }
        if sound {
            let mut c = command("afplay");
            c.arg("/System/Library/Sounds/Tink.aiff");
            let _ = detached(c);
        }
    }

    fn popup(&self, _o: &PopupOptions) -> bool {
        false
    }

    fn system_layout(&self) -> String {
        let mut g = lock(&self.layout);
        if let Some((t, l)) = g.as_ref()
            && t.elapsed() < Duration::from_secs(30)
        {
            return l.clone();
        }
        let out = run_out(Duration::from_secs(3), "defaults", &["read", "com.apple.HIToolbox", "AppleCurrentKeyboardLayoutInputSourceID"]);
        let l = hid::layout_from_input_source(out.trim()).to_string();
        *g = Some((Instant::now(), l.clone()));
        l
    }

    fn adapt_stroke(&self, mut s: Stroke, layout: &Layout) -> Stroke {
        s.mods = invert_mods(&self.mod_mapping(), s.mods);
        if layout.name == "es-iso" && self.keyboard_type() == 40 {
            s.usage = match s.usage {
                0x35 => 0x64,
                0x64 => 0x35,
                u => u,
            };
        }
        s
    }

    fn info(&self, l: &Layout) -> SystemInfo {
        let kt = self.keyboard_type();
        let mut remap: Vec<String> = self
            .mod_mapping()
            .iter()
            .filter(|(s, d)| s != d && !mod_name(**s).is_empty() && !mod_name(**d).is_empty())
            .map(|(s, d)| format!("{} -> {}", mod_name(*s), mod_name(*d)))
            .collect();
        remap.sort();
        SystemInfo {
            os: String::new(),
            typing_layout: l.name.into(),
            keyboard_type: match kt {
                40 => "ansi",
                41 => "iso",
                42 => "jis",
                _ => "",
            }
            .into(),
            mod_remap: remap,
            keys_swapped: l.name == "es-iso" && kt == 40,
        }
    }

    fn shell(&self, cmd: &str) -> Command {
        let mut c = command("/bin/sh");
        c.arg("-c").arg(cmd);
        c
    }

    fn quote_arg(&self, s: &str) -> String {
        unix_quote(s)
    }

    fn log_file(&self) -> Option<PathBuf> {
        Some(home().join("Library/Logs/typedeck.log"))
    }

    fn config_dir(&self) -> PathBuf {
        home().join(".config/typedeck")
    }

    fn doctor(&self) -> Vec<DoctorCheck> {
        vec![
            DoctorCheck::new(
                have("lsappinfo"),
                "lsappinfo available (per-application layers)",
                "lsappinfo is missing: without it there are no per-application layers",
            ),
            DoctorCheck::new(have("osascript"), "osascript available", "osascript is missing: opening and closing applications needs it"),
        ]
    }

    fn install_service(&self, exe: &Path) -> Result<String, String> {
        remove_legacy_agent();
        let plist = home().join("Library/LaunchAgents").join(format!("{LABEL}.plist"));
        let log = self.log_file().unwrap_or_default().display().to_string();
        let body = format!(
            "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n<key>Label</key><string>{LABEL}</string>\n<key>ProgramArguments</key><array><string>{}</string></array>\n<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>\n<key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string></dict>\n<key>StandardOutPath</key><string>{}</string><key>StandardErrorPath</key><string>{}</string>\n</dict></plist>\n",
            xml(&exe.display().to_string()),
            xml(&log),
            xml(&log)
        );
        if let Some(dir) = plist.parent() {
            std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
        }
        std::fs::write(&plist, body).map_err(|e| e.to_string())?;
        let _ = launchctl(&["bootout", &format!("gui/{}/{LABEL}", uid())]);
        launchctl(&["bootstrap", &format!("gui/{}", uid()), &plist.display().to_string()]).map_err(|e| format!("launchctl: {e}"))?;
        Ok(format!("Service installed and running: {}", plist.display()))
    }

    fn uninstall_service(&self) -> Result<String, String> {
        remove_legacy_agent();
        let plist = home().join("Library/LaunchAgents").join(format!("{LABEL}.plist"));
        let _ = launchctl(&["bootout", &format!("gui/{}/{LABEL}", uid())]);
        match std::fs::remove_file(&plist) {
            Err(e) if e.kind() != std::io::ErrorKind::NotFound => Err(e.to_string()),
            _ => Ok("Service removed (the configuration is kept)".into()),
        }
    }
}
