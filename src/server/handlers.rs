use std::collections::HashSet;
use std::time::{Duration, SystemTime};

use serde::Serialize;
use serde_json::{Map, Value, json};

use super::{Request, Response, Server, fail, json_response, ok};
use crate::capture::{BOARD, SOFTWARE};
use crate::config::lenient::{self, Kind, Node};
use crate::config::{self, Action, Config, KeyDef, Keyboard, Keys, Layer};
use crate::jsonx::{self, Body};
use crate::{actions, layouts, obs, packs, platform, setup};

type Handler = fn(&Server, &Request) -> Response;

pub fn go_number(v: f64) -> Value {
    if v.is_finite() && v.fract() == 0.0 && v.abs() < 1e15 { json!(v as i64) } else { json!(v) }
}

impl Server {
    pub(super) fn allowed(&self, path: &str) -> Option<&'static str> {
        match (self.route("GET", path).is_some(), self.route("POST", path).is_some()) {
            (true, true) => Some("GET, HEAD, POST"),
            (true, false) => Some("GET, HEAD"),
            (false, true) => Some("POST"),
            _ => None,
        }
    }

    pub(super) fn route(&self, method: &str, path: &str) -> Option<Handler> {
        let h: Handler = match (method, path) {
            ("GET", "/api/config") => |s, _| json_response(200, &s.eng.config()),
            ("POST", "/api/config") => post_config,
            ("GET", "/api/backups") => |s, _| json_response(200, &config::list_backups(&s.paths)),
            ("POST", "/api/backups/restore") => restore,
            ("POST", "/api/backups/create") => create_backup,
            ("POST", "/api/editing") => set_editing,
            ("GET", "/api/status") => status,
            ("GET", "/api/keyboard") => keyboard,
            ("GET", "/api/setup") => |s, _| json_response(200, &setup::describe(&s.eng.config(), s.src.as_ref(), &s.eng.system())),
            ("POST", "/api/keyboard/layout") => set_layout,
            ("POST", "/api/board") => board_cmd,
            ("POST", "/api/capture") => set_capture,
            ("GET", "/api/learn") => learn_get,
            ("POST", "/api/learn") => learn_set,
            ("POST", "/api/layer") => set_layer,
            ("POST", "/api/dev/raw") if std::env::var("TYPEDECK_DEV").as_deref() == Ok("1") => dev_raw,
            ("GET", "/api/apps") => |_, _| json_response(200, &platform::get().installed_apps()),
            ("GET", "/api/layouts") => |_, _| json_response(200, &layouts::all()),
            ("GET", "/api/compat") => {
                |_, _| Response::new(200).header("Content-Type", "application/json; charset=utf-8").with_body(setup::COMPAT.as_bytes().to_vec())
            }
            ("GET", "/api/packs") => |_, _| {
                let os = platform::get().name();
                json_response(200, &json!({"categories": packs::categories(), "packs": packs::for_os(os), "os": os}))
            },
            ("POST", "/api/packs/resolve") => resolve_pack,
            ("POST", "/api/macros/inspect") => inspect_pack,
            ("GET", "/api/system") => |s, _| json_response(200, &s.eng.system()),
            ("GET", "/api/events") => events,
            ("GET", "/api/stats") => |s, _| json_response(200, &s.eng.stats()),
            ("POST", "/api/stats/reset") => |s, _| {
                s.eng.reset_stats();
                ok()
            },
            ("GET", "/api/history") => history,
            ("POST", "/api/history/clear") => |s, _| match s.eng.keylog().clear() {
                Ok(()) => ok(),
                Err(e) => fail(500, &e.to_string()),
            },
            ("POST", "/api/test") => test,
            ("POST", "/api/hud") => hud,
            ("POST", "/api/obs/probe") => obs_probe,
            _ => return None,
        };
        Some(h)
    }
}

fn body(req: &Request) -> Result<Body, Response> {
    Body::parse(&req.body).map_err(|_| fail(400, "json"))
}

fn post_config(s: &Server, req: &Request) -> Response {
    let c: Config = match lenient::decode(&req.body, Kind::Config) {
        Ok(c) => c,
        Err(e) => return fail(400, &format!("json: {e}")),
    };
    match s.eng.set_config_keep_capture(c, req.header("x-tab")) {
        Ok(loaded) => json_response(200, &json!({"ok": true, "config": loaded})),
        Err(e) => fail(400, &e),
    }
}

fn restore(s: &Server, req: &Request) -> Response {
    let name = match body(req).and_then(|b| b.str("name").map_err(|_| fail(400, "json"))) {
        Ok(n) => n,
        Err(r) => return r,
    };
    match config::restore_backup(&s.paths, &name) {
        Ok(c) => json_response(200, &json!({"ok": true, "config": s.eng.apply_restored(c)})),
        Err(e) => fail(400, &e),
    }
}

