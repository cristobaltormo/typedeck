use std::collections::{BTreeSet, HashSet};
use std::sync::OnceLock;

use serde::Serialize;

use crate::jsonx::is_false;

#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Key {
    #[serde(rename = "u")]
    pub usage: u8,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub x: f64,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub y: f64,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub w: f64,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub h: f64,
    #[serde(rename = "mod", skip_serializing_if = "is_false")]
    pub modifier: bool,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Layout {
    pub id: String,
    pub family: String,
    pub percent: i64,
    pub standard: String,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub width: f64,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub height: f64,
    pub keys: Vec<Key>,
}

pub fn is_modifier(u: u8) -> bool {
    (0xE0..=0xE7).contains(&u)
}

#[derive(Clone, Copy)]
struct Item {
    u: u8,
    w: f64,
    h: f64,
}

fn k(u: u8, w: f64) -> Item {
    Item { u, w, h: 1.0 }
}

fn tall(u: u8, w: f64) -> Item {
    Item { u, w, h: 2.0 }
}

fn gap(w: f64) -> Item {
    Item { u: 0, w, h: 1.0 }
}

fn keys(us: &[u8]) -> Vec<Item> {
    us.iter().map(|&u| k(u, 1.0)).collect()
}

fn cat(parts: &[&[Item]]) -> Vec<Item> {
    parts.iter().flat_map(|p| p.iter().copied()).collect()
}

fn build(id: &str, family: &str, percent: i64, standard: &str, rows: &[(f64, Vec<Item>)]) -> Layout {
    let mut l = Layout { id: id.into(), family: family.into(), percent, standard: standard.into(), width: 0.0, height: 0.0, keys: vec![] };
    for (y, items) in rows {
        let mut x = 0.0;
        for it in items {
            if it.u != 0 {
                l.keys.push(Key { usage: it.u, x, y: *y, w: it.w, h: it.h, modifier: is_modifier(it.u) });
            }
            x += it.w;
            if x > l.width {
                l.width = x;
            }
            if it.u != 0 && y + it.h > l.height {
                l.height = y + it.h;
            }
        }
    }
    l
}

struct Parts {
    f1: Vec<Item>,
    f2: Vec<Item>,
    f3: Vec<Item>,
    numbers: Vec<Item>,
    q: Vec<Item>,
    a: Vec<Item>,
    z: Vec<Item>,
    space: Vec<Item>,
}

fn parts() -> Parts {
    Parts {
        f1: keys(&[0x3A, 0x3B, 0x3C, 0x3D]),
        f2: keys(&[0x3E, 0x3F, 0x40, 0x41]),
        f3: keys(&[0x42, 0x43, 0x44, 0x45]),
        numbers: keys(&[0x35, 0x1E, 0x1F, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x2D, 0x2E]),
        q: keys(&[0x14, 0x1A, 0x08, 0x15, 0x17, 0x1C, 0x18, 0x0C, 0x12, 0x13, 0x2F, 0x30]),
        a: keys(&[0x04, 0x16, 0x07, 0x09, 0x0A, 0x0B, 0x0D, 0x0E, 0x0F, 0x33, 0x34]),
        z: keys(&[0x1D, 0x1B, 0x06, 0x19, 0x05, 0x11, 0x10, 0x36, 0x37, 0x38]),
        space: vec![k(0xE0, 1.25), k(0xE3, 1.25), k(0xE2, 1.25), k(0x2C, 6.25), k(0xE6, 1.25), k(0xE7, 1.25), k(0x65, 1.25), k(0xE4, 1.25)],
    }
}

fn letter_rows(p: &Parts, iso: bool, rshift: f64) -> (Vec<Item>, Vec<Item>, Vec<Item>) {
    if iso {
        return (
            cat(&[&[k(0x2B, 1.5)], &p.q, &[gap(0.25)], &[tall(0x28, 1.25)]]),
            cat(&[&[k(0x39, 1.75)], &p.a, &[k(0x32, 1.0)], &[gap(1.25)]]),
            cat(&[&[k(0xE1, 1.25)], &[k(0x64, 1.0)], &p.z, &[k(0xE5, rshift)]]),
        );
    }
    (cat(&[&[k(0x2B, 1.5)], &p.q, &[k(0x31, 1.5)]]), cat(&[&[k(0x39, 1.75)], &p.a, &[k(0x28, 2.25)]]), cat(&[&[k(0xE1, 2.25)], &p.z, &[k(0xE5, rshift)]]))
}

fn std_name(iso: bool) -> &'static str {
    if iso { "iso" } else { "ansi" }
}

