use super::*;

struct FakeEnv {
    layer: usize,
}

impl Env for FakeEnv {
    fn settings(&self) -> Settings {
        Settings::default()
    }
    fn hud(&self, _t: &str, _s: &str, _f: bool) {}
    fn goto(&self, _t: &Value) {}
    fn toggle_layer(&self, _t: &Value) {}
    fn toggle_caffeinate(&self) -> bool {
        false
    }
    fn start_timer(&self, _m: f64, _l: &str) {}
    fn hardware(&self) -> Option<Arc<dyn Source>> {
        None
    }
    fn layer(&self) -> usize {
        self.layer
    }
    fn layer_name(&self) -> String {
        ["Uno", "Dos"][self.layer].into()
    }
    fn set_key_history(&self, mode: &str) -> bool {
        mode == "on"
    }
}

fn hud(s: &str) -> Action {
    Action { text: s.into(), ..Action::new("hud") }
}

#[test]
fn substitute_and_describe() {
    assert_eq!(substitute("sin llaves", false), "sin llaves");
    assert_eq!(substitute("{date}", false).len(), 10);
    assert_eq!(substitute("{datetime}", false).len(), 16);
    assert_eq!(substitute("{time}", false).len(), 5);
    let want = if cfg!(windows) { r#"^"a'b^""# } else { r"'a'\''b'" };
    assert_eq!(platform::get().quote_arg("a'b"), want);
    assert_eq!(describe(&Action { minutes: 2.5, ..Action::new("timer") }), "2.5 min");
    assert_eq!(describe(&Action { minutes: 5.0, ..Action::new("timer") }), "5 min");
    assert_eq!(describe(&Action { url: "https://example.com/a".into(), ..Action::new("url") }), "example.com/a");
    assert_eq!(describe(&Action { steps: vec![hud("a")], ..Action::new("sequence") }), "1 steps");
}

#[test]
fn substitution_is_a_single_pass_and_reads_the_clipboard_once() {
    let calls = std::cell::Cell::new(0);
    let clip = || {
        calls.set(calls.get() + 1);
        "{portapapeles} {date}".to_string()
    };
    assert_eq!(substitute_with("[{clipboard}|{portapapeles}] {x} {", false, clip), "[{portapapeles} {date}|{portapapeles} {date}] {x} {");
    assert_eq!(calls.get(), 1);
    assert_eq!(substitute_with("{date}", false, || unreachable!()).len(), 10);
    assert_eq!(substitute_with("{{time}}", false, || unreachable!()).len(), 7);
}

#[test]
fn time_ranges() {
    for (spec, h, m, want) in [
        ("09:00-18:00", 12, 0, true),
        ("09:00-18:00", 18, 0, false),
        ("22:00-06:00", 23, 30, true),
        ("22:00-06:00", 5, 59, true),
        ("22:00-06:00", 12, 0, false),
        ("basura", 12, 0, false),
        ("9:00-18:00", 12, 0, true),
        ("9:00-18:00", 8, 59, false),
        (" 7:05 - 9:30 ", 7, 5, true),
        ("09:5-18:00", 12, 0, false),
        ("009:00-18:00", 12, 0, false),
        ("+9:00-18:00", 12, 0, false),
        ("24:00-06:00", 1, 0, false),
    ] {
        assert_eq!(in_time_range(spec, h * 60 + m), want, "{spec} {h}:{m}");
    }
}

#[test]
fn conditional_runs_the_right_branch() {
    let a = Action {
        cond: "layer".into(),
        target: "2".into(),
        steps: vec![hud("then")],
        otherwise: vec![Action { ms: 1, ..Action::new("wait") }, Action::new("app")],
        ..Action::new("if")
    };
    assert!(execute(&a, &FakeEnv { layer: 1 }, false).ok);
    assert!(!execute(&a, &FakeEnv { layer: 0 }, false).ok);
    let by_name = Action { cond: "layer".into(), target: "dos".into(), ..Action::new("if") };
    assert!(condition(&by_name, &FakeEnv { layer: 1 }).unwrap());
    let not = Action { cond: "os".into(), target: platform::get().name().into(), not: true, ..Action::new("if") };
    assert!(!condition(&not, &FakeEnv { layer: 0 }).unwrap());
}

#[test]
fn history_action_reports_the_new_state() {
    let r = execute(&Action { cmd: "on".into(), ..Action::new("history") }, &FakeEnv { layer: 0 }, false);
    assert!(r.ok && r.output == "on");
    let r = execute(&Action { cmd: "off".into(), ..Action::new("history") }, &FakeEnv { layer: 0 }, false);
    assert!(r.ok && r.output == "off");
    let r = execute(&Action::new("nope"), &FakeEnv { layer: 0 }, false);
    assert!(!r.ok);
}

#[cfg(unix)]
#[test]
fn shell_output_is_captured_and_long_output_is_cut_at_a_character() {
    let a = Action { cmd: "printf 'ñ%.0s' $(seq 1 400)".into(), ..Action::new("shell") };
    let r = execute(&a, &FakeEnv { layer: 0 }, true);
    assert!(r.ok);
    assert!(r.output.len() <= 600 && r.output.chars().all(|c| c == 'ñ'));
}
