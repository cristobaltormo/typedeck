use std::collections::BTreeMap;

use serde::Serialize;

use crate::capture::{Info, SOFTWARE, Source, Sys};
use crate::config::Config;
use crate::kbdb::{self, Identity};
use crate::layouts;
use crate::platform::{self, SystemInfo};

pub const COMPAT: &str = include_str!("data/compat.json");

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct LayoutChoice {
    pub id: String,
    pub percent: i64,
    pub standard: String,
    pub source: String,
    pub confident: bool,
}

pub fn identify(info: &Info) -> Identity {
    if info.system {
        return Identity { display: "System keyboard".into(), confidence: "unknown".into(), source: "system".into(), ..Default::default() };
    }
    kbdb::identify(&info.vid, &info.pid, &info.mfr, &info.prod)
}

pub fn resolve(cfg: &Config, info: Option<&Info>, id: &Identity, sys: &SystemInfo) -> LayoutChoice {
    let pick = |layout_id: &str, source: &str, confident: bool| {
        layouts::get(layout_id)
            .map(|l| LayoutChoice { id: l.id.clone(), percent: l.percent, standard: l.standard.clone(), source: source.into(), confident })
            .unwrap_or_default()
    };
    if let Some(info) = info
        && let Some(kb) = cfg.keyboards.get(&info.id())
        && !kb.layout.is_empty()
    {
        let c = pick(&kb.layout, "user", true);
        if !c.id.is_empty() {
            return c;
        }
    }
    if !id.layout.is_empty() {
        let verified = id.confidence == "verified";
        let c = pick(&id.layout, if verified { "verified" } else { "name" }, verified);
        if !c.id.is_empty() {
            return c;
        }
    }
    let iso = sys.keyboard_type == "iso" || (sys.keyboard_type.is_empty() && matches!(sys.typing_layout.as_str(), "es-iso" | "es-pc" | "es-win"));
    let family = if id.form_factor.is_empty() { "full" } else { id.form_factor.as_str() };
    pick(&format!("{family}-{}", if iso { "iso" } else { "ansi" }), "default", false)
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct Check {
    pub level: String,
    pub code: String,
    #[serde(skip_serializing_if = "BTreeMap::is_empty")]
    pub args: BTreeMap<String, String>,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct Host {
    pub os: String,
    pub version: String,
    pub arch: String,
    pub typing_layout: String,
    pub keyboard_type: String,
    pub mod_remap: Vec<String>,
    pub keys_swapped: bool,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
pub struct BoardInfo {
    pub capture: String,
    pub connected: bool,
    pub port: String,
    pub firmware: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sys: Option<Sys>,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
pub struct KeyboardInfo {
    pub present: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub info: Option<Info>,
    pub identity: Identity,
    pub layout: LayoutChoice,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
pub struct Setup {
    pub board: Option<BoardInfo>,
    pub keyboard: Option<KeyboardInfo>,
    pub host: Host,
    pub checks: Vec<Check>,
    pub summary: String,
}

pub fn arch() -> &'static str {
    match std::env::consts::ARCH {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        "x86" => "386",
        other => other,
    }
}

impl Setup {
    fn add(&mut self, level: &str, code: &str, kv: &[(&str, String)]) {
        self.checks.push(Check { level: level.into(), code: code.into(), args: kv.iter().map(|(k, v)| (k.to_string(), v.clone())).collect() });
    }

    fn software(&mut self, cfg: &Config, sys: &SystemInfo, connected: bool, info: Option<&Info>, src: &dyn Source) {
        let Some(info) = info.filter(|_| connected) else {
            self.add("error", software_failure(&src.error()), &[]);
            self.summary = "Software capture is not running".into();
            return;
        };
        let id = identify(info);
        let ch = resolve(cfg, Some(info), &id, sys);
        self.keyboard = Some(KeyboardInfo { present: true, info: Some(info.clone()), identity: id.clone(), layout: ch.clone() });
        self.add("ok", "software_ok", &[]);
        self.add("info", "software_limits", &[]);
        for w in src.warnings() {
            self.add("warn", &w, &[]);
        }
        self.host_checks(sys, &id, &ch);
        self.summary = format!("System keyboard on {} {} ({}), software capture", os_label(&self.host.os), self.host.version, self.host.arch);
    }

    fn power_checks(&mut self, bs: &Sys, info: &Info) {
        if bs.vcc_mv > 0 {
            if bs.vcc_mv < 4500 {
                self.add("error", "vcc_low", &[("mv", bs.vcc_mv.to_string())]);
            } else if bs.vcc_mv < 4750 && info.power_ma >= 300 {
                self.add("warn", "vcc_marginal", &[("mv", bs.vcc_mv.to_string()), ("ma", info.power_ma.to_string())]);
            }
        }
        if info.power_ma > 500 {
            self.add("warn", "power_over", &[("ma", info.power_ma.to_string())]);
        }
    }

    fn report_checks(&mut self, info: &Info) {
        let input = info.reports.iter().filter(|r| r.dir == "input");
        let (mut boot, mut consumer, mut other) = (false, false, false);
        for r in input {
            match r.kind.as_str() {
                "keyboard" => boot = true,
                "consumer" => consumer = true,
                "mouse" | "system" | "nkro" => other = true,
                _ => {}
            }
        }
        if !info.reports.is_empty() && !boot {
            self.add("error", "no_boot_keyboard", &[]);
        } else if boot {
            self.add("ok", "boot_keyboard", &[]);
        }
        if consumer {
            self.add("ok", "media_forwarded", &[]);
        }
        if other {
            self.add("info", "extras_not_forwarded", &[]);
        }
        self.add("info", "six_key_limit", &[]);
    }

    fn host_checks(&mut self, sys: &SystemInfo, id: &Identity, ch: &LayoutChoice) {
        if !ch.confident {
            self.add("info", "layout_unconfirmed", &[]);
        }
        if id.generic || id.vendor_is_chip {
            self.add("info", "generic_keyboard", &[]);
        }
        if !matches!(sys.typing_layout.as_str(), "us" | "es-iso" | "es-pc" | "es-win") {
            self.add("warn", "typing_layout_unknown", &[]);
        }
        if !sys.mod_remap.is_empty() {
            self.add("info", "mod_remap", &[("map", sys.mod_remap.join(", "))]);
        }
        if sys.keys_swapped {
            self.add("info", "ansi_iso_swapped", &[]);
        }
    }
}

pub fn software_failure(code: &str) -> &'static str {
    match code {
        "permission" => "no_permission",
        "helper_missing" => "no_helper",
        "no_uinput" => "no_uinput",
        "uinput_access" => "no_uinput_access",
        "input_access" => "no_input_access",
        _ => "no_hook",
    }
}

pub fn os_label(os: &str) -> &str {
    match os {
        "darwin" => "macOS",
        "linux" => "Linux",
        "windows" => "Windows",
        other => other,
    }
}

fn board_name(s: &Sys) -> String {
    match s.board.as_str() {
        "leonardo" => format!("Arduino Leonardo (ATmega32U4, {} MHz)", s.f_cpu / 1_000_000),
        "micro" => format!("Arduino Micro (ATmega32U4, {} MHz)", s.f_cpu / 1_000_000),
        _ => "ATmega32U4 board".into(),
    }
}

fn shield_name(s: &Sys) -> String {
    if s.max3421_rev.is_empty() {
        return "USB Host Shield".into();
    }
    let rev = u8::from_str_radix(&s.max3421_rev, 16).map(|n| (n & 0x0F).to_string()).unwrap_or_else(|_| s.max3421_rev.clone());
    format!("USB Host Shield (MAX3421E rev {rev})")
}

pub fn describe(cfg: &Config, src: &dyn Source, sys: &SystemInfo) -> Setup {
    let host = Host {
        os: platform::get().name().into(),
        version: platform::get().version(),
        arch: arch().into(),
        typing_layout: sys.typing_layout.clone(),
        keyboard_type: sys.keyboard_type.clone(),
        mod_remap: sys.mod_remap.clone(),
        keys_swapped: sys.keys_swapped,
    };
    let mut su = Setup { host, ..Default::default() };
    let st = src.status();
    su.board = Some(BoardInfo { capture: src.kind().into(), connected: st.connected, port: st.port.clone(), ..Default::default() });
    if src.kind() == SOFTWARE {
        su.software(cfg, sys, st.connected, st.info.as_ref(), src);
        return su;
    }
    if !st.connected {
        su.add("error", "no_board", &[]);
        su.summary = "No board connected".into();
        return su;
    }
    let bs = src.sys().ok();
    if let Some(b) = su.board.as_mut() {
        b.sys = bs.clone();
        b.firmware = st.info.as_ref().map(|i| i.firmware.clone()).unwrap_or_default();
    }
    let Some(info) = st.info.filter(|i| i.present) else {
        su.add("warn", "no_keyboard", &[]);
        su.summary = "Board connected, no keyboard on the shield".into();
        return su;
    };
    let id = identify(&info);
    let ch = resolve(cfg, Some(&info), &id, sys);
    su.keyboard = Some(KeyboardInfo { present: true, info: Some(info.clone()), identity: id.clone(), layout: ch.clone() });
    su.add("ok", "board_ok", &[]);
    let bsys = bs.clone().unwrap_or_default();
    su.power_checks(&bsys, &info);
    su.report_checks(&info);
    su.host_checks(sys, &id, &ch);
    let os = os_label(&su.host.os).to_string();
    su.summary = match bs {
        None => format!("Board + {} on {os} {}", id.display.trim(), su.host.version),
        Some(b) => format!("{} + {} + {} on {os} {} ({})", board_name(&b), shield_name(&b), id.display.trim(), su.host.version, su.host.arch),
    };
    su
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn software_failure_codes() {
        for (input, want) in [
            ("permission", "no_permission"),
            ("helper_missing", "no_helper"),
            ("hook_failed", "no_hook"),
            ("", "no_hook"),
            ("no_uinput", "no_uinput"),
            ("uinput_access", "no_uinput_access"),
            ("input_access", "no_input_access"),
        ] {
            assert_eq!(software_failure(input), want);
        }
    }

    #[test]
    fn layout_resolution() {
        let mut cfg = Config::initial();
        let info = Info { present: true, vid: "1234".into(), pid: "5678".into(), ..Default::default() };
        let id = Identity::default();
        let sys = SystemInfo { typing_layout: "es-pc".into(), ..Default::default() };
        assert_eq!(resolve(&cfg, Some(&info), &id, &sys).id, "full-iso");
        let us = SystemInfo { typing_layout: "us".into(), ..Default::default() };
        assert_eq!(
            resolve(&cfg, Some(&info), &id, &us),
            LayoutChoice { id: "full-ansi".into(), percent: 100, standard: "ansi".into(), source: "default".into(), confident: false }
        );
        cfg.keyboards.insert("1234:5678".into(), crate::config::Keyboard { layout: "60-ansi".into(), name: String::new() });
        let c = resolve(&cfg, Some(&info), &id, &sys);
        assert_eq!((c.id.as_str(), c.source.as_str(), c.confident), ("60-ansi", "user", true));
        assert_eq!(shield_name(&Sys { max3421_rev: "13".into(), ..Default::default() }), "USB Host Shield (MAX3421E rev 3)");
    }
}
