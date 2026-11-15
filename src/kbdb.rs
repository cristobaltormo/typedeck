use std::collections::BTreeMap;
use std::sync::OnceLock;

use serde::{Deserialize, Serialize};

use crate::jsonx::is_false;

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default)]
pub struct Model {
    pub vid: String,
    pub pid: String,
    pub mfr: String,
    pub prod: String,
    pub brand: String,
    pub model: String,
    pub layout: String,
}

#[derive(Default, Deserialize)]
#[serde(default)]
struct Db {
    vendors: BTreeMap<String, String>,
    chip_vendors: BTreeMap<String, String>,
    models: Vec<Model>,
}

fn db() -> &'static Db {
    static DB: OnceLock<Db> = OnceLock::new();
    DB.get_or_init(|| serde_json::from_str(include_str!("data/db.json")).unwrap_or_default())
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Identity {
    pub brand: String,
    pub model: String,
    pub display: String,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    pub vendor: String,
    #[serde(skip_serializing_if = "is_false", default)]
    pub vendor_is_chip: bool,
    #[serde(rename = "generic_name", skip_serializing_if = "is_false", default)]
    pub generic: bool,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    pub form_factor: String,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    pub standard: String,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    pub layout: String,
    pub confidence: String,
    pub source: String,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub notes: Vec<String>,
}

fn clean(s: &str) -> String {
    s.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn is_word_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_'
}

fn has_word(text: &str, word: &str) -> bool {
    let t = text.as_bytes();
    let mut from = 0;
    while let Some(pos) = text[from..].find(word) {
        let start = from + pos;
        let end = start + word.len();
        let left = start == 0 || !is_word_byte(t[start - 1]);
        let right = end == t.len() || !is_word_byte(t[end]);
        if left && right {
            return true;
        }
        from = start + 1;
        while !text.is_char_boundary(from) {
            from += 1;
        }
    }
    false
}

fn is_generic(prod: &str) -> bool {
    const WORDS: &[&str] = &["usb", "hid", "gaming", "standard", "generic", "keyboard", "teclado", "device", "composite"];
    let s = prod.to_ascii_lowercase();
    let mut rest = s.as_str();
    if rest.is_empty() {
        return false;
    }
    while !rest.is_empty() {
        if let Some(r) = rest.strip_prefix([' ', '\t', '\n', '\r', '\x0c', '\x0b', '-', '_']) {
            rest = r;
            continue;
        }
        match WORDS.iter().find(|w| rest.starts_with(**w)) {
            Some(w) => rest = &rest[w.len()..],
            None => return false,
        }
    }
    true
}

fn percent(text: &str) -> Option<&'static str> {
    let t = text.as_bytes();
    for i in 0..t.len() {
        if !text.is_char_boundary(i) || (i > 0 && t[i - 1].is_ascii_alphanumeric()) {
            continue;
        }
        for n in ["60", "65", "75", "80", "96", "100"] {
            let rest = &text[i..];
            if let Some(after) = rest.strip_prefix(n) {
                let after = after.strip_prefix([' ', '\t', '\n', '\r', '\x0c', '\x0b']).unwrap_or(after);
                if after.starts_with('%') {
                    return Some(n);
                }
            }
        }
    }
    None
}

fn form_factor(text: &str) -> String {
    let lower = text.to_ascii_lowercase();
    if ["tkl", "tenkeyless", "ten-key-less"].iter().any(|w| has_word(&lower, w)) {
        return "tkl".into();
    }
    if let Some(p) = percent(&lower) {
        return match p {
            "60" => "60",
            "65" => "65",
            "75" => "75",
            "80" => "tkl",
            "100" => "full",
            _ => "",
        }
        .into();
    }
    if ["full-size", "full size", "fullsize"].iter().any(|w| has_word(&lower, w)) {
        return "full".into();
    }
    String::new()
}

fn standard(text: &str) -> String {
    let lower = text.to_ascii_lowercase();
    if has_word(&lower, "iso") {
        "iso".into()
    } else if has_word(&lower, "ansi") {
        "ansi".into()
    } else {
        String::new()
    }
}

pub fn identify(vid: &str, pid: &str, mfr: &str, prod: &str) -> Identity {
    identify_with(db(), vid, pid, mfr, prod)
}

