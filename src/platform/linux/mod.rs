pub mod desktop;
pub mod front;
pub mod layout;
pub mod session;

use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

use session::wayland;

use super::{
    DoctorCheck, Platform, PopupOptions, StopFn, SystemInfo, UNSUPPORTED, command, detached, env_var, have, launch, run, set_env, split_hotkey, unix_quote,
};
use crate::hid::{self, Layout, Stroke};
use crate::sync::lock;

const T3: Duration = Duration::from_secs(3);

pub struct Linux;

impl Linux {
    pub fn new() -> Linux {
        let uid = unsafe { libc::getuid() };
        if env_var("DISPLAY").is_none() && env_var("WAYLAND_DISPLAY").is_none() && Path::new("/tmp/.X11-unix/X0").exists() {
            set_env("DISPLAY", ":0");
        }
        let rt = format!("/run/user/{uid}");
        if env_var("XDG_RUNTIME_DIR").is_none() && Path::new(&rt).is_dir() {
            set_env("XDG_RUNTIME_DIR", &rt);
        }
        let bus = format!("{rt}/bus");
        if env_var("DBUS_SESSION_BUS_ADDRESS").is_none() && Path::new(&bus).exists() {
            set_env("DBUS_SESSION_BUS_ADDRESS", &format!("unix:path={bus}"));
        }
        Linux
    }
}

type PasteKeys = Box<dyn Fn(bool) -> Result<(), String> + Send + Sync>;
static PASTE_KEYS: Mutex<Option<PasteKeys>> = Mutex::new(None);

pub fn set_paste_keys(f: Option<PasteKeys>) {
    *lock(&PASTE_KEYS) = f;
}

fn xkey(u: u8) -> String {
    match u {
        0x04..=0x1D => ((b'a' + u - 0x04) as char).to_string(),
        0x1E..=0x26 => ((b'1' + u - 0x1E) as char).to_string(),
        0x27 => "0".into(),
        0x3A..=0x45 => format!("F{}", u - 0x3A + 1),
        0x68..=0x73 => format!("F{}", u - 0x68 + 13),
        _ => match u {
            0x28 => "Return",
            0x29 => "Escape",
            0x2A => "BackSpace",
            0x2B => "Tab",
            0x2C => "space",
            0x2D => "minus",
            0x2E => "equal",
            0x2F => "bracketleft",
            0x30 => "bracketright",
            0x31 => "backslash",
            0x33 => "semicolon",
            0x34 => "apostrophe",
            0x35 => "grave",
            0x36 => "comma",
            0x37 => "period",
            0x38 => "slash",
            0x39 => "Caps_Lock",
            0x46 => "Print",
            0x47 => "Scroll_Lock",
            0x48 => "Pause",
            0x49 => "Insert",
            0x4A => "Home",
            0x4B => "Prior",
            0x4C => "Delete",
            0x4D => "End",
            0x4E => "Next",
            0x4F => "Right",
            0x50 => "Left",
            0x51 => "Down",
            0x52 => "Up",
            0x53 => "Num_Lock",
            _ => "",
        }
        .into(),
    }
}

fn punct_key(k: &str) -> Option<&'static str> {
    Some(match k {
        "+" => "plus",
        "-" => "minus",
        "." => "period",
        "," => "comma",
        "/" => "slash",
        ";" => "semicolon",
        "'" => "apostrophe",
        "[" => "bracketleft",
        "]" => "bracketright",
        "\\" => "backslash",
        "=" => "equal",
        "`" => "grave",
        _ => return None,
    })
}

fn press(mods: &[&str], key: &str) -> Result<(), String> {
    if !wayland() && have("xdotool") {
        let mut combo: Vec<&str> = mods.iter().map(|m| if *m == "gui" { "super" } else { m }).collect();
        combo.push(key);
        return run(T3, "", "xdotool", &["key", "--clearmodifiers", &combo.join("+")]).map(|_| ());
    }
    if have("wtype") {
        let name = |m: &str| if m == "gui" { "logo".to_string() } else { m.to_string() };
        let mut args: Vec<String> = Vec::new();
        for m in mods {
            args.extend(["-M".into(), name(m)]);
        }
        args.extend(["-k".into(), key.into()]);
        for m in mods.iter().rev() {
            args.extend(["-m".into(), name(m)]);
        }
        let a: Vec<&str> = args.iter().map(String::as_str).collect();
        return run(T3, "", "wtype", &a).map(|_| ());
    }
    if wayland() {
        return Err("without the board, software keys need wtype (Wayland with wlroots) or X11; with the board connected they are not needed".into());
    }
    Err("without the board, software keys need xdotool; with the board connected it is not needed".into())
}

