use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use crate::sync::lock;

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct DesktopEntry {
    pub id: String,
    pub name: String,
    pub exec: String,
    pub wm_class: String,
    pub path: PathBuf,
}

pub fn parse_desktop_text(id: &str, path: &Path, text: &str) -> Option<DesktopEntry> {
    let mut e = DesktopEntry { id: id.to_string(), path: path.to_path_buf(), ..Default::default() };
    let (mut in_main, mut typ, mut hidden) = (false, String::new(), false);
    for l in text.lines() {
        let l = l.trim();
        if l.starts_with('[') {
            in_main = l == "[Desktop Entry]";
            continue;
        }
        if !in_main || l.is_empty() || l.starts_with('#') {
            continue;
        }
        let Some((k, v)) = l.split_once('=') else { continue };
        match k {
            "Name" => e.name = v.to_string(),
            "Exec" => e.exec = v.to_string(),
            "StartupWMClass" => e.wm_class = v.to_string(),
            "Type" => typ = v.to_string(),
            "NoDisplay" | "Hidden" if v.eq_ignore_ascii_case("true") => hidden = true,
            _ => {}
        }
    }
    (typ == "Application" && !hidden && !e.name.is_empty() && !e.exec.is_empty()).then_some(e)
}

pub fn data_dirs() -> Vec<PathBuf> {
    let home = super::super::env_var("HOME").map(PathBuf::from).unwrap_or_default();
    let mut dirs = vec![super::super::env_var("XDG_DATA_HOME").map(PathBuf::from).unwrap_or_else(|| home.join(".local/share"))];
    let xdg = super::super::env_var("XDG_DATA_DIRS").unwrap_or_else(|| "/usr/local/share:/usr/share".into());
    dirs.extend(xdg.split(':').filter(|d| !d.is_empty()).map(PathBuf::from));
    dirs.extend([PathBuf::from("/var/lib/flatpak/exports/share"), home.join(".local/share/flatpak/exports/share"), PathBuf::from("/var/lib/snapd/desktop")]);
    dirs
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
        } else if p.extension().is_some_and(|e| e == "desktop") {
            out.push(p);
        }
    }
}

pub fn load_entries_from(dirs: &[PathBuf]) -> Vec<DesktopEntry> {
    let mut out: Vec<DesktopEntry> = Vec::new();
    for d in dirs {
        let mut files = Vec::new();
        walk(&d.join("applications"), 0, &mut files);
        for p in files {
            let id = p.file_stem().map(|s| s.to_string_lossy().into_owned()).unwrap_or_default();
            if out.iter().any(|e| e.id == id) {
                continue;
            }
            let Ok(text) = std::fs::read_to_string(&p) else { continue };
            if let Some(e) = parse_desktop_text(&id, &p, &text) {
                out.push(e);
            }
        }
    }
    out.sort_by_key(|e| e.name.to_lowercase());
    out
}

static CACHE: Mutex<Option<(Instant, Vec<DesktopEntry>)>> = Mutex::new(None);

pub fn entries() -> Vec<DesktopEntry> {
    let mut c = lock(&CACHE);
    if let Some((at, list)) = c.as_ref()
        && at.elapsed() < Duration::from_secs(60)
    {
        return list.clone();
    }
    let list = load_entries_from(&data_dirs());
    *c = Some((Instant::now(), list.clone()));
    list
}

pub fn find_entry_in(list: &[DesktopEntry], name: &str) -> Option<DesktopEntry> {
    let n = name.trim().to_lowercase();
    if n.is_empty() {
        return None;
    }
    list.iter()
        .find(|e| e.name.to_lowercase() == n || e.id.to_lowercase() == n || (!e.wm_class.is_empty() && e.wm_class.to_lowercase() == n))
        .or_else(|| list.iter().find(|e| e.name.to_lowercase().contains(&n) || e.id.to_lowercase().ends_with(&format!(".{n}"))))
        .cloned()
}

pub fn entry_for_in(list: &[DesktopEntry], class: &str, comm: &str) -> Option<DesktopEntry> {
    list.iter()
        .find(|e| !class.is_empty() && (e.wm_class.eq_ignore_ascii_case(class) || e.id.eq_ignore_ascii_case(class)))
        .or_else(|| list.iter().find(|e| !comm.is_empty() && exec_base(&e.exec).eq_ignore_ascii_case(comm)))
        .cloned()
}

pub fn exec_base(exec: &str) -> String {
    let f: Vec<&str> = exec.split_whitespace().skip_while(|w| *w == "env" || w.contains('=')).collect();
    let Some(first) = f.first() else { return String::new() };
    let p = first.trim_matches('"');
    Path::new(p).file_name().map(|s| s.to_string_lossy().into_owned()).unwrap_or_default()
}

pub fn expand_exec(exec: &str) -> Vec<String> {
    exec.split_whitespace().filter(|w| !(w.len() == 2 && w.starts_with('%'))).map(|w| w.trim_matches('"').to_string()).collect()
}
