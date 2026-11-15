use std::sync::{Arc, Mutex};
use std::time::Duration;

use super::*;
use crate::sync::wait_until;

#[derive(Default)]
pub struct FakeBackend {
    pub start_err: Mutex<Option<CaptureError>>,
    pub alive: Mutex<bool>,
    pub starts: Mutex<u32>,
    pub attempts: Mutex<u32>,
    pub injected: Mutex<Vec<Stroke>>,
    pub filter: Mutex<Option<Arc<Filter>>>,
}

impl Backend for Arc<FakeBackend> {
    fn start(&self, f: &Arc<Filter>) -> Result<(), CaptureError> {
        *lock(&self.attempts) += 1;
        if let Some(e) = lock(&self.start_err).clone() {
            return Err(e);
        }
        *lock(&self.alive) = true;
        *lock(&self.filter) = Some(f.clone());
        *lock(&self.starts) += 1;
        Ok(())
    }
    fn alive(&self) -> bool {
        *lock(&self.alive)
    }
    fn stop(&self) {
        *lock(&self.alive) = false;
    }
    fn inject(&self, s: Stroke, _hold_ms: u32) -> Result<(), String> {
        lock(&self.injected).push(s);
        Ok(())
    }
    fn consumer(&self, _usage: u16) -> Result<(), String> {
        Ok(())
    }
    fn type_text(&self, _text: &str) -> Result<(), String> {
        Ok(())
    }
    fn latency(&self) -> Latency {
        Latency { count: 3, avg_us: 5, max_us: 9 }
    }
    fn checks(&self) -> Vec<Check> {
        vec![]
    }
}

#[derive(Default)]
pub struct Recorder {
    pub events: Mutex<Vec<Event>>,
    pub conn: Mutex<u32>,
    pub disc: Mutex<u32>,
    pub mask: Mutex<[u8; 32]>,
}

impl Recorder {
    pub fn handlers(self: &Arc<Self>) -> Handlers {
        let (a, b, c, d) = (self.clone(), self.clone(), self.clone(), self.clone());
        Handlers {
            on_event: Some(Arc::new(move |ev| lock(&a.events).push(ev))),
            mask: Some(Arc::new(move || *lock(&b.mask))),
            on_connected: Some(Arc::new(move |_| *lock(&c.conn) += 1)),
            on_disconnect: Some(Arc::new(move || *lock(&d.disc) += 1)),
        }
    }

    fn events(&self) -> Vec<Event> {
        lock(&self.events).clone()
    }
}

pub fn mask_of(us: &[u8]) -> [u8; 32] {
    let mut m = [0u8; 32];
    for u in us {
        m[usize::from(u >> 3)] |= 1 << (u & 7);
    }
    m
}

fn pop(f: &Filter) -> Option<Event> {
    match f.queue().pop(Duration::from_millis(50)) {
        Some(Msg::Event(e)) => Some(e),
        _ => None,
    }
}

#[test]
fn captured_key_is_swallowed_and_reported() {
    let f = Filter::new();
    f.set_mask(mask_of(&[0x04]));
    assert!(f.handle(0x04, true));
    assert_eq!(pop(&f), Some(Event::new(b'D', 0x04, 0)));
    assert!(f.handle(0x04, false));
    assert_eq!(pop(&f), Some(Event::new(b'U', 0x04, 0)));
}

#[test]
fn uncaptured_key_passes_and_is_invisible() {
    let f = Filter::new();
    assert!(!f.handle(0x05, true));
    assert!(!f.handle(0x05, false));
    assert!(pop(&f).is_none());
}

#[test]
fn report_all_shows_plain_keys() {
    let f = Filter::new();
    f.set_report_all(true);
    assert!(!f.handle(0x05, true));
    assert!(!f.handle(0x05, false));
    assert_eq!(pop(&f), Some(Event::new(b'P', 0x05, 0)));
    assert_eq!(pop(&f), Some(Event::new(b'R', 0x05, 0)));
}

#[test]
fn autorepeat_of_a_swallowed_key_is_swallowed_without_events() {
    let f = Filter::new();
    f.set_mask(mask_of(&[0x04]));
    assert!(f.handle(0x04, true));
    assert!(f.handle(0x04, true));
    assert!(f.handle(0x04, true));
    assert_eq!(pop(&f).map(|e| e.kind), Some(b'D'));
    assert!(pop(&f).is_none());
}

