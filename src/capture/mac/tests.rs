use std::path::PathBuf;

use super::*;
use crate::capture::{Event, Msg};
use crate::hid::{MOD_ALT, MOD_ALTGR, MOD_CTRL, MOD_GUI, MOD_SHIFT};
use crate::sync::wait_until;
use crate::testutil::temp_dir;

const FAKE_HELPER: &str = r#"#!/bin/sh
LOG="$(dirname "$0")/log"
MODE="$(cat "$(dirname "$0")/mode" 2>/dev/null)"
EVENTS="$(cat "$(dirname "$0")/events" 2>/dev/null)"
if [ "$1" = "--check" ]; then
  printf 'accessibility 1\ninput_monitoring 0\ntap TAP\nlayout iso\n'
  exit 0
fi
echo "$@" > "$LOG.args"
case "$MODE" in
perm) echo "ERR permission accessibility=1 input_monitoring=0"; exit 3;;
silent) exit 1;;
crash) echo "READY iso"; sleep 0.3; echo "D 13 108"; sleep 0.2; exit 1;;
esac
echo "READY iso"
if [ -n "$EVENTS" ]; then (sleep 0.3; for e in $EVENTS; do echo "$e" | tr '_' ' '; done) & fi
while read -r line; do
  echo "$line" >> "$LOG"
  [ "$line" = "QUIT" ] && exit 0
done
echo eof >> "$LOG"
"#;

struct FakeMac {
    dir: PathBuf,
    path: PathBuf,
}

impl FakeMac {
    fn new(mode: &str, tap: &str) -> FakeMac {
        let dir = temp_dir("mac");
        let path = dir.join(HELPER_NAME);
        std::fs::write(&path, FAKE_HELPER.replace("TAP", tap)).unwrap();
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
        std::fs::write(dir.join("mode"), mode).unwrap();
        FakeMac { dir, path }
    }

    fn events(&self, ev: &str) {
        std::fs::write(self.dir.join("events"), ev).unwrap();
    }

    fn received(&self) -> Vec<String> {
        std::fs::read_to_string(self.dir.join("log")).unwrap_or_default().lines().map(String::from).collect()
    }

    fn args(&self) -> String {
        std::fs::read_to_string(self.dir.join("log.args")).unwrap_or_default().trim().to_string()
    }

    fn backend(&self) -> MacBackend {
        let p = self.path.clone();
        MacBackend::new(Box::new(move || p.clone()))
    }
}

impl Drop for FakeMac {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.dir);
    }
}

fn next_events(f: &Filter, n: usize) -> Vec<Event> {
    let mut got = Vec::new();
    let deadline = std::time::Instant::now() + Duration::from_secs(3);
    while got.len() < n && std::time::Instant::now() < deadline {
        if let Some(Msg::Event(ev)) = f.queue().pop(Duration::from_millis(50)) {
            got.push(ev);
        }
    }
    got
}

#[test]
fn starts_and_configures() {
    let fm = FakeMac::new("", "0");
    let be = fm.backend();
    let flt = Arc::new(Filter::new());
    be.start(&flt).unwrap();
    assert!(be.alive());
    let mut m = [0u8; 32];
    m[0] |= 1 << 4;
    flt.set_mask(m);
    assert!(wait_until(Duration::from_secs(2), || fm.received().iter().any(|l| l.starts_with("CFG 01000000"))));
    assert_eq!(fm.args(), "run --prompt");
    be.stop();
}

#[test]
fn asks_for_permission_once() {
    let fm = FakeMac::new("", "0");
    let be = fm.backend();
    let flt = Arc::new(Filter::new());
    be.start(&flt).unwrap();
    be.stop();
    be.start(&flt).unwrap();
    assert_eq!(fm.args(), "run");
    be.stop();
}

#[test]
fn relays_keys_through_the_filter() {
    let fm = FakeMac::new("", "0");
    fm.events("D_13_108 U_13 P_0 R_0");
    let be = fm.backend();
    let flt = Arc::new(Filter::new());
    be.start(&flt).unwrap();
    let got = next_events(&flt, 3);
    be.stop();
    assert_eq!(got.len(), 3, "{got:?}");
    assert_eq!(got[0], Event::new(b'D', 0x1A, MOD_GUI));
    assert_eq!(got[1], Event::new(b'U', 0x1A, 0));
    assert_eq!(got[2], Event::new(b'P', 0x04, 0));
}

