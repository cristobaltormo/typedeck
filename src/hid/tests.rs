use std::collections::HashMap;

use super::*;

const RDESC0: &str = "05 01 09 06 A1 01 05 07 19 E0 29 E7 15 00 25 01 95 08 75 01 81 02 95 01 75 08 81 03 95 06 75 08 15 00 26 FF 00 05 07 19 00 2A FF 00 81 00 25 01 95 05 75 01 05 08 19 01 29 05 91 02 95 01 75 03 91 03 C0";

const RDESC1: &str = "06 01 00 09 80 A1 01 85 01 19 81 29 83 15 00 25 01 95 03 75 01 81 02 95 01 75 05 81 01 C0 05 0C 09 01 A1 01 85 02 19 00 2A FF 02 15 00 26 FF 7F 95 01 75 10 81 00 C0 06 00 FF 09 01 A1 01 85 03 15 00 26 FF 00 09 2F 75 08 95 03 81 02 C0 05 01 09 06 A1 01 85 04 05 07 19 04 29 70 15 00 25 01 75 01 95 78 81 02 C0 06 00 FF 09 01 A1 01 85 05 15 00 26 FF 00 19 01 29 02 75 08 95 05 B1 02 C0 06 00 FF 09 01 A1 01 85 06 15 00 26 FF 00 19 01 29 02 75 08 96 07 04 B1 02 C0 05 01 09 02 A1 01 85 07 09 01 A1 00 05 09 15 00 25 01 19 01 29 05 75 01 95 05 81 02 95 03 81 01 05 01 16 00 80 26 FF 7F 09 30 09 31 75 10 95 02 81 06 15 81 25 7F 09 38 75 08 95 01 81 06 05 0C 0A 38 02 95 01 81 06 C0 C0";

fn kinds(rs: &[Report]) -> HashMap<i64, String> {
    rs.iter().filter(|r| r.dir == "input").map(|r| (r.id, r.kind.clone())).collect()
}

#[test]
fn report_descriptor_real_keyboard() {
    let r0 = parse_report_descriptor_hex(RDESC0).unwrap();
    assert_eq!(kinds(&r0)[&0], "keyboard");
    let k = kinds(&parse_report_descriptor_hex(RDESC1).unwrap());
    for (id, want) in [(1, "system"), (2, "consumer"), (3, "vendor"), (4, "nkro"), (7, "mouse")] {
        assert_eq!(k[&id], want, "report {id}");
    }
    assert!(parse_report_descriptor_hex("zz").is_err());
    assert!(parse_report_descriptor(&[0xFE]).is_empty());
    assert!(parse_report_descriptor(&[0x81]).len() == 1);
}

#[test]
fn hotkey_and_layouts() {
    let es = layout_by_name("es-iso");
    assert_eq!(parse_hotkey("cmd+shift+4", es).unwrap(), Stroke::new(0x21, MOD_GUI | MOD_SHIFT));
    assert_eq!(parse_hotkey("ctrl+alt+f13", es).unwrap(), Stroke::new(0x68, MOD_CTRL | MOD_ALT));
    assert!(parse_hotkey("cmd+nada", es).is_err());
    assert!(parse_hotkey("hyper+a", es).is_err());
    assert!(parse_hotkey("", es).is_err());
    assert!(parse_hotkey("ctrl+", es).is_err());
    assert_eq!(es.strokes('ñ').unwrap(), &[Stroke::new(0x33, 0)]);
    let a = es.strokes('á').unwrap();
    assert_eq!((a.len(), a[0].usage, a[1].usage), (2, 0x34, 0x04));
    let t = es.strokes('~').unwrap();
    assert_eq!((t.len(), t[1].usage), (2, 0x2C));
    assert!(es.typeable("Hola, ¿qué tal? @#{}"));
    assert!(!layout_by_name("us").typeable("ñ"));
    assert_eq!(layout_from_input_source("com.apple.keylayout.Spanish-ISO"), "es-iso");
    assert_eq!(layout_from_input_source("com.apple.keylayout.US"), "us");
    for (id, want) in [
        ("com.apple.keylayout.British", "qwerty"),
        ("com.apple.keylayout.French", ""),
        ("com.apple.keylayout.German", ""),
        ("com.apple.keylayout.Dvorak", ""),
        ("com.apple.keylayout.Colemak", ""),
        ("com.apple.keylayout.Spanish", "qwerty"),
        ("", ""),
    ] {
        assert_eq!(layout_from_input_source(id), want, "{id}");
    }
    assert!(!layout_by_name("").safe_for_keys());
    assert!(layout_by_name("qwerty").safe_for_keys());
    assert!(!layout_by_name("qwerty").typeable("a,b"));
    assert!(layout_by_name("qwerty").typeable("Hola 123\n"));
}

