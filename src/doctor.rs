use std::io::{Read, Write};
use std::net::TcpStream;
use std::time::Duration;

use serde_json::Value;

use crate::app::VERSION;
use crate::board::{Connector, SerialConnector};
use crate::capture::SOFTWARE;
use crate::config::{self, Paths};
use crate::platform;

pub fn editor_port() -> u16 {
    std::fs::read_to_string(Paths::default_paths().port_file()).ok().and_then(|s| s.trim().parse().ok()).unwrap_or(crate::app::DEFAULT_PORT)
}

pub fn editor_url() -> String {
    crate::server::editor_url(editor_port())
}

fn http_get(port: u16, path: &str, token: &str) -> Option<String> {
    let mut s = TcpStream::connect_timeout(&([127, 0, 0, 1], port).into(), Duration::from_secs(2)).ok()?;
    s.set_read_timeout(Some(Duration::from_secs(2))).ok()?;
    let mut req = format!("GET {path} HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nConnection: close\r\n");
    if !token.is_empty() {
        req.push_str(&format!("X-Token: {token}\r\n"));
    }
    req.push_str("\r\n");
    s.write_all(req.as_bytes()).ok()?;
    let mut out = Vec::new();
    let _ = s.read_to_end(&mut out);
    let text = String::from_utf8_lossy(&out).into_owned();
    let (_, body) = text.split_once("\r\n\r\n")?;
    Some(body.to_string())
}

pub struct Service {
    port: u16,
    token: Option<String>,
}

impl Service {
    pub fn find() -> Option<Service> {
        let port = editor_port();
        let page = http_get(port, "/", "")?;
        let token = page.split("name=\"token\" content=\"").nth(1).and_then(|r| r.split('"').next()).map(String::from);
        Some(Service { port, token })
    }

    pub fn get(&self, path: &str) -> Option<Value> {
        let token = self.token.as_deref()?;
        serde_json::from_str(&http_get(self.port, path, token)?).ok()
    }
}

struct Report {
    bad: usize,
}

impl Report {
    fn check(&mut self, ok: bool, ok_msg: &str, bad_msg: &str) {
        if ok {
            println!("  ok       {ok_msg}");
        } else if let Some(note) = bad_msg.strip_prefix("optional: ") {
            println!("  note     {note}");
        } else {
            println!("  FAIL     {bad_msg}");
            self.bad += 1;
        }
    }
}

fn board_checks(r: &mut Report) {
    match Service::find() {
        Some(svc) => {
            r.check(true, &format!("Typedeck is already running (editor at {})", editor_url()), "");
            let kb = svc.get("/api/keyboard");
            let connected = kb.as_ref().and_then(|k| k["connected"].as_bool()).unwrap_or(false);
            let info = kb.as_ref().map(|k| k["info"].clone()).unwrap_or(Value::Null);
            let fw = info["firmware"].as_str().unwrap_or("").to_string();
            r.check(connected, &format!("board connected, firmware {fw}"), "the service cannot see the board (cable or port)");
            let s = |k: &str| info[k].as_str().unwrap_or("").to_string();
            let keyboard = if info["present"].as_bool() == Some(true) {
                format!("{} ({}:{})", format!("{} {}", s("mfr"), s("prod")).trim(), s("vid"), s("pid"))
            } else {
                String::new()
            };
            r.check(!keyboard.is_empty(), &format!("keyboard detected: {keyboard}"), "the board sees no keyboard on the shield");
        }
        None => match SerialConnector::default().open() {
            Ok((port, name)) => {
                port.close();
                r.check(true, &format!("board found at {name}"), "");
            }
            Err(_) => r.check(false, "", "board not found (cable, port, or a running program using the port)"),
        },
    }
}

fn software_checks(r: &mut Report) {
    for c in crate::capture::new_software().checks() {
        r.check(c.ok, &c.message, &c.problem);
    }
    let Some(svc) = Service::find() else { return };
    r.check(true, &format!("Typedeck is already running (editor at {})", editor_url()), "");
    let mode = svc.get("/api/status").and_then(|s| s["capture"].as_str().map(String::from)).unwrap_or_default();
    if mode != SOFTWARE {
        r.check(false, "", "the running program still uses the board; restart it to apply the software mode");
        return;
    }
    let connected = svc.get("/api/keyboard").and_then(|k| k["connected"].as_bool()).unwrap_or(false);
    r.check(connected, "software capture is active in the running program", "the running program has no keyboard hook");
    let checks = svc.get("/api/setup").map(|s| s["checks"].clone()).unwrap_or(Value::Null);
    for c in checks.as_array().into_iter().flatten() {
        if c["level"] == "warn" && c["code"] == "grab_failed" {
            r.check(false, "", "the running program could not take one of the keyboards (another program holds it): keys typed there are not captured");
        }
    }
}

pub fn run() -> i32 {
    let mut r = Report { bad: 0 };
    println!("Typedeck {VERSION}");
    let paths = Paths::default_paths();
    let cfg = config::load(&paths);
    r.check(
        cfg.is_ok(),
        &format!("valid configuration in {}", paths.config().display()),
        &format!("configuration: {}", cfg.as_ref().err().cloned().unwrap_or_default()),
    );
    if platform::get().name() == "darwin" {
        let hud = crate::hud::binary();
        r.check(
            hud.is_some_and(|h| h.is_file()),
            "floating popup installed",
            &format!("the floating popup is missing ({}); notifications will be used", hud.map(|h| h.display().to_string()).unwrap_or_default()),
        );
    }
    for c in platform::get().doctor() {
        r.check(c.ok, &c.message, &c.problem);
    }
    if cfg.is_ok_and(|c| c.settings.capture == SOFTWARE) {
        software_checks(&mut r);
    } else {
        board_checks(&mut r);
    }
    if r.bad > 0 {
        println!("{} checks failed", r.bad);
        return 1;
    }
    println!("all good");
    0
}