#[test]
fn releases_held_keys_when_the_helper_dies() {
    let fm = FakeMac::new("crash", "0");
    let be = fm.backend();
    let flt = Arc::new(Filter::new());
    be.start(&flt).unwrap();
    let kinds: Vec<u8> = next_events(&flt, 2).iter().map(|e| e.kind).collect();
    assert_eq!(kinds, b"DU");
    assert!(wait_until(Duration::from_secs(2), || !be.alive()));
}

#[test]
fn stop_sends_quit() {
    let fm = FakeMac::new("", "0");
    let be = fm.backend();
    be.start(&Arc::new(Filter::new())).unwrap();
    be.stop();
    assert!(!be.alive());
    assert!(fm.received().iter().any(|l| l == "QUIT"));
}

#[test]
fn permission_error() {
    let fm = FakeMac::new("perm", "0");
    let err = fm.backend().start(&Arc::new(Filter::new())).unwrap_err();
    assert_eq!(err.code, "permission");
    assert!(err.msg.contains("Input Monitoring") && !err.msg.contains("Accessibility"));
}

#[test]
fn missing_helper() {
    let dir = temp_dir("nomac");
    let p = dir.join(HELPER_NAME);
    let be = MacBackend::new(Box::new(move || p.clone()));
    assert_eq!(be.start(&Arc::new(Filter::new())).unwrap_err().code, "helper_missing");
    let checks = be.checks();
    assert_eq!(checks.len(), 1);
    assert!(!checks[0].ok && checks[0].problem.contains("next to the typedeck binary"));
    let _ = std::fs::remove_dir_all(dir);
}

#[test]
fn helper_that_says_nothing() {
    let fm = FakeMac::new("silent", "0");
    assert!(fm.backend().start(&Arc::new(Filter::new())).is_err());
}

#[test]
fn injection() {
    let fm = FakeMac::new("", "0");
    let be = fm.backend();
    be.start(&Arc::new(Filter::new())).unwrap();
    be.inject(Stroke::new(0x06, MOD_GUI), 1).unwrap();
    be.inject(Stroke::new(0x39, 0), 0).unwrap();
    be.consumer(0xE9).unwrap();
    be.type_text("a\nñ").unwrap();
    assert!(be.consumer(0x999).is_err());
    be.stop();
    let got = fm.received()[1..].join("|");
    for want in ["KEY d 55 100008|KEY d 8 100008|KEY u 8 100008|KEY u 55 0", "CAPS", "MEDIA 0", "TEXT 61|KEY d 36 0|KEY u 36 0|TEXT c3b1"] {
        assert!(got.contains(want), "missing {want} in {got}");
    }
}

#[test]
fn inject_without_helper() {
    let fm = FakeMac::new("", "0");
    assert!(fm.backend().inject(Stroke::new(0x04, 0), 0).is_err());
}

#[test]
fn checks_say_what_is_missing() {
    let fm = FakeMac::new("", "0");
    let bad: Vec<String> = fm.backend().checks().into_iter().filter(|c| !c.ok).map(|c| c.problem).collect();
    let joined = bad.join("\n");
    assert!(joined.contains("Input Monitoring is not allowed") && !joined.contains("Accessibility is not allowed"));
    assert!(joined.contains("System Settings > Privacy & Security") && joined.contains(&fm.path.display().to_string()));
    let ok = FakeMac::new("", "1");
    for c in ok.backend().checks() {
        assert!(c.ok || c.problem.starts_with("optional: "), "{}", c.problem);
    }
}

#[test]
fn software_source_keeps_the_permission_code() {
    let fm = FakeMac::new("perm", "0");
    let s = crate::capture::SoftwareSource::new(Box::new(fm.backend()));
    use crate::capture::Source;
    assert!(s.enable().is_err());
    assert_eq!(s.error(), "permission");
    s.close();
}

