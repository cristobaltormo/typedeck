use crate::capture::{Event, Info, Sys};

pub fn parse_event(line: &str) -> Option<Event> {
    let f: Vec<&str> = line.split_whitespace().collect();
    if f.len() < 2 || f[0].len() != 1 {
        return None;
    }
    let kind = f[0].as_bytes()[0];
    match kind {
        b'D' | b'U' | b'W' | b'P' | b'R' => {
            if f[1].len() > 2 {
                return None;
            }
            let usage = u8::from_str_radix(f[1], 16).ok()?;
            let mods = f.get(2).and_then(|m| u8::from_str_radix(m, 16).ok()).unwrap_or(0);
            Some(Event::new(kind, usage, mods))
        }
        b'K' if f[1] == "1" || f[1] == "0" => Some(Event { kind: b'K', usage: 0, mods: 0, arg: f[1].as_bytes()[0] - b'0' }),
        _ => None,
    }
}

pub fn key_values(line: &str) -> Vec<(String, String)> {
    let b = line.as_bytes();
    let mut out = Vec::new();
    let mut i = 0;
    while i < b.len() {
        if !(b[i].is_ascii_alphanumeric() || b[i] == b'_') {
            i += 1;
            continue;
        }
        let ks = i;
        while i < b.len() && (b[i].is_ascii_alphanumeric() || b[i] == b'_') {
            i += 1;
        }
        if i >= b.len() || b[i] != b'=' || i + 1 >= b.len() {
            continue;
        }
        let key = &line[ks..i];
        i += 1;
        let value = if b[i] == b'"' {
            match line[i + 1..].find('"') {
                Some(end) => {
                    let v = &line[i + 1..i + 1 + end];
                    i += end + 2;
                    v.to_string()
                }
                None => {
                    let vs = i;
                    while i < b.len() && !b[i].is_ascii_whitespace() {
                        i += 1;
                    }
                    line[vs..i].trim_matches('"').to_string()
                }
            }
        } else {
            let vs = i;
            while i < b.len() && !b[i].is_ascii_whitespace() {
                i += 1;
            }
            line[vs..i].trim_matches('"').to_string()
        };
        if value.is_empty() && !line[..i].ends_with("\"\"") {
            continue;
        }
        out.push((key.to_string(), value));
    }
    out
}

fn num(v: &str) -> i64 {
    v.parse().unwrap_or(0)
}

pub fn parse_info(lines: &[String]) -> Info {
    if lines.is_empty() {
        return Info::default();
    }
    let mut info = Info { present: true, ..Default::default() };
    for l in lines {
        if l.starts_with("NODEVICE") || l.starts_with("ERR") {
            return Info::default();
        }
        for (k, v) in key_values(l) {
            match k.as_str() {
                "vid" => info.vid = v.to_uppercase(),
                "pid" => info.pid = v.to_uppercase(),
                "bcd" => info.bcd = v,
                "usb" => info.usb = v,
                "class" => info.class = v,
                "ep0" => info.ep0 = num(&v),
                "mfr" => info.mfr = v,
                "prod" => info.prod = v,
                "serial" => info.serial = v,
                "cfglen" => info.cfglen = num(&v),
                "ifaces" => info.ifaces = num(&v),
                "power" => info.power_ma = num(&v),
                k if k.starts_with("hidlen") => {
                    let idx: usize = k["hidlen".len()..].parse().unwrap_or(0);
                    if idx < 16 {
                        if info.hid_len.len() <= idx {
                            info.hid_len.resize(idx + 1, 0);
                        }
                        info.hid_len[idx] = num(&v);
                    }
                }
                _ => {}
            }
        }
    }
    info
}

pub fn parse_sys(line: &str) -> Sys {
    let mut s = Sys::default();
    for (k, v) in key_values(line) {
        match k.as_str() {
            "mcu" => s.mcu = v,
            "board" => s.board = v,
            "f_cpu" => s.f_cpu = num(&v),
            "fw" => s.fw = v,
            "vcc_mv" => s.vcc_mv = num(&v),
            "free_ram" => s.free_ram = num(&v),
            "max3421e_rev" => s.max3421_rev = v,
            "uptime_s" => s.uptime_s = num(&v),
            _ => {}
        }
    }
    s
}

pub fn parse_usb_key(name: &str) -> Option<(String, String)> {
    let n = name.to_uppercase();
    if !n.starts_with("VID_") || n.get(8..13) != Some("&PID_") {
        return None;
    }
    Some((n.get(4..8)?.to_string(), n.get(13..17)?.to_string()))
}

const BOARD_IDS: [&str; 5] = ["2341:8036", "2341:0036", "2341:8037", "1B4F:9206", "1B4F:9204"];

pub fn is_board_key(name: &str) -> bool {
    parse_usb_key(name).is_some_and(|(v, p)| BOARD_IDS.contains(&format!("{v}:{p}").as_str()))
}

pub fn board_ports(usb: &[(String, Vec<String>)], present: &[String]) -> Vec<String> {
    let live: Vec<String> = present.iter().map(|p| p.to_uppercase()).collect();
    let mut out: Vec<String> = Vec::new();
    for (key, ports) in usb {
        if !is_board_key(key) {
            continue;
        }
        for p in ports {
            if live.contains(&p.to_uppercase()) && !out.iter().any(|o| o.eq_ignore_ascii_case(p)) {
                out.push(p.clone());
            }
        }
    }
    out.sort();
    out
}
