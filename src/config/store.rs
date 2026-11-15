use std::fs;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

use serde::Serialize;

use super::{Config, is_backup_name, parse, validate};
use crate::platform;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Paths {
    pub dir: PathBuf,
}

impl Paths {
    pub fn new(dir: impl Into<PathBuf>) -> Paths {
        Paths { dir: dir.into() }
    }

    pub fn config(&self) -> PathBuf {
        self.dir.join("config.json")
    }

    pub fn backups(&self) -> PathBuf {
        self.dir.join("backups")
    }

    pub fn stats(&self) -> PathBuf {
        self.dir.join("stats.json")
    }

    pub fn port_file(&self) -> PathBuf {
        self.dir.join("port")
    }

    pub fn keystrokes(&self) -> PathBuf {
        platform::private_dir(&self.dir).join("keystrokes.jsonl")
    }

    pub fn capture_consent(&self) -> PathBuf {
        self.dir.join("capture-consent.json")
    }

    pub fn default_paths() -> Paths {
        let p = Paths { dir: platform::get().config_dir() };
        if !p.dir.exists()
            && let Some(parent) = p.dir.parent()
        {
            for old in crate::app::OLD_SLUGS {
                let dir = parent.join(old);
                if dir.join("config.json").exists() {
                    let _ = copy_dir(&dir, &p.dir);
                    break;
                }
            }
        }
        platform::migrate_private(&p.dir, "keystrokes.jsonl");
        p
    }
}

fn copy_dir(src: &Path, dst: &Path) -> std::io::Result<()> {
    fs::create_dir_all(dst)?;
    for e in fs::read_dir(src)? {
        let e = e?;
        let target = dst.join(e.file_name());
        if e.file_type()?.is_dir() {
            copy_dir(&e.path(), &target)?;
        } else {
            write_private(&target, &fs::read(e.path())?)?;
        }
    }
    Ok(())
}

pub fn write_private(path: &Path, data: &[u8]) -> std::io::Result<()> {
    let mut opts = fs::OpenOptions::new();
    opts.write(true).create(true).truncate(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        opts.mode(0o600);
    }
    use std::io::Write;
    opts.open(path)?.write_all(data)
}

pub fn write_atomic(path: &Path, data: &[u8]) -> std::io::Result<()> {
    use std::sync::atomic::{AtomicU64, Ordering};
    static SEQ: AtomicU64 = AtomicU64::new(0);
    let mut tmp = path.as_os_str().to_owned();
    tmp.push(format!(".tmp-{}-{}", std::process::id(), SEQ.fetch_add(1, Ordering::Relaxed)));
    let tmp = PathBuf::from(tmp);
    if let Err(e) = write_private(&tmp, data) {
        let _ = fs::remove_file(&tmp);
        return Err(e);
    }
    fs::rename(&tmp, path).inspect_err(|_| {
        let _ = fs::remove_file(&tmp);
    })
}

fn backup_files(p: &Paths) -> Vec<PathBuf> {
    let mut files: Vec<PathBuf> = fs::read_dir(p.backups())
        .map(|rd| rd.flatten().map(|e| e.path()).filter(|f| f.file_name().and_then(|n| n.to_str()).is_some_and(is_backup_name)).collect())
        .unwrap_or_default();
    files.sort();
    files
}

pub fn load(p: &Paths) -> Result<Config, String> {
    load_with_note(p).map(|(c, _)| c)
}

pub fn load_with_note(p: &Paths) -> Result<(Config, String), String> {
    let data = match fs::read(p.config()) {
        Ok(d) => d,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            let c = Config::initial();
            save(p, &c, false, false)?;
            return Ok((validate(&c)?, String::new()));
        }
        Err(e) => return Err(e.to_string()),
    };
    let perr = match parse(&data) {
        Ok(c) => return Ok((c, String::new())),
        Err(e) => e,
    };
    let broken = p.dir.join(format!("config.broken-{}.json", crate::clock::now_local().stamp()));
    let _ = fs::rename(p.config(), &broken);
    let broken_name = broken.file_name().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default();
    for f in backup_files(p).iter().rev() {
        let Ok(raw) = fs::read(f) else { continue };
        if let Ok(rc) = parse(&raw)
            && save(p, &rc, false, false).is_ok()
        {
            let name = f.file_name().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default();
            return Ok((rc, format!("the configuration was damaged ({perr}): backup {name} was restored and the original was kept as {broken_name}")));
        }
    }
    let d = Config::initial();
    save(p, &d, false, false)?;
    Ok((
        validate(&d)?,
        format!("the configuration was damaged ({perr}) and there were no valid backups: starting from scratch; the original was kept as {broken_name}"),
    ))
}

pub fn to_pretty(c: &Config) -> Result<Vec<u8>, String> {
    serde_json::to_vec_pretty(c).map_err(|e| e.to_string())
}

static SAVE: std::sync::Mutex<()> = std::sync::Mutex::new(());

pub fn save(p: &Paths, c: &Config, backup: bool, force: bool) -> Result<(), String> {
    let c = validate(c)?;
    let _g = crate::sync::lock(&SAVE);
    fs::create_dir_all(&p.dir).map_err(|e| e.to_string())?;
    if backup && let Ok(old) = fs::read(p.config()) {
        let _ = fs::create_dir_all(p.backups());
        let mut files = backup_files(p);
        let recent = files
            .last()
            .and_then(|f| fs::metadata(f).ok())
            .and_then(|m| m.modified().ok())
            .is_some_and(|t| SystemTime::now().duration_since(t).unwrap_or_default() < Duration::from_secs(120));
        if force || !recent {
            let mut name = p.backups().join(format!("config-{}.json", crate::clock::now_local().stamp()));
            if files.contains(&name) {
                name = unique_backup(p, &files);
            }
            if write_private(&name, &old).is_ok() {
                files.push(name);
                files.sort();
            }
            while files.len() > 40 {
                let _ = fs::remove_file(&files[0]);
                files.remove(0);
            }
        }
    }
    write_atomic(&p.config(), &to_pretty(&c)?).map_err(|e| e.to_string())
}

fn unique_backup(p: &Paths, files: &[PathBuf]) -> PathBuf {
    let mut t = SystemTime::now();
    loop {
        t += Duration::from_secs(1);
        let name = p.backups().join(format!("config-{}.json", crate::clock::local(t).stamp()));
        if !files.contains(&name) {
            return name;
        }
    }
}

#[derive(Clone, Debug, Serialize, PartialEq)]
pub struct BackupInfo {
    pub name: String,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub ts: f64,
    pub layers: usize,
    pub keys: usize,
}

pub fn list_backups(p: &Paths) -> Vec<BackupInfo> {
    let mut out = Vec::new();
    for f in backup_files(p).iter().rev() {
        let Ok(b) = fs::read(f) else { continue };
        let Ok(c) = parse(&b) else { continue };
        let ts = fs::metadata(f).and_then(|m| m.modified()).map(crate::clock::unix_secs).unwrap_or(0) as f64;
        out.push(BackupInfo {
            name: f.file_name().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default(),
            ts,
            layers: c.layers.len(),
            keys: c.key_count(),
        });
    }
    out
}

pub fn restore_backup(p: &Paths, name: &str) -> Result<Config, String> {
    if !is_backup_name(name) {
        return Err("unknown backup".into());
    }
    let b = fs::read(p.backups().join(name)).map_err(|e| e.to_string())?;
    let c = parse(&b)?;
    save(p, &c, true, true)?;
    Ok(c)
}