#[test]
fn parse_lines() {
    let ok = |line: &str, kind: &str| {
        let m = parse_helper_line(line).unwrap();
        assert_eq!(m.kind, kind);
        m
    };
    assert_eq!(ok("D 13 100108", "D").flags, 0x100108);
    assert_eq!(ok("U 13", "U").code, 13);
    assert_eq!(ok("R 126", "R").code, 126);
    assert_eq!(ok("READY iso", "READY").text, "iso");
    assert_eq!(ok("TAPOFF timeout", "TAPOFF").text, "timeout");
    ok("TAPON", "TAPON");
    assert_eq!(ok("ERR permission accessibility=0 input_monitoring=1", "ERR").text, "permission accessibility=0 input_monitoring=1");
    assert_eq!(ok("LAT 10 5 90", "LAT").counts, [10, 5, 90]);
    for bad in ["", "D 13", "D x 1", "D 200 1", "D 5 zz", "LAT 1 2", "NOPE 1"] {
        assert!(parse_helper_line(bad).is_none(), "{bad}");
    }
}

#[test]
fn config_key_and_text_lines() {
    let mut s = Snapshot::default();
    s.mask[0] |= 1 << 4;
    s.mask[0x39 >> 3] |= 1 << (0x39 & 7);
    assert_eq!(config_line(&s, LAYOUT_ISO), "CFG 01000000000000020000000000000000 0 1");
    s.report_all = true;
    s.inactive = true;
    assert!(config_line(&s, LAYOUT_ISO).ends_with(" 1 0"));
    assert_eq!(key_line(KeyStep { down: true, code: 8, flags: 0x100108 }), "KEY d 8 100108");
    assert_eq!(key_line(KeyStep { down: false, code: 55, flags: 0 }), "KEY u 55 0");
    assert_eq!(text_line("ñ€"), "TEXT c3b1e282ac");
    assert_eq!(media_line(16), "MEDIA 16");
    let st = parse_check("accessibility 1\ninput_monitoring 0\ntap 0\nlayout iso\n");
    assert_eq!((st["accessibility"].as_str(), st["input_monitoring"].as_str(), st["layout"].as_str()), ("1", "0", "iso"));
}

#[test]
fn keycodes() {
    for layout in [LAYOUT_ANSI, LAYOUT_ISO, "jis"] {
        for (&u, &want) in &keymap().code {
            let c = mac_keycode(u, layout).unwrap();
            if matches!(u, 0x32 | 0x35 | 0x64) {
                continue;
            }
            assert_eq!(c, want);
            assert_eq!(mac_usage(c, layout), u);
        }
    }
    for (u, c) in [
        (0x04, 0),
        (0x05, 11),
        (0x1D, 6),
        (0x1E, 18),
        (0x27, 29),
        (0x28, 36),
        (0x2C, 49),
        (0x39, 57),
        (0x3A, 122),
        (0x45, 111),
        (0x4F, 124),
        (0x52, 126),
        (0x59, 83),
        (0x62, 82),
        (0x68, 105),
        (0xE0, 59),
        (0xE3, 55),
        (0xE7, 54),
    ] {
        assert_eq!(mac_keycode(u, LAYOUT_ISO), Some(c), "{u:02X}");
    }
    for (layout, u, c) in
        [(LAYOUT_ISO, 0x35, 10), (LAYOUT_ISO, 0x64, 50), ("jis", 0x35, 10), ("jis", 0x64, 50), (LAYOUT_ANSI, 0x35, 50), (LAYOUT_ANSI, 0x64, 10)]
    {
        assert_eq!(mac_keycode(u, layout), Some(c));
        assert_eq!(mac_usage(c, layout), u);
    }
    for c in [63, 66, 127, 200] {
        assert_eq!(mac_usage(c, LAYOUT_ISO), 0);
    }
}

#[test]
fn modifier_flags() {
    for (flags, mods) in [
        (0, 0),
        (0x8 | FLAG_COMMAND, MOD_GUI),
        (0x10 | FLAG_COMMAND, 0x80),
        (0x1 | 0x2 | FLAG_CONTROL | FLAG_SHIFT, MOD_CTRL | MOD_SHIFT),
        (0x20 | FLAG_OPTION, MOD_ALT),
        (0x40 | FLAG_OPTION, MOD_ALTGR),
        (0x2000 | FLAG_CONTROL, 0x10),
        (0x4 | FLAG_SHIFT, 0x20),
        (FLAG_SHIFT, MOD_SHIFT),
        (FLAG_COMMAND | FLAG_OPTION, MOD_GUI | MOD_ALT),
    ] {
        assert_eq!(mods_from_flags(flags), mods, "{flags:x}");
    }
    for m in 0..=255u8 {
        assert_eq!(mods_from_flags(flags_from_mods(m)), m);
    }
}

