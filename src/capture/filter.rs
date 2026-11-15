use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

use super::Event;
use crate::queue::Queue;
use crate::sync::{Signal, lock};

const HELD: u8 = 1;
const SWALLOWED: u8 = 2;
const REPORTED: u8 = 4;
pub const EVENT_BUFFER: usize = 1024;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Msg {
    Event(Event),
    Up(bool),
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Snapshot {
    pub mask: [u8; 32],
    pub report_all: bool,
    pub inactive: bool,
}

struct Keys {
    state: [u8; 256],
    mods: u8,
}

pub struct Filter {
    mask: [AtomicU64; 4],
    report_all: AtomicBool,
    paused: AtomicBool,
    dropped: AtomicU64,
    queue: Arc<Queue<Msg>>,
    changed: Signal,
    keys: Mutex<Keys>,
}

impl Default for Filter {
    fn default() -> Self {
        Self::new()
    }
}

impl Filter {
    pub fn new() -> Filter {
        Filter {
            mask: Default::default(),
            report_all: AtomicBool::new(false),
            paused: AtomicBool::new(false),
            dropped: AtomicU64::new(0),
            queue: Arc::new(Queue::new(usize::MAX)),
            changed: Signal::new(),
            keys: Mutex::new(Keys { state: [0; 256], mods: 0 }),
        }
    }

    pub fn queue(&self) -> &Arc<Queue<Msg>> {
        &self.queue
    }

    pub fn changed(&self) -> &Signal {
        &self.changed
    }

    pub fn snapshot(&self) -> Snapshot {
        let mut s = Snapshot::default();
        for (i, w) in self.mask.iter().enumerate() {
            s.mask[i * 8..i * 8 + 8].copy_from_slice(&w.load(Ordering::SeqCst).to_le_bytes());
        }
        s.report_all = self.report_all.load(Ordering::SeqCst);
        s.inactive = self.paused.load(Ordering::SeqCst);
        s
    }

    pub fn dropped(&self) -> u64 {
        self.dropped.load(Ordering::SeqCst)
    }

    fn emit(&self, ev: Event) {
        let events = self.queue.len();
        if events >= EVENT_BUFFER {
            self.dropped.fetch_add(1, Ordering::SeqCst);
            return;
        }
        self.queue.push(Msg::Event(ev));
    }

    pub fn set_mask(&self, m: [u8; 32]) {
        for (i, w) in self.mask.iter().enumerate() {
            let mut b = [0u8; 8];
            b.copy_from_slice(&m[i * 8..i * 8 + 8]);
            w.store(u64::from_le_bytes(b), Ordering::SeqCst);
        }
        self.changed.notify();
    }

    pub fn set_report_all(&self, on: bool) {
        self.report_all.store(on, Ordering::SeqCst);
        self.changed.notify();
    }

    fn captured(&self, u: u8) -> bool {
        self.mask[usize::from(u >> 6)].load(Ordering::SeqCst) >> (u & 63) & 1 == 1
    }

    pub fn observe(&self, kind: u8, usage: u8, mods: u8) {
        let mut k = lock(&self.keys);
        let st = k.state[usize::from(usage)];
        match kind {
            b'D' if st & HELD == 0 => {
                k.state[usize::from(usage)] = HELD | SWALLOWED;
                self.emit(Event::new(b'D', usage, mods));
            }
            b'P' if st & HELD == 0 => {
                k.state[usize::from(usage)] = HELD | REPORTED;
                self.emit(Event::new(b'P', usage, 0));
            }
            b'U' if st & SWALLOWED != 0 => {
                k.state[usize::from(usage)] = 0;
                self.emit(Event::new(b'U', usage, 0));
            }
            b'R' if st & REPORTED != 0 => {
                k.state[usize::from(usage)] = 0;
                self.emit(Event::new(b'R', usage, 0));
            }
            _ => {}
        }
    }

    pub fn handle(&self, u: u8, down: bool) -> bool {
        if u == 0 || self.paused.load(Ordering::SeqCst) {
            return false;
        }
        let mut k = lock(&self.keys);
        if down { self.press(&mut k, u) } else { self.release(&mut k, u) }
    }

    fn press(&self, k: &mut Keys, u: u8) -> bool {
        let st = k.state[usize::from(u)];
        if st & HELD != 0 {
            return st & SWALLOWED != 0;
        }
        track_mods(k, u, true);
        if self.captured(u) {
            k.state[usize::from(u)] = HELD | SWALLOWED;
            self.emit(Event::new(b'D', u, k.mods));
            return true;
        }
        if self.report_all.load(Ordering::SeqCst) {
            k.state[usize::from(u)] = HELD | REPORTED;
            self.emit(Event::new(b'P', u, 0));
        } else {
            k.state[usize::from(u)] = HELD;
        }
        false
    }

    fn release(&self, k: &mut Keys, u: u8) -> bool {
        let st = k.state[usize::from(u)];
        if st & HELD == 0 {
            return false;
        }
        k.state[usize::from(u)] = 0;
        track_mods(k, u, false);
        if st & SWALLOWED != 0 {
            self.emit(Event::new(b'U', u, 0));
            return true;
        }
        if st & REPORTED != 0 {
            self.emit(Event::new(b'R', u, 0));
        }
        false
    }

    pub fn release_all(&self) {
        let mut k = lock(&self.keys);
        for (u, st) in k.state.iter_mut().enumerate() {
            if *st & SWALLOWED != 0 {
                self.emit(Event::new(b'U', u as u8, 0));
            } else if *st & REPORTED != 0 {
                self.emit(Event::new(b'R', u as u8, 0));
            }
            *st = 0;
        }
        k.mods = 0;
    }

    pub fn pause(&self) {
        self.paused.store(true, Ordering::SeqCst);
        self.changed.notify();
        self.release_all();
    }

    pub fn resume(&self) {
        self.paused.store(false, Ordering::SeqCst);
        self.changed.notify();
    }

    pub fn is_paused(&self) -> bool {
        self.paused.load(Ordering::SeqCst)
    }
}

fn track_mods(k: &mut Keys, u: u8, down: bool) {
    if !(0xE0..=0xE7).contains(&u) {
        return;
    }
    let bit = 1u8 << (u - 0xE0);
    if down {
        k.mods |= bit;
    } else {
        k.mods &= !bit;
    }
}