#[test]
fn mask_change_while_held_keeps_the_decision() {
    let f = Filter::new();
    f.set_mask(mask_of(&[0x04]));
    assert!(f.handle(0x04, true));
    f.set_mask([0; 32]);
    assert!(f.handle(0x04, false));
    assert!(!f.handle(0x05, true));
    f.set_mask(mask_of(&[0x05]));
    assert!(!f.handle(0x05, false));
}

#[test]
fn modifiers_travel_with_the_event() {
    let f = Filter::new();
    f.set_mask(mask_of(&[0x04]));
    assert!(!f.handle(0xE0, true));
    assert!(!f.handle(0xE1, true));
    assert!(f.handle(0x04, true));
    assert_eq!(pop(&f), Some(Event::new(b'D', 0x04, 0x03)));
    f.handle(0xE0, false);
    f.handle(0xE1, false);
    f.handle(0x04, false);
}

#[test]
fn a_swallowed_modifier_is_tracked() {
    let f = Filter::new();
    f.set_mask(mask_of(&[0xE3, 0x06]));
    assert!(f.handle(0xE3, true));
    assert!(f.handle(0x06, true));
    let _ = pop(&f);
    assert_eq!(pop(&f), Some(Event::new(b'D', 0x06, 0x08)));
}

#[test]
fn unknown_key_up_passes() {
    let f = Filter::new();
    assert!(!f.handle(0x07, false));
    assert!(!f.handle(0, true));
}

#[test]
fn pause_releases_and_passes_everything() {
    let f = Filter::new();
    f.set_mask(mask_of(&[0x04]));
    assert!(f.handle(0x04, true));
    let _ = pop(&f);
    f.pause();
    assert_eq!(pop(&f), Some(Event::new(b'U', 0x04, 0)));
    assert!(!f.handle(0x04, true));
    assert!(f.snapshot().inactive);
    f.resume();
    assert!(f.handle(0x04, true));
}

#[test]
fn a_full_queue_never_blocks() {
    let f = Filter::new();
    f.set_report_all(true);
    for i in 0..3000u32 {
        let u = 4 + (i % 50) as u8;
        f.handle(u, true);
        f.handle(u, false);
    }
    assert!(f.dropped() > 0);
}

#[test]
fn concurrent_mask_updates() {
    let f = Arc::new(Filter::new());
    let hs: Vec<_> = (0..4)
        .map(|i| {
            let f = f.clone();
            std::thread::spawn(move || {
                for n in 0..500u32 {
                    f.set_mask(mask_of(&[4 + ((n + i) % 40) as u8]));
                    f.handle(0x10, n % 2 == 0);
                }
            })
        })
        .collect();
    for h in hs {
        h.join().unwrap();
    }
}

fn new_soft() -> (Arc<SoftwareSource>, Arc<FakeBackend>, Arc<Recorder>) {
    let be = Arc::new(FakeBackend::default());
    let s = SoftwareSource::new(Box::new(be.clone()));
    s.set_retry(Duration::from_millis(20), Duration::from_millis(80));
    let r = Arc::new(Recorder::default());
    s.bind(r.handlers());
    let s2 = s.clone();
    std::thread::spawn(move || s2.run());
    (s, be, r)
}

#[test]
fn software_delivers_captured_keys() {
    let (s, be, r) = new_soft();
    *lock(&r.mask) = mask_of(&[0x04]);
    s.enable().unwrap();
    assert!(wait_until(Duration::from_secs(1), || *lock(&r.conn) == 1));
    let f = lock(&be.filter).clone().unwrap();
    assert!(f.handle(0x04, true));
    assert!(f.handle(0x04, false));
    assert!(wait_until(Duration::from_secs(1), || r.events().len() == 2));
    assert_eq!(r.events()[0], Event::new(b'D', 0x04, 0));
    s.close();
}

#[test]
fn software_follows_mask_and_keys_requests() {
    let (s, _be, r) = new_soft();
    s.enable().unwrap();
    *lock(&r.mask) = mask_of(&[0x09]);
    s.sync_mask(false);
    assert_eq!(s.filter().snapshot().mask, mask_of(&[0x09]));
    s.keys(true).unwrap();
    assert!(s.filter().snapshot().report_all);
    s.close();
}

