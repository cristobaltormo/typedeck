use std::time::Duration;

use crate::platform::{env_var, have, run};

pub fn xkb_to_hid(layout: &str, variant: &str) -> &'static str {
    let (layout, variant) = (layout.trim().to_lowercase(), variant.trim().to_lowercase());
    if ["dvorak", "colemak", "workman", "norman", "dvp", "neo"].iter().any(|v| variant.starts_with(v)) {
        return "";
    }
    match layout.as_str() {
        "us" if variant.is_empty() => "us",
        "es" if matches!(variant.as_str(), "" | "winkeys" | "nodeadkeys") => "es-pc",
        "us" | "gb" | "es" | "pt" | "br" | "it" | "latam" | "ie" | "ca" => "qwerty",
        _ => "",
    }
}

fn field(out: &str, key: &str) -> String {
    for line in out.lines() {
        if let Some((k, v)) = line.split_once(':')
            && k.trim() == key
        {
            return v.trim().split(',').next().unwrap_or("").to_string();
        }
    }
    String::new()
}

pub fn system_layout() -> String {
    let t = Duration::from_secs(2);
    if have("gsettings")
        && let Ok(out) = run(t, "", "gsettings", &["get", "org.gnome.desktop.input-sources", "mru-sources"])
        && let Some(i) = out.find("'xkb', '")
    {
        let v = &out[i + 8..];
        if let Some(j) = v.find('\'')
            && j > 0
        {
            let (l, var) = v[..j].split_once('+').unwrap_or((&v[..j], ""));
            return xkb_to_hid(l, var).into();
        }
    }
    if have("setxkbmap") && env_var("DISPLAY").is_some() {
        let out = run(t, "", "setxkbmap", &["-query"]).unwrap_or_else(|e| e);
        let l = field(&out, "layout");
        if !l.is_empty() {
            return xkb_to_hid(&l, &field(&out, "variant")).into();
        }
    }
    if have("localectl") {
        let out = run(t, "", "localectl", &["status"]).unwrap_or_else(|e| e);
        let l = field(&out, "X11 Layout");
        if !l.is_empty() {
            return xkb_to_hid(&l, &field(&out, "X11 Variant")).into();
        }
    }
    String::new()
}