fn create_backup(s: &Server, _req: &Request) -> Response {
    match config::save(&s.paths, &s.eng.config(), true, true) {
        Ok(()) => ok(),
        Err(e) => fail(400, &e),
    }
}

fn set_editing(s: &Server, req: &Request) -> Response {
    let on = match body(req).and_then(|b| b.bool("on").map_err(|_| fail(400, "json"))) {
        Ok(v) => v,
        Err(r) => return r,
    };
    s.eng.set_editing(on || s.hub.editing());
    ok()
}

fn set_layer(s: &Server, req: &Request) -> Response {
    if req.body.len() > 1 << 10 {
        return fail(400, "json");
    }
    let index = match body(req).and_then(|b| b.int("index").map_err(|_| fail(400, "json"))) {
        Ok(v) => v,
        Err(r) => return r,
    };
    s.eng.goto(&Value::from(index));
    ok()
}

fn dev_raw(s: &Server, req: &Request) -> Response {
    let b = match body(req) {
        Ok(b) => b,
        Err(r) => return r,
    };
    let (Ok(line), Ok(multi)) = (b.str("line"), b.bool("multi")) else { return fail(400, "json") };
    let (lines, err) = s.src.raw(&line, multi, Duration::from_secs(3));
    json_response(200, &json!({"reply": lines, "error": err.unwrap_or_else(|| "<nil>".into())}))
}

fn test(s: &Server, req: &Request) -> Response {
    let a: Action = match lenient::decode(&req.body, Kind::Action) {
        Ok(a) => a,
        Err(_) => return fail(400, "json"),
    };
    let mut keys = Keys::new();
    keys.insert("04".into(), KeyDef::tap(a));
    let c = Config { layers: vec![Layer { name: "t".into(), keys, ..Default::default() }], ..Default::default() };
    match config::validate(&c) {
        Ok(clean) => match clean.layers[0].keys.get("04").and_then(|k| k.tap.clone()) {
            Some(a) => json_response(200, &s.eng.test_action(&a)),
            None => fail(400, "invalid action"),
        },
        Err(e) => fail(400, &e),
    }
}

fn hud(s: &Server, req: &Request) -> Response {
    let b = match body(req) {
        Ok(b) => b,
        Err(r) => return r,
    };
    let (Ok(title), Ok(sub)) = (b.str("title"), b.str("subtitle")) else { return fail(400, "json") };
    let clip = |s: &str, n: usize| s.chars().take(n).collect::<String>();
    s.eng.hud(&clip(&title, 60), &clip(&sub, 80), true);
    json_response(200, &json!({"ok": true}))
}

fn obs_probe(s: &Server, req: &Request) -> Response {
    let mut b: config::Obs = match jsonx::first_value(&req.body).map(|v| lenient::normalize(v, Node::Obj(Kind::Obs))) {
        Ok(Value::Object(m)) => {
            let get_s = |k: &str| m.get(k).and_then(Value::as_str).unwrap_or("").to_string();
            config::Obs { host: get_s("host"), port: m.get("port").and_then(Value::as_i64).unwrap_or(0), password: get_s("password") }
        }
        _ => return fail(400, "json"),
    };
    if b.host.is_empty() && b.port == 0 && b.password.is_empty() {
        b = s.eng.config().settings.obs;
    }
    match obs::probe(&obs::Config { host: b.host, port: b.port, password: b.password }) {
        Ok(info) => json_response(200, &json!({"ok": true, "info": info})),
        Err(e) => json_response(200, &json!({"ok": false, "error": e})),
    }
}

fn board_cmd(s: &Server, req: &Request) -> Response {
    let b = match body(req) {
        Ok(b) => b,
        Err(r) => return r,
    };
    let (Ok(cmd), Ok(arg)) = (b.str("cmd"), b.int("arg")) else { return fail(400, "json") };
    match s.src.command(&cmd, arg) {
        Ok(lines) => json_response(200, &json!({"ok": true, "reply": lines})),
        Err(e) => fail(400, &e),
    }
}

fn set_capture(s: &Server, req: &Request) -> Response {
    let b = match body(req) {
        Ok(b) => b,
        Err(r) => return r,
    };
    let (Ok(mode), Ok(consent)) = (b.str("mode"), b.bool("consent")) else { return fail(400, "json") };
    if mode == SOFTWARE && !consent {
        return fail(400, "software capture needs explicit consent");
    }
    if mode != BOARD && mode != SOFTWARE {
        return fail(400, "unknown capture mode");
    }
    match s.eng.set_capture(&mode, consent) {
        Ok(()) => json_response(200, &json!({"ok": true, "capture": mode, "config": s.eng.config()})),
        Err(e) => fail(400, &e),
    }
}