fn identify_with(db: &Db, vid: &str, pid: &str, mfr: &str, prod: &str) -> Identity {
    let (vid, pid) = (vid.to_lowercase(), pid.to_lowercase());
    let (mfr, prod) = (clean(mfr), clean(prod));
    let mut id = Identity { confidence: "unknown".into(), source: "none".into(), ..Default::default() };
    if let Some(v) = db.vendors.get(&vid) {
        id.vendor = v.clone();
    } else if let Some(v) = db.chip_vendors.get(&vid) {
        id.vendor = v.clone();
        id.vendor_is_chip = true;
    }
    let verified = db.models.iter().find(|m| {
        m.vid.eq_ignore_ascii_case(&vid)
            && m.pid.eq_ignore_ascii_case(&pid)
            && (m.mfr.is_empty() || m.mfr.eq_ignore_ascii_case(&mfr))
            && (m.prod.is_empty() || m.prod.eq_ignore_ascii_case(&prod))
    });
    match verified {
        Some(m) => {
            id.brand = m.brand.clone();
            id.model = m.model.clone();
            id.layout = m.layout.clone();
            id.confidence = "verified".into();
            id.source = "verified model database".into();
        }
        None => {
            id.brand = mfr.clone();
            id.model = prod.clone();
            if !prod.is_empty() || !mfr.is_empty() {
                id.confidence = "name".into();
                id.source = "name reported by the keyboard".into();
            }
            if id.brand.is_empty() && !id.vendor.is_empty() && !id.vendor_is_chip {
                id.brand = id.vendor.clone();
                id.confidence = "vendor".into();
                id.source = "vendor of the USB identifier".into();
            }
            if id.brand.is_empty() && id.vendor_is_chip {
                id.brand = id.vendor.clone();
            }
        }
    }
    if !prod.is_empty() && is_generic(&prod) {
        id.generic = true;
        id.notes.push("generic_name".into());
    }
    if id.vendor_is_chip {
        id.notes.push("chip_vendor".into());
    }
    let text = format!("{mfr} {prod}");
    id.form_factor = form_factor(&text);
    id.standard = standard(&text);
    if id.layout.is_empty() && !id.form_factor.is_empty() && !id.standard.is_empty() {
        id.layout = format!("{}-{}", id.form_factor, id.standard);
    }
    id.display = match (id.brand.is_empty(), id.model.is_empty()) {
        (false, false) => format!("{} {}", id.brand, id.model),
        (_, false) => id.model.clone(),
        _ => id.brand.clone(),
    };
    id
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identify_your_keyboard() {
        let id = identify("258A", "0016", "BY Tech", "Usb Gaming Keyboard");
        assert_eq!((id.brand.as_str(), id.model.as_str(), id.display.as_str()), ("BY Tech", "Usb Gaming Keyboard", "BY Tech Usb Gaming Keyboard"));
        assert!(id.vendor == "SINO WEALTH" && id.vendor_is_chip && id.generic);
        assert_eq!((id.confidence.as_str(), id.layout.as_str()), ("verified", "full-iso"));
    }

    #[test]
    fn vendor_only_brand() {
        let id = identify("046d", "c31c", "", "");
        assert_eq!((id.brand.as_str(), id.confidence.as_str()), ("Logitech", "vendor"));
        let id = identify("ffff", "0001", "", "");
        assert_eq!((id.confidence.as_str(), id.display.as_str()), ("unknown", ""));
    }

    #[test]
    fn form_factor_from_name() {
        for (name, want) in [
            ("Keychron K8 TKL", "tkl"),
            ("Ducky One 2 Mini 60%", "60"),
            ("GMMK Pro 75%", "75"),
            ("Royal Kludge RK68 65 %", "65"),
            ("Das Keyboard 4 Professional", ""),
            ("Full Size Gaming Keyboard", "full"),
            ("Redragon 80% Mechanical", "tkl"),
            ("Model 100%", "full"),
            ("Board 96%", ""),
            ("X160%", ""),
            ("Teclado ñandú 60%", "60"),
        ] {
            assert_eq!(identify("1234", "0001", "", name).form_factor, want, "{name}");
        }
    }

    #[test]
    fn layout_needs_form_and_standard() {
        assert_eq!(identify("1234", "1", "", "Foo TKL ISO").layout, "tkl-iso");
        assert_eq!(identify("1234", "1", "", "Foo TKL").layout, "");
        assert_eq!(identify("1234", "1", "", "Foo TKLISO").standard, "");
    }

    #[test]
    fn verified_model_wins() {
        let mut d = Db::default();
        d.models.push(Model {
            vid: "1234".into(),
            pid: "00aa".into(),
            brand: "Acme".into(),
            model: "K1".into(),
            layout: "75-ansi".into(),
            ..Default::default()
        });
        let id = identify_with(&d, "1234", "00AA", "x", "y");
        assert_eq!((id.brand.as_str(), id.layout.as_str(), id.confidence.as_str()), ("Acme", "75-ansi", "verified"));
    }

    #[test]
    fn shared_controller_ids_are_not_auto_verified() {
        let id = identify("258A", "0016", "Otra Marca", "Mini 60% Keyboard");
        assert_ne!(id.confidence, "verified");
        assert_eq!(id.form_factor, "60");
    }

    #[test]
    fn generic_names() {
        assert!(is_generic("Usb Gaming Keyboard"));
        assert!(is_generic("USB-HID_device"));
        assert!(!is_generic("Keychron K8"));
        assert!(!is_generic(""));
    }
}
