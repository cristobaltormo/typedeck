use std::sync::Mutex;

use super::*;
use crate::capture::{CaptureError, Check, Filter, Latency, Selector, SoftwareSource};
use crate::config::{KeyDef, Keys};
use crate::sync::wait_until;
use crate::testutil::temp_dir;

#[derive(Default)]
struct Rec {
    fired: Mutex<Vec<String>>,
    huds: Mutex<Vec<String>>,
    injects: Mutex<Vec<u8>>,
}

impl Rec {
    fn fired(&self) -> Vec<String> {
        lock(&self.fired).clone()
    }
    fn huds(&self) -> Vec<String> {
        lock(&self.huds).clone()
    }
    fn injects(&self) -> Vec<u8> {
        lock(&self.injects).clone()
    }
}

fn new_test_with(src: Option<Arc<dyn Source>>, mutate: impl FnOnce(&mut Config)) -> (Arc<Engine>, Arc<Rec>) {
    let mut c = Config::initial();
    c.settings.hold_ms = 200;
    c.settings.double_ms = 120;
    c.layers[0].keys = Keys::new();
    mutate(&mut c);
    let p = Paths::new(temp_dir("engine"));
    config::save(&p, &c, false, false).unwrap();
    let has_src = src.is_some();
    let e = Engine::new(p, src).unwrap();
    let r = Arc::new(Rec::default());
    let (r1, r2, r3) = (r.clone(), r.clone(), r.clone());
    let me = Arc::downgrade(&e);
    e.set_test_hooks(
        Some(Box::new(move |a, _| {
            if a.kind == "layer" {
                if let Some(e) = me.upgrade() {
                    e.goto(a.to.as_ref().unwrap_or(&Value::Null));
                }
                return ActResult { ok: true, ..Default::default() };
            }
            lock(&r1.fired).push(format!("{}:{}{}", a.kind, a.text, a.cmd));
            ActResult { ok: true, ..Default::default() }
        })),
        Some(Box::new(move |t, _, _, _| lock(&r2.huds).push(t.to_string()))),
        if has_src { None } else { Some(Box::new(move |s: Stroke, _| lock(&r3.injects).push(s.usage))) },
    );
    (e, r)
}

fn new_test(mutate: impl FnOnce(&mut Config)) -> (Arc<Engine>, Arc<Rec>) {
    new_test_with(None, mutate)
}

fn act(text: &str) -> Option<Action> {
    Some(Action { text: text.into(), ..Action::new("hud") })
}

fn tap(text: &str) -> KeyDef {
    KeyDef { tap: act(text), ..Default::default() }
}

fn momentary(to: i64) -> Option<Action> {
    Some(Action { to: Some(Value::from(to)), momentary: true, ..Action::new("layer") })
}

fn settle() {
    std::thread::sleep(Duration::from_millis(60));
}

fn wait_for(mut done: impl FnMut() -> bool) {
    let t0 = Instant::now();
    while !done() && t0.elapsed() < Duration::from_secs(2) {
        ms(5);
    }
}

fn ms(n: u64) {
    std::thread::sleep(Duration::from_millis(n));
}

fn bit(m: &[u8; 32], u: u8) -> bool {
    m[usize::from(u >> 3)] & (1 << (u & 7)) != 0
}

#[test]
fn plain_tap_fires_on_down() {
    let (e, r) = new_test(|c| drop(c.layers[0].keys.insert("04".into(), tap("a"))));
    e.key_down(0x04, 0);
    settle();
    assert_eq!(r.fired(), vec!["hud:a"]);
}

#[test]
fn hold_versus_tap() {
    let (e, r) = new_test(|c| drop(c.layers[0].keys.insert("04".into(), KeyDef { tap: act("tap"), hold: act("hold"), ..Default::default() })));
    e.key_down(0x04, 0);
    ms(30);
    e.key_up(0x04);
    settle();
    assert_eq!(r.fired(), vec!["hud:tap"]);
    e.key_down(0x04, 0);
    ms(350);
    e.key_up(0x04);
    settle();
    assert_eq!(r.fired(), vec!["hud:tap", "hud:hold"]);
}

