mod transcribe;

use std::collections::HashMap;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, Weak};
use std::time::{Duration, Instant, SystemTime};

use serde::{Deserialize, Serialize};

pub use transcribe::transcribe;

use crate::clock;
use crate::hid::{self, Layout};
use crate::sync::lock;
use crate::timer::{self, Timer};

const MAX_BYTES: u64 = 4 << 20;
const FLUSH_AFTER: Duration = Duration::from_millis(1500);
const PRUNE_EVERY: Duration = Duration::from_secs(3600);

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct Entry {
    pub t: i64,
    pub u: u8,
    pub k: String,
    pub d: i64,
}

struct State {
    on: bool,
    keep_for: Duration,
    last_prune: Option<Instant>,
    down: HashMap<u8, SystemTime>,
    buf: Vec<Entry>,
    timer: Option<Timer>,
}

pub struct Log {
    path: PathBuf,
    me: Weak<Log>,
    st: Mutex<State>,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct KeyCount {
    pub k: String,
    pub u: u8,
    pub n: i64,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct Summary {
    pub total: i64,
    pub today: i64,
    pub avg_ms: i64,
    pub top: Option<Vec<KeyCount>>,
}

pub struct Report {
    pub entries: Vec<Entry>,
    pub summary: Summary,
    pub text: String,
}

impl Log {
    pub fn new(path: impl Into<PathBuf>) -> Arc<Log> {
        Arc::new_cyclic(|me| Log {
            path: path.into(),
            me: me.clone(),
            st: Mutex::new(State { on: false, keep_for: Duration::ZERO, last_prune: None, down: HashMap::new(), buf: Vec::new(), timer: None }),
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn set_retention(&self, d: Duration) {
        let mut st = lock(&self.st);
        st.keep_for = d;
        self.prune_locked(&mut st, SystemTime::now());
    }

    fn prune_locked(&self, st: &mut State, now: SystemTime) {
        st.last_prune = Some(Instant::now());
        if st.keep_for.is_zero() {
            return;
        }
        let cutoff = clock::unix_millis(now) - st.keep_for.as_millis() as i64;
        let all = self.read_all_locked(st);
        let keep: Vec<&Entry> = all.iter().filter(|e| e.t >= cutoff).collect();
        if keep.len() == all.len() {
            return;
        }
        if keep.is_empty() {
            let _ = fs::remove_file(&self.path);
            return;
        }
        let mut b = Vec::new();
        for e in keep {
            if let Ok(line) = serde_json::to_vec(e) {
                b.extend_from_slice(&line);
                b.push(b'\n');
            }
        }
        let _ = write_private(&self.path, &b);
    }

    pub fn enabled(&self) -> bool {
        lock(&self.st).on
    }

    pub fn set_enabled(&self, on: bool) {
        let mut st = lock(&self.st);
        if st.on && !on {
            self.flush_locked(&mut st);
        }
        st.on = on;
        st.down.clear();
    }

    pub fn press(&self, u: u8, at: SystemTime) {
        let mut st = lock(&self.st);
        if st.on {
            st.down.entry(u).or_insert(at);
        }
    }

    pub fn release(&self, u: u8, at: SystemTime) {
        let mut st = lock(&self.st);
        if !st.on {
            return;
        }
        let Some(start) = st.down.remove(&u) else { return };
        let d = at.duration_since(start).map(|d| d.as_millis() as i64).unwrap_or(0);
        st.buf.push(Entry { t: clock::unix_millis(start), u, k: name(u), d });
        if st.timer.is_none() {
            let me = self.me.clone();
            st.timer = Some(timer::after(FLUSH_AFTER, move || {
                crate::sync::background(move || {
                    if let Some(l) = me.upgrade() {
                        let mut st = lock(&l.st);
                        l.flush_locked(&mut st);
                    }
                });
            }));
        }
    }

    fn flush_locked(&self, st: &mut State) {
        if let Some(t) = st.timer.take() {
            t.cancel();
        }
        if st.buf.is_empty() {
            return;
        }
        let buf = std::mem::take(&mut st.buf);
        let mut opts = fs::OpenOptions::new();
        opts.create(true).append(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            opts.mode(0o600);
        }
        let Ok(mut f) = opts.open(&self.path) else { return };
        let mut out = Vec::new();
        for e in &buf {
            if let Ok(b) = serde_json::to_vec(e) {
                out.extend_from_slice(&b);
                out.push(b'\n');
            }
        }
        let _ = f.write_all(&out);
        let size = f.metadata().map(|m| m.len()).unwrap_or(0);
        drop(f);
        if size > MAX_BYTES {
            self.trim();
        }
        if st.last_prune.is_none_or(|t| t.elapsed() > PRUNE_EVERY) {
            self.prune_locked(st, SystemTime::now());
        }
    }

    fn trim(&self) {
        let Ok(data) = fs::read(&self.path) else { return };
        let mut cut = data.len() / 2;
        while cut < data.len() && data[cut] != b'\n' {
            cut += 1;
        }
        let _ = write_private(&self.path, &data[(cut + 1).min(data.len())..]);
    }

    fn read_all_locked(&self, st: &mut State) -> Vec<Entry> {
        self.flush_locked(st);
        let Ok(data) = fs::read(&self.path) else { return Vec::new() };
        data.split(|b| *b == b'\n').filter_map(|l| serde_json::from_slice::<Entry>(l).ok()).collect()
    }

    pub fn recent(&self, limit: usize) -> (Vec<Entry>, usize) {
        let mut st = lock(&self.st);
        let all = self.read_all_locked(&mut st);
        (newest(&all, limit), all.len())
    }

    pub fn report(&self, limit: usize, text_entries: usize, now: SystemTime, layout: &Layout) -> Report {
        let all = {
            let mut st = lock(&self.st);
            self.read_all_locked(&mut st)
        };
        let tail = &all[all.len().saturating_sub(text_entries)..];
        Report { entries: newest(&all, limit), summary: summarize(&all, now), text: transcribe(tail, Some(layout)) }
    }

    pub fn summary(&self, now: SystemTime) -> Summary {
        let all = {
            let mut st = lock(&self.st);
            self.read_all_locked(&mut st)
        };
        summarize(&all, now)
    }

    pub fn clear(&self) -> std::io::Result<()> {
        let mut st = lock(&self.st);
        st.buf.clear();
        st.down.clear();
        if let Some(t) = st.timer.take() {
            t.cancel();
        }
        match fs::remove_file(&self.path) {
            Err(e) if e.kind() != std::io::ErrorKind::NotFound => Err(e),
            _ => Ok(()),
        }
    }

    pub fn flush(&self) {
        let mut st = lock(&self.st);
        self.flush_locked(&mut st);
    }
}

fn write_private(path: &Path, data: &[u8]) -> std::io::Result<()> {
    crate::config::write_private_file(path, data)
}

fn newest(all: &[Entry], limit: usize) -> Vec<Entry> {
    all.iter().rev().take(limit).cloned().collect()
}

fn summarize(all: &[Entry], now: SystemTime) -> Summary {
    let mut sum = Summary::default();
    let midnight = clock::local_midnight_millis(now);
    let mut counts: Vec<KeyCount> = Vec::new();
    let mut total_ms: i64 = 0;
    for e in all {
        sum.total += 1;
        total_ms += e.d;
        if e.t >= midnight {
            sum.today += 1;
        }
        match counts.iter_mut().find(|c| c.u == e.u) {
            Some(c) => c.n += 1,
            None => counts.push(KeyCount { k: e.k.clone(), u: e.u, n: 1 }),
        }
    }
    if sum.total > 0 {
        sum.avg_ms = total_ms / sum.total;
    }
    counts.sort_by(|a, b| b.n.cmp(&a.n).then(a.u.cmp(&b.u)));
    counts.truncate(5);
    if !counts.is_empty() {
        sum.top = Some(counts);
    }
    sum
}

const MOD_NAMES: [&str; 8] = ["ctrl", "shift", "alt", "cmd", "ctrl", "shift", "alt", "cmd"];

pub fn name(u: u8) -> String {
    match u {
        0x04..=0x1D => ((b'A' + u - 0x04) as char).to_string(),
        0x1E..=0x26 => ((b'1' + u - 0x1E) as char).to_string(),
        0x27 => "0".into(),
        0xE0..=0xE7 => MOD_NAMES[usize::from(u - 0xE0)].into(),
        _ => hid::usage_name(u),
    }
}

#[cfg(test)]
mod tests;