fn nav(a: u8, b: u8, c: u8) -> Vec<Item> {
    cat(&[&[gap(0.25)], &keys(&[a, b, c])])
}

fn np(items: &[Item]) -> Vec<Item> {
    cat(&[&[gap(0.25)], items])
}

fn full(iso: bool) -> Layout {
    let p = parts();
    let (tab, caps, shift) = letter_rows(&p, iso, 2.75);
    let rows = vec![
        (0.0, cat(&[&[k(0x29, 1.0)], &[gap(1.0)], &p.f1, &[gap(0.5)], &p.f2, &[gap(0.5)], &p.f3, &nav(0x46, 0x47, 0x48)])),
        (1.5, cat(&[&p.numbers, &[k(0x2A, 2.0)], &nav(0x49, 0x4A, 0x4B), &np(&[k(0x53, 1.0), k(0x54, 1.0), k(0x55, 1.0), k(0x56, 1.0)])])),
        (2.5, cat(&[&tab, &nav(0x4C, 0x4D, 0x4E), &np(&[k(0x5F, 1.0), k(0x60, 1.0), k(0x61, 1.0), tall(0x57, 1.0)])])),
        (3.5, cat(&[&caps, &[gap(3.25)], &np(&[k(0x5C, 1.0), k(0x5D, 1.0), k(0x5E, 1.0)])])),
        (4.5, cat(&[&shift, &[gap(0.25), gap(1.0), k(0x52, 1.0), gap(1.0)], &np(&[k(0x59, 1.0), k(0x5A, 1.0), k(0x5B, 1.0), tall(0x58, 1.0)])])),
        (5.5, cat(&[&p.space, &nav(0x50, 0x51, 0x4F), &np(&[k(0x62, 2.0), k(0x63, 1.0)])])),
    ];
    build(&format!("full-{}", std_name(iso)), "full", 100, std_name(iso), &rows)
}

fn tkl(iso: bool) -> Layout {
    let p = parts();
    let (tab, caps, shift) = letter_rows(&p, iso, 2.75);
    let rows = vec![
        (0.0, cat(&[&[k(0x29, 1.0)], &[gap(1.0)], &p.f1, &[gap(0.5)], &p.f2, &[gap(0.5)], &p.f3, &nav(0x46, 0x47, 0x48)])),
        (1.5, cat(&[&p.numbers, &[k(0x2A, 2.0)], &nav(0x49, 0x4A, 0x4B)])),
        (2.5, cat(&[&tab, &nav(0x4C, 0x4D, 0x4E)])),
        (3.5, caps),
        (4.5, cat(&[&shift, &[gap(0.25), gap(1.0), k(0x52, 1.0)]])),
        (5.5, cat(&[&p.space, &nav(0x50, 0x51, 0x4F)])),
    ];
    build(&format!("tkl-{}", std_name(iso)), "tkl", 80, std_name(iso), &rows)
}

fn compact(iso: bool, seventy_five: bool) -> Layout {
    let p = parts();
    let (tab, mut caps, shift) = letter_rows(&p, iso, 1.75);
    let right = |u: u8| vec![k(u, 1.0)];
    let bottom = cat(&[&[k(0xE0, 1.25), k(0xE3, 1.25), k(0xE2, 1.25), k(0x2C, 6.25), k(0xE6, 1.0), k(0x65, 1.0), k(0xE4, 1.0)], &keys(&[0x50, 0x51, 0x4F])]);
    if iso {
        caps.pop();
        caps.push(gap(1.25));
    }
    let std = std_name(iso);
    if seventy_five {
        let rows = vec![
            (0.0, cat(&[&[k(0x29, 1.0)], &p.f1, &p.f2, &p.f3, &[k(0x4C, 1.0)]])),
            (1.25, cat(&[&p.numbers, &[k(0x2A, 2.0)], &right(0x4A)])),
            (2.25, cat(&[&tab, &right(0x4B)])),
            (3.25, cat(&[&caps, &right(0x4E)])),
            (4.25, cat(&[&shift, &[k(0x52, 1.0)], &right(0x4D)])),
            (5.25, bottom),
        ];
        return build(&format!("75-{std}"), "75", 75, std, &rows);
    }
    let rows = vec![
        (0.0, cat(&[&p.numbers, &[k(0x2A, 2.0)], &right(0x4C)])),
        (1.0, cat(&[&tab, &right(0x4B)])),
        (2.0, cat(&[&caps, &right(0x4E)])),
        (3.0, cat(&[&shift, &[k(0x52, 1.0)], &right(0x4D)])),
        (4.0, bottom),
    ];
    build(&format!("65-{std}"), "65", 65, std, &rows)
}