#[test]
fn double_versus_single() {
    let (e, r) = new_test(|c| drop(c.layers[0].keys.insert("05".into(), KeyDef { tap: act("tap"), double: act("double"), ..Default::default() })));
    e.key_down(0x05, 0);
    ms(20);
    e.key_up(0x05);
    ms(40);
    e.key_down(0x05, 0);
    ms(20);
    e.key_up(0x05);
    ms(300);
    assert_eq!(r.fired(), vec!["hud:double"]);
    e.key_down(0x05, 0);
    ms(20);
    e.key_up(0x05);
    ms(300);
    assert_eq!(r.fired(), vec!["hud:double", "hud:tap"]);
}

#[test]
fn global_fallback_and_layer_priority() {
    let (e, r) = new_test(|c| {
        c.layers[0].keys.insert("06".into(), tap("capa"));
        c.global.insert("06".into(), tap("global"));
        c.global.insert("07".into(), tap("global7"));
    });
    e.key_down(0x06, 0);
    e.key_down(0x07, 0);
    settle();
    let mut got = r.fired();
    got.sort();
    assert_eq!(got, vec!["hud:capa", "hud:global7"]);
}

#[test]
fn layer_cycle_key_and_mask() {
    let (e, _r) = new_test(|c| drop(c.layers[1].keys.insert("08".into(), tap("solo capa 2"))));
    let m = e.mask_bytes();
    assert!(bit(&m, 0x53) && !bit(&m, 0x08));
    e.key_down(0x53, 0);
    settle();
    assert_eq!(e.status().layer, 1);
    assert!(bit(&e.mask_bytes(), 0x08));
    e.key_up(0x53);
    e.key_down(0x53, 0);
    settle();
    assert_eq!(e.status().layer, 0);
}

#[test]
fn confirm_needs_two_presses() {
    let (e, r) = new_test(|c| {
        drop(c.layers[0].keys.insert(
            "09".into(),
            KeyDef { tap: Some(Action { text: "peligro".into(), confirm: true, ..Action::new("hud") }), label: "Borrar".into(), ..Default::default() },
        ))
    });
    e.key_down(0x09, 0);
    e.key_up(0x09);
    settle();
    assert!(r.fired().is_empty());
    e.key_down(0x09, 0);
    settle();
    assert_eq!(r.fired().len(), 1);
    assert_eq!(r.huds()[0], "Confirm");
}

#[test]
fn momentary_layer_on_hold() {
    let (e, r) = new_test(|c| {
        c.layers[0].keys.insert("39".into(), KeyDef { hold: momentary(1), ..Default::default() });
        c.layers[1].keys.insert("04".into(), tap("capa2-a"));
    });
    e.key_down(0x39, 0);
    ms(350);
    assert_eq!(e.status().layer, 1);
    e.key_down(0x04, 0);
    e.key_up(0x04);
    e.key_up(0x39);
    settle();
    assert_eq!(e.status().layer, 0);
    assert_eq!(r.fired(), vec!["hud:capa2-a"]);
}

#[test]
fn hold_only_key_reinjects_on_short_tap() {
    let (e, r) = new_test(|c| drop(c.layers[0].keys.insert("39".into(), KeyDef { hold: act("mantener"), ..Default::default() })));
    e.key_down(0x39, 0);
    ms(30);
    e.key_up(0x39);
    settle();
    assert_eq!(r.injects(), vec![0x39]);
    assert!(r.fired().is_empty());
}

#[test]
fn unmapped_key_is_ignored() {
    let (e, r) = new_test(|_| {});
    e.key_down(0x1A, 0);
    e.key_up(0x1A);
    settle();
    assert!(r.fired().is_empty());
}

