use std::collections::{BTreeMap, HashSet};
use std::sync::OnceLock;

use serde::{Deserialize, Serialize};

use crate::config::{Action, KeyDef, Keys, Layer};
use crate::layouts;

pub type Text = BTreeMap<String, String>;

pub fn text_get(t: &Text, lang: &str) -> String {
    match t.get(lang) {
        Some(s) if !s.is_empty() => s.clone(),
        _ => t.get("es").cloned().unwrap_or_default(),
    }
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct Macro {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub key: String,
    pub label: Text,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub color: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tap: Option<Action>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hold: Option<Action>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub double: Option<Action>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tap_pc: Option<Action>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hold_pc: Option<Action>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub double_pc: Option<Action>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct Pack {
    pub id: String,
    pub category: String,
    pub icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub color: String,
    pub name: Text,
    pub description: Text,
    #[serde(skip_serializing_if = "BTreeMap::is_empty")]
    pub notes: Text,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub apps: Vec<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub auto_apps: Vec<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
    pub region: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub os: Vec<String>,
    pub macros: Vec<Macro>,
    #[serde(skip_serializing_if = "BTreeMap::is_empty")]
    pub global: Keys,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct Category {
    pub id: String,
    pub icon: String,
    pub name: Text,
}

#[derive(Default, Deserialize)]
#[serde(default)]
struct Catalog {
    categories: Vec<Category>,
    packs: Vec<Pack>,
}

fn catalog() -> &'static Catalog {
    static C: OnceLock<Catalog> = OnceLock::new();
    C.get_or_init(|| serde_json::from_str(include_str!("data/packs.json")).unwrap_or_default())
}

pub fn all() -> &'static [Pack] {
    &catalog().packs
}

pub fn categories() -> &'static [Category] {
    &catalog().categories
}

pub fn get(id: &str) -> Option<&'static Pack> {
    all().iter().find(|p| p.id == id)
}

impl Pack {
    pub fn for_os(&self, os: &str) -> bool {
        self.os.is_empty() || self.os.iter().any(|o| o == os)
    }
}

pub fn for_os(os: &str) -> Vec<&'static Pack> {
    all().iter().filter(|p| p.for_os(os)).collect()
}

fn region_keys(region: &str) -> &'static [u8] {
    match region {
        "numpad" => &[0x5F, 0x60, 0x61, 0x5C, 0x5D, 0x5E, 0x59, 0x5A, 0x5B, 0x62, 0x63, 0x54, 0x55, 0x56, 0x57],
        "fn" => &[0x3A, 0x3B, 0x3C, 0x3D, 0x3E, 0x3F, 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48],
        "numrow" => &[0x1E, 0x1F, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x2D, 0x2E],
        "letters" => &[0x14, 0x1A, 0x08, 0x15, 0x17, 0x1C, 0x18, 0x0C, 0x12, 0x13, 0x04, 0x16, 0x07, 0x09, 0x0A, 0x0B, 0x0D, 0x0E, 0x0F],
        _ => &[],
    }
}

pub fn is_region(region: &str) -> bool {
    !region_keys(region).is_empty()
}

#[derive(Clone, Debug, Default, Serialize)]
pub struct Resolved {
    pub layer: Layer,
    pub global: Keys,
    pub region: String,
    pub dropped: usize,
}

fn pc_action(a: &Action) -> Action {
    let mut c = a.clone();
    match c.kind.as_str() {
        "hotkey" => {
            let mut parts: Vec<String> = Vec::new();
            let mut seen = HashSet::new();
            for t in c.keys.to_lowercase().split('+') {
                let t = if matches!(t, "cmd" | "command" | "gui") { "ctrl" } else { t };
                if t == "ctrl" && seen.contains(t) {
                    continue;
                }
                seen.insert(t.to_string());
                parts.push(t.to_string());
            }
            c.keys = parts.join("+");
        }
        "sequence" => c.steps = a.steps.iter().map(pc_action).collect(),
        _ => {}
    }
    c
}

