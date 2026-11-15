use std::collections::HashMap;
use std::sync::OnceLock;

use super::{MOD_ALT, MOD_ALTGR, MOD_SHIFT, Stroke};

#[derive(Debug)]
pub struct Layout {
    pub name: &'static str,
    chars: HashMap<char, Vec<Stroke>>,
    rev: HashMap<(u8, u8), char>,
}

impl Layout {
    fn new(name: &'static str) -> Layout {
        Layout { name, chars: HashMap::new(), rev: HashMap::new() }
    }

    fn add(&mut self, c: char, usage: u8, mods: u8) {
        self.chars.insert(c, vec![Stroke::new(usage, mods)]);
    }

    fn seq(&mut self, c: char, strokes: &[(u8, u8)]) {
        self.chars.insert(c, strokes.iter().map(|&(u, m)| Stroke::new(u, m)).collect());
    }

    fn finish(mut self) -> Layout {
        let mut rev: HashMap<(u8, u8), char> = HashMap::new();
        for (c, st) in &self.chars {
            if st.len() != 1 {
                continue;
            }
            let k = (st[0].mods, st[0].usage);
            match rev.get(&k) {
                Some(old) if *old <= *c => {}
                _ => {
                    rev.insert(k, *c);
                }
            }
        }
        self.rev = rev;
        self
    }

    pub fn rune(&self, mods: u8, usage: u8) -> Option<char> {
        self.rev.get(&(mods, usage)).copied()
    }

    pub fn strokes(&self, c: char) -> Option<&[Stroke]> {
        self.chars.get(&c).map(Vec::as_slice)
    }

    pub fn typeable(&self, text: &str) -> bool {
        text.chars().all(|c| self.chars.contains_key(&c))
    }

    pub fn safe_for_keys(&self) -> bool {
        self.name != "none"
    }
}

fn base(name: &'static str) -> Layout {
    let mut l = Layout::new(name);
    for i in 0..26u8 {
        l.add((b'a' + i) as char, 0x04 + i, 0);
        l.add((b'A' + i) as char, 0x04 + i, MOD_SHIFT);
    }
    for i in 1..=9u8 {
        l.add((b'0' + i) as char, 0x1D + i, 0);
    }
    l.add('0', 0x27, 0);
    l.add(' ', 0x2C, 0);
    l.add('\n', 0x28, 0);
    l.add('\t', 0x2B, 0);
    l
}

fn us() -> Layout {
    let mut l = base("us");
    for (i, c) in "!@#$%^&*()".chars().enumerate() {
        l.add(c, 0x1E + i as u8, MOD_SHIFT);
    }
    for (u, a, b) in [
        (0x2D, '-', '_'),
        (0x2E, '=', '+'),
        (0x2F, '[', '{'),
        (0x30, ']', '}'),
        (0x31, '\\', '|'),
        (0x33, ';', ':'),
        (0x34, '\'', '"'),
        (0x35, '`', '~'),
        (0x36, ',', '<'),
        (0x37, '.', '>'),
        (0x38, '/', '?'),
    ] {
        l.add(a, u, 0);
        l.add(b, u, MOD_SHIFT);
    }
    l.finish()
}

fn accents(l: &mut Layout) {
    for (acute, diaer, key) in [('á', 'ä', 0x04), ('é', 'ë', 0x08), ('í', 'ï', 0x0C), ('ó', 'ö', 0x12), ('ú', 'ü', 0x18)] {
        let upper = |c: char| char::from_u32(c as u32 - 0x20).unwrap_or(c);
        l.seq(acute, &[(0x34, 0), (key, 0)]);
        l.seq(upper(acute), &[(0x34, 0), (key, MOD_SHIFT)]);
        l.seq(diaer, &[(0x34, MOD_SHIFT), (key, 0)]);
        l.seq(upper(diaer), &[(0x34, MOD_SHIFT), (key, MOD_SHIFT)]);
    }
}