#[test]
fn events_long_poll_and_stats() {
    let (e, _r) = new_test(|c| drop(c.layers[0].keys.insert("0A".into(), tap("x"))));
    let e2 = e.clone();
    let h = std::thread::spawn(move || e2.events_since(0, Duration::from_secs(2), &|| false).0);
    ms(50);
    let t0 = Instant::now();
    e.key_down(0x0A, 0);
    let evs = h.join().unwrap();
    assert!(t0.elapsed() < Duration::from_secs(1));
    assert_eq!(evs[0].kind, "down");
    settle();
    let s = e.stats();
    assert_eq!((s.total, s.keys.get("0A").copied()), (1, Some(1)));
    e.reset_stats();
    assert_eq!(e.stats().total, 0);
    let (none, _) = e.events_since(1000, Duration::from_millis(50), &|| false);
    assert!(none.is_empty());
}

#[test]
fn apply_config_clamps_layer() {
    let (e, _r) = new_test(|_| {});
    e.goto(&Value::from(1));
    let mut c = e.config();
    c.layers.truncate(1);
    e.set_config(c).unwrap();
    assert_eq!(e.status().layer, 0);
}

#[test]
fn bounce_is_ignored() {
    let (e, r) = new_test(|c| drop(c.layers[0].keys.insert("04".into(), tap("a"))));
    e.key_down(0x04, 0);
    e.key_up(0x04);
    e.key_down(0x04, 0);
    settle();
    assert_eq!(r.fired().len(), 1);
    ms(80);
    e.key_down(0x04, 0);
    settle();
    assert_eq!(r.fired().len(), 2);
}

#[test]
fn editing_releases_the_mask_until_it_ends() {
    let (e, _r) = new_test(|c| drop(c.layers[0].keys.insert("04".into(), tap("a"))));
    assert!(bit(&e.mask_bytes(), 0x04) && !e.editing());
    e.set_editing(true);
    assert!(e.mask_bytes() == [0; 32] && e.editing());
    e.set_editing(false);
    assert!(bit(&e.mask_bytes(), 0x04) && !e.editing());
}

#[test]
fn keyboard_lost_warns_once_and_announces_the_return() {
    let (e, r) = new_test(|_| {});
    e.set_kbd_lost_after(Duration::from_millis(40));
    e.keyboard_changed(false);
    e.keyboard_changed(true);
    ms(120);
    assert!(r.huds().is_empty());
    e.keyboard_changed(false);
    wait_for(|| !r.huds().is_empty());
    assert_eq!(r.huds().len(), 1);
    e.keyboard_changed(true);
    wait_for(|| r.huds().len() >= 2);
    assert_eq!(r.huds().len(), 2);
    lock(&e.st).cfg.settings.notify_keyboard = false;
    e.keyboard_changed(false);
    ms(120);
    assert_eq!(r.huds().len(), 2);
}

#[test]
fn plain_keys_are_reported_and_recorded_only_when_history_is_on() {
    let (e, _r) = new_test(|c| c.settings.key_history = true);
    let seen = Arc::new(Mutex::new(Vec::new()));
    let s2 = seen.clone();
    e.set_plain_key(Box::new(move |u, down| lock(&s2).push(format!("{}{}", hex_key(u), if down { "v" } else { "^" }))));
    e.handle_event(CaptureEvent::new(b'P', 0x04, 0));
    ms(15);
    e.handle_event(CaptureEvent::new(b'R', 0x04, 0));
    assert_eq!(*lock(&seen), vec!["04v", "04^"]);
    let (got, n) = e.keylog().recent(5);
    assert_eq!((n, got[0].k.as_str()), (1, "A"));
    assert!(got[0].d >= 10);
    let mut c = e.config();
    c.settings.key_history = false;
    e.apply_restored(c);
    e.handle_event(CaptureEvent::new(b'P', 0x05, 0));
    e.handle_event(CaptureEvent::new(b'R', 0x05, 0));
    assert_eq!(e.keylog().recent(5).1, 1);
}

#[test]
fn history_can_be_toggled_by_an_action() {
    let (e, r) = new_test(|_| {});
    assert!(!e.config().settings.key_history);
    assert!(e.set_key_history("toggle") && e.config().settings.key_history && e.keylog().enabled());
    assert!(e.set_key_history("on"));
    assert!(!e.set_key_history("off"));
    assert!(!e.keylog().enabled());
    assert_eq!(r.huds().len(), 3);
}