#[test]
fn software_reports_hook_failure_and_retries() {
    let (s, be, _r) = new_soft();
    *lock(&be.start_err) = Some(CaptureError::new("permission", "nope"));
    assert!(s.enable().is_err());
    assert_eq!(s.error(), "permission");
    assert!(!s.connected());
    *lock(&be.start_err) = None;
    assert!(wait_until(Duration::from_secs(2), || s.connected()));
    assert_eq!(s.error(), "");
    s.close();
}

#[test]
fn software_disable_releases_and_notifies() {
    let (s, be, r) = new_soft();
    *lock(&r.mask) = mask_of(&[0x04]);
    s.enable().unwrap();
    let f = lock(&be.filter).clone().unwrap();
    assert!(f.handle(0x04, true));
    s.disable();
    assert!(wait_until(Duration::from_secs(1), || *lock(&r.disc) == 1));
    let kinds: Vec<u8> = r.events().iter().map(|e| e.kind).collect();
    assert_eq!(kinds, b"DU");
    assert!(!be.alive());
    s.close();
}

#[test]
fn software_injection_needs_the_hook() {
    let (s, be, _r) = new_soft();
    assert_eq!(s.tap(Stroke::new(0x04, 0)).unwrap_err(), software::NOT_RUNNING);
    s.enable().unwrap();
    s.tap(Stroke::new(0x04, 0)).unwrap();
    assert_eq!(lock(&be.injected).len(), 1);
    s.close();
}

#[test]
fn software_identity_and_stats() {
    let (s, _be, _r) = new_soft();
    s.enable().unwrap();
    let st = s.status();
    assert!(st.connected);
    let info = st.info.unwrap();
    assert!(info.system && info.present);
    assert_eq!(info.id(), SYSTEM_KEYBOARD_ID);
    assert_eq!(s.command("ping", 0).unwrap(), vec!["PONG"]);
    assert_eq!(s.command("stats", 0).unwrap(), vec!["n=3 avg_us=5 max_us=9 capture=1 dropped=0"]);
    assert!(s.command("reboot", 0).is_err());
    s.close();
    s.close();
}

#[test]
fn retries_back_off_while_the_hook_keeps_failing() {
    assert_eq!(software::next_retry(Duration::from_secs(4), Duration::from_secs(120)), Duration::from_secs(8));
    assert_eq!(software::next_retry(Duration::from_secs(100), Duration::from_secs(120)), Duration::from_secs(120));
    let (s, be, _r) = new_soft();
    *lock(&be.start_err) = Some(CaptureError::new("hook_failed", "x"));
    let _ = s.enable();
    std::thread::sleep(Duration::from_millis(400));
    let attempts = *lock(&be.attempts);
    assert!((3..=12).contains(&attempts), "{attempts}");
    s.close();
}

#[derive(Default)]
struct FakeBoard {
    h: Mutex<Handlers>,
    connected: Mutex<bool>,
    syncs: Mutex<u32>,
    keys: Mutex<Vec<bool>>,
    taps: Mutex<u32>,
}

impl Source for Arc<FakeBoard> {
    fn kind(&self) -> &'static str {
        BOARD
    }
    fn bind(&self, h: Handlers) {
        *lock(&self.h) = h;
    }
    fn run(&self) {}
    fn close(&self) {}
    fn status(&self) -> Status {
        Status {
            connected: *lock(&self.connected),
            port: "COM3".into(),
            info: Some(Info { present: true, vid: "2341".into(), pid: "8036".into(), ..Default::default() }),
            since: None,
        }
    }
    fn error(&self) -> String {
        String::new()
    }
    fn sync_mask(&self, _force: bool) {
        *lock(&self.syncs) += 1;
    }
    fn keys(&self, on: bool) -> Result<(), String> {
        lock(&self.keys).push(on);
        Ok(())
    }
    fn pause(&self) {}
    fn resume(&self) {}
    fn tap_held(&self, _s: Stroke, _hold_ms: u32) -> Result<(), String> {
        *lock(&self.taps) += 1;
        Ok(())
    }
    fn consumer(&self, _usage: u16) -> Result<(), String> {
        Ok(())
    }
    fn command(&self, _name: &str, _arg: i64) -> Result<Vec<String>, String> {
        Ok(vec!["OK".into()])
    }
    fn sys(&self) -> Result<Sys, String> {
        Ok(Sys { board: "leonardo".into(), ..Default::default() })
    }
}