fn region_slots(region: &str, have: &HashSet<u8>, reserved: &HashSet<String>) -> Vec<u8> {
    region_keys(region).iter().copied().filter(|u| have.contains(u) && !reserved.contains(&format!("{u:02X}"))).collect()
}

fn choose_region(p: &Pack, have: &HashSet<u8>, reserved: &HashSet<String>) -> (String, Vec<u8>) {
    let mut order = vec![p.region.as_str()];
    order.extend(["numpad", "fn", "numrow", "letters"].into_iter().filter(|r| *r != p.region));
    let need = p.macros.len();
    let mut best: Option<(&str, Vec<u8>)> = None;
    for r in order {
        let s = region_slots(r, have, reserved);
        if s.len() >= need || (s.len() >= need.div_ceil(2) && !s.is_empty() && r == p.region) {
            return (r.to_string(), s);
        }
        if best.as_ref().is_none_or(|(_, b)| s.len() > b.len()) {
            best = Some((r, s));
        }
    }
    best.map(|(r, s)| (r.to_string(), s)).unwrap_or_default()
}

fn key_def_for(os: &str, m: &Macro, lang: &str) -> KeyDef {
    let (tap, hold, double) = if os == "darwin" {
        (m.tap.clone(), m.hold.clone(), m.double.clone())
    } else {
        let pick = |pc: &Option<Action>, mac: &Option<Action>| pc.clone().or_else(|| mac.as_ref().map(pc_action));
        (pick(&m.tap_pc, &m.tap), pick(&m.hold_pc, &m.hold), pick(&m.double_pc, &m.double))
    };
    KeyDef { tap, hold, double, label: text_get(&m.label, lang), icon: m.icon.clone(), color: m.color.clone() }
}

