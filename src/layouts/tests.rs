use std::collections::HashSet;

use super::*;

#[test]
fn every_layout_is_consistent() {
    let want = [
        ("full-ansi", 104),
        ("full-iso", 105),
        ("tkl-ansi", 87),
        ("tkl-iso", 88),
        ("75-ansi", 82),
        ("75-iso", 83),
        ("60-ansi", 61),
        ("60-iso", 62),
        ("numpad", 17),
    ];
    assert_eq!(all().len(), 11);
    for l in all() {
        let mut seen = HashSet::new();
        for k in &l.keys {
            assert!(seen.insert(k.usage), "{}: usage {:02X} repeated", l.id, k.usage);
            assert!(k.x > -0.001 && k.y > -0.001 && k.x + k.w <= l.width + 0.001 && k.y + k.h <= l.height + 0.001, "{}: {:02X} out of bounds", l.id, k.usage);
        }
        for (i, a) in l.keys.iter().enumerate() {
            for b in &l.keys[i + 1..] {
                let overlap = a.x < b.x + b.w - 0.01 && b.x < a.x + a.w - 0.01 && a.y < b.y + b.h - 0.01 && b.y < a.y + a.h - 0.01;
                assert!(!overlap, "{}: {:02X} overlaps {:02X}", l.id, a.usage, b.usage);
            }
        }
        if let Some((_, n)) = want.iter().find(|(id, _)| *id == l.id) {
            assert_eq!(l.keys.len(), *n, "{}", l.id);
        }
    }
}

#[test]
fn percent_and_standard() {
    for (id, p) in [("full-iso", 100), ("tkl-ansi", 80), ("75-iso", 75), ("65-ansi", 65), ("60-ansi", 60)] {
        assert_eq!(get(id).unwrap().percent, p, "{id}");
    }
    assert_eq!(get("iso-full").unwrap().id, "full-iso");
    assert_eq!(get("full-iso").unwrap().standard, "iso");
    assert!(get("nope").is_none());
}

#[test]
fn widths_are_realistic_for_each_form_factor() {
    for (id, w) in [("full-ansi", 22.5), ("tkl-ansi", 18.25), ("75-ansi", 16.0), ("65-ansi", 16.0), ("60-ansi", 15.0)] {
        assert!((get(id).unwrap().width - w).abs() < 0.01, "{id}");
    }
}

fn score_of(r: &[Suggestion], id: &str) -> f64 {
    r.iter().find(|s| s.id == id).map(|s| s.score).unwrap_or(0.0)
}

#[test]
fn match_recognises_each_layout() {
    for l in all() {
        let set = l.key_set();
        let r = match_seen(&set);
        assert_eq!(r[0].id, l.id);
        let mut sorted: Vec<u8> = set.iter().copied().collect();
        sorted.sort_unstable();
        let part: HashSet<u8> = sorted.iter().enumerate().filter(|(i, _)| i % 7 != 0).map(|(_, u)| *u).collect();
        let r = match_seen(&part);
        if !r.is_empty() && r[0].id != l.id {
            assert!(r[0].score - score_of(&r, &l.id) <= 0.05, "{}: {} wins", l.id, r[0].id);
        }
    }
}

#[test]
fn iso_vs_ansi_is_decided_by_the_extra_keys() {
    assert_eq!(match_seen(&get("full-iso").unwrap().key_set())[0].id, "full-iso");
    assert_eq!(match_seen(&get("tkl-ansi").unwrap().key_set())[0].id, "tkl-ansi");
    assert!(match_seen(&HashSet::new()).is_empty());
}

#[test]
fn ambiguity_asks_for_the_deciding_keys() {
    let iso = get("full-iso").unwrap();
    let mut seen = iso.key_set();
    seen.remove(&0x64);
    seen.remove(&0x32);
    let (amb, hints) = ambiguity(&match_seen(&seen));
    assert!(amb && !hints.is_empty());
    assert!(!ambiguity(&match_seen(&iso.key_set())).0);
}

#[test]
fn missing_keys_are_reported() {
    let mut seen = get("tkl-ansi").unwrap().key_set();
    seen.remove(&0x2B);
    let r = match_seen(&seen);
    let tkl = r.iter().find(|s| s.id == "tkl-ansi").unwrap();
    assert_eq!(tkl.missing, vec![0x2B]);
}

#[test]
fn iso_with_backslash_alias_still_wins_as_iso() {
    let mut seen = get("full-iso").unwrap().key_set();
    seen.remove(&0x32);
    seen.insert(0x31);
    let r = match_seen(&seen);
    assert_eq!(r[0].id, "full-iso");
    assert!(r[0].extra.is_empty());
    assert_eq!(match_seen(&get("full-ansi").unwrap().key_set())[0].id, "full-ansi");
    seen.remove(&0x64);
    let (amb, hints) = ambiguity(&match_seen(&seen));
    assert!(amb && !hints.is_empty());
}

#[test]
fn json_shape() {
    let v = serde_json::to_value(get("numpad").unwrap()).unwrap();
    assert_eq!(v["keys"][0], serde_json::json!({"u": 0x53, "x": 0, "y": 0, "w": 1, "h": 1}));
    let full = serde_json::to_value(get("full-iso").unwrap()).unwrap();
    assert!(full["keys"].as_array().unwrap().iter().any(|k| k["mod"] == serde_json::json!(true)));
}
