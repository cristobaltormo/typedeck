use super::*;

use crate::testutil::temp_dir;

fn act(kind: &str) -> Action {
    Action::new(kind)
}

#[test]
fn migrate_v1() {
    let old = br#"{"settings":{"hud_on_action":true},"global":{"KPENTER":{"type":"url","url":"http://x","label":"Ed"}},
    "layers":[{"name":"A","keys":{"KP7":{"type":"open","app":"Safari"},"KP/":{"type":"media","cmd":"mute"}}}]}"#;
    let c = parse(old).unwrap();
    assert_eq!(c.version, 3);
    assert!(c.settings.hud.on_action);
    let k = &c.layers[0].keys["5F"];
    let tap = k.tap.as_ref().unwrap();
    assert_eq!((tap.kind.as_str(), tap.app.as_str()), ("app", "Safari"));
    assert_eq!(c.global["58"].label, "Ed");
    assert_eq!(c.global["53"].tap.as_ref().unwrap().kind, "layer");
    assert_eq!(c.layers[0].keys["54"].tap.as_ref().unwrap().cmd, "mute");
}

#[test]
fn migrate_v2_keeps_everything() {
    let v2 = br##"{"version":2,"settings":{"language":"en","theme":"dark","accent":"#8b5cf6","keysize":100,"hud":{"enabled":false,"position":"top","seconds":3}},
    "global":{"KP.":{"tap":{"type":"system","cmd":"lock","confirm":true},"label":"L"}},
    "layers":[{"name":"Trabajo","color":"#2563eb","auto_apps":["Code"],"keys":{"KP8":{"tap":{"type":"app","app":"Termius","mode":"toggle"},"hold":{"type":"ssh","host":"my-server","cmd":"uptime"},"color":"#16a34a"}}}]}"##;
    let c = parse(v2).unwrap();
    assert_eq!(c.settings.language, "en");
    assert_eq!(c.settings.accent, "#8b5cf6");
    assert!(!c.settings.hud.enabled);
    assert_eq!(c.settings.hud.position, "top");
    let k = &c.layers[0].keys["60"];
    assert_eq!(k.tap.as_ref().unwrap().mode, "toggle");
    let hold = k.hold.as_ref().unwrap();
    assert_eq!(hold.host, "my-server");
    assert_eq!(hold.show_output, Some(true));
    assert!(c.global["63"].tap.as_ref().unwrap().confirm);
}

#[test]
fn validate_rejects() {
    type Mutate = Box<dyn Fn(&mut Config)>;
    let cases: Vec<(&str, Mutate)> = vec![
        ("no layers", Box::new(|c| c.layers.clear())),
        ("too many layers", Box::new(|c| c.layers = vec![Layer::default(); 10])),
        ("made up key", Box::new(|c| drop(c.layers[0].keys.insert("ZZ".into(), KeyDef { label: "x".into(), ..Default::default() })))),
        ("key out of range", Box::new(|c| drop(c.layers[0].keys.insert("01".into(), KeyDef { label: "x".into(), ..Default::default() })))),
        ("bad type", Box::new(|c| drop(c.layers[0].keys.insert("5F".into(), KeyDef::tap(act("nope")))))),
        ("bad color", Box::new(|c| drop(c.layers[0].keys.insert("5F".into(), KeyDef { color: "red".into(), ..Default::default() })))),
        ("layer without a name", Box::new(|c| c.layers[0].name = "  ".into())),
        ("huge text", Box::new(|c| drop(c.layers[0].keys.insert("5F".into(), KeyDef::tap(Action { text: "a".repeat(5000), ..act("text") }))))),
        ("nested sequence", Box::new(|c| drop(c.layers[0].keys.insert("5F".into(), KeyDef::tap(Action { steps: vec![act("sequence")], ..act("sequence") }))))),
    ];
    for (name, mutate) in cases {
        let mut c = Config::initial();
        mutate(&mut c);
        assert!(validate(&c).is_err(), "{name}: should have failed");
    }
}

#[test]
fn validate_clamps_and_normalises() {
    let mut c = Config::initial();
    c.settings.hold_ms = 99999;
    c.settings.keysize = 1;
    c.settings.accent = "azul".into();
    c.settings.input = "raro".into();
    c.layers[0].keys.insert("5f".into(), KeyDef::tap(Action { minutes: 99999.0, ..act("timer") }));
    let out = validate(&c).unwrap();
    let s = &out.settings;
    assert_eq!((s.hold_ms, s.keysize, s.accent.as_str(), s.input.as_str()), (1500, 64, "#2563eb", "auto"));
    assert_eq!(out.layers[0].keys["5F"].tap.as_ref().unwrap().minutes, 600.0);
}

