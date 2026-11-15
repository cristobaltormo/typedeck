use std::sync::Mutex;

use serde_json::Value;

use super::*;
use crate::capture::{Backend, CaptureError, Check, Filter, Latency, Selector, SoftwareSource};
use crate::config::Config;
use crate::hid::Stroke;
use crate::sync::lock;
use crate::testutil::temp_dir;

#[derive(Default)]
struct Stub(Mutex<bool>);

impl Backend for Arc<Stub> {
    fn start(&self, _f: &Arc<Filter>) -> Result<(), CaptureError> {
        *lock(&self.0) = true;
        Ok(())
    }
    fn alive(&self) -> bool {
        *lock(&self.0)
    }
    fn stop(&self) {
        *lock(&self.0) = false;
    }
    fn inject(&self, _s: Stroke, _ms: u32) -> Result<(), String> {
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

const GOOD: &str = "127.0.0.1:7788";

fn new_srv() -> Arc<Server> {
    let soft = SoftwareSource::new(Box::new(Arc::new(Stub::default())));
    let src = Selector::new(crate::board::Board::new(Box::new(NoBoard)), soft);
    let paths = Paths::new(temp_dir("server"));
    let eng = Engine::new(paths.clone(), Some(src.clone())).unwrap();
    Server::new(eng, src, paths, 7788).unwrap()
}

fn call(s: &Server, method: &str, path: &str, host: &str, token: &str, body: Option<Value>) -> Response {
    let mut req = Request::new(method, path, host);
    if !token.is_empty() {
        req.headers.push(("x-token".into(), token.into()));
    }
    if let Some(b) = body {
        req.body = serde_json::to_vec(&b).unwrap();
    }
    s.handle(&req)
}

fn get(s: &Server, path: &str) -> Value {
    serde_json::from_slice(&call(s, "GET", path, GOOD, s.token(), None).body).unwrap()
}

fn post(s: &Server, path: &str, body: Value) -> Response {
    call(s, "POST", path, GOOD, s.token(), Some(body))
}

#[test]
fn api_needs_token_and_host() {
    let s = new_srv();
    let t = s.token().to_string();
    for (host, token, want) in [
        (GOOD, "", 403),
        (GOOD, "x", 403),
        ("evil.com", t.as_str(), 403),
        ("127.0.0.1:9999", t.as_str(), 403),
        ("localhost:7788", t.as_str(), 200),
        (GOOD, t.as_str(), 200),
    ] {
        assert_eq!(call(&s, "GET", "/api/config", host, token, None).status, want, "{host} {token}");
    }
    for p in ["/api/config", "/api/test", "/api/hud", "/api/layer", "/api/board", "/api/backups/restore", "/api/learn"] {
        assert_eq!(call(&s, "POST", p, GOOD, "", Some(json!({}))).status, 403, "{p}");
    }
    let r = call(&s, "POST", "/api/status", GOOD, &t, None);
    assert_eq!((r.status, r.get("Allow")), (405, "GET, HEAD"));
    let r = call(&s, "GET", "/api/test", GOOD, &t, None);
    assert_eq!((r.status, r.get("Allow")), (405, "POST"));
    assert_eq!(get(&s, "/api/status")["last_ts"], json!(0));
    assert!(get(&s, "/api/status")["last_ts"].is_i64());
}

#[test]
fn config_round_trip_and_validation() {
    let s = new_srv();
    let mut c: Config = serde_json::from_value(get(&s, "/api/config")).unwrap();
    assert_eq!(c.layers.len(), 2);
    c.layers[0].name = "Nueva".into();
    assert_eq!(post(&s, "/api/config", serde_json::to_value(&c).unwrap()).status, 200);
    assert_eq!(get(&s, "/api/config")["layers"][0]["name"], "Nueva");
    assert_eq!(post(&s, "/api/config", json!({"version": 3, "layers": []})).status, 400);
    let evil = json!({"version": 3, "layers": [{"name": "x", "keys": {"04": {"tap": {"type": "rm -rf"}}}}]});
    assert_eq!(post(&s, "/api/config", evil).status, 400);
    let mut req = Request::new("POST", "/api/config", GOOD);
    req.headers.push(("x-token".into(), s.token().into()));
    req.body = b"{not json".to_vec();
    assert_eq!(s.handle(&req).status, 400);
}

#[test]
fn static_assets_headers_and_caching() {
    let s = new_srv();
    let idx = call(&s, "GET", "/", GOOD, "", None);
    assert_eq!(idx.status, 200);
    assert!(idx.text().contains(s.token()) && !idx.text().contains("__TOKEN__"));
    assert!(!idx.get("Content-Security-Policy").is_empty() && idx.get("X-Frame-Options") == "DENY");
    assert_eq!(call(&s, "GET", "/", "evil.com", "", None).status, 403);
    let js = call(&s, "GET", "/js/app.js", GOOD, "", None);
    assert_eq!(js.status, 200);
    assert!(js.get("Content-Type").starts_with("text/javascript"));
    let mut req = Request::new("GET", "/js/app.js", GOOD);
    req.headers.push(("if-none-match".into(), js.get("ETag").to_string()));
    assert_eq!(s.handle(&req).status, 304);
    for p in ["/../Cargo.toml", "/js/../../Cargo.toml", "/%2e%2e/Cargo.toml"] {
        assert_ne!(call(&s, "GET", p, GOOD, "", None).status, 200, "{p}");
    }
    assert_eq!(call(&s, "DELETE", "/", GOOD, "", None).status, 405);
}

#[test]
fn events_long_poll_returns_on_timeout() {
    let s = new_srv();
    let t0 = std::time::Instant::now();
    let v = get(&s, "/api/events?since=999999&wait=0.2");
    assert!(v["events"].as_array().unwrap().is_empty());
    assert!(t0.elapsed() >= Duration::from_millis(150));
}

#[test]
fn test_endpoint_runs_a_safe_action_and_rejects_invalid() {
    let s = new_srv();
    let r = post(&s, "/api/test", json!({"type": "wait", "ms": 5}));
    assert!(r.status == 200 && r.text().contains(r#""ok":true"#), "{}", r.text());
    assert_eq!(post(&s, "/api/test", json!({"type": "nope"})).status, 400);
}

#[test]
fn layouts_packs_setup_and_compat() {
    let s = new_srv();
    let ls = get(&s, "/api/layouts");
    assert_eq!(ls.as_array().unwrap().len(), 11);
    let pk = get(&s, "/api/packs");
    assert!(pk["packs"].as_array().unwrap().len() >= 25 && pk["categories"].as_array().unwrap().len() >= 8);
    let r = post(&s, "/api/packs/resolve", json!({"id": "zoom", "layout": "tkl-ansi", "lang": "en"}));
    assert!(r.status == 200 && r.text().contains(r#""region":"fn""#) && r.text().contains(r#""name":"Zoom""#), "{}", r.text());
    assert_eq!(post(&s, "/api/packs/resolve", json!({"id": "nope"})).status, 404);
    assert!(serde_json::to_string(&get(&s, "/api/setup")).unwrap().contains("no_board"));
    assert!(get(&s, "/api/compat")["groups"].is_array());
    assert_eq!(post(&s, "/api/keyboard/layout", json!({"layout": "no-existe"})).status, 400);
    assert_eq!(post(&s, "/api/keyboard/layout", json!({"layout": "iso-full"})).status, 200);
    assert_eq!(get(&s, "/api/config")["keyboards"]["default"]["layout"], "full-iso");
    assert!(get(&s, "/api/learn").get("suggestions").is_some());
    assert!(get(&s, "/api/apps").is_array());
    assert_eq!(get(&s, "/api/status")["app_name"], "Typedeck");
}

#[test]
fn inspect_pack_cleans_and_flags_risky_actions() {
    let s = new_srv();
    let pack = json!({"pack": {"typedeck_macros": "1", "name": "demo", "layers": [{"name": "Dev", "keys": {
        "04": {"tap": {"type": "shell", "cmd": "rm -rf ~"}},
        "05": {"tap": {"type": "sequence", "steps": [{"type": "http", "url": "https://x.test", "method": "POST"}, {"type": "wait", "ms": 99999999}]}},
        "06": {"tap": {"type": "url", "url": "https://example.com"}}}}]}});
    let r = post(&s, "/api/macros/inspect", pack);
    assert_eq!(r.status, 200);
    let v: Value = serde_json::from_slice(&r.body).unwrap();
    assert_eq!((v["keys"].as_i64(), v["risky"].as_array().map(Vec::len)), (Some(3), Some(2)));
    assert_eq!(v["pack"]["layers"][0]["keys"]["05"]["tap"]["steps"][1]["ms"], 60000);
    assert_eq!(post(&s, "/api/macros/inspect", json!({"pack": {"name": "x", "layers": []}})).status, 400);
    let global_only = json!({"pack": {"typedeck_macros": "1", "name": "g", "global": {"3A": {"tap": {"type": "hud", "text": "x"}}}}});
    assert_eq!(post(&s, "/api/macros/inspect", global_only).status, 200);
    let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("examples");
    if !dir.is_dir() {
        return;
    }
    let mut n = 0;
    for e in std::fs::read_dir(dir).unwrap().flatten() {
        if e.path().extension().is_some_and(|x| x == "json") {
            let pack: Value = serde_json::from_slice(&std::fs::read(e.path()).unwrap()).unwrap();
            assert_eq!(post(&s, "/api/macros/inspect", json!({"pack": pack})).status, 200, "{:?}", e.path());
            n += 1;
        }
    }
    assert!(n >= 3);
}

#[test]
fn history_endpoints_read_and_clear() {
    let s = new_srv();
    let mut cfg = Config::initial();
    cfg.settings.key_history = true;
    s.eng.apply_restored(cfg);
    let now = std::time::SystemTime::now();
    s.eng.keylog().press(0x04, now);
    s.eng.keylog().release(0x04, now + Duration::from_millis(50));
    let v = get(&s, "/api/history?limit=10");
    assert_eq!((v["enabled"].as_bool(), v["count"].as_i64(), v["entries"][0]["k"].as_str()), (Some(true), Some(1), Some("A")));
    assert_eq!(v["text"].as_str().unwrap().to_lowercase(), "a");
    assert_eq!(call(&s, "GET", "/api/history", GOOD, "", None).status, 403);
    assert_eq!(post(&s, "/api/history/clear", json!({})).status, 200);
    assert_eq!(get(&s, "/api/history")["count"], 0);
}

#[test]
fn software_capture_needs_consent_every_time() {
    let s = new_srv();
    assert_eq!(post(&s, "/api/capture", json!({"mode": "software"})).status, 400);
    assert_eq!(s.eng.capture_mode(), "board");
    assert_eq!(post(&s, "/api/capture", json!({"mode": "software", "consent": true})).status, 200);
    assert_eq!(s.eng.capture_mode(), "software");
    let st = get(&s, "/api/status");
    assert_eq!((st["capture"].as_str(), st["connected"].as_bool(), st["keyboard_present"].as_bool()), (Some("software"), Some(true), Some(true)));
    let kb = get(&s, "/api/keyboard");
    assert_eq!(kb["info"]["system"], true);
    assert_eq!(kb["identity"]["display"], "System keyboard");
    assert_eq!(post(&s, "/api/board", json!({"cmd": "ping"})).status, 200);
    assert_eq!(post(&s, "/api/board", json!({"cmd": "reboot"})).status, 400);
    assert_eq!(post(&s, "/api/capture", json!({"mode": "board"})).status, 200);
    assert_eq!(s.eng.capture_mode(), "board");
    assert_eq!(post(&s, "/api/capture", json!({"mode": "software"})).status, 400);
    assert_eq!(post(&s, "/api/capture", json!({"mode": "x", "consent": true})).status, 400);
}

#[test]
fn config_post_and_backups_cannot_enable_software_capture() {
    let s = new_srv();
    let mut c = s.eng.config();
    c.settings.capture = "software".into();
    assert_eq!(post(&s, "/api/config", serde_json::to_value(&c).unwrap()).status, 200);
    assert_eq!(s.eng.config().settings.capture, "board");
    std::fs::create_dir_all(s.paths.backups()).unwrap();
    let name = "config-20260101-000000.json";
    let mut c = Config::initial();
    c.settings.capture = "software".into();
    std::fs::write(s.paths.backups().join(name), serde_json::to_vec(&c).unwrap()).unwrap();
    let r = post(&s, "/api/backups/restore", json!({"name": name}));
    let v: Value = serde_json::from_slice(&r.body).unwrap();
    assert_eq!(v["config"]["settings"]["capture"], "board");
    assert_eq!(s.eng.capture_mode(), "board");
    assert_eq!(crate::config::load(&s.paths).unwrap().settings.capture, "board");
    post(&s, "/api/capture", json!({"mode": "software", "consent": true}));
    post(&s, "/api/capture", json!({"mode": "board"}));
    post(&s, "/api/backups/restore", json!({"name": name}));
    assert_eq!(s.eng.capture_mode(), "software");
    assert_eq!(post(&s, "/api/backups/restore", json!({"name": "../x"})).status, 400);
}

#[test]
fn websocket_hello_editing_and_plain_keys() {
    let s = new_srv();
    let ln = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let port = ln.local_addr().unwrap().port();
    let s2 = s.clone();
    std::thread::spawn(move || s2.serve(ln));
    let mut c = TcpStream::connect(("127.0.0.1", port)).unwrap();
    use std::io::{BufRead, Write};
    write!(c, "GET /api/ws HTTP/1.1\r\nHost: {GOOD}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: typedeck.{}\r\n\r\n", s.token()).unwrap();
    let mut r = BufReader::new(c.try_clone().unwrap());
    let mut status = String::new();
    r.read_line(&mut status).unwrap();
    assert!(status.contains("101"), "{status}");
    loop {
        let mut l = String::new();
        r.read_line(&mut l).unwrap();
        if l.trim().is_empty() {
            break;
        }
    }
    let hello = crate::ws::read_frame(&mut r, 1 << 16, false).unwrap();
    assert!(String::from_utf8_lossy(&hello.payload).contains(r#""t":"hello""#));
    assert!(crate::sync::wait_until(Duration::from_secs(1), || s.hub.count() == 1));
    crate::ws::write_frame(&mut c, 1, br#"{"t":"editing","on":true}"#, Some([1, 2, 3, 4])).unwrap();
    assert!(crate::sync::wait_until(Duration::from_secs(1), || s.eng.editing()));
    s.eng.handle_event(crate::capture::Event::new(b'P', 0x04, 0));
    let mut got_key = false;
    for _ in 0..5 {
        let f = crate::ws::read_frame(&mut r, 1 << 16, false).unwrap();
        if String::from_utf8_lossy(&f.payload).contains(r#""t":"key""#) {
            got_key = true;
            break;
        }
    }
    assert!(got_key);
    let mut req = Request::new("POST", "/api/config", GOOD);
    req.headers.push(("x-token".into(), s.token().into()));
    req.headers.push(("x-tab".into(), "tab-a".into()));
    req.body = serde_json::to_vec(&Config::initial()).unwrap();
    assert_eq!(s.handle(&req).status, 200);
    let mut got_config = false;
    for _ in 0..10 {
        let f = crate::ws::read_frame(&mut r, 1 << 16, false).unwrap();
        let v: Value = serde_json::from_slice(&f.payload).unwrap_or_default();
        if v["t"] == "event" && v["ev"]["kind"] == "config" {
            assert_eq!(v["ev"]["name"], "tab-a");
            got_config = true;
            break;
        }
    }
    assert!(got_config);
    drop(c);
    drop(r);
    assert!(crate::sync::wait_until(Duration::from_secs(2), || s.hub.count() == 0 && !s.eng.editing()));
    let mut bad = TcpStream::connect(("127.0.0.1", port)).unwrap();
    write!(bad, "GET /api/ws HTTP/1.1\r\nHost: {GOOD}\r\nUpgrade: websocket\r\nSec-WebSocket-Key: x\r\nSec-WebSocket-Protocol: typedeck.wrong\r\n\r\n")
        .unwrap();
    let mut line = String::new();
    BufReader::new(bad).read_line(&mut line).unwrap();
    assert!(line.contains("403"));
}

#[test]
fn every_config_replacement_is_announced_with_its_origin() {
    let s = new_srv();
    let last = get(&s, "/api/events?since=0")["last_id"].as_i64().unwrap();
    let mut req = Request::new("POST", "/api/config", GOOD);
    req.headers.push(("x-token".into(), s.token().into()));
    req.headers.push(("x-tab".into(), "ab12<script>".into()));
    req.body = serde_json::to_vec(&Config::initial()).unwrap();
    assert_eq!(s.handle(&req).status, 200);
    assert_eq!(post(&s, "/api/backups/create", json!({})).status, 200);
    let name = get(&s, "/api/backups")[0]["name"].as_str().unwrap().to_string();
    assert_eq!(post(&s, "/api/backups/restore", json!({"name": name})).status, 200);
    assert_eq!(post(&s, "/api/config", json!({"version": 3})).status, 400);
    let evs = get(&s, &format!("/api/events?since={last}"));
    let names: Vec<&str> = evs["events"].as_array().unwrap().iter().filter(|e| e["kind"] == "config").map(|e| e["name"].as_str().unwrap_or("")).collect();
    assert_eq!(names, vec!["ab12script", ""]);
}
