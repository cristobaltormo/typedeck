use serde::Serialize;

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, serde::Deserialize)]
pub struct Report {
    pub id: i64,
    pub dir: String,
    pub kind: String,
    pub bits: i64,
    pub detail: String,
}

#[derive(Clone, Copy, Default)]
struct Globals {
    page: i64,
    size: i64,
    count: i64,
    id: i64,
}

struct Field {
    id: i64,
    page: i64,
    size: i64,
    count: i64,
    usages: Vec<i64>,
    umin: i64,
    flags: i64,
    dir: &'static str,
}

pub fn parse_report_descriptor(data: &[u8]) -> Vec<Report> {
    let mut fields = Vec::new();
    let mut g = Globals::default();
    let mut stack: Vec<Globals> = Vec::new();
    let mut usages: Vec<i64> = Vec::new();
    let mut umin = -1i64;
    let mut i = 0usize;
    while i < data.len() {
        let b = data[i];
        if b == 0xFE {
            if i + 1 >= data.len() {
                break;
            }
            i += 3 + usize::from(data[i + 1]);
            continue;
        }
        let size = match b & 3 {
            3 => 4,
            n => usize::from(n),
        };
        let (typ, tag) = ((b >> 2) & 3, b >> 4);
        let mut val: i64 = 0;
        for k in 0..size {
            if i + 1 + k < data.len() {
                val |= i64::from(data[i + 1 + k]) << (8 * k);
            }
        }
        i += 1 + size;
        match typ {
            1 => match tag {
                0 => g.page = val,
                7 => g.size = val,
                8 => g.id = val,
                9 => g.count = val,
                0xA => stack.push(g),
                0xB => {
                    if let Some(prev) = stack.pop() {
                        g = prev;
                    }
                }
                _ => {}
            },
            2 => match tag {
                0 => usages.push(val),
                1 => umin = val,
                _ => {}
            },
            0 => {
                let dir = match tag {
                    8 => Some("input"),
                    9 => Some("output"),
                    0xB => Some("feature"),
                    _ => None,
                };
                if let Some(dir) = dir {
                    fields.push(Field { id: g.id, page: g.page, size: g.size, count: g.count, usages: usages.clone(), umin, flags: val, dir });
                }
                if tag != 0xC {
                    usages.clear();
                    umin = -1;
                }
            }
            _ => {}
        }
    }
    summarise(&fields)
}

pub fn parse_report_descriptor_hex(s: &str) -> Result<Vec<Report>, String> {
    let joined: String = s.split_whitespace().collect();
    let bytes = crate::crypto::unhex(&joined).ok_or_else(|| "invalid hex".to_string())?;
    Ok(parse_report_descriptor(&bytes))
}

fn summarise(fields: &[Field]) -> Vec<Report> {
    let mut order: Vec<(i64, &'static str)> = Vec::new();
    for f in fields {
        if !order.contains(&(f.id, f.dir)) {
            order.push((f.id, f.dir));
        }
    }
    order.sort_by_key(|k| k.0);
    let mut out = Vec::new();
    for (id, dir) in order {
        let mut r = Report { id, dir: dir.into(), kind: "other".into(), ..Default::default() };
        let mut detail: Vec<String> = Vec::new();
        for f in fields.iter().filter(|f| f.id == id && f.dir == dir) {
            r.bits += f.size * f.count;
            let has = |u: i64| f.usages.contains(&u);
            if f.page == 0x07 && f.flags & 1 == 0 && f.size == 1 && f.count == 8 && (f.umin == 0xE0 || has(0xE0)) {
                r.kind = "keyboard".into();
                detail.push("modifiers".into());
            } else if f.page == 0x07 && f.size == 1 && f.count > 8 && f.umin >= 0 {
                r.kind = "nkro".into();
                detail.push(format!("{} keys at once", f.count));
            } else if f.page == 0x07 && f.size == 8 && f.count > 1 && f.flags & 2 == 0 {
                r.kind = "keyboard".into();
                detail.push(format!("up to {} keys at once", f.count));
            } else if f.page == 0x08 && f.dir == "output" {
                detail.push("keyboard LEDs".into());
            } else if f.page == 0x0C && r.kind != "mouse" {
                r.kind = "consumer".into();
                detail.push("media keys".into());
            } else if f.page == 0x01 && has(0x80) || f.page == 0x01 && f.umin == 0x81 {
                r.kind = "system".into();
                detail.push("power, sleep and wake".into());
            } else if f.page == 0x01 && (has(0x30) || has(0x31)) {
                r.kind = "mouse".into();
                detail.push("mouse movement".into());
            } else if f.page == 0x09 {
                if r.kind == "other" {
                    r.kind = "mouse".into();
                }
                detail.push("buttons".into());
            } else if f.page >= 0xFF00 {
                r.kind = "vendor".into();
            }
        }
        if r.kind == "mouse" && r.dir == "input" && !detail.is_empty() {
            detail = vec!["built-in mouse".into()];
        }
        if r.kind == "vendor" {
            detail = vec!["vendor data".into()];
        }
        let mut seen = Vec::new();
        for d in detail {
            if !seen.contains(&d) {
                seen.push(d);
            }
        }
        r.detail = seen.join(", ");
        out.push(r);
    }
    out
}