pub fn resolve_for(os: &str, p: &Pack, layout_id: &str, lang: &str, reserved: &HashSet<String>) -> Resolved {
    let l = layouts::get(layout_id).or_else(|| layouts::get("full-iso"));
    let have = l.map(layouts::Layout::key_set).unwrap_or_default();
    let (region, slots) = choose_region(p, &have, reserved);
    let mut res = Resolved {
        region,
        layer: Layer { name: text_get(&p.name, lang), color: p.color.clone(), icon: p.icon.clone(), auto_apps: p.auto_apps.clone(), keys: Keys::new() },
        ..Default::default()
    };
    let mut free = slots;
    let mut pending = Vec::new();
    for m in &p.macros {
        let fixed = (m.key.len() == 2).then(|| u8::from_str_radix(&m.key, 16).ok()).flatten();
        if let Some(u) = fixed
            && have.contains(&u)
            && !reserved.contains(&m.key)
        {
            res.layer.keys.insert(m.key.clone(), key_def_for(os, m, lang));
            free.retain(|f| *f != u);
            continue;
        }
        pending.push(m);
    }
    for (i, m) in pending.into_iter().enumerate() {
        match free.get(i) {
            Some(u) => {
                res.layer.keys.insert(format!("{u:02X}"), key_def_for(os, m, lang));
            }
            None => res.dropped += 1,
        }
    }
    res.global = p.global.clone();
    res
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{Config, validate};

    fn none() -> HashSet<String> {
        HashSet::new()
    }

    #[test]
    fn catalog_is_consistent() {
        let cats: HashSet<&str> = categories().iter().map(|c| c.id.as_str()).collect();
        for c in categories() {
            assert!(!text_get(&c.name, "es").is_empty() && c.name.get("en").is_some_and(|s| !s.is_empty()), "{}", c.id);
        }
        let mut ids = HashSet::new();
        for p in all() {
            assert!(ids.insert(p.id.as_str()), "repeated {}", p.id);
            assert!(cats.contains(p.category.as_str()), "{}", p.id);
            for f in [&p.name, &p.description] {
                assert!(f.get("es").is_some_and(|s| !s.is_empty()) && f.get("en").is_some_and(|s| !s.is_empty()), "{}", p.id);
            }
            if !p.notes.is_empty() {
                assert!(p.notes.contains_key("es") && p.notes.contains_key("en"), "{}", p.id);
            }
            assert!((3..=15).contains(&p.macros.len()), "{}", p.id);
            assert!(is_region(&p.region), "{}", p.id);
            for m in &p.macros {
                assert!(m.label.contains_key("es") && m.label.contains_key("en"), "{}", p.id);
                assert!(m.tap.is_some() || m.hold.is_some() || m.double.is_some(), "{}", p.id);
            }
        }
        assert_eq!(all().len(), 38);
    }

    #[test]
    fn every_pack_fits_every_keyboard() {
        let reserved: HashSet<String> = ["53".to_string(), "58".to_string()].into();
        for p in all() {
            for l in layouts::all() {
                let r = resolve_for("darwin", p, &l.id, "es", &reserved);
                let placed = r.layer.keys.len();
                assert!(placed > 0, "{} on {}", p.id, l.id);
                assert_eq!(placed + r.dropped, p.macros.len(), "{} on {}", p.id, l.id);
                if l.id != "numpad" {
                    assert!(r.dropped <= p.macros.len() / 2, "{} on {}", p.id, l.id);
                }
                assert!(r.layer.keys.keys().all(|k| !reserved.contains(k)));
                let mut cfg = Config::initial();
                cfg.layers.push(r.layer.clone());
                cfg.global.extend(r.global.clone());
                assert!(validate(&cfg).is_ok(), "{} on {}: {:?}", p.id, l.id, validate(&cfg).err());
            }
        }
    }

    #[test]
    fn placement_adapts_to_the_keyboard() {
        let p = get("zoom").unwrap();
        assert_eq!(resolve_for("darwin", p, "full-iso", "es", &none()).region, "numpad");
        let r = resolve_for("darwin", p, "60-ansi", "es", &none());
        assert!(r.region == "numrow" || r.region == "letters");
        let r = resolve_for("darwin", p, "tkl-ansi", "en", &none());
        assert_eq!(r.region, "fn");
        assert_eq!(r.layer.name, "Zoom");
    }

    #[test]
    fn mnemonic_keys_are_respected() {
        let r = resolve_for("darwin", get("launcher").unwrap(), "full-iso", "es", &none());
        assert_eq!(r.layer.keys["06"].tap.as_ref().unwrap().app, "Google Chrome");
        assert!(r.global.contains_key("39"));
    }

    #[test]
    fn specific_apps_are_covered() {
        for id in ["obs", "discord", "obs-discord", "zoom", "slack", "vscode", "photoshop", "premiere"] {
            assert!(get(id).is_some(), "{id}");
        }
    }

    #[test]
    fn pc_variants_and_os_filter() {
        let find = |r: &Resolved, label: &str| {
            r.layer.keys.values().find(|k| k.label == label).and_then(|k| k.tap.as_ref()).map(|a| a.keys.clone()).unwrap_or_default()
        };
        let zoom = get("zoom").unwrap();
        assert_eq!(find(&resolve_for("darwin", zoom, "full-iso", "es", &none()), "Micro"), "cmd+shift+a");
        assert_eq!(find(&resolve_for("windows", zoom, "full-iso", "es", &none()), "Micro"), "alt+a");
        let disc = get("discord").unwrap();
        let pc = resolve_for("linux", disc, "full-iso", "es", &none());
        assert_eq!(find(&pc, "Silenciar"), "ctrl+shift+m");
        assert_eq!(find(&pc, "Servidor arriba"), "ctrl+alt+up");
        for id in ["xcode", "finalcut", "mac-productivity"] {
            let p = get(id).unwrap();
            assert!(!p.for_os("linux") && !p.for_os("windows") && p.for_os("darwin"), "{id}");
        }
        assert!(for_os("linux").len() < all().len());
        assert_eq!(for_os("darwin").len(), all().len());
    }

    #[test]
    fn pc_shortcuts_merge_duplicate_control() {
        let a = Action { keys: "cmd+ctrl+K".into(), ..Action::new("hotkey") };
        assert_eq!(pc_action(&a).keys, "ctrl+k");
    }
}