fn learn_set(s: &Server, req: &Request) -> Response {
    let on = match body(req).and_then(|b| b.bool("on").map_err(|_| fail(400, "json"))) {
        Ok(v) => v,
        Err(r) => return r,
    };
    if on {
        if let Err(e) = s.eng.start_learn() {
            return fail(400, &e);
        }
    } else {
        s.eng.stop_learn();
    }
    json_response(200, &json!({"ok": true, "learning": on}))
}

fn put_since(out: &mut Map<String, Value>, connected: bool, since: Option<SystemTime>) {
    if connected && let Some(t) = since {
        out.insert("connected_since".into(), go_number(crate::clock::unix_secs(t) as f64));
    }
}

fn status(s: &Server, _req: &Request) -> Response {
    let st = s.src.status();
    let es = s.eng.status();
    let mut out = json!({
        "connected": st.connected, "port": st.port, "layer": es.layer, "manual_layer": es.manual_layer, "last": es.last,
        "last_ts": go_number(es.last_ts), "front": es.front, "version": crate::app::VERSION, "uptime": es.uptime, "auto_active": es.auto_active,
        "last_id": es.last_id, "learning": es.learning, "firmware": "", "keyboard_present": false, "board_error": s.src.error(),
        "capture": s.eng.capture_mode(), "app_name": crate::app::NAME, "editing": s.eng.editing(), "editors": s.hub.count(),
    });
    if let Value::Object(m) = &mut out {
        put_since(m, st.connected, st.since);
        if let Some(info) = &st.info {
            m.insert("firmware".into(), json!(info.firmware));
            m.insert("keyboard_present".into(), json!(info.present));
        }
    }
    json_response(200, &out)
}

fn keyboard(s: &Server, _req: &Request) -> Response {
    let st = s.src.status();
    let mut out = Map::new();
    out.insert("connected".into(), json!(st.connected));
    out.insert("port".into(), json!(st.port));
    put_since(&mut out, st.connected, st.since);
    if let Some(info) = &st.info {
        out.insert("info".into(), json!(info));
        let cfg = s.eng.config();
        if let Some(prefs) = cfg.keyboards.get(&info.id()) {
            out.insert("prefs".into(), json!(prefs));
        }
        if info.present {
            let id = setup::identify(info);
            out.insert("layout".into(), json!(setup::resolve(&cfg, Some(info), &id, &s.eng.system())));
            out.insert("identity".into(), json!(id));
        }
    }
    json_response(200, &Value::Object(out))
}

fn set_layout(s: &Server, req: &Request) -> Response {
    let layout = match body(req).and_then(|b| b.str("layout").map_err(|_| fail(400, "json"))) {
        Ok(v) => v,
        Err(r) => return r,
    };
    let Some(l) = layouts::get(&layout) else { return fail(400, "unknown layout") };
    let info = s.src.status().info;
    let (key, name) = match info.as_ref().filter(|i| !i.id().is_empty()) {
        Some(i) => (i.id(), format!("{} {}", i.mfr, i.prod).trim().to_string()),
        None => ("default".to_string(), String::new()),
    };
    let layout = l.id.clone();
    match s.eng.update_config(move |cfg| {
        cfg.keyboards.insert(key, Keyboard { layout, name });
    }) {
        Ok(loaded) => json_response(200, &json!({"ok": true, "config": loaded})),
        Err(e) => fail(400, &e),
    }
}

fn events(s: &Server, req: &Request) -> Response {
    let since: i64 = req.query_param("since").parse().unwrap_or(0);
    let wait: f64 = req.query_param("wait").parse().unwrap_or(0.0);
    let wait = if wait.is_finite() { wait.clamp(0.0, 30.0) } else { 0.0 };
    let (evs, last) = s.eng.events_since(since, Duration::from_secs_f64(wait), &*req.closed);
    json_response(200, &json!({"events": evs, "last_id": last}))
}

fn learn_get(s: &Server, _req: &Request) -> Response {
    let seen = s.eng.seen();
    let set: HashSet<u8> = seen.keys().filter_map(|k| u8::from_str_radix(k, 16).ok()).collect();
    let mut sugs = layouts::match_seen(&set);
    sugs.truncate(4);
    let (amb, hints) = layouts::ambiguity(&sugs);
    json_response(200, &json!({"learning": s.eng.status().learning, "seen": seen, "suggestions": sugs, "ambiguous": amb, "hint_keys": hints}))
}

