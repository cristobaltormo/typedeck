use super::{Entry, name};
use crate::hid::{Layout, MOD_ALT, MOD_ALTGR, MOD_SHIFT};

const MAX_TRANSCRIPT: usize = 4000;
const HOLD_SLACK_MS: i64 = 40;

pub fn transcribe(entries: &[Entry], layout: Option<&Layout>) -> String {
    let mut sorted: Vec<&Entry> = entries.iter().collect();
    sorted.sort_by_key(|e| e.t);
    let held: Vec<(i64, i64, u8)> =
        sorted.iter().filter(|e| (0xE0..=0xE7).contains(&e.u)).map(|e| (e.t, e.t + e.d + HOLD_SLACK_MS, 1u8 << (e.u - 0xE0))).collect();
    let mac_option = layout.is_some_and(|l| l.name == "es-iso");
    let mut out: Vec<char> = Vec::new();
    let mut caps = false;
    for e in sorted {
        if (0xE0..=0xE7).contains(&e.u) {
            continue;
        }
        if e.u == 0x39 {
            caps = !caps;
            continue;
        }
        let bits = held.iter().filter(|(from, to, _)| e.t >= *from && e.t <= *to).fold(0u8, |acc, (_, _, b)| acc | b);
        let mut shift = bits & 0x22 != 0;
        let ctrl = bits & 0x11 != 0;
        let gui = bits & 0x88 != 0;
        let alt_l = bits & 0x04 != 0;
        let alt_gr = bits & 0x40 != 0;
        if (0x04..=0x1D).contains(&e.u) && caps {
            shift = !shift;
        }
        if ctrl || gui || (alt_l && !mac_option) {
            out.extend(shortcut(e, ctrl, gui, alt_l, shift).chars());
            continue;
        }
        if e.u == 0x2A {
            out.pop();
            continue;
        }
        let mut mods = 0u8;
        if shift {
            mods |= MOD_SHIFT;
        }
        if alt_l {
            mods |= MOD_ALT;
        }
        if alt_gr {
            mods |= MOD_ALTGR;
        }
        if let Some(c) = layout.and_then(|l| l.rune(mods, e.u)) {
            out.push(c);
        }
    }
    let start = out.len().saturating_sub(MAX_TRANSCRIPT);
    out[start..].iter().collect()
}

fn shortcut(e: &Entry, ctrl: bool, gui: bool, alt: bool, shift: bool) -> String {
    let mut parts = Vec::new();
    if ctrl {
        parts.push("Ctrl".to_string());
    }
    if alt {
        parts.push("Alt".to_string());
    }
    if shift {
        parts.push("Shift".to_string());
    }
    if gui {
        parts.push("Cmd".to_string());
    }
    parts.push(name(e.u));
    format!("[{}]", parts.join("+"))
}
