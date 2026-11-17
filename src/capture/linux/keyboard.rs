use std::collections::HashSet;
use std::io;
use std::os::fd::{AsRawFd, OwnedFd};
use std::sync::Arc;
use std::time::Instant;

use super::session::Session;
use super::sys::*;
use crate::capture::{Filter, usage_from_evdev};

#[derive(Default)]
pub struct KeyState {
    pub swallowed: HashSet<u16>,
    pub forwarded: HashSet<u16>,
}

impl KeyState {
    pub fn consume(&mut self, filter: &Filter, ev: &InputEvent) -> bool {
        match ev.value {
            1 => {
                if filter.handle(usage_from_evdev(ev.code), true) {
                    self.swallowed.insert(ev.code);
                    return true;
                }
                self.forwarded.insert(ev.code);
                false
            }
            0 => {
                let was = self.swallowed.remove(&ev.code);
                self.forwarded.remove(&ev.code);
                filter.handle(usage_from_evdev(ev.code), false);
                was
            }
            _ => self.swallowed.contains(&ev.code),
        }
    }

    pub fn known(&self) -> Vec<u16> {
        let mut all: Vec<u16> = self.swallowed.union(&self.forwarded).copied().collect();
        all.sort_unstable();
        all
    }
}

pub fn drop_scan(out: &mut Vec<InputEvent>) {
    if out.last().is_some_and(|e| e.kind == EV_MSC && e.code == MSC_SCAN) {
        out.pop();
    }
}

pub struct Keyboard {
    pub path: String,
    pub fd: Arc<OwnedFd>,
    keys: KeyState,
    pending: Vec<InputEvent>,
    dropped: bool,
}

impl Keyboard {
    pub fn new(path: String, fd: Arc<OwnedFd>) -> Keyboard {
        Keyboard { path, fd, keys: KeyState::default(), pending: Vec::new(), dropped: false }
    }

    pub fn run(mut self, s: &Session) {
        if !self.wait_idle(s) {
            return;
        }
        self.drain();
        if let Err(e) = ioctl_val(self.fd.as_raw_fd(), EVIOCGRAB, 1) {
            crate::log!("capture: cannot take {}: {e}", self.path);
            s.grab_failed(&self.path, &e);
            return;
        }
        s.grabbed(&self.path);
        self.read(s);
        self.release_all(s);
    }

    fn wait_idle(&self, s: &Session) -> bool {
        loop {
            let Ok(bits) = pressed_keys(self.fd.as_raw_fd()) else { return false };
            if bits.iter().all(|b| *b == 0) {
                return true;
            }
            match wait_readable(&[s.stop_fd()], 100) {
                Some(r) if !r[0] => {}
                _ => return false,
            }
        }
    }

    fn drain(&self) {
        let mut buf = [InputEvent::syn(); 64];
        while read_events(self.fd.as_raw_fd(), &mut buf).is_ok() {}
    }

    fn read(&mut self, s: &Session) {
        let mut buf = [InputEvent::syn(); 64];
        loop {
            let Some(ready) = wait_readable(&[self.fd.as_raw_fd(), s.stop_fd()], -1) else { return };
            if ready[1] {
                return;
            }
            match read_events(self.fd.as_raw_fd(), &mut buf) {
                Ok(n) => {
                    let start = Instant::now();
                    self.handle(s, &buf[..n], start);
                }
                Err(e) if e.kind() == io::ErrorKind::WouldBlock || e.kind() == io::ErrorKind::Interrupted => {}
                Err(_) => return,
            }
        }
    }

    pub fn handle(&mut self, s: &Session, evs: &[InputEvent], start: Instant) {
        for ev in evs {
            if ev.kind == EV_SYN && ev.code == SYN_DROPPED {
                self.dropped = true;
                self.pending.clear();
            } else if ev.kind == EV_SYN && ev.code == SYN_REPORT {
                if self.dropped {
                    self.dropped = false;
                    self.resync();
                }
                self.flush(s, start);
            } else if !self.dropped {
                self.pending.push(*ev);
            }
        }
    }

    fn flush(&mut self, s: &Session, start: Instant) {
        let mut out = Vec::with_capacity(self.pending.len() + 1);
        let mut saw_key = false;
        for ev in std::mem::take(&mut self.pending) {
            if ev.kind == EV_KEY {
                saw_key = true;
                if self.keys.consume(s.filter(), &ev) {
                    drop_scan(&mut out);
                    continue;
                }
            }
            out.push(ev);
        }
        if out.is_empty() {
            return;
        }
        out.push(InputEvent::syn());
        if let Ok(v) = s.virt() {
            let _ = v.write(&out);
        }
        if saw_key {
            s.latency().record(start.elapsed());
        }
    }

    fn resync(&mut self) {
        let Ok(bits) = pressed_keys(self.fd.as_raw_fd()) else { return };
        for code in self.keys.known() {
            if !bit_set(&bits, usize::from(code)) {
                self.pending.push(InputEvent::key(code, 0));
            }
        }
        for code in 1..=KEY_MAX as u16 {
            if bit_set(&bits, usize::from(code)) && !self.keys.swallowed.contains(&code) && !self.keys.forwarded.contains(&code) {
                self.pending.push(InputEvent::key(code, 1));
            }
        }
    }

    fn release_all(&mut self, s: &Session) {
        for code in self.keys.known() {
            self.pending.push(InputEvent::key(code, 0));
        }
        self.flush(s, Instant::now());
    }
}