type Rig = (Arc<Selector>, Arc<FakeBoard>, Arc<SoftwareSource>, Arc<FakeBackend>, Arc<Recorder>);

fn new_selector() -> Rig {
    let fb = Arc::new(FakeBoard::default());
    *lock(&fb.connected) = true;
    let be = Arc::new(FakeBackend::default());
    let soft = SoftwareSource::new(Box::new(be.clone()));
    let sel = Selector::new(Arc::new(fb.clone()), soft.clone());
    let r = Arc::new(Recorder::default());
    *lock(&r.mask) = mask_of(&[0x04]);
    sel.bind(r.handlers());
    let s2 = soft.clone();
    std::thread::spawn(move || s2.run());
    (sel, fb, soft, be, r)
}

#[test]
fn selector_starts_on_the_board() {
    let (sel, fb, soft, _be, r) = new_selector();
    assert_eq!((sel.active(), sel.kind()), (BOARD, BOARD));
    let h = lock(&fb.h).clone();
    (h.on_event.unwrap())(Event::new(b'D', 0x04, 0));
    assert_eq!(r.events().len(), 1);
    assert_eq!((h.mask.unwrap())(), mask_of(&[0x04]));
    soft.close();
}

#[test]
fn switching_to_software_silences_the_board() {
    let (sel, fb, soft, be, r) = new_selector();
    sel.select(SOFTWARE).unwrap();
    assert_eq!(sel.active(), SOFTWARE);
    assert!(be.alive());
    assert_eq!(*lock(&fb.keys), vec![false]);
    let h = lock(&fb.h).clone();
    (h.on_event.unwrap())(Event::new(b'D', 0x04, 0));
    assert_eq!((h.mask.unwrap())(), [0; 32]);
    assert!(r.events().is_empty());
    sel.tap(Stroke::new(0x04, 0)).unwrap();
    assert_eq!((lock(&be.injected).len(), *lock(&fb.taps)), (1, 0));
    assert_eq!(sel.sys().unwrap_err(), NEEDS_BOARD);
    assert!(sel.command("ping", 0).is_ok());
    soft.close();
}

#[test]
fn switching_back_restores_the_board_and_notifies() {
    let (sel, fb, soft, be, r) = new_selector();
    sel.select(SOFTWARE).unwrap();
    let before = *lock(&r.conn);
    sel.select(BOARD).unwrap();
    assert_eq!(sel.active(), BOARD);
    assert!(!be.alive());
    assert!(*lock(&r.conn) > before);
    assert!(*lock(&fb.syncs) >= 2);
    soft.close();
}

#[test]
fn a_failed_select_falls_back_to_the_board() {
    let (sel, _fb, soft, be, _r) = new_selector();
    *lock(&be.start_err) = Some(CaptureError::new("permission", "denied"));
    assert_eq!(sel.select(SOFTWARE).unwrap_err(), "denied");
    assert_eq!(sel.active(), BOARD);
    assert!(sel.select("telepathy").is_err());
    soft.close();
}

#[test]
fn concurrent_selects_leave_a_consistent_state() {
    let (sel, _fb, soft, be, _r) = new_selector();
    let hs: Vec<_> = (0..8)
        .map(|i| {
            let sel = sel.clone();
            std::thread::spawn(move || {
                for n in 0..20 {
                    let _ = sel.select(if (n + i) % 2 == 0 { SOFTWARE } else { BOARD });
                }
            })
        })
        .collect();
    for h in hs {
        h.join().unwrap();
    }
    assert_eq!(sel.active() == SOFTWARE, be.alive());
    soft.close();
}