#[test]
fn save_load_backup_restore() {
    let p = Paths::new(temp_dir("store"));
    let mut c = load(&p).unwrap();
    assert_eq!(c.layers.len(), 2);
    c.layers[0].name = "Otra".into();
    save(&p, &c, true, true).unwrap();
    assert_eq!(load(&p).unwrap().layers[0].name, "Otra");
    let b = list_backups(&p);
    assert_eq!(b.len(), 1);
    assert_eq!(b[0].layers, 2);
    let r = restore_backup(&p, &b[0].name).unwrap();
    assert_eq!(r.layers[0].name, "Layer 1");
    assert!(restore_backup(&p, "../../etc/passwd").is_err());
    assert!(std::fs::read_dir(&p.dir).unwrap().flatten().all(|e| !e.file_name().to_string_lossy().contains(".tmp")));
    let _ = std::fs::remove_dir_all(&p.dir);
}

#[test]
fn backups_in_the_same_second_do_not_overwrite_each_other() {
    let p = Paths::new(temp_dir("burst"));
    let mut c = load(&p).unwrap();
    for i in 0..3 {
        c.layers[0].name = format!("n{i}");
        save(&p, &c, true, true).unwrap();
    }
    let names: Vec<String> = list_backups(&p).into_iter().map(|b| b.name).collect();
    assert_eq!(names.len(), 3, "{names:?}");
    let _ = std::fs::remove_dir_all(&p.dir);
}

#[test]
fn layer_targets_normalised() {
    let mut c = Config::initial();
    c.layers[0].keys.insert("5F".into(), KeyDef::tap(Action { to: Some(serde_json::json!(1.0)), ..act("layer") }));
    c.layers[0].keys.insert("60".into(), KeyDef::tap(Action { to: Some("prev".into()), ..act("layer") }));
    c.layers[0].keys.insert("61".into(), KeyDef::tap(Action { to: Some("basura".into()), ..act("layer") }));
    c.layers[0].keys.insert("62".into(), KeyDef::tap(Action { to: Some("2".into()), ..act("layer") }));
    let out = validate(&c).unwrap();
    let to = |k: &str| out.layers[0].keys[k].tap.as_ref().unwrap().to.clone().unwrap();
    assert_eq!(to("5F"), serde_json::json!(1));
    assert_eq!(to("60"), serde_json::json!("prev"));
    assert_eq!(to("61"), serde_json::json!("next"));
    assert_eq!(to("62"), serde_json::json!(2));
}

#[test]
fn corrupt_config_recovers_from_backup() {
    let p = Paths::new(temp_dir("corrupt"));
    let mut c = Config::initial();
    c.layers[0].name = "Buena".into();
    save(&p, &c, false, false).unwrap();
    c.layers[0].name = "Segunda".into();
    save(&p, &c, true, true).unwrap();
    std::fs::write(p.config(), "{esto no es json").unwrap();
    let (got, note) = load_with_note(&p).unwrap();
    assert_eq!(got.layers[0].name, "Buena");
    assert!(!note.is_empty());
    let broken = std::fs::read_dir(&p.dir).unwrap().flatten().filter(|e| e.file_name().to_string_lossy().starts_with("config.broken-")).count();
    assert_eq!(broken, 1);
    let (again, note) = load_with_note(&p).unwrap();
    assert!(note.is_empty());
    assert_eq!(again.layers[0].name, "Buena");
    let _ = std::fs::remove_dir_all(&p.dir);
}

