use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use windows_sys::Win32::UI::Shell::ShellExecuteW;
use windows_sys::Win32::UI::WindowsAndMessaging::SW_SHOWNORMAL;

use super::front::{TopWindow, bring_to_front, close_windows, foreground, process_image, top_windows, windows_of_image};
use super::reg::wide;
use crate::platform::{command, detached, env_var, lnk_target, run};
use crate::sync::lock;

#[derive(Clone, Debug, Default)]
pub struct Shortcut {
    pub name: String,
    pub path: PathBuf,
    pub target: String,
    pub aumid: String,
}

const STORE_SCRIPT: &str = "[Console]::OutputEncoding=[Text.Encoding]::UTF8;Get-StartApps | ForEach-Object { $_.Name + '|' + $_.AppID }";

static LNKS: Mutex<Option<(Instant, Vec<Shortcut>)>> = Mutex::new(None);
type Cached = Option<(Instant, Vec<Shortcut>)>;

static STORE: Mutex<(Cached, bool)> = Mutex::new((None, false));

fn is_uninstaller(name: &str) -> bool {
    let n = name.to_lowercase();
    n.contains("uninstall") || n.contains("desinstalar")
}

fn walk(dir: &Path, depth: u32, out: &mut Vec<PathBuf>) {
    let Ok(rd) = std::fs::read_dir(dir) else { return };
    let mut paths: Vec<PathBuf> = rd.flatten().map(|e| e.path()).collect();
    paths.sort();
    for p in paths {
        if p.is_dir() {
            if depth < 8 {
                walk(&p, depth + 1, out);
            }
        } else if p.extension().is_some_and(|e| e.eq_ignore_ascii_case("lnk")) {
            out.push(p);
        }
    }
}

fn scan_shortcuts() -> Vec<Shortcut> {
    let mut out: Vec<Shortcut> = Vec::new();
    for var in ["ProgramData", "APPDATA"] {
        let Some(base) = env_var(var) else { continue };
        let mut files = Vec::new();
        walk(&Path::new(&base).join(r"Microsoft\Windows\Start Menu\Programs"), 0, &mut files);
        for p in files {
            let name = p.file_stem().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default();
            if is_uninstaller(&name) || out.iter().any(|s| s.name.eq_ignore_ascii_case(&name)) {
                continue;
            }
            let target = std::fs::read(&p).map(|d| lnk_target(&d)).unwrap_or_default();
            out.push(Shortcut { name, path: p, target, aumid: String::new() });
        }
    }
    out
}

pub fn parse_store_apps(out: &str) -> Vec<Shortcut> {
    out.lines()
        .filter_map(|l| l.trim().split_once('|'))
        .filter(|(n, id)| !n.is_empty() && id.contains('!') && !is_uninstaller(n))
        .map(|(n, id)| Shortcut { name: n.into(), aumid: id.into(), ..Default::default() })
        .collect()
}

pub(super) fn store_apps() -> Vec<Shortcut> {
    let mut g = lock(&STORE);
    let fresh = g.0.as_ref().is_some_and(|(t, _)| t.elapsed() < Duration::from_secs(600));
    if !g.1 && !fresh {
        g.1 = true;
        crate::sync::spawn("store-apps", || {
            let out = run(Duration::from_secs(15), "", "powershell.exe", &["-NoProfile", "-NonInteractive", "-Command", STORE_SCRIPT]);
            let mut g = lock(&STORE);
            g.1 = false;
            match out {
                Ok(o) => g.0 = Some((Instant::now(), parse_store_apps(&o))),
                Err(_) => {
                    let keep = g.0.take().map(|(_, l)| l).unwrap_or_default();
                    g.0 = Some((Instant::now(), keep));
                }
            }
        });
    }
    g.0.as_ref().map(|(_, l)| l.clone()).unwrap_or_default()
}