#[test]
fn hotkey_with_a_character_of_the_layout_keeps_its_alt_gr() {
    let pc = layout_by_name("es-pc");
    assert_eq!(parse_hotkey("ctrl+@", pc).unwrap(), Stroke::new(0x1F, MOD_CTRL | MOD_ALTGR));
    assert_eq!(parse_hotkey("ctrl+plus", layout_by_name("us")).unwrap(), Stroke::new(0x2E, MOD_CTRL));
}

#[test]
fn spanish_pc_layout() {
    let l = layout_by_name("es-pc");
    assert_eq!(l.name, "es-pc");
    assert_eq!(layout_by_name("es-iso").name, "es-iso");
    let cases: &[(char, &[(u8, u8)])] = &[
        ('@', &[(0x1F, MOD_ALTGR)]),
        ('#', &[(0x20, MOD_ALTGR)]),
        ('~', &[(0x21, MOD_ALTGR)]),
        ('€', &[(0x08, MOD_ALTGR)]),
        ('½', &[(0x22, MOD_ALTGR)]),
        ('\\', &[(0x35, MOD_ALTGR)]),
        ('|', &[(0x1E, MOD_ALTGR)]),
        ('[', &[(0x2F, MOD_ALTGR)]),
        (']', &[(0x30, MOD_ALTGR)]),
        ('{', &[(0x34, MOD_ALTGR)]),
        ('}', &[(0x32, MOD_ALTGR)]),
        ('ñ', &[(0x33, 0)]),
        ('Ñ', &[(0x33, MOD_SHIFT)]),
        ('ç', &[(0x32, 0)]),
        ('<', &[(0x64, 0)]),
        ('>', &[(0x64, MOD_SHIFT)]),
        ('á', &[(0x34, 0), (0x04, 0)]),
        ('^', &[(0x2F, MOD_SHIFT), (0x2C, 0)]),
        ('`', &[(0x2F, 0), (0x2C, 0)]),
        ('/', &[(0x24, MOD_SHIFT)]),
        ('=', &[(0x27, MOD_SHIFT)]),
    ];
    for (c, want) in cases {
        let want: Vec<Stroke> = want.iter().map(|&(u, m)| Stroke::new(u, m)).collect();
        assert_eq!(l.strokes(*c).unwrap(), want.as_slice(), "{c}");
    }
    assert!(l.strokes('∞').is_none());
    assert!(layout_by_name("es-iso").strokes('∞').is_some());
}

#[test]
fn spanish_windows_tilde_is_a_dead_key() {
    let (w, pc) = (layout_by_name("es-win"), layout_by_name("es-pc"));
    assert_eq!(w.name, "es-win");
    assert_eq!(w.strokes('~').unwrap(), &[Stroke::new(0x21, MOD_ALTGR), Stroke::new(0x2C, 0)]);
    assert_eq!(pc.strokes('~').unwrap().len(), 1);
    assert_eq!(w.strokes('@').unwrap()[0].mods, MOD_ALTGR);
}

#[test]
fn reverse_lookup_prefers_the_lowest_character() {
    let us = layout_by_name("us");
    assert_eq!(us.rune(0, 0x04), Some('a'));
    assert_eq!(us.rune(MOD_SHIFT, 0x04), Some('A'));
    assert_eq!(layout_by_name("es-pc").rune(MOD_ALTGR, 0x08), Some('€'));
    assert_eq!(us.rune(MOD_ALT, 0x04), None);
}

#[test]
fn names() {
    assert_eq!(key_usage("F1"), Some(0x3A));
    assert_eq!(key_usage("f24"), Some(0x73));
    assert_eq!(key_usage("f25"), None);
    assert_eq!(key_usage("f05"), None);
    assert_eq!(key_usage("Escape"), Some(0x29));
    assert_eq!(usage_name(0x28), "return");
    assert_eq!(usage_name(0x2A), "backspace");
    assert_eq!(usage_name(0x29), "esc");
    assert_eq!(usage_name(0x3B), "f2");
    assert_eq!(usage_name(0x04), "A");
    assert_eq!(usage_name(0x27), "0");
    assert_eq!(usage_name(0xE0), "0xE0");
}
