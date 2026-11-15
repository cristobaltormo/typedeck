const HEADER_SIZE: usize = 0x4C;
const HAS_TARGET_LIST: u32 = 0x1;
const HAS_LINK_INFO: u32 = 0x2;
const LOCAL_BASE_PATH: u32 = 0x1;
const INFO_UNICODE_MIN: u32 = 0x24;

fn le16(b: &[u8], at: usize) -> Option<u16> {
    Some(u16::from_le_bytes([*b.get(at)?, *b.get(at + 1)?]))
}

fn le32(b: &[u8], at: usize) -> Option<u32> {
    Some(u32::from_le_bytes([*b.get(at)?, *b.get(at + 1)?, *b.get(at + 2)?, *b.get(at + 3)?]))
}

pub fn lnk_target(data: &[u8]) -> String {
    parse(data).unwrap_or_default()
}

fn parse(data: &[u8]) -> Option<String> {
    if data.len() < HEADER_SIZE || le32(data, 0)? as usize != HEADER_SIZE {
        return None;
    }
    let flags = le32(data, 0x14)?;
    let mut off = HEADER_SIZE;
    if flags & HAS_TARGET_LIST != 0 {
        off += 2 + usize::from(le16(data, off)?);
    }
    if flags & HAS_LINK_INFO == 0 || off + 0x1C > data.len() {
        return None;
    }
    let info = &data[off..];
    if le32(info, 8)? & LOCAL_BASE_PATH == 0 {
        return None;
    }
    if le32(info, 4)? >= INFO_UNICODE_MIN && info.len() >= 0x24 {
        let base = utf16_at(info, le32(info, 0x1C)?);
        let suffix = utf16_at(info, le32(info, 0x20)?);
        if !base.is_empty() {
            return Some(base + &suffix);
        }
    }
    Some(ansi_at(info, le32(info, 0x10)?) + &ansi_at(info, le32(info, 0x18)?))
}

fn ansi_at(info: &[u8], off: u32) -> String {
    let off = off as usize;
    if off == 0 || off >= info.len() {
        return String::new();
    }
    let end = info[off..].iter().position(|b| *b == 0).map_or(info.len(), |p| off + p);
    String::from_utf8_lossy(&info[off..end]).into_owned()
}

fn utf16_at(info: &[u8], off: u32) -> String {
    let off = off as usize;
    if off == 0 || off + 2 > info.len() {
        return String::new();
    }
    let units: Vec<u16> = info[off..].as_chunks::<2>().0.iter().map(|c| u16::from_le_bytes([c[0], c[1]])).take_while(|c| *c != 0).collect();
    String::from_utf16_lossy(&units)
}