#[test]
fn corrupt_config_without_backups_starts_fresh() {
    let p = Paths::new(temp_dir("fresh"));
    std::fs::write(p.config(), r#"{"version":3,"layers":[]}"#).unwrap();
    let (got, note) = load_with_note(&p).unwrap();
    assert!(!note.is_empty());
    assert_eq!(got.layers.len(), 2);
    let _ = std::fs::remove_dir_all(&p.dir);
}

#[test]
fn obs_action_and_settings() {
    let mut c = Config::initial();
    c.settings.obs = Obs { host: " bad host/ ".into(), port: 99999, password: "x".repeat(500) };
    c.layers[0].keys.insert("04".into(), KeyDef::tap(Action { cmd: "borrar-todo".into(), target: "Mic".into(), ..act("obs") }));
    c.layers[0].keys.insert("05".into(), KeyDef::tap(Action { cmd: "scene".into(), target: "#2".into(), ..act("obs") }));
    let out = validate(&c).unwrap();
    assert_eq!(out.settings.obs.host, "127.0.0.1");
    assert_eq!(out.settings.obs.port, 65535);
    assert_eq!(out.settings.obs.password.len(), 200);
    assert_eq!(out.layers[0].keys["04"].tap.as_ref().unwrap().cmd, "record");
    let a = out.layers[0].keys["05"].tap.as_ref().unwrap();
    assert_eq!((a.cmd.as_str(), a.target.as_str()), ("scene", "#2"));
}

#[test]
fn conditional_and_sequence_limits() {
    let wrap = |a: Action| {
        let mut keys = Keys::new();
        keys.insert("04".into(), KeyDef::tap(a));
        Config { layers: vec![Layer { name: "x".into(), keys, ..Default::default() }], ..Default::default() }
    };
    let ok = Action {
        steps: vec![Action {
            cond: "os".into(),
            target: "linux".into(),
            steps: vec![Action { text: "a".into(), ..act("hud") }],
            otherwise: vec![Action { ms: 5, ..act("wait") }],
            ..act("if")
        }],
        ..act("sequence")
    };
    let c = validate(&wrap(ok)).unwrap();
    let got = &c.layers[0].keys["04"].tap.as_ref().unwrap().steps[0];
    assert_eq!(got.cond, "os");
    assert_eq!(got.otherwise.len(), 1);
    let nested = Action {
        cond: "os".into(),
        steps: vec![Action { cond: "os".into(), steps: vec![Action { cond: "os".into(), ..act("if") }], ..act("if") }],
        ..act("if")
    };
    assert!(validate(&wrap(nested)).is_err());
    assert!(validate(&wrap(Action { cond: "nope".into(), ..act("if") })).is_err());
    let mut long = act("sequence");
    long.steps = vec![Action { ms: 1, ..act("wait") }; MAX_STEPS + 1];
    assert!(validate(&wrap(long.clone())).is_err());
    long.steps.truncate(MAX_STEPS);
    assert!(validate(&wrap(long)).is_ok());
}

#[test]
fn default_language_is_english() {
    assert_eq!(Config::initial().settings.language, "en");
    let c = parse(br#"{"version":3,"settings":{"language":"es"},"layers":[{"name":"x","keys":{}}]}"#).unwrap();
    assert_eq!(c.settings.language, "es");
}

#[test]
fn key_history_retention_is_clamped() {
    let mut c = Config::initial();
    c.settings.key_history_days = 0;
    assert_eq!(validate(&c).unwrap().settings.key_history_days, 30);
    c.settings.key_history_days = 9999;
    assert_eq!(validate(&c).unwrap().settings.key_history_days, 365);
    c.settings.key_history_days = 7;
    assert_eq!(validate(&c).unwrap().settings.key_history_days, 7);
}

#[test]
fn capture_mode_defaults_to_the_board() {
    assert_eq!(Settings::default().capture, "board");
    for raw in [
        r#"{"version":3,"settings":{"language":"es"},"global":{},"layers":[{"name":"A","keys":{}}]}"#,
        r#"{"version":3,"settings":{"capture":"telepathy"},"global":{},"layers":[{"name":"A","keys":{}}]}"#,
        r#"{"version":2,"settings":{},"global":{},"layers":[{"name":"A","keys":{}}]}"#,
        r#"{"layers":[{"name":"A","keys":{}}]}"#,
    ] {
        assert_eq!(parse(raw.as_bytes()).unwrap().settings.capture, "board", "{raw}");
    }
}

#[test]
fn software_capture_mode_survives_save_and_load() {
    let p = Paths::new(temp_dir("sw"));
    let mut c = Config::initial();
    c.settings.capture = "software".into();
    save(&p, &c, false, false).unwrap();
    let got = load(&p).unwrap();
    assert_eq!(got.settings.capture, "software");
    assert_eq!(got.settings.input, "auto");
    let _ = std::fs::remove_dir_all(&p.dir);
}

#[test]
fn validate_leaves_the_input_untouched_and_truncates_at_char_boundaries() {
    let mut c = Config::initial();
    c.layers[0].name = "  padded  ".into();
    c.keyboards.insert("x".into(), Keyboard { layout: "a".repeat(40), name: String::new() });
    let out = validate(&c).unwrap();
    assert_eq!(c.layers[0].name, "  padded  ");
    assert_eq!(c.keyboards.len(), 1);
    assert_eq!(out.layers[0].name, "padded");
    assert!(out.keyboards.is_empty());
    c.layers[0].name = "ñ".repeat(20);
    let out = validate(&c).unwrap();
    assert_eq!(out.layers[0].name, "ñ".repeat(15));
}

#[test]
fn nulls_behave_like_missing_fields() {
    let c = parse(br#"{"version":3,"settings":{"hud":null,"language":null},"global":null,"layers":[{"name":"A","keys":{"04":{"tap":{"type":"layer","to":null}}},"auto_apps":null}]}"#).unwrap();
    assert!(c.settings.hud.enabled);
    assert_eq!(c.settings.language, "en");
    assert_eq!(c.layers[0].keys["04"].tap.as_ref().unwrap().to, Some(serde_json::json!("next")));
}

#[test]
fn saved_json_keeps_the_go_layout() {
    let p = Paths::new(temp_dir("layout"));
    save(&p, &Config::initial(), false, false).unwrap();
    let text = std::fs::read_to_string(p.config()).unwrap();
    assert!(text.starts_with("{\n  \"version\": 3,\n  \"settings\": {\n    \"language\": \"en\","), "{text}");
    assert!(text.contains("\"seconds\": 1.2"));
    assert!(text.contains("\"auto_apps\": []"));
    let _ = std::fs::remove_dir_all(&p.dir);
}

#[test]
fn consent_is_only_known_once_recorded() {
    let p = Paths::new(temp_dir("consent"));
    assert!(!has_capture_consent(&p));
    record_capture_consent(&p).unwrap();
    assert!(has_capture_consent(&p));
    for body in [r#"{"version":0,"at":"2026-01-01T00:00:00Z"}"#, "nope", ""] {
        std::fs::write(p.capture_consent(), body).unwrap();
        assert!(!has_capture_consent(&p), "{body}");
    }
    let mut c = Config::initial();
    c.settings.capture = "software".into();
    let _ = std::fs::remove_file(p.capture_consent());
    save(&p, &c, false, false).unwrap();
    assert!(!has_capture_consent(&p));
    let _ = std::fs::remove_dir_all(&p.dir);
}

#[test]
fn field_names_are_case_insensitive_and_null_elements_are_zero_values() {
    let c = parse(
        br#"{"version":3,"Settings":{"Language":"es","HUD":{"Seconds":2}},"GLOBAL":{"04":{"Label":"A","TAP":{"Type":"hud","Text":"x"}},"05":null},
        "Layers":[{"Name":"B","Auto_Apps":["Safari",null],"Keys":{"06":{"tap":{"type":"sequence","Steps":[{"Type":"hud","text":"y"}]}}}}]}"#,
    )
    .unwrap();
    assert_eq!(c.settings.language, "es");
    assert_eq!(c.settings.hud.seconds, 2.0);
    assert_eq!(c.global["04"].label, "A");
    assert_eq!(c.global["04"].tap.as_ref().unwrap().text, "x");
    assert_eq!(c.layers.len(), 1);
    assert_eq!(c.layers[0].name, "B");
    assert!(c.layers[0].auto_apps.iter().all(|a| a == "Safari"));
    assert_eq!(parse(br#"{"version":3,"layers":[null,{"name":"B"}]}"#).unwrap_err(), "layer 0: the name is missing");
    let null_step = br#"{"version":3,"layers":[{"name":"A","keys":{"06":{"tap":{"type":"sequence","steps":[null]}}}}]}"#;
    assert_eq!(parse(null_step).unwrap_err(), "layer 0/06.tap.steps[0]: invalid action type");
    assert_eq!(c.layers[0].keys["06"].tap.as_ref().unwrap().steps[0].text, "y");
    let exact = parse(br#"{"version":3,"settings":{"language":"es","LANGUAGE":"de"},"layers":[{"name":"A"}]}"#).unwrap();
    assert_eq!(exact.settings.language, "es");
}

#[test]
fn config_files_reject_trailing_data_but_the_editor_api_does_not() {
    let ok = br#"{"version":3,"layers":[{"name":"A"}]}"#;
    assert!(parse(ok).is_ok());
    assert!(parse(&[ok.as_slice(), b"\n"].concat()).is_ok());
    assert!(parse(&[ok.as_slice(), b" {}"].concat()).unwrap_err().starts_with("invalid json"));
    assert!(parse(&[ok.as_slice(), b" x"].concat()).unwrap_err().starts_with("invalid json"));
    let c: Config = lenient::decode(br#"{"version":3,"Settings":{"theme":"dark"}} trailing"#, lenient::Kind::Config).unwrap();
    assert_eq!(c.settings.theme, "dark");
}

#[test]
fn concurrent_saves_never_leave_a_broken_file() {
    let p = Paths::new(temp_dir("concurrent"));
    let workers: Vec<_> = (0..4)
        .map(|i| {
            let p = p.clone();
            std::thread::spawn(move || {
                for j in 0..20 {
                    let mut c = Config::initial();
                    c.layers[0].name = format!("L{i}-{j}");
                    save(&p, &c, j % 5 == 0, false).unwrap();
                }
            })
        })
        .collect();
    for w in workers {
        w.join().unwrap();
    }
    assert!(load(&p).unwrap().layers[0].name.starts_with('L'));
    assert!(std::fs::read_dir(&p.dir).unwrap().flatten().all(|e| !e.file_name().to_string_lossy().contains(".tmp")));
    let _ = std::fs::remove_dir_all(&p.dir);
}
