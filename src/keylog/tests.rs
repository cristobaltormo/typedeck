use std::time::{Duration, SystemTime};

use super::*;
use crate::hid::layout_by_name;
use crate::testutil::temp_dir;

fn new_log(tag: &str) -> (Arc<Log>, PathBuf) {
    let p = temp_dir(tag).join("keystrokes.jsonl");
    (Log::new(p.clone()), p)
}

fn ms(n: u64) -> Duration {
    Duration::from_millis(n)
}

#[test]
fn records_key_and_duration_only_when_enabled() {
    let (l, p) = new_log("kl1");
    let t0 = SystemTime::now();
    l.press(0x04, t0);
    l.release(0x04, t0 + ms(80));
    let (got, n) = l.recent(10);
    assert_eq!((n, got.len()), (0, 0));
    assert!(!p.exists());
    l.set_enabled(true);
    l.press(0x04, t0);
    l.press(0x04, t0 + ms(30));
    l.release(0x04, t0 + ms(120));
    l.press(0xE1, t0 + ms(200));
    l.release(0xE1, t0 + ms(260));
    let (got, n) = l.recent(10);
    assert_eq!(n, 2);
    assert_eq!(got[0].k, "shift");
    assert_eq!((got[1].k.as_str(), got[1].d, got[1].t), ("A", 120, clock::unix_millis(t0)));
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        assert_eq!(fs::metadata(&p).unwrap().permissions().mode() & 0o777, 0o600);
    }
}

#[test]
fn clear_deletes_everything_and_trim_keeps_the_newest() {
    let (l, p) = new_log("kl2");
    l.set_enabled(true);
    let now = SystemTime::now();
    for _ in 0..5 {
        l.press(0x05, now);
        l.release(0x05, now + ms(1));
    }
    assert_eq!(l.recent(100).1, 5);
    l.clear().unwrap();
    assert!(!p.exists());
    assert_eq!(l.recent(100).1, 0);
    let line = format!("{}\n", "x".repeat(99));
    fs::write(&p, format!("{}{}\n", line.repeat(100), r#"{"t":1,"u":4,"k":"A","d":9}"#)).unwrap();
    l.trim();
    let data = fs::read_to_string(&p).unwrap();
    assert!(data.len() < 100 * 100 + 30);
    assert!(data.ends_with("\"d\":9}\n"));
}

#[test]
fn names() {
    for (u, want) in [(0x04, "A"), (0x1E, "1"), (0x27, "0"), (0x28, "return"), (0xE3, "cmd"), (0xFE, "0xFE")] {
        assert_eq!(name(u), want);
    }
}

#[test]
fn summary_counts_today_top_keys_and_average() {
    let (l, p) = new_log("kl3");
    let now = SystemTime::now();
    let midnight = clock::from_unix_millis(clock::local_midnight_millis(now));
    let base = if now.duration_since(midnight).unwrap_or_default() > Duration::from_secs(7200) { now } else { midnight + Duration::from_secs(7200) };
    let entries = [
        (0x04u8, Duration::from_secs(48 * 3600), 100i64),
        (0x04, Duration::from_secs(3600), 80),
        (0x04, Duration::from_secs(1800), 120),
        (0x05, Duration::from_secs(600), 60),
    ];
    let mut text = String::new();
    for (u, ago, d) in entries {
        let t = clock::unix_millis(base - ago);
        text.push_str(&format!("{{\"t\":{t},\"u\":{u},\"k\":\"{}\",\"d\":{d}}}\n", name(u)));
    }
    fs::write(&p, text).unwrap();
    let sum = l.summary(base);
    assert_eq!((sum.total, sum.today, sum.avg_ms), (4, 3, 90));
    let top = sum.top.unwrap();
    assert_eq!((top.len(), top[0].k.as_str(), top[0].n), (2, "A", 3));
    let empty = Log::new(temp_dir("kl4").join("x")).summary(now);
    assert_eq!(empty.total, 0);
    assert!(empty.top.is_none());
}

#[test]
fn transcribe_rebuilds_what_was_typed() {
    let us = layout_by_name("us");
    let at = 1_000_000i64;
    let e = |u: u8, dt: i64, d: i64| Entry { t: at + dt, u, k: name(u), d };
    let es1 = vec![
        e(0xE1, 0, 400),
        e(0x0B, 100, 50),
        e(0x08, 600, 50),
        e(0x0F, 700, 50),
        e(0x0F, 800, 50),
        e(0x12, 900, 50),
        e(0x2C, 1000, 50),
        e(0x1A, 1100, 50),
        e(0x12, 1200, 50),
        e(0x15, 1300, 50),
        e(0x2A, 1400, 50),
        e(0x0F, 1500, 50),
        e(0x28, 1600, 50),
    ];
    assert_eq!(transcribe(&es1, Some(us)), "Hello wol\n");
    let caps = vec![e(0x39, 0, 30), e(0x04, 100, 40), e(0x39, 200, 30), e(0x04, 300, 40)];
    assert_eq!(transcribe(&caps, Some(us)), "Aa");
    let combo = vec![e(0xE3, 0, 300), e(0x06, 50, 40), e(0x06, 500, 40)];
    assert_eq!(transcribe(&combo, Some(us)), "[Cmd+C]c");
    let altgr = vec![e(0xE6, 0, 300), e(0x1F, 50, 40)];
    assert_eq!(transcribe(&altgr, Some(layout_by_name("es-pc"))), "@");
}

#[test]
fn prune_drops_entries_older_than_the_retention() {
    let (l, p) = new_log("kl5");
    let now = SystemTime::now();
    let line = |at: SystemTime, k: &str| format!("{{\"t\":{},\"u\":4,\"k\":\"{k}\",\"d\":50}}\n", clock::unix_millis(at));
    fs::write(&p, line(now - Duration::from_secs(10 * 86400), "OLD") + &line(now - Duration::from_secs(3600), "NEW")).unwrap();
    l.set_retention(Duration::from_secs(7 * 86400));
    let (got, n) = l.recent(10);
    assert_eq!((n, got[0].k.as_str()), (1, "NEW"));
    l.set_retention(Duration::from_secs(60));
    assert_eq!(l.recent(10).1, 0);
    assert!(!p.exists());
}

#[test]
fn buffered_entries_are_flushed_by_the_timer() {
    let (l, p) = new_log("kl6");
    l.set_enabled(true);
    let now = SystemTime::now();
    l.press(0x06, now);
    l.release(0x06, now + ms(5));
    assert!(!p.exists());
    std::thread::sleep(ms(1800));
    assert!(fs::read_to_string(&p).unwrap().contains("\"k\":\"C\""));
}
