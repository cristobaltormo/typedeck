use std::sync::Arc;
use std::time::Duration;

use super::keyboard::{KeyState, drop_scan};
use super::session::{LatencyStats, Session};
use super::sys::*;
use super::*;
use crate::capture::{Event, Filter, Msg};

#[test]
fn ioctl_numbers() {
    for (name, got, want) in [
        ("EVIOCGRAB", EVIOCGRAB, 0x40044590u64),
        ("EVIOCGBIT(EV_KEY, 96)", eviocgbit(u64::from(EV_KEY), 96), 0x80604521),
        ("EVIOCGNAME(256)", eviocgname(256), 0x81004506),
        ("EVIOCGPHYS(256)", eviocgphys(256), 0x81004507),
        ("EVIOCGKEY(96)", eviocgkey(96), 0x80604518),
        ("UI_DEV_CREATE", UI_DEV_CREATE, 0x5501),
        ("UI_DEV_DESTROY", UI_DEV_DESTROY, 0x5502),
        ("UI_DEV_SETUP", UI_DEV_SETUP, 0x405C5503),
        ("UI_SET_EVBIT", UI_SET_EVBIT, 0x40045564),
        ("UI_SET_KEYBIT", UI_SET_KEYBIT, 0x40045565),
        ("UI_SET_LEDBIT", UI_SET_LEDBIT, 0x40045569),
    ] {
        assert_eq!(got, want, "{name}");
    }
}

#[test]
fn struct_sizes() {
    assert_eq!(std::mem::size_of::<UinputSetup>() as u64, SETUP_SIZE);
    const { assert!(EVENT_SIZE == 24 || EVENT_SIZE == 16) };
}

fn filter_with(mask: &[u8]) -> Arc<Filter> {
    let f = Arc::new(Filter::new());
    let mut m = [0u8; 32];
    for u in mask {
        m[usize::from(u >> 3)] |= 1 << (u & 7);
    }
    f.set_mask(m);
    f
}

fn key(code: u16, value: i32) -> InputEvent {
    InputEvent::key(code, value)
}

#[test]
fn consume_swallows_press_repeat_and_release() {
    let f = filter_with(&[0x04]);
    let mut k = KeyState::default();
    for ev in [key(30, 1), key(30, 2), key(30, 2), key(30, 0)] {
        assert!(k.consume(&f, &ev));
    }
    assert!(k.swallowed.is_empty() && k.forwarded.is_empty());
}

#[test]
fn consume_forwards_other_keys_untouched() {
    let f = filter_with(&[0x04]);
    let mut k = KeyState::default();
    for ev in [key(18, 1), key(18, 2), key(18, 0), key(113, 1), key(113, 0)] {
        assert!(!k.consume(&f, &ev));
    }
}

#[test]
fn consume_keeps_a_pressed_key_consistent_when_the_mask_changes() {
    let f = filter_with(&[]);
    let mut k = KeyState::default();
    assert!(!k.consume(&f, &key(30, 1)));
    let mut m = [0u8; 32];
    m[0] = 0x10;
    f.set_mask(m);
    assert!(!k.consume(&f, &key(30, 2)));
    assert!(!k.consume(&f, &key(30, 0)));
}

#[test]
fn a_paused_filter_still_swallows_the_release_of_a_key_it_took() {
    let f = filter_with(&[0x16]);
    let mut k = KeyState::default();
    assert!(k.consume(&f, &key(31, 1)));
    f.pause();
    assert!(k.consume(&f, &key(31, 2)));
    assert!(k.consume(&f, &key(31, 0)));
}

#[test]
fn modifiers_are_forwarded_and_tracked() {
    let f = filter_with(&[0x04]);
    let mut k = KeyState::default();
    assert!(!k.consume(&f, &key(29, 1)));
    assert!(k.consume(&f, &key(30, 1)));
    assert_eq!(f.queue().pop(Duration::from_millis(100)), Some(Msg::Event(Event::new(b'D', 0x04, 0x01))));
}

#[test]
fn drop_scan_removes_the_scancode_of_a_swallowed_key() {
    let mut out = vec![InputEvent::new(EV_MSC, MSC_SCAN, 30)];
    drop_scan(&mut out);
    assert!(out.is_empty());
    let mut keep = vec![InputEvent::new(EV_LED, 1, 0)];
    drop_scan(&mut keep);
    assert_eq!(keep.len(), 1);
}

#[test]
fn stroke_codes_follow_the_modifiers() {
    assert_eq!(stroke_codes(Stroke::new(0x09, MOD_CTRL | MOD_SHIFT)).unwrap(), vec![29, 42, 33]);
    assert!(stroke_codes(Stroke::new(0xFE, 0)).is_err());
}

#[test]
fn no_uinput_device_is_created_without_read_access() {
    fn probe() -> (usize, usize) {
        (0, 2)
    }
    fn make(_: super::uinput::LedFn) -> std::io::Result<super::uinput::VirtualKeyboard> {
        panic!("must not be called");
    }
    let err = Session::start_with(Arc::new(Filter::new()), Arc::new(LatencyStats::default()), probe, make).err().unwrap();
    assert_eq!(err.code, "input_access");
}

#[test]
fn virtual_keyboard_declares_keys_above_255_but_no_buttons() {
    let declared = super::uinput::virtual_keys();
    for want in [1u16, 30, 255, 0x160, 0x1d0, 0x2bf] {
        assert!(declared.contains(&want), "{want:#x}");
    }
    for banned in [0u16, 0x100, 0x110, 0x120, 0x130, 0x15f, 0x2c0, 0x2e7] {
        assert!(!declared.contains(&banned), "{banned:#x}");
    }
}

#[test]
fn grab_failures_are_reported() {
    assert!(grab_check(&BTreeMap::new()).ok);
    let mut m = BTreeMap::new();
    m.insert("/dev/input/event3".to_string(), "busy".to_string());
    let c = grab_check(&m);
    assert!(!c.ok && c.code == "grab_failed" && c.problem.contains("event3"));
    let s = Session::for_test(Arc::new(Filter::new()));
    s.grab_failed("/dev/input/event3", &std::io::Error::from_raw_os_error(libc::EBUSY));
    let b = LinuxBackend::new();
    *lock(&b.sess) = Some(s.clone());
    assert_eq!(b.warnings(), vec!["grab_failed".to_string()]);
    s.grabbed("/dev/input/event3");
    assert!(b.warnings().is_empty());
}

#[test]
fn latency_statistics() {
    let l = LatencyStats::default();
    l.record(Duration::from_micros(100));
    l.record(Duration::from_micros(300));
    let s = l.snapshot();
    assert_eq!((s.count, s.avg_us, s.max_us), (2, 200, 300));
}