fn spanish_common(l: &mut Layout, alt: u8) {
    l.add('º', 0x35, 0);
    l.add('ª', 0x35, MOD_SHIFT);
    l.add('\\', 0x35, alt);
    l.add('\'', 0x2D, 0);
    l.add('?', 0x2D, MOD_SHIFT);
    l.add('¡', 0x2E, 0);
    l.add('¿', 0x2E, MOD_SHIFT);
    l.add('[', 0x2F, alt);
    l.add('+', 0x30, 0);
    l.add('*', 0x30, MOD_SHIFT);
    l.add(']', 0x30, alt);
    l.add('ñ', 0x33, 0);
    l.add('Ñ', 0x33, MOD_SHIFT);
    l.seq('`', &[(0x2F, 0), (0x2C, 0)]);
    l.seq('^', &[(0x2F, MOD_SHIFT), (0x2C, 0)]);
    l.seq('´', &[(0x34, 0), (0x2C, 0)]);
    l.seq('¨', &[(0x34, MOD_SHIFT), (0x2C, 0)]);
    l.add('{', 0x34, alt);
    l.add('ç', 0x32, 0);
    l.add('Ç', 0x32, MOD_SHIFT);
    l.add('}', 0x32, alt);
    l.add('<', 0x64, 0);
    l.add('>', 0x64, MOD_SHIFT);
    l.add(',', 0x36, 0);
    l.add(';', 0x36, MOD_SHIFT);
    l.add('.', 0x37, 0);
    l.add(':', 0x37, MOD_SHIFT);
    l.add('-', 0x38, 0);
    l.add('_', 0x38, MOD_SHIFT);
    accents(l);
}

fn es_iso() -> Layout {
    let mut l = base("es-iso");
    for (u, shift, alt) in [
        (0x1E, '!', '|'),
        (0x1F, '"', '@'),
        (0x20, '·', '#'),
        (0x21, '$', '¢'),
        (0x22, '%', '∞'),
        (0x23, '&', '¬'),
        (0x24, '/', '÷'),
        (0x25, '(', '“'),
        (0x26, ')', '”'),
        (0x27, '=', '≠'),
    ] {
        l.add(shift, u, MOD_SHIFT);
        l.add(alt, u, MOD_ALT);
    }
    spanish_common(&mut l, MOD_ALT);
    l.seq('~', &[(0x33, MOD_ALT), (0x2C, 0)]);
    l.finish()
}

fn es_pc(name: &'static str) -> Layout {
    let mut l = base(name);
    for (u, shift, alt) in [
        (0x1E, '!', Some('|')),
        (0x1F, '"', Some('@')),
        (0x20, '·', Some('#')),
        (0x21, '$', Some('~')),
        (0x22, '%', Some('½')),
        (0x23, '&', Some('¬')),
        (0x24, '/', None),
        (0x25, '(', None),
        (0x26, ')', None),
        (0x27, '=', None),
    ] {
        l.add(shift, u, MOD_SHIFT);
        if let Some(a) = alt {
            l.add(a, u, MOD_ALTGR);
        }
    }
    l.add('€', 0x08, MOD_ALTGR);
    spanish_common(&mut l, MOD_ALTGR);
    if name == "es-win" {
        l.seq('~', &[(0x21, MOD_ALTGR), (0x2C, 0)]);
    }
    l.finish()
}

struct All {
    us: Layout,
    es_iso: Layout,
    es_pc: Layout,
    es_win: Layout,
    qwerty: Layout,
    none: Layout,
}

fn all() -> &'static All {
    static ALL: OnceLock<All> = OnceLock::new();
    ALL.get_or_init(|| All {
        us: us(),
        es_iso: es_iso(),
        es_pc: es_pc("es-pc"),
        es_win: es_pc("es-win"),
        qwerty: base("qwerty").finish(),
        none: Layout::new("none").finish(),
    })
}

pub fn layout_by_name(name: &str) -> &'static Layout {
    let a = all();
    match name {
        "es-pc" => &a.es_pc,
        "es-win" => &a.es_win,
        "us" => &a.us,
        "qwerty" => &a.qwerty,
        n if n.starts_with("es") => &a.es_iso,
        _ => &a.none,
    }
}

const QWERTY_FAMILY: &[&str] = &[
    "British",
    "Australian",
    "Canadian",
    "Irish",
    "LatinAmerican",
    "Portuguese",
    "Brazilian",
    "Italian",
    "Dutch",
    "Swedish",
    "Norwegian",
    "Danish",
    "Finnish",
    "USExtended",
    "Spanish",
    "Hawaiian",
    "Icelandic",
    "Polish",
    "Turkish-QWERTY",
    "Czech",
];

pub fn layout_from_input_source(id: &str) -> &'static str {
    if id.contains("Spanish-ISO") {
        return "es-iso";
    }
    if id.ends_with(".US") || id.ends_with(".ABC") {
        return "us";
    }
    let name = id.strip_prefix("com.apple.keylayout.").unwrap_or(id);
    for f in QWERTY_FAMILY {
        if name == *f || name.starts_with(&format!("{f}-")) {
            return "qwerty";
        }
    }
    ""
}