pub fn shortcuts() -> Vec<Shortcut> {
    let base = {
        let mut g = lock(&LNKS);
        if g.as_ref().is_none_or(|(t, _)| t.elapsed() >= Duration::from_secs(60)) {
            *g = Some((Instant::now(), scan_shortcuts()));
        }
        g.as_ref().map(|(_, l)| l.clone()).unwrap_or_default()
    };
    let mut out = base.clone();
    for s in store_apps() {
        if !base.iter().any(|b| b.name.eq_ignore_ascii_case(&s.name)) {
            out.push(s);
        }
    }
    out.sort_by_key(|s| s.name.to_lowercase());
    out
}

fn find_shortcut(name: &str) -> Option<Shortcut> {
    let n = name.trim().to_lowercase();
    let all = shortcuts();
    all.iter().find(|s| s.name.to_lowercase() == n).or_else(|| all.iter().find(|s| s.name.to_lowercase().contains(&n))).cloned()
}

fn file_name(p: &str) -> String {
    Path::new(p).file_name().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default()
}

fn image_for(name: &str) -> String {
    if let Some(s) = find_shortcut(name) {
        if !s.target.is_empty() {
            return file_name(&s.target);
        }
        if !s.aumid.is_empty() {
            return String::new();
        }
    }
    if Path::new(name).extension().is_some_and(|e| e.eq_ignore_ascii_case("exe")) {
        return file_name(name);
    }
    format!("{name}.exe")
}

fn shell_execute(file: &str) -> Result<(), String> {
    let (op, f) = (wide("open"), wide(file));
    let r = unsafe { ShellExecuteW(std::ptr::null_mut(), op.as_ptr(), f.as_ptr(), std::ptr::null(), std::ptr::null(), SW_SHOWNORMAL) } as isize;
    if r <= 32 {
        return Err(format!("could not open \"{file}\" (code {r})"));
    }
    Ok(())
}

pub fn open_app(name: &str) -> Result<(), String> {
    let name = name.trim();
    if name.is_empty() {
        return Err("the app name is missing".into());
    }
    let found = find_shortcut(name);
    let image = image_for(name);
    if !image.is_empty()
        && let Some(w) = windows_of_image(&image).first()
    {
        bring_to_front(w.hwnd);
        return Ok(());
    }
    match found {
        Some(s) if !s.aumid.is_empty() => {
            let mut c = command("explorer.exe");
            c.arg(format!(r"shell:AppsFolder\{}", s.aumid));
            detached(c)
        }
        Some(s) => shell_execute(&s.path.display().to_string()),
        None => shell_execute(name),
    }
}

pub fn quit_app(name: &str) -> Result<(), String> {
    let name = name.trim();
    if name.is_empty() {
        return Err("the app name is missing".into());
    }
    if let Some(s) = find_shortcut(name).filter(|s| !s.aumid.is_empty()) {
        let ws: Vec<TopWindow> = top_windows().into_iter().filter(|w| w.class == "ApplicationFrameWindow" && w.title.eq_ignore_ascii_case(&s.name)).collect();
        if ws.is_empty() {
            return Err(format!("could not close \"{}\" (is it open?)", s.name));
        }
        close_windows(&ws);
        return Ok(());
    }
    let exe = image_for(name);
    let ws = windows_of_image(&exe);
    if !ws.is_empty() {
        close_windows(&ws);
        return Ok(());
    }
    if run(Duration::from_secs(5), "", "taskkill", &["/IM", &exe]).is_ok() {
        return Ok(());
    }
    Err(format!("could not close \"{name}\" (is it open?)"))
}

pub fn front_names() -> Vec<String> {
    let (_, pid) = foreground();
    if pid == 0 {
        return vec![];
    }
    let img = process_image(pid);
    if img.is_empty() {
        return vec![];
    }
    let base = Path::new(&img).file_stem().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default();
    let img_name = file_name(&img);
    let mut names = vec![base.clone()];
    for s in shortcuts() {
        if (!s.target.is_empty() && file_name(&s.target).eq_ignore_ascii_case(&img_name))
            || (s.target.is_empty() && s.aumid.is_empty() && s.name.eq_ignore_ascii_case(&base))
        {
            names.push(s.name);
        }
    }
    names
}
