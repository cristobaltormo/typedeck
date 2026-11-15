mod layout;
mod rdesc;

pub use layout::{Layout, layout_by_name, layout_from_input_source};
pub use rdesc::{Report, parse_report_descriptor, parse_report_descriptor_hex};

pub const MOD_CTRL: u8 = 0x01;
pub const MOD_SHIFT: u8 = 0x02;
pub const MOD_ALT: u8 = 0x04;
pub const MOD_GUI: u8 = 0x08;
pub const MOD_ALTGR: u8 = 0x40;

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct Stroke {
    pub usage: u8,
    pub mods: u8,
}

impl Stroke {
    pub const fn new(usage: u8, mods: u8) -> Stroke {
        Stroke { usage, mods }
    }
}

const NAMED: &[(&str, u8)] = &[
    ("return", 0x28),
    ("enter", 0x28),
    ("esc", 0x29),
    ("escape", 0x29),
    ("backspace", 0x2A),
    ("delete", 0x2A),
    ("tab", 0x2B),
    ("space", 0x2C),
    ("capslock", 0x39),
    ("printscreen", 0x46),
    ("scrolllock", 0x47),
    ("pause", 0x48),
    ("insert", 0x49),
    ("home", 0x4A),
    ("pageup", 0x4B),
    ("forwarddelete", 0x4C),
    ("end", 0x4D),
    ("pagedown", 0x4E),
    ("right", 0x4F),
    ("left", 0x50),
    ("down", 0x51),
    ("up", 0x52),
    ("numlock", 0x53),
    ("minus", 0x2D),
    ("equals", 0x2E),
    ("comma", 0x36),
    ("period", 0x37),
    ("slash", 0x38),
];

pub fn key_usage(name: &str) -> Option<u8> {
    let name = name.to_lowercase();
    if let Some((_, u)) = NAMED.iter().find(|(n, _)| *n == name) {
        return Some(*u);
    }
    let b = name.as_bytes();
    if b.len() == 1 {
        return match b[0] {
            c @ b'a'..=b'z' => Some(0x04 + (c - b'a')),
            b'0' => Some(0x27),
            c @ b'1'..=b'9' => Some(0x1D + (c - b'0')),
            _ => None,
        };
    }
    let n: u8 = name.strip_prefix('f')?.parse().ok()?;
    if name.len() > 1 && name.as_bytes()[1] == b'0' {
        return None;
    }
    match n {
        1..=12 => Some(0x3A + n - 1),
        13..=24 => Some(0x68 + n - 13),
        _ => None,
    }
}

pub fn usage_name(u: u8) -> String {
    if let Some((n, _)) = NAMED.iter().find(|(n, v)| *v == u && !matches!(*n, "enter" | "escape" | "delete")) {
        return (*n).to_string();
    }
    match u {
        0x3A..=0x45 => format!("f{}", u - 0x3A + 1),
        0x68..=0x73 => format!("f{}", u - 0x68 + 13),
        0x04..=0x1D => ((b'A' + u - 0x04) as char).to_string(),
        0x1E..=0x26 => ((b'1' + u - 0x1E) as char).to_string(),
        0x27 => "0".into(),
        _ => format!("0x{u:02X}"),
    }
}

fn mod_bit(name: &str) -> Option<u8> {
    match name {
        "ctrl" | "control" => Some(MOD_CTRL),
        "shift" => Some(MOD_SHIFT),
        "alt" | "option" | "opt" => Some(MOD_ALT),
        "cmd" | "command" | "gui" | "win" => Some(MOD_GUI),
        _ => None,
    }
}

pub fn parse_hotkey(spec: &str, l: &Layout) -> Result<Stroke, String> {
    let spec = spec.trim().to_lowercase();
    let parts: Vec<&str> = spec.split('+').collect();
    let Some((last, mods)) = parts.split_last() else {
        return Err("empty shortcut".into());
    };
    if last.is_empty() {
        return Err("empty shortcut".into());
    }
    let mut st = Stroke::default();
    for p in mods {
        st.mods |= mod_bit(p.trim()).ok_or_else(|| format!("unknown modifier: {p}"))?;
    }
    let mut key = last.trim();
    if let Some(u) = key_usage(key) {
        st.usage = u;
        return Ok(st);
    }
    if key == "plus" {
        key = "+";
    }
    let mut chars = key.chars();
    if let (Some(c), None) = (chars.next(), chars.next())
        && let Some(strokes) = l.strokes(c)
        && strokes.len() == 1
    {
        st.usage = strokes[0].usage;
        st.mods |= strokes[0].mods & !MOD_SHIFT;
        return Ok(st);
    }
    Err(format!("unknown key: {key}"))
}

#[cfg(test)]
mod tests;