#[test]
fn learning_captures_every_key_and_runs_nothing() {
    let (e, r) = new_test(|c| drop(c.layers[0].keys.insert("04".into(), tap("a"))));
    e.set_learning_for_test(true);
    let m = e.mask_bytes();
    for u in [0x04, 0x1E, 0x31, 0x32, 0x64, 0x4F] {
        assert!(bit(&m, u), "{u:02X}");
    }
    e.key_down(0x04, 0);
    e.key_down(0x31, 0);
    e.key_up(0x04);
    settle();
    assert!(r.fired().is_empty());
    let seen = e.seen();
    assert_eq!((seen["04"], seen["31"]), (1, 1));
    e.set_learning_for_test(false);
    assert!(bit(&e.mask_bytes(), 0x04) && !bit(&e.mask_bytes(), 0x31));
}

#[test]
fn momentary_layer_activates_at_once_when_another_key_is_pressed() {
    let (e, r) = new_test(|c| {
        c.settings.hold_ms = 1500;
        c.layers[0].keys.insert("39".into(), KeyDef { hold: momentary(1), ..Default::default() });
        c.layers[1].keys.insert("1A".into(), tap("whatsapp"));
    });
    assert!(bit(&e.mask_bytes(), 0x1A));
    e.key_down(0x39, 0);
    e.key_down(0x1A, 0);
    e.key_up(0x1A);
    settle();
    assert_eq!(r.fired(), vec!["hud:whatsapp"]);
    assert!(r.injects().is_empty());
    assert_eq!(e.status().layer, 1);
    e.key_up(0x39);
    settle();
    assert_eq!(e.status().layer, 0);
    e.key_down(0x39, 0);
    e.key_up(0x39);
    settle();
    assert_eq!(r.injects(), vec![0x39]);
    assert!(r.huds().is_empty());
}

#[test]
fn layer_key_is_a_modifier_from_the_press_on_tap() {
    let (e, r) = new_test(|c| {
        c.settings.hold_ms = 1500;
        c.layers[0].keys.insert("39".into(), KeyDef { tap: momentary(1), ..Default::default() });
        c.layers[1].keys.insert("1A".into(), tap("w"));
    });
    e.key_down(0x39, 0);
    assert_eq!(e.status().layer, 1);
    e.key_down(0x39, 0);
    e.key_down(0x1A, 0);
    e.key_up(0x1A);
    e.key_up(0x39);
    settle();
    assert_eq!(e.status().layer, 0);
    assert_eq!(r.fired(), vec!["hud:w"]);
}

#[test]
fn short_press_of_a_layer_key_with_a_tap_runs_the_tap() {
    let (e, r) = new_test(|c| {
        c.settings.hold_ms = 400;
        c.layers[0].keys.insert("39".into(), KeyDef { tap: act("tap"), hold: momentary(1), ..Default::default() });
        c.layers[1].keys.insert("1A".into(), tap("w"));
    });
    e.key_down(0x39, 0);
    e.key_up(0x39);
    settle();
    assert_eq!(r.fired(), vec!["hud:tap"]);
    e.key_down(0x39, 0);
    e.key_down(0x1A, 0);
    e.key_up(0x1A);
    e.key_up(0x39);
    settle();
    assert_eq!(r.fired(), vec!["hud:tap", "hud:w"]);
}

#[test]
fn captured_key_without_action_in_the_held_layer_passes_through() {
    let (e, r) = new_test(|c| {
        c.settings.hold_ms = 1500;
        c.layers[0].keys.insert("39".into(), KeyDef { hold: momentary(1), ..Default::default() });
        c.layers[1].keys.insert("1A".into(), tap("x"));
    });
    e.key_down(0x39, 0);
    e.key_down(0x05, 0);
    settle();
    assert_eq!(r.injects(), vec![0x05]);
}

#[test]
fn toggle_layer_goes_back_on_second_use() {
    let (e, _r) = new_test(|_| {});
    e.toggle_layer(&Value::from(1));
    assert_eq!(e.status().layer, 1);
    e.toggle_layer(&Value::from(1));
    assert_eq!(e.status().layer, 0);
}