fn sixty(iso: bool) -> Layout {
    let p = parts();
    let (tab, mut caps, shift) = letter_rows(&p, iso, 2.75);
    if iso {
        caps.pop();
    }
    let rows = vec![(0.0, cat(&[&p.numbers, &[k(0x2A, 2.0)]])), (1.0, tab), (2.0, caps), (3.0, shift), (4.0, p.space.clone())];
    build(&format!("60-{}", std_name(iso)), "60", 60, std_name(iso), &rows)
}

fn numpad() -> Layout {
    let rows = vec![
        (0.0, keys(&[0x53, 0x54, 0x55, 0x56])),
        (1.0, cat(&[&keys(&[0x5F, 0x60, 0x61]), &[tall(0x57, 1.0)]])),
        (2.0, keys(&[0x5C, 0x5D, 0x5E])),
        (3.0, cat(&[&keys(&[0x59, 0x5A, 0x5B]), &[tall(0x58, 1.0)]])),
        (4.0, vec![k(0x62, 2.0), k(0x63, 1.0)]),
    ];
    build("numpad", "numpad", 0, "", &rows)
}

pub fn all() -> &'static [Layout] {
    static ALL: OnceLock<Vec<Layout>> = OnceLock::new();
    ALL.get_or_init(|| {
        let mut out = Vec::new();
        for iso in [true, false] {
            out.extend([full(iso), tkl(iso), compact(iso, true), compact(iso, false), sixty(iso)]);
        }
        out.push(numpad());
        out
    })
}

pub fn canonical(id: &str) -> &str {
    match id {
        "iso-full" => "full-iso",
        "ansi-full" => "full-ansi",
        "iso-tkl" => "tkl-iso",
        "ansi-tkl" => "tkl-ansi",
        other => other,
    }
}

pub fn get(id: &str) -> Option<&'static Layout> {
    let id = canonical(id);
    all().iter().find(|l| l.id == id)
}

impl Layout {
    pub fn key_set(&self) -> HashSet<u8> {
        self.keys.iter().filter(|k| !k.modifier).map(|k| k.usage).collect()
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Suggestion {
    pub id: String,
    pub score: f64,
    pub precision: f64,
    pub recall: f64,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub extra: Vec<i64>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub missing: Vec<i64>,
}

pub fn match_seen(seen_raw: &HashSet<u8>) -> Vec<Suggestion> {
    let mut out = Vec::new();
    for l in all() {
        let set = l.key_set();
        let mut seen = seen_raw.clone();
        if l.standard == "iso" && seen_raw.contains(&0x31) && !seen_raw.contains(&0x32) {
            seen.remove(&0x31);
            seen.insert(0x32);
        }
        let counted: BTreeSet<u8> = seen.iter().copied().filter(|&u| !is_modifier(u) && u >= 4).collect();
        let n = counted.len() as f64;
        if n == 0.0 {
            continue;
        }
        let inter = counted.iter().filter(|u| set.contains(u)).count() as f64;
        let extra: Vec<i64> = counted.iter().filter(|u| !set.contains(u)).map(|&u| i64::from(u)).collect();
        let mut missing: Vec<i64> = set.iter().filter(|u| !seen.contains(u)).map(|&u| i64::from(u)).collect();
        missing.sort_unstable();
        let (p, r) = (inter / n, inter / set.len() as f64);
        let score = if p + r > 0.0 { 2.0 * p * r / (p + r) } else { 0.0 };
        out.push(Suggestion { id: l.id.clone(), score, precision: p, recall: r, extra, missing });
    }
    out.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
    out
}

pub fn ambiguity(r: &[Suggestion]) -> (bool, Vec<i64>) {
    if r.len() < 2 {
        return (false, vec![]);
    }
    let (Some(a), Some(b)) = (get(&r[0].id), get(&r[1].id)) else {
        return (false, vec![]);
    };
    if a.family != b.family
        || a.standard == b.standard
        || a.standard.is_empty()
        || r[0].score - r[1].score > 0.06
        || !r[0].extra.is_empty()
        || !r[1].extra.is_empty()
    {
        return (false, vec![]);
    }
    let hints: Vec<i64> = [0x64, 0x32, 0x31].into_iter().filter(|u| r[0].missing.contains(u) || r[1].missing.contains(u)).collect();
    (!hints.is_empty(), hints)
}

#[cfg(test)]
mod tests;
