use std::sync::OnceLock;

pub struct Tables {
    pub list: Vec<(u8, u16)>,
    from_code: [u8; 768],
    from_usage: [u16; 256],
}

const EXTRA: &[(u8, u16)] = &[
    (0x28, 28),
    (0x29, 1),
    (0x2A, 14),
    (0x2B, 15),
    (0x2C, 57),
    (0x2D, 12),
    (0x2E, 13),
    (0x2F, 26),
    (0x30, 27),
    (0x31, 43),
    (0x33, 39),
    (0x34, 40),
    (0x35, 41),
    (0x36, 51),
    (0x37, 52),
    (0x38, 53),
    (0x39, 58),
    (0x44, 87),
    (0x45, 88),
    (0x46, 99),
    (0x47, 70),
    (0x48, 119),
    (0x49, 110),
    (0x4A, 102),
    (0x4B, 104),
    (0x4C, 111),
    (0x4D, 107),
    (0x4E, 109),
    (0x4F, 106),
    (0x50, 105),
    (0x51, 108),
    (0x52, 103),
    (0x53, 69),
    (0x54, 98),
    (0x55, 55),
    (0x56, 74),
    (0x57, 78),
    (0x58, 96),
    (0x59, 79),
    (0x5A, 80),
    (0x5B, 81),
    (0x5C, 75),
    (0x5D, 76),
    (0x5E, 77),
    (0x5F, 71),
    (0x60, 72),
    (0x61, 73),
    (0x62, 82),
    (0x63, 83),
    (0x64, 86),
    (0x65, 127),
    (0x67, 117),
    (0x85, 121),
    (0x87, 89),
    (0x88, 93),
    (0x89, 124),
    (0x8A, 92),
    (0x8B, 94),
    (0x90, 122),
    (0x91, 123),
    (0xE0, 29),
    (0xE1, 42),
    (0xE2, 56),
    (0xE3, 125),
    (0xE4, 97),
    (0xE5, 54),
    (0xE6, 100),
    (0xE7, 126),
];

const LETTERS: [u16; 26] = [30, 48, 46, 32, 18, 33, 34, 35, 23, 36, 37, 38, 50, 49, 24, 25, 16, 19, 31, 20, 22, 47, 17, 45, 21, 44];

pub fn evdev_table() -> &'static Tables {
    static T: OnceLock<Tables> = OnceLock::new();
    T.get_or_init(|| {
        let mut list = Vec::new();
        for (i, c) in LETTERS.iter().enumerate() {
            list.push((0x04 + i as u8, *c));
        }
        for i in 0..10u8 {
            list.push((0x1E + i, 2 + u16::from(i)));
        }
        for i in 0..10u8 {
            list.push((0x3A + i, 59 + u16::from(i)));
        }
        for i in 0..12u8 {
            list.push((0x68 + i, 183 + u16::from(i)));
        }
        list.extend_from_slice(EXTRA);
        let mut from_code = [0u8; 768];
        let mut from_usage = [0u16; 256];
        for &(u, c) in &list {
            from_code[usize::from(c)] = u;
            from_usage[usize::from(u)] = c;
        }
        Tables { list, from_code, from_usage }
    })
}

pub fn usage_from_evdev(code: u16) -> u8 {
    evdev_table().from_code.get(usize::from(code)).copied().unwrap_or(0)
}

pub fn evdev_from_usage(usage: u8) -> Option<u16> {
    let usage = if usage == 0x32 { 0x31 } else { usage };
    let c = evdev_table().from_usage[usize::from(usage)];
    (c != 0).then_some(c)
}

pub fn consumer_evdev(usage: u16) -> Option<u16> {
    Some(match usage {
        0xCD => 164,
        0xB5 => 163,
        0xB6 => 165,
        0xB7 => 166,
        0xE2 => 113,
        0xE9 => 115,
        0xEA => 114,
        0x18A => 155,
        0x192 => 140,
        0x221 => 217,
        0x223 => 172,
        0x224 => 158,
        0x225 => 159,
        _ => return None,
    })
}