#[derive(Default)]
struct Stub {
    start_err: Mutex<Option<CaptureError>>,
    alive: Mutex<bool>,
    injected: Mutex<Vec<Stroke>>,
}

impl crate::capture::Backend for Arc<Stub> {
    fn start(&self, _f: &Arc<Filter>) -> Result<(), CaptureError> {
        if let Some(e) = lock(&self.start_err).clone() {
            return Err(e);
        }
        *lock(&self.alive) = true;
        Ok(())
    }
    fn alive(&self) -> bool {
        *lock(&self.alive)
    }
    fn stop(&self) {
        *lock(&self.alive) = false;
    }
    fn inject(&self, s: Stroke, _ms: u32) -> Result<(), String> {
        lock(&self.injected).push(s);
        Ok(())
    }
    fn consumer(&self, _u: u16) -> Result<(), String> {
        Ok(())
    }
    fn type_text(&self, _t: &str) -> Result<(), String> {
        Ok(())
    }
    fn latency(&self) -> Latency {
        Latency::default()
    }
    fn checks(&self) -> Vec<Check> {
        vec![]
    }
}

struct NoBoard;

impl crate::board::Connector for NoBoard {
    fn open(&self) -> Result<(Arc<dyn crate::board::Port>, String), crate::board::OpenError> {
        Err(crate::board::OpenError::NotFound)
    }
}

fn soft_test(mutate: impl FnOnce(&mut Config)) -> (Arc<Engine>, Arc<Rec>, Arc<SoftwareSource>, Arc<Stub>) {
    let stub = Arc::new(Stub::default());
    let soft = SoftwareSource::new(Box::new(stub.clone()));
    let (e, r) = new_test_with(Some(soft.clone()), mutate);
    let s2 = soft.clone();
    std::thread::spawn(move || s2.run());
    soft.enable().unwrap();
    (e, r, soft, stub)
}

#[test]
fn software_tap_swallows_the_key_and_runs_the_action() {
    let (_e, r, soft, _stub) = soft_test(|c| drop(c.layers[0].keys.insert("04".into(), tap("a"))));
    let f = soft.filter();
    assert!(f.handle(0x04, true) && f.handle(0x04, false));
    assert!(!f.handle(0x05, true) && !f.handle(0x05, false));
    assert!(wait_until(Duration::from_secs(1), || r.fired() == vec!["hud:a"]));
    soft.close();
}

#[test]
fn software_hold_only_key_is_typed_back_on_a_short_press() {
    let (_e, _r, soft, stub) = soft_test(|c| drop(c.layers[0].keys.insert("04".into(), KeyDef { hold: act("h"), ..Default::default() })));
    let f = soft.filter();
    assert!(f.handle(0x04, true));
    ms(20);
    assert!(f.handle(0x04, false));
    assert!(wait_until(Duration::from_secs(1), || lock(&stub.injected).iter().any(|s| s.usage == 0x04)));
    soft.close();
}

#[test]
fn software_momentary_layer_captures_keys_of_the_layer() {
    let (_e, r, soft, _stub) = soft_test(|c| {
        c.settings.hold_ms = 1500;
        c.layers[0].keys.insert("39".into(), KeyDef { hold: momentary(1), ..Default::default() });
        c.layers[1].keys.insert("1A".into(), tap("w"));
    });
    let f = soft.filter();
    assert!(f.handle(0x39, true));
    assert!(f.handle(0x1A, true));
    assert!(f.handle(0x1A, false));
    assert!(f.handle(0x39, false));
    assert!(wait_until(Duration::from_secs(1), || r.fired() == vec!["hud:w"]));
    soft.close();
}

#[test]
fn software_reports_every_key_only_for_history_or_learning() {
    let (e, _r, soft, _stub) = soft_test(|_| {});
    e.set_editor_open(true);
    ms(50);
    assert!(!soft.filter().snapshot().report_all);
    let mut c = e.config();
    c.settings.key_history = true;
    e.set_config(c).unwrap();
    assert!(wait_until(Duration::from_secs(1), || soft.filter().snapshot().report_all));
    soft.close();
}