#[test]
fn scancodes_round_trip() {
    let mut seen = std::collections::HashMap::new();
    for u in (0x04..=0x65u8).chain([0x67, 0x85, 0x87, 0x88, 0x89, 0x8A, 0x8B]).chain(0xE0..=0xE7).chain(0x68..=0x73) {
        let k = scan_from_usage(u).unwrap_or_else(|| panic!("{u:02X}"));
        if k.vk != 0 || u == 0x32 || u == 0x53 {
            continue;
        }
        assert_eq!(usage_from_scan(u32::from(k.code), k.ext, 0), u, "{u:02X}");
        assert!(seen.insert((k.code, k.ext), u).is_none(), "{u:02X}");
    }
    for (code, ext, want) in [
        (0x1C, false, 0x28),
        (0x1C, true, 0x58),
        (0x35, false, 0x38),
        (0x35, true, 0x54),
        (0x47, false, 0x5F),
        (0x47, true, 0x4A),
        (0x53, false, 0x63),
        (0x53, true, 0x4C),
        (0x1D, true, 0xE4),
        (0x38, true, 0xE6),
        (0x5B, true, 0xE3),
        (0x37, true, 0x46),
        (0x37, false, 0x55),
        (0x56, false, 0x64),
        (0x2B, false, 0x31),
    ] {
        assert_eq!(usage_from_scan(code, ext, 0), want, "{code:02X} {ext}");
    }
    assert_eq!(usage_from_scan(0x45, false, scancodes::VK_PAUSE), 0x48);
    assert_eq!(usage_from_scan(0x45, true, scancodes::VK_NUMLOCK), 0x53);
    assert_eq!(usage_from_scan(0x54, false, scancodes::VK_SNAPSHOT), 0x46);
    assert_eq!(usage_from_scan(0x46, true, 0x03), 0x48);
    for (code, ext) in [(0x21D, false), (0x22A, false), (0x236, false), (0x2A, true), (0x36, true)] {
        assert_eq!(usage_from_scan(code, ext, 0), 0);
    }
    assert_eq!(usage_from_scan(0x7F, false, 0), 0);
    for u in [0x31, 0x32] {
        let k = scan_from_usage(u).unwrap();
        assert!(k.code == 0x2B && !k.ext);
    }
}

#[test]
fn media_keys_match_on_windows_and_linux() {
    for u in 0..0x300u16 {
        assert_eq!(consumer_vk(u).is_some(), consumer_evdev(u).is_some(), "{u:#x}");
    }
    assert!(consumer_vk(0xCD).is_some());
}

#[test]
fn evdev_table() {
    let t = evdev_codes::evdev_table();
    let mut usages = std::collections::HashSet::new();
    let mut codes = std::collections::HashSet::new();
    for &(u, c) in &t.list {
        assert!(usages.insert(u) && codes.insert(c), "{u:02X} {c}");
        assert_eq!(evdev_from_usage(u), Some(c));
        assert_eq!(usage_from_evdev(c), u);
    }
    for u in (0x04..=0x65u8).chain([0x67, 0x85, 0x87, 0x88, 0x89, 0x8A, 0x8B]).chain(0xE0..=0xE7).chain(0x68..=0x73) {
        assert!(evdev_from_usage(u).is_some(), "{u:02X}");
    }
    for (code, u) in [
        (30, 0x04),
        (44, 0x1D),
        (2, 0x1E),
        (11, 0x27),
        (28, 0x28),
        (1, 0x29),
        (57, 0x2C),
        (43, 0x31),
        (86, 0x64),
        (183, 0x68),
        (194, 0x73),
        (99, 0x46),
        (119, 0x48),
        (96, 0x58),
        (125, 0xE3),
        (126, 0xE7),
    ] {
        assert_eq!(usage_from_evdev(code), u);
    }
    for code in [0, 113, 163, 224, 0x110] {
        assert_eq!(usage_from_evdev(code), 0);
    }
    assert_eq!(evdev_from_usage(0x32), Some(43));
    assert_eq!(consumer_evdev(0xCD), Some(164));
    assert_eq!(consumer_evdev(0xFFFF), None);
}

#[test]
fn info_json_matches_the_go_shape() {
    let i = Info { firmware: "15".into(), present: true, vid: "258A".into(), pid: "0016".into(), power_ma: 500, ..Default::default() };
    let v = serde_json::to_value(&i).unwrap();
    assert_eq!(v, serde_json::json!({"firmware": "15", "present": true, "vid": "258A", "pid": "0016", "power_ma": 500}));
    assert_eq!(i.id(), "258A:0016");
}