fn set_clipboard(text: &str) -> Result<(), String> {
    let w = wayland();
    for c in [&["wl-copy"][..], &["xclip", "-selection", "clipboard"], &["xsel", "-ib"]] {
        if have(c[0]) && (c[0] == "wl-copy") == w {
            let mut cmd = command(c[0]);
            cmd.args(&c[1..]);
            if super::run_command(cmd, T3, text).is_ok() {
                return Ok(());
            }
        }
    }
    Err("pasting text needs wl-clipboard (Wayland) or xclip (X11)".into())
}

fn paste_keys() -> Result<(), String> {
    let shift = front::is_terminal(&front::front().class);
    if let Some(f) = lock(&PASTE_KEYS).as_ref()
        && f(shift).is_ok()
    {
        return Ok(());
    }
    if shift { press(&["ctrl", "shift"], "v") } else { press(&["ctrl"], "v") }
}

fn first_of(cmds: &[&[&str]]) -> Result<(), String> {
    for c in cmds {
        if have(c[0]) {
            return run(Duration::from_secs(5), "", c[0], &c[1..]).map(|_| ());
        }
    }
    Err(UNSUPPORTED.into())
}

fn detached_args(name: &str, args: &[&str]) -> Result<(), String> {
    let mut c = command(name);
    c.args(args);
    detached(c)
}

fn config_dir() -> PathBuf {
    if let Some(x) = env_var("XDG_CONFIG_HOME") {
        return PathBuf::from(x).join("typedeck");
    }
    PathBuf::from(env_var("HOME").unwrap_or_default()).join(".config").join("typedeck")
}

const UNIT_NAME: &str = "typedeck.service";

fn unit_path() -> PathBuf {
    config_dir().parent().map(Path::to_path_buf).unwrap_or_default().join("systemd").join("user").join(UNIT_NAME)
}

