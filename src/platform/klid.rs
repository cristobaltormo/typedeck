const QWERTY_LANGUAGES: [u32; 8] = [0x09, 0x0A, 0x16, 0x10, 0x13, 0x1D, 0x14, 0x06];

pub fn klid_from_hkl(hkl: u32, by_layout_id: Option<&dyn Fn(u16, u16) -> String>) -> String {
    let lang = hkl as u16;
    let mut dev = (hkl >> 16) as u16;
    if dev & 0xF000 == 0xF000 {
        return by_layout_id.map(|f| f(dev & 0x0FFF, lang)).unwrap_or_default();
    }
    if dev == 0 {
        dev = lang;
    }
    format!("0000{dev:04X}")
}

pub fn layout_from_klid(klid: &str) -> &'static str {
    let klid = klid.trim().to_uppercase();
    if klid.len() != 8 || !klid.starts_with("0000") {
        return "";
    }
    match klid.as_str() {
        "0000040A" => return "es-win",
        "00000409" => return "us",
        _ => {}
    }
    match u32::from_str_radix(&klid[4..], 16) {
        Ok(id) if QWERTY_LANGUAGES.contains(&(id & 0x3FF)) => "qwerty",
        _ => "",
    }
}