#[test]
fn keycode_mask_skips_the_alias() {
    let mut s = Snapshot::default();
    for u in [0x04u8, 0x39, 0x35, 0x64, 0x32, 0x46] {
        s.mask[usize::from(u >> 3)] |= 1 << (u & 7);
    }
    let iso = keycode_mask(&s, LAYOUT_ISO);
    for c in [0u8, 57, 10, 50] {
        assert!(iso[usize::from(c >> 3)] >> (c & 7) & 1 == 1, "{c}");
    }
    assert_eq!(iso[42 >> 3] >> (42 & 7) & 1, 0);
    assert_eq!(iso.iter().map(|b| b.count_ones()).sum::<u32>(), 4);
}

#[test]
fn stroke_plans() {
    let cmd = flags_from_mods(MOD_GUI);
    let (down, up) = stroke_plan(Stroke::new(0x06, MOD_GUI), LAYOUT_ISO).unwrap();
    assert_eq!(down, vec![KeyStep { down: true, code: 55, flags: cmd }, KeyStep { down: true, code: 8, flags: cmd }]);
    assert_eq!(up, vec![KeyStep { down: false, code: 8, flags: cmd }, KeyStep { down: false, code: 55, flags: 0 }]);
    let (down, up) = stroke_plan(Stroke::new(0x16, MOD_CTRL | MOD_SHIFT), LAYOUT_ISO).unwrap();
    assert_eq!((down[0].code, down[1].code, up[1].code, up[2].code), (59, 56, 56, 59));
    assert_eq!((up[1].flags, up[2].flags), (flags_from_mods(MOD_CTRL), 0));
    let (down, up) = stroke_plan(Stroke::new(0xE1, 0), LAYOUT_ISO).unwrap();
    assert_eq!((down.len(), up.len(), down[0].code), (1, 1, 56));
    assert!(down[0].flags & 0x2 != 0 && up[0].flags == 0);
    assert!(stroke_plan(Stroke::new(0x46, 0), LAYOUT_ISO).is_err());
    assert_eq!(consumer_key_type(0xCD), Some(16));
    assert_eq!(consumer_key_type(0x999), None);
}

#[cfg(target_os = "macos")]
fn real_helper() -> Option<PathBuf> {
    std::env::var_os("TYPEDECK_CAPTURE_HELPER").map(PathBuf::from)
}

#[cfg(target_os = "macos")]
#[test]
fn real_helper_check_reports_every_field() {
    let Some(path) = real_helper() else { return };
    let st = MacBackend::run_check(&path).unwrap();
    for k in ["accessibility", "input_monitoring", "tap", "layout"] {
        assert!(st.get(k).is_some_and(|v| !v.is_empty()), "missing {k} in {st:?}");
    }
    assert!(matches!(st["layout"].as_str(), "ansi" | "iso" | "jis"));
}

#[cfg(target_os = "macos")]
#[test]
fn real_helper_says_what_is_missing_or_is_ready() {
    use std::io::{BufRead, BufReader};
    use std::process::{Command, Stdio};
    let Some(path) = real_helper() else { return };
    let mut child = Command::new(&path).arg("run").stdin(Stdio::piped()).stdout(Stdio::piped()).spawn().unwrap();
    let mut line = String::new();
    BufReader::new(child.stdout.take().unwrap()).read_line(&mut line).unwrap();
    let line = line.trim().to_string();
    assert!(line.starts_with("ERR permission accessibility=") || line.starts_with("READY "), "{line}");
    drop(child.stdin.take());
    assert!(wait_until(Duration::from_secs(3), || matches!(child.try_wait(), Ok(Some(_)))), "the helper did not exit");
    assert!(!Command::new(&path).arg("nope").status().unwrap().success());
}

#[cfg(target_os = "macos")]
#[test]
fn real_helper_through_the_backend() {
    let Some(path) = real_helper() else { return };
    let be = MacBackend::new(Box::new(move || path.clone()));
    be.prompted.store(true, std::sync::atomic::Ordering::SeqCst);
    match be.start(&Arc::new(Filter::new())) {
        Ok(()) => {
            be.stop();
            assert!(!be.alive());
        }
        Err(e) => {
            assert_eq!(e.code, "permission", "{}", e.msg);
            assert!(e.msg.contains("System Settings"));
            assert!(!be.alive());
        }
    }
}
