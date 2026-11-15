use std::collections::BTreeMap;

pub fn parse_modifier_mapping(out: &str) -> BTreeMap<u8, u8> {
    let mut m = BTreeMap::new();
    let mut rest = out;
    while let Some(i) = rest.find("HIDKeyboardModifierMappingDst") {
        rest = &rest[i + "HIDKeyboardModifierMappingDst".len()..];
        let num = |s: &str| -> Option<(i64, usize)> {
            let s2 = s.trim_start().strip_prefix('=')?.trim_start();
            let digits: String = s2.chars().take_while(char::is_ascii_digit).collect();
            let consumed = s.len() - s2.len() + digits.len();
            Some((digits.parse().ok()?, consumed))
        };
        let Some((dst, used)) = num(rest) else { continue };
        let after = rest[used..].trim_start();
        let Some(after) = after.strip_prefix(';') else { continue };
        let Some(after) = after.trim_start().strip_prefix("HIDKeyboardModifierMappingSrc") else { continue };
        let Some((src, _)) = num(after) else { continue };
        m.insert((src & 0xFF) as u8, (dst & 0xFF) as u8);
    }
    m
}

pub fn invert_mods(mapping: &BTreeMap<u8, u8>, wanted: u8) -> u8 {
    if mapping.is_empty() {
        return wanted;
    }
    let mut out = 0u8;
    for bit in 0..4u8 {
        if wanted & (1 << bit) == 0 {
            continue;
        }
        let send = mapping.iter().find(|(src, dst)| **dst == 0xE0 + bit && (0xE0..=0xE3).contains(*src)).map(|(src, _)| src - 0xE0).unwrap_or(bit);
        out |= 1 << send;
    }
    out | (wanted & 0xF0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hid;

    #[test]
    fn modifier_compensation() {
        let m = parse_modifier_mapping(
            "(\n {\n HIDKeyboardModifierMappingDst = 30064771299;\n HIDKeyboardModifierMappingSrc = 30064771298;\n },\n {\n HIDKeyboardModifierMappingDst = 30064771298;\n HIDKeyboardModifierMappingSrc = 30064771299;\n }\n)",
        );
        assert_eq!(m.get(&0xE2), Some(&0xE3));
        assert_eq!(m.get(&0xE3), Some(&0xE2));
        assert_eq!(invert_mods(&m, hid::MOD_GUI), hid::MOD_ALT);
        assert_eq!(invert_mods(&m, hid::MOD_CTRL | hid::MOD_ALTGR), hid::MOD_CTRL | hid::MOD_ALTGR);
        assert_eq!(invert_mods(&BTreeMap::new(), hid::MOD_GUI), hid::MOD_GUI);
    }
}