fn resolve_pack(s: &Server, req: &Request) -> Response {
    let b = match body(req) {
        Ok(b) => b,
        Err(r) => return r,
    };
    let (Ok(id), Ok(mut layout), Ok(mut lang)) = (b.str("id"), b.str("layout"), b.str("lang")) else { return fail(400, "json") };
    let Some(p) = packs::get(&id) else { return fail(404, "unknown pack") };
    let cfg = s.eng.config();
    if layout.is_empty() {
        let info = s.src.status().info;
        let ident = info.as_ref().filter(|i| i.present).map(setup::identify).unwrap_or_default();
        layout = setup::resolve(&cfg, info.as_ref(), &ident, &s.eng.system()).id;
    }
    if lang.is_empty() {
        lang = cfg.settings.language.clone();
    }
    let reserved: HashSet<String> = cfg.global.keys().cloned().collect();
    json_response(200, &packs::resolve_for(platform::get().name(), p, &layout, &lang, &reserved))
}

fn history(s: &Server, req: &Request) -> Response {
    let limit: usize = req.query_param("limit").parse().ok().filter(|l| (1..=500).contains(l)).unwrap_or(100);
    let mut layout = actions::typing_layout(s.eng.as_ref());
    if layout.name == "none" {
        layout = crate::hid::layout_by_name("us");
    }
    let rep = s.eng.keylog().report(limit, 2500, SystemTime::now(), layout);
    json_response(
        200,
        &json!({"enabled": s.eng.keylog().enabled(), "count": rep.summary.total, "entries": rep.entries, "summary": rep.summary, "text": rep.text}),
    )
}

#[derive(Serialize)]
struct Risky {
    #[serde(rename = "type")]
    kind: String,
    text: String,
    key: String,
    at: String,
}

fn collect_risky(a: &Action, key: &str, at: &str, out: &mut Vec<Risky>) {
    if matches!(a.kind.as_str(), "shell" | "ssh" | "http") {
        out.push(Risky { kind: a.kind.clone(), text: actions::describe(a), key: key.into(), at: at.into() });
    }
    for s in a.steps.iter().chain(&a.otherwise) {
        collect_risky(s, key, at, out);
    }
}

fn inspect_pack(_s: &Server, req: &Request) -> Response {
    let Ok(Value::Object(root)) = jsonx::first_value(&req.body).map(|v| lenient::normalize(v, Node::Obj(Kind::PackFile))) else { return fail(400, "json") };
    let pack = root.get("pack").cloned().unwrap_or(Value::Object(Map::new()));
    let Value::Object(mut p) = pack else { return fail(400, "json") };
    let format = p.get("typedeck_macros").and_then(Value::as_str).unwrap_or("").to_string();
    let layers: Vec<Layer> = match p.get("layers").cloned().map(serde_json::from_value).transpose() {
        Ok(l) => l.unwrap_or_default(),
        Err(_) => return fail(400, "json"),
    };
    let global: Keys = match p.get("global").cloned().map(serde_json::from_value).transpose() {
        Ok(g) => g.unwrap_or_default(),
        Err(_) => return fail(400, "json"),
    };
    if format.is_empty() || (layers.is_empty() && global.is_empty()) {
        return fail(400, "not a Typedeck macro file");
    }
    if layers.len() > config::MAX_LAYERS {
        return fail(400, "too many layers");
    }
    let has_layers = !layers.is_empty();
    let probe = Config { layers: if has_layers { layers } else { vec![Layer { name: "x".into(), ..Default::default() }] }, global, ..Default::default() };
    let clean = match config::validate(&probe) {
        Ok(c) => c,
        Err(e) => return fail(400, &e),
    };
    let layers = if has_layers { clean.layers } else { vec![] };
    let mut risky = Vec::new();
    let mut keys = clean.global.len();
    let scan = |m: &Keys, at: &str, risky: &mut Vec<Risky>| {
        for (id, kd) in m {
            for a in [&kd.tap, &kd.hold, &kd.double].into_iter().flatten() {
                collect_risky(a, id, at, risky);
            }
        }
    };
    scan(&clean.global, "global", &mut risky);
    for l in &layers {
        keys += l.keys.len();
        scan(&l.keys, &l.name, &mut risky);
    }
    let name = p.get("name").and_then(Value::as_str).unwrap_or("").to_string();
    let author = p.get("author").and_then(Value::as_str).unwrap_or("").to_string();
    p.clear();
    p.insert("typedeck_macros".into(), json!(format));
    p.insert("name".into(), json!(name));
    if !author.is_empty() {
        p.insert("author".into(), json!(author));
    }
    p.insert("layers".into(), json!(layers));
    if !clean.global.is_empty() {
        p.insert("global".into(), json!(clean.global));
    }
    json_response(200, &json!({"ok": true, "pack": p, "keys": keys, "risky": risky}))
}
