use std::collections::HashMap;
use std::sync::OnceLock;

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ScanKey {
    pub code: u16,
    pub ext: bool,
    pub vk: u16,
}

pub const VK_PAUSE: u32 = 0x13;
pub const VK_SNAPSHOT: u32 = 0x2C;
pub const VK_NUMLOCK: u32 = 0x90;
const FAKE_MARKER: u32 = 0x200;

pub struct ScanTables {
    to_usage: HashMap<u16, u8>,
    from_usage: HashMap<u8, ScanKey>,
}

const NAMED: &[(u8, u16, bool)] = &[
    (0x28, 0x1C, false),
    (0x29, 0x01, false),
    (0x2A, 0x0E, false),
    (0x2B, 0x0F, false),
    (0x2C, 0x39, false),
    (0x2D, 0x0C, false),
    (0x2E, 0x0D, false),
    (0x2F, 0x1A, false),
    (0x30, 0x1B, false),
    (0x31, 0x2B, false),
    (0x33, 0x27, false),
    (0x34, 0x28, false),
    (0x35, 0x29, false),
    (0x36, 0x33, false),
    (0x37, 0x34, false),
    (0x38, 0x35, false),
    (0x39, 0x3A, false),
    (0x44, 0x57, false),
    (0x45, 0x58, false),
    (0x46, 0x37, true),
    (0x47, 0x46, false),
    (0x48, 0x45, false),
    (0x49, 0x52, true),
    (0x4A, 0x47, true),
    (0x4B, 0x49, true),
    (0x4C, 0x53, true),
    (0x4D, 0x4F, true),
    (0x4E, 0x51, true),
    (0x4F, 0x4D, true),
    (0x50, 0x4B, true),
    (0x51, 0x50, true),
    (0x52, 0x48, true),
    (0x53, 0x45, true),
    (0x54, 0x35, true),
    (0x55, 0x37, false),
    (0x56, 0x4A, false),
    (0x57, 0x4E, false),
    (0x58, 0x1C, true),
    (0x59, 0x4F, false),
    (0x5A, 0x50, false),
    (0x5B, 0x51, false),
    (0x5C, 0x4B, false),
    (0x5D, 0x4C, false),
    (0x5E, 0x4D, false),
    (0x5F, 0x47, false),
    (0x60, 0x48, false),
    (0x61, 0x49, false),
    (0x62, 0x52, false),
    (0x63, 0x53, false),
    (0x64, 0x56, false),
    (0x65, 0x5D, true),
    (0x67, 0x59, false),
    (0x85, 0x7E, false),
    (0x87, 0x73, false),
    (0x88, 0x70, false),
    (0x89, 0x7D, false),
    (0x8A, 0x79, false),
    (0x8B, 0x7B, false),
    (0xE0, 0x1D, false),
    (0xE1, 0x2A, false),
    (0xE2, 0x38, false),
    (0xE3, 0x5B, true),
    (0xE4, 0x1D, true),
    (0xE5, 0x36, false),
    (0xE6, 0x38, true),
    (0xE7, 0x5C, true),
];

const LETTERS: [u16; 26] = [
    0x1E, 0x30, 0x2E, 0x20, 0x12, 0x21, 0x22, 0x23, 0x17, 0x24, 0x25, 0x26, 0x32, 0x31, 0x18, 0x19, 0x10, 0x13, 0x1F, 0x14, 0x16, 0x2F, 0x11, 0x2D, 0x15, 0x2C,
];
const F13_F24: [u16; 12] = [0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x6B, 0x6C, 0x6D, 0x6E, 0x76];

fn scan_id(code: u16, ext: bool) -> u16 {
    if ext { code | 0x100 } else { code }
}

pub fn scan_tables() -> &'static ScanTables {
    static T: OnceLock<ScanTables> = OnceLock::new();
    T.get_or_init(|| {
        let mut list = Vec::new();
        for (i, c) in LETTERS.iter().enumerate() {
            list.push((0x04 + i as u8, *c, false));
        }
        for i in 0..10u8 {
            list.push((0x1E + i, 0x02 + u16::from(i), false));
        }
        list.extend_from_slice(NAMED);
        for i in 0..10u8 {
            list.push((0x3A + i, 0x3B + u16::from(i), false));
        }
        for (i, c) in F13_F24.iter().enumerate() {
            list.push((0x68 + i as u8, *c, false));
        }
        let mut to_usage = HashMap::new();
        let mut from_usage = HashMap::new();
        for &(u, code, ext) in &list {
            to_usage.insert(scan_id(code, ext), u);
            from_usage.insert(u, ScanKey { code, ext, vk: 0 });
        }
        from_usage.insert(0x32, ScanKey { code: 0x2B, ext: false, vk: 0 });
        from_usage.insert(0x48, ScanKey { code: 0, ext: false, vk: VK_PAUSE as u16 });
        to_usage.insert(scan_id(0x54, false), 0x46);
        to_usage.insert(scan_id(0x46, true), 0x48);
        ScanTables { to_usage, from_usage }
    })
}

pub fn usage_from_scan(code: u32, ext: bool, vk: u32) -> u8 {
    match vk {
        VK_PAUSE => return 0x48,
        VK_NUMLOCK => return 0x53,
        VK_SNAPSHOT => return 0x46,
        _ => {}
    }
    if code & FAKE_MARKER != 0 || (ext && (code == 0x2A || code == 0x36)) {
        return 0;
    }
    scan_tables().to_usage.get(&scan_id((code & 0xFF) as u16, ext)).copied().unwrap_or(0)
}

pub fn scan_from_usage(usage: u8) -> Option<ScanKey> {
    scan_tables().from_usage.get(&usage).copied()
}

pub fn consumer_vk(usage: u16) -> Option<u16> {
    Some(match usage {
        0xCD => 0xB3,
        0xB5 => 0xB0,
        0xB6 => 0xB1,
        0xB7 => 0xB2,
        0xE2 => 0xAD,
        0xE9 => 0xAF,
        0xEA => 0xAE,
        0x18A => 0xB4,
        0x192 => 0xB7,
        0x221 => 0xAA,
        0x223 => 0xAC,
        0x224 => 0xA6,
        0x225 => 0xA7,
        _ => return None,
    })
}