impl Platform for Linux {
    fn name(&self) -> &'static str {
        "linux"
    }

    fn version(&self) -> String {
        let text = std::fs::read_to_string("/etc/os-release").unwrap_or_default();
        text.lines().find_map(|l| l.strip_prefix("PRETTY_NAME=")).map(|v| v.trim_matches('"').to_string()).unwrap_or_default()
    }

    fn open_url(&self, url: &str) -> Result<(), String> {
        if !have("xdg-open") {
            return Err("xdg-open is missing (xdg-utils package)".into());
        }
        detached_args("xdg-open", &[url])
    }

    fn focus_editor(&self, _url: &str) -> bool {
        front::focus_editor()
    }

    fn open_app(&self, name: &str) -> Result<(), String> {
        if let Some(e) = desktop::find_entry_in(&desktop::entries(), name) {
            let launched = if have("gtk-launch") {
                let mut c = command("gtk-launch");
                c.arg(&e.id);
                launch(c).is_ok()
            } else if have("gio") {
                let mut c = command("gio");
                c.arg("launch").arg(&e.path);
                launch(c).is_ok()
            } else {
                false
            };
            if launched {
                return Ok(());
            }
            let args = desktop::expand_exec(&e.exec);
            if let Some((first, rest)) = args.split_first() {
                let mut c = command(first);
                c.args(rest);
                return detached(c);
            }
        }
        if let Some(p) = super::look_path(&name.trim().to_lowercase()) {
            return detached(command(&p.display().to_string()));
        }
        Err(format!("application \"{name}\" not found"))
    }

    fn quit_app(&self, name: &str) -> Result<(), String> {
        let mut base = name.trim().to_lowercase();
        if let Some(e) = desktop::find_entry_in(&desktop::entries(), name) {
            base = desktop::exec_base(&e.exec);
        }
        if base.is_empty() {
            return Err("the app name is missing".into());
        }
        let base: String = base.chars().take(15).collect();
        run(T3, "", "pkill", &["-TERM", "-x", &base]).map(|_| ()).map_err(|_| format!("could not close \"{name}\" (is it open?)"))
    }

    fn hide_app(&self, _name: &str) -> Result<(), String> {
        if !wayland() && have("xdotool") {
            return run(T3, "", "xdotool", &["getactivewindow", "windowminimize"]).map(|_| ());
        }
        if have("hyprctl") {
            return run(T3, "", "hyprctl", &["dispatch", "movetoworkspacesilent", "special"]).map(|_| ());
        }
        Err(UNSUPPORTED.into())
    }

    fn front_id(&self) -> String {
        front::front().id
    }

    fn front_names(&self, id: &str) -> Vec<String> {
        front::front_names(id)
    }

    fn installed_apps(&self) -> Vec<String> {
        desktop::entries().into_iter().map(|e| e.name).collect()
    }

    fn soft_hotkey(&self, spec: &str) -> Result<(), String> {
        let (mods, key) = split_hotkey(spec)?;
        if let Some(u) = hid::key_usage(&key) {
            let k = xkey(u);
            if !k.is_empty() {
                return press(&mods, &k);
            }
        }
        if let Some(k) = punct_key(&key) {
            return press(&mods, k);
        }
        if key.chars().count() == 1 {
            return press(&mods, &key);
        }
        Err(format!("unknown key: {key}"))
    }

    fn soft_stroke(&self, st: Stroke) -> Result<(), String> {
        let mods: Vec<&str> = [(hid::MOD_GUI, "gui"), (hid::MOD_SHIFT, "shift"), (hid::MOD_ALT, "alt"), (hid::MOD_CTRL, "ctrl")]
            .iter()
            .filter(|(b, _)| st.mods & b != 0)
            .map(|(_, n)| *n)
            .collect();
        let k = xkey(st.usage);
        if k.is_empty() {
            return Err(format!("key without an equivalent: 0x{:02X}", st.usage));
        }
        press(&mods, &k)
    }

    fn paste(&self, text: &str) -> Result<(), String> {
        let old = self.clipboard();
        set_clipboard(text)?;
        let res = paste_keys();
        std::thread::sleep(Duration::from_millis(300));
        let _ = set_clipboard(&old);
        res
    }

    fn clipboard(&self) -> String {
        let w = wayland();
        for c in [&["wl-paste", "-n"][..], &["xclip", "-selection", "clipboard", "-o"], &["xsel", "-ob"]] {
            if have(c[0]) && (c[0] == "wl-paste") == w {
                let mut cmd = command(c[0]);
                cmd.args(&c[1..]).stderr(Stdio::null());
                if let Ok(out) = super::run_command(cmd, T3, "") {
                    return out;
                }
            }
        }
        String::new()
    }

    fn soft_media(&self, cmd: &str, step: i64) -> Result<String, String> {
        let none = "wpctl, pactl or amixer is missing";
        match cmd {
            "volup" | "voldown" => {
                let sign = if cmd == "volup" { "+" } else { "-" };
                if have("wpctl") {
                    return run(T3, "", "wpctl", &["set-volume", "@DEFAULT_AUDIO_SINK@", &format!("{step}%{sign}")]).map(|_| String::new());
                }
                if have("pactl") {
                    return run(T3, "", "pactl", &["set-sink-volume", "@DEFAULT_SINK@", &format!("{sign}{step}%")]).map(|_| String::new());
                }
                if have("amixer") {
                    return run(T3, "", "amixer", &["-q", "set", "Master", &format!("{step}%{sign}")]).map(|_| String::new());
                }
                Err(none.into())
            }
            "mute" => {
                if have("wpctl") {
                    return run(T3, "", "wpctl", &["set-mute", "@DEFAULT_AUDIO_SINK@", "toggle"]).map(|_| String::new());
                }
                if have("pactl") {
                    return run(T3, "", "pactl", &["set-sink-mute", "@DEFAULT_SINK@", "toggle"]).map(|_| String::new());
                }
                if have("amixer") {
                    return run(T3, "", "amixer", &["-q", "set", "Master", "toggle"]).map(|_| String::new());
                }
                Err(none.into())
            }
            _ => {
                let verb = match cmd {
                    "playpause" => "play-pause",
                    "next" => "next",
                    "prev" => "previous",
                    _ => return Err(format!("unknown media command: {cmd}")),
                };
                if !have("playerctl") {
                    return Err("playerctl is missing to control the player (not needed with the board connected)".into());
                }
                run(T3, "", "playerctl", &[verb]).map(|_| String::new())
            }
        }
    }

    fn system(&self, cmd: &str) -> Result<(), String> {
        match cmd {
            "lock" => first_of(&[&["loginctl", "lock-session"], &["xdg-screensaver", "lock"]]),
            "sleep" => first_of(&[&["systemctl", "suspend"]]),
            "sleepdisplay" => {
                if env_var("HYPRLAND_INSTANCE_SIGNATURE").is_some() {
                    first_of(&[&["hyprctl", "dispatch", "dpms", "off"]])
                } else if env_var("SWAYSOCK").is_some() {
                    first_of(&[&["swaymsg", "output * dpms off"]])
                } else if !wayland() {
                    first_of(&[&["xset", "dpms", "force", "off"]])
                } else {
                    Err(format!("turning the display off is not available on this Wayland desktop: {UNSUPPORTED}"))
                }
            }
            "screensaver" => first_of(&[&["xdg-screensaver", "activate"], &["loginctl", "lock-session"]]),
            "screenshot" => {
                for c in [&["gnome-screenshot", "-a", "-c"][..], &["spectacle", "-r", "-b", "-c"], &["flameshot", "gui"], &["xfce4-screenshooter", "-r", "-c"]]
                {
                    if have(c[0]) {
                        return detached_args(c[0], &c[1..]);
                    }
                }
                if have("grim") && have("slurp") && have("wl-copy") {
                    return detached_args("sh", &["-c", r#"grim -g "$(slurp)" - | wl-copy"#]);
                }
                Err(format!("a screenshot tool is missing (gnome-screenshot, spectacle, flameshot or grim+slurp): {UNSUPPORTED}"))
            }
            "darkmode" => {
                if !have("gsettings") {
                    return Err(format!("switching the theme is only available on GNOME: {UNSUPPORTED}"));
                }
                let cur = run(T3, "", "gsettings", &["get", "org.gnome.desktop.interface", "color-scheme"])?;
                let next = if cur.contains("prefer-dark") { "default" } else { "prefer-dark" };
                run(T3, "", "gsettings", &["set", "org.gnome.desktop.interface", "color-scheme", next]).map(|_| ())
            }
            _ => Err(UNSUPPORTED.into()),
        }
    }

    fn lock_stroke(&self) -> Option<Stroke> {
        None
    }

    fn keep_awake(&self) -> Result<StopFn, String> {
        if !have("systemd-inhibit") {
            return Err(UNSUPPORTED.into());
        }
        let mut c = command("systemd-inhibit");
        c.args(["--what=idle:sleep", "--who=Typedeck", "--why=Keep awake", "--mode=block", "sleep", "infinity"])
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null());
        let mut child = c.spawn().map_err(|e| e.to_string())?;
        Ok(Box::new(move || {
            let _ = child.kill();
            super::proc::reap(child);
        }))
    }

    fn notify(&self, title: &str, body: &str, sound: bool) {
        if (!title.is_empty() || !body.is_empty()) && have("notify-send") {
            let _ = detached_args("notify-send", &["-a", "Typedeck", "-t", "2500", title, body]);
        }
        if sound {
            if have("canberra-gtk-play") {
                let _ = detached_args("canberra-gtk-play", &["-i", "message"]);
            } else if have("paplay") && Path::new("/usr/share/sounds/freedesktop/stereo/message.oga").exists() {
                let _ = detached_args("paplay", &["/usr/share/sounds/freedesktop/stereo/message.oga"]);
            }
        }
    }

    fn popup(&self, _o: &PopupOptions) -> bool {
        false
    }

    fn system_layout(&self) -> String {
        layout::system_layout()
    }

    fn adapt_stroke(&self, st: Stroke, _layout: &Layout) -> Stroke {
        st
    }

    fn info(&self, l: &Layout) -> SystemInfo {
        SystemInfo { typing_layout: l.name.into(), ..Default::default() }
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
        None
    }

    fn config_dir(&self) -> PathBuf {
        config_dir()
    }

    fn doctor(&self) -> Vec<DoctorCheck> {
        let sess = if wayland() { "Wayland" } else { "X11" };
        let mut checks = vec![
            DoctorCheck::new(
                env_var("DISPLAY").is_some() || env_var("WAYLAND_DISPLAY").is_some(),
                format!("graphical session: {sess}"),
                "no graphical session was found: run typedeck from your desktop session, or restart the service after logging in (systemctl --user restart typedeck)",
            ),
            DoctorCheck::new(have("xdg-open"), "xdg-open available (links and files)", "xdg-open is missing: install xdg-utils to open links"),
            DoctorCheck::new(have("notify-send"), "notify-send available (popups)", "notify-send is missing: install libnotify-bin to see the popups"),
            DoctorCheck::new(
                !front::front().id.is_empty(),
                "the front application can be read (per-application layers)",
                "optional: the active window cannot be read, so per-application layers are unavailable (they need X11 with xprop, Sway or Hyprland; GNOME and KDE on Wayland do not allow it)",
            ),
            DoctorCheck::new(
                (!wayland() && have("xdotool")) || have("wtype"),
                "software keys available (xdotool or wtype)",
                "optional: without xdotool or wtype, software keys do not work; they are not needed with the board connected",
            ),
        ];
        let mut acm: Vec<String> = std::fs::read_dir("/dev")
            .map(|rd| rd.flatten().filter_map(|e| e.file_name().into_string().ok()).filter(|n| n.starts_with("ttyACM")).map(|n| format!("/dev/{n}")).collect())
            .unwrap_or_default();
        acm.sort();
        match acm.first() {
            None => checks.push(DoctorCheck::new(true, "no /dev/ttyACM*: the port permission cannot be checked (plug the board in)", "")),
            Some(first) => {
                let ok = acm.iter().any(|p| std::fs::OpenOptions::new().read(true).write(true).open(p).is_ok());
                checks.push(DoctorCheck::new(
                    ok,
                    format!("your user can open {first}"),
                    format!("your user cannot open {first}: copy packaging/linux/70-typedeck.rules to /etc/udev/rules.d/ or add yourself to the dialout group (and sign in again)"),
                ));
            }
        }
        checks
    }

    fn install_service(&self, exe: &Path) -> Result<String, String> {
        if !have("systemctl") {
            return Err("systemd is needed; without it, start typedeck from your desktop session startup".into());
        }
        let unit = format!(
            "[Unit]\nDescription=Typedeck: macros for your keyboard\nAfter=graphical-session.target\n\n[Service]\nExecStart={}\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=default.target\n",
            exe.display()
        );
        let p = unit_path();
        if let Some(dir) = p.parent() {
            std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
        }
        std::fs::write(&p, unit).map_err(|e| e.to_string())?;
        for c in [&["daemon-reload"][..], &["enable", "--now", UNIT_NAME]] {
            let mut args = vec!["--user"];
            args.extend_from_slice(c);
            run(Duration::from_secs(30), "", "systemctl", &args).map_err(|e| format!("systemctl --user {}: {e}", c[0]))?;
        }
        Ok(format!("Service installed and running: {}\nLog: journalctl --user -u typedeck", p.display()))
    }

    fn uninstall_service(&self) -> Result<String, String> {
        if have("systemctl") {
            let _ = run(Duration::from_secs(30), "", "systemctl", &["--user", "disable", "--now", UNIT_NAME]);
        }
        match std::fs::remove_file(unit_path()) {
            Err(e) if e.kind() != std::io::ErrorKind::NotFound => return Err(e.to_string()),
            _ => {}
        }
        if have("systemctl") {
            let _ = run(Duration::from_secs(30), "", "systemctl", &["--user", "daemon-reload"]);
        }
        Ok("Service removed (the configuration is kept)".into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::temp_dir;
    use std::path::PathBuf;

    fn write_desktop(dir: &Path, id: &str, body: &str) {
        let p = dir.join("applications").join(format!("{id}.desktop"));
        std::fs::create_dir_all(p.parent().unwrap()).unwrap();
        std::fs::write(p, body).unwrap();
    }

    #[test]
    fn desktop_entries() {
        let dir = temp_dir("desktop");
        write_desktop(
            &dir,
            "code",
            "[Desktop Entry]\nType=Application\nName=Visual Studio Code\nName[es]=Código\nExec=/usr/share/code/code --unity-launch %F\nStartupWMClass=Code\n",
        );
        write_desktop(&dir, "hidden", "[Desktop Entry]\nType=Application\nName=Oculta\nExec=oculta\nNoDisplay=true\n");
        write_desktop(
            &dir,
            "org.mozilla.firefox",
            "[Desktop Entry]\nType=Application\nName=Firefox\nExec=env MOZ_X=1 firefox %u\n[Desktop Action new-window]\nName=Nueva ventana\nExec=firefox --new-window\n",
        );
        write_desktop(&dir, "link", "[Desktop Entry]\nType=Link\nName=Enlace\nURL=x\n");
        let list = desktop::load_entries_from(&[dir.clone(), PathBuf::from(&dir)]);
        let names: Vec<&str> = list.iter().map(|e| e.name.as_str()).collect();
        assert_eq!(names, vec!["Firefox", "Visual Studio Code"]);
        assert_eq!(desktop::find_entry_in(&list, "code").unwrap().name, "Visual Studio Code");
        assert_eq!(desktop::find_entry_in(&list, "visual studio").unwrap().id, "code");
        assert_eq!(desktop::entry_for_in(&list, "Code", "").unwrap().id, "code");
        assert_eq!(desktop::entry_for_in(&list, "", "firefox").unwrap().name, "Firefox");
        assert!(desktop::find_entry_in(&list, "oculta").is_none());
        assert_eq!(desktop::exec_base("env MOZ_X=1 firefox %u"), "firefox");
        assert_eq!(desktop::expand_exec("/usr/share/code/code --unity-launch %F"), vec!["/usr/share/code/code", "--unity-launch"]);
        let _ = std::fs::remove_dir_all(dir);
    }

    #[test]
    fn xkb_to_hid_cases() {
        for (l, v, want) in [
            ("us", "", "us"),
            ("es", "", "es-pc"),
            ("es", "winkeys", "es-pc"),
            ("us", "intl", "qwerty"),
            ("gb", "", "qwerty"),
            ("es", "cat", "qwerty"),
            ("fr", "", ""),
            ("de", "", ""),
            ("us", "dvorak", ""),
        ] {
            assert_eq!(layout::xkb_to_hid(l, v), want, "{l}/{v}");
        }
    }

    #[test]
    fn x_key_names() {
        for (u, want) in [(0x04, "a"), (0x27, "0"), (0x3A, "F1"), (0x45, "F12"), (0x28, "Return"), (0x4F, "Right"), (0x2C, "space"), (0x68, "F13")] {
            assert_eq!(xkey(u), want);
        }
    }

    #[test]
    fn focused_sway_node() {
        let tree = serde_json::json!({"nodes": [{"nodes": [{"id": 7.0, "focused": true, "app_id": "firefox"}]}]});
        assert_eq!(front::focused_node(&tree).unwrap()["app_id"], "firefox");
        assert!(front::focused_node(&serde_json::json!({"nodes": []})).is_none());
    }

    #[test]
    fn discover_session_finds_the_compositor_sockets() {
        let dir = temp_dir("rt");
        for f in ["sway-ipc.1000.42.sock", "wayland-1", "wayland-1.lock"] {
            std::fs::write(dir.join(f), "").unwrap();
        }
        std::fs::create_dir_all(dir.join("hypr").join("abc123_1_2")).unwrap();
        let found = session::discover_session_in(&dir);
        let get = |k: &str| found.iter().find(|(n, _)| n == k).map(|(_, v)| v.clone());
        if env_var("SWAYSOCK").is_none() {
            assert_eq!(get("SWAYSOCK"), Some(dir.join("sway-ipc.1000.42.sock").display().to_string()));
        }
        if env_var("WAYLAND_DISPLAY").is_none() {
            assert_eq!(get("WAYLAND_DISPLAY").as_deref(), Some("wayland-1"));
        }
        if env_var("HYPRLAND_INSTANCE_SIGNATURE").is_none() {
            assert_eq!(get("HYPRLAND_INSTANCE_SIGNATURE").as_deref(), Some("abc123_1_2"));
        }
        let _ = std::fs::remove_dir_all(dir);
    }

    #[test]
    fn terminals_paste_with_shift() {
        for (class, want) in
            [("foot", true), ("Alacritty", true), ("gnome-terminal-server", true), ("org.kde.konsole", true), ("firefox", false), ("Code", false), ("", false)]
        {
            assert_eq!(front::is_terminal(class), want, "{class}");
        }
    }
}