fn selector_test() -> (Arc<Engine>, Arc<Selector>, Arc<Stub>) {
    let stub = Arc::new(Stub::default());
    let soft = SoftwareSource::new(Box::new(stub.clone()));
    let board = crate::board::Board::new(Box::new(NoBoard));
    let sel = Selector::new(board, soft);
    let (e, _r) = new_test_with(Some(sel.clone()), |_| {});
    (e, sel, stub)
}

#[test]
fn set_capture_needs_consent_records_it_and_persists() {
    let (e, sel, stub) = selector_test();
    assert!(e.set_capture(SOFTWARE, false).is_err());
    assert!(!config::has_capture_consent(e.paths()));
    e.set_capture(SOFTWARE, true).unwrap();
    assert!(config::has_capture_consent(e.paths()));
    assert_eq!(sel.active(), SOFTWARE);
    assert!(*lock(&stub.alive));
    assert_eq!(config::load(e.paths()).unwrap().settings.capture, SOFTWARE);
    e.set_capture(BOARD, false).unwrap();
    assert_eq!(e.capture_mode(), BOARD);
    sel.close();
}

#[test]
fn a_failing_set_capture_changes_nothing() {
    let (e, sel, stub) = selector_test();
    *lock(&stub.start_err) = Some(CaptureError::new("permission", "no"));
    assert!(e.set_capture(SOFTWARE, true).is_err());
    assert_eq!(sel.active(), BOARD);
    assert_eq!(e.config().settings.capture, BOARD);
    sel.close();
}

#[test]
fn config_without_consent_cannot_turn_on_software_capture() {
    let (e, sel, _stub) = selector_test();
    let mut c = e.config();
    c.settings.capture = SOFTWARE.into();
    let got = e.set_config(c.clone()).unwrap();
    assert_eq!(got.settings.capture, BOARD);
    assert_eq!(e.apply_restored(c).settings.capture, BOARD);
    config::record_capture_consent(e.paths()).unwrap();
    let mut c = e.config();
    c.settings.capture = SOFTWARE.into();
    assert_eq!(e.apply_restored(c).settings.capture, SOFTWARE);
    assert_eq!(sel.active(), SOFTWARE);
    sel.close();
}

#[test]
fn startup_with_software_capture_falls_back_without_consent() {
    let p = Paths::new(temp_dir("startup"));
    let mut c = Config::initial();
    c.settings.capture = SOFTWARE.into();
    config::save(&p, &c, false, false).unwrap();
    let e = Engine::new(p.clone(), None).unwrap();
    assert_eq!(e.config().settings.capture, BOARD);
    assert_eq!(config::load(&p).unwrap().settings.capture, BOARD);
    config::record_capture_consent(&p).unwrap();
    let mut c = Config::initial();
    c.settings.capture = SOFTWARE.into();
    config::save(&p, &c, false, false).unwrap();
    let e = Engine::new(p, None).unwrap();
    assert_eq!(e.config().settings.capture, SOFTWARE);
}

#[test]
fn board_loss_while_a_layer_key_is_held_goes_back_to_the_previous_layer() {
    let (e, r) = new_test(|c| {
        c.layers[0].keys.insert("39".into(), KeyDef { tap: momentary(1), ..Default::default() });
    });
    e.key_down(0x39, 0);
    assert_eq!(e.status().layer, 1);
    e.on_board_disconnected();
    assert_eq!(e.status().layer, 0);
    e.key_up(0x39);
    settle();
    assert_eq!(e.status().layer, 0);
    assert!(r.fired().is_empty());
}

#[test]
fn concurrent_setting_changes_are_not_lost() {
    let (e, _r) = new_test(|_| {});
    let workers: Vec<_> = (0..4)
        .map(|i| {
            let e = e.clone();
            std::thread::spawn(move || {
                for _ in 0..5 {
                    e.update_config(|c| c.settings.hold_ms += 1).unwrap();
                    if i == 0 {
                        e.set_key_history("toggle");
                    }
                }
            })
        })
        .collect();
    for w in workers {
        w.join().unwrap();
    }
    assert_eq!(e.config().settings.hold_ms, 200 + 20);
}
