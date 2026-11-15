mod handlers;
pub mod http;
mod hub;

use std::io::BufReader;
use std::net::{Shutdown, TcpListener, TcpStream};
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

use serde::Serialize;
use serde_json::json;

pub use http::{Request, Response};
use hub::Hub;

use crate::capture::Source;
use crate::config::Paths;
use crate::engine::Engine;

const MAX_CONNECTIONS: usize = 64;

pub struct Asset {
    pub path: &'static str,
    pub ctype: &'static str,
    pub etag: &'static str,
    pub data: &'static [u8],
}

mod assets {
    use super::Asset;
    include!(concat!(env!("OUT_DIR"), "/assets.rs"));
}

pub fn asset(path: &str) -> Option<&'static Asset> {
    assets::ASSETS.iter().find(|a| a.path == path)
}

pub struct Server {
    pub(crate) eng: Arc<Engine>,
    pub(crate) src: Arc<dyn Source>,
    pub(crate) paths: Paths,
    token: String,
    port: u16,
    pub(crate) hub: Arc<Hub>,
    connections: AtomicUsize,
}

fn security_headers(r: &mut Response) {
    r.set("X-Frame-Options", "DENY");
    r.set("X-Content-Type-Options", "nosniff");
    r.set("Referrer-Policy", "no-referrer");
    r.set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'");
}

pub fn json_response(code: u16, v: &impl Serialize) -> Response {
    match serde_json::to_vec(v) {
        Ok(body) => Response {
            status: code,
            headers: vec![("Content-Type".into(), "application/json; charset=utf-8".into()), ("Cache-Control".into(), "no-store".into())],
            body,
        },
        Err(_) => Response { status: 500, headers: vec![], body: br#"{"error":"json"}"#.to_vec() },
    }
}

pub fn fail(code: u16, msg: &str) -> Response {
    json_response(code, &json!({"error": msg}))
}

pub fn ok() -> Response {
    json_response(200, &json!({"ok": true}))
}

impl Server {
    pub fn new(eng: Arc<Engine>, src: Arc<dyn Source>, paths: Paths, port: u16) -> Result<Arc<Server>, String> {
        let mut tok = [0u8; 24];
        crate::crypto::random(&mut tok).map_err(|e| e.to_string())?;
        let s = Arc::new(Server {
            eng: eng.clone(),
            src,
            paths,
            token: crate::crypto::base64_url_raw(&tok),
            port,
            hub: Arc::new(Hub::default()),
            connections: AtomicUsize::new(0),
        });
        let hub = s.hub.clone();
        eng.set_plain_key(Box::new(move |u, down| hub.broadcast(&json!({"t": "key", "k": format!("{u:02X}"), "d": down}))));
        let (hub, weak) = (s.hub.clone(), Arc::downgrade(&eng));
        crate::sync::spawn("event-pump", move || {
            let mut last = weak.upgrade().map(|e| e.events_since(0, Duration::ZERO, &|| false).1).unwrap_or(0);
            loop {
                let Some(e) = weak.upgrade() else { return };
                let (evs, l) = e.events_since(last, Duration::from_secs(25), &|| false);
                drop(e);
                last = l;
                for ev in evs {
                    hub.broadcast(&json!({"t": "event", "ev": ev}));
                }
            }
        });
        Ok(s)
    }

    pub fn token(&self) -> &str {
        &self.token
    }

    pub fn port(&self) -> u16 {
        self.port
    }

    pub fn editor_url(&self) -> String {
        editor_url(self.port)
    }

    fn host_ok(&self, req: &Request) -> bool {
        let h = req.host();
        h == format!("127.0.0.1:{}", self.port) || h == format!("localhost:{}", self.port)
    }

    fn authed(&self, req: &Request) -> bool {
        self.host_ok(req) && crate::crypto::constant_eq(req.header("x-token").as_bytes(), self.token.as_bytes())
    }

    pub fn focus_editor(&self) -> bool {
        if self.hub.count() == 0 {
            return false;
        }
        self.hub.broadcast(&json!({"t": "focus"}));
        crate::platform::get().focus_editor(&self.editor_url())
    }

    pub fn handle(&self, req: &Request) -> Response {
        let path = http::percent_decode(&req.path);
        let method = if req.method == "HEAD" { "GET" } else { req.method.as_str() };
        let mut resp = match self.route(method, &path) {
            Some(h) if self.authed(req) => h(self, req),
            Some(_) => fail(403, "token"),
            None => match self.allowed(&path) {
                Some(allow) if self.host_ok(req) => {
                    let mut r = fail(405, "method not allowed");
                    r.set("Allow", allow);
                    r
                }
                _ => self.static_file(req, &path),
            },
        };
        security_headers(&mut resp);
        resp
    }

    fn static_file(&self, req: &Request, path: &str) -> Response {
        if !self.host_ok(req) {
            return fail(403, "host");
        }
        if req.method != "GET" && req.method != "HEAD" {
            return fail(405, "method not allowed");
        }
        let mut p = http::clean_path(path);
        if p.is_empty() {
            p = "index.html".into();
        }
        let Some(a) = asset(&p) else { return fail(404, "no") };
        if p == "index.html" {
            let body = String::from_utf8_lossy(a.data).replace("__TOKEN__", &self.token).into_bytes();
            return Response::new(200).header("Content-Type", a.ctype).header("Cache-Control", "no-store").with_body(body);
        }
        let r = Response::new(200).header("ETag", a.etag).header("Cache-Control", "no-cache");
        if req.header("if-none-match") == a.etag {
            return Response { status: 304, ..r };
        }
        r.header("Content-Type", a.ctype).with_body(a.data.to_vec())
    }

    pub fn serve(self: &Arc<Self>, ln: TcpListener) {
        for conn in ln.incoming() {
            let Ok(stream) = conn else { continue };
            if self.connections.load(Ordering::SeqCst) >= MAX_CONNECTIONS {
                let _ = stream.shutdown(Shutdown::Both);
                continue;
            }
            self.connections.fetch_add(1, Ordering::SeqCst);
            let slot = Slot(self.clone());
            crate::sync::spawn("http", move || {
                let _ = crate::sync::catching(|| slot.0.connection(stream));
                drop(slot);
            });
        }
    }

    fn connection(&self, stream: TcpStream) {
        let _ = stream.set_nodelay(true);
        let _ = stream.set_write_timeout(Some(Duration::from_secs(10)));
        let Ok(write) = stream.try_clone() else { return };
        let Ok(peek) = stream.try_clone() else { return };
        let peek = Arc::new(peek);
        let mut r = BufReader::new(stream);
        let mut w = write;
        loop {
            let mut req = match http::read_request(&mut r, Duration::from_secs(5), Duration::from_secs(10)) {
                Ok(req) => req,
                Err(http::ReadError::Closed) => return,
                Err(http::ReadError::Bad(code)) => {
                    let _ = http::write_response(&mut w, &fail(code, "bad request"), false, false);
                    return;
                }
            };
            if req.path == "/api/ws" && req.method == "GET" {
                let _ = r.get_ref().set_read_timeout(None);
                hub::serve_ws(self, &req, r, w);
                return;
            }
            let p = peek.clone();
            req.closed = Box::new(move || peer_closed(&p));
            let resp = crate::sync::catching(|| self.handle(&req)).unwrap_or_else(|_| fail(500, "internal error"));
            let keep = !req.wants_close();
            if http::write_response(&mut w, &resp, req.method == "HEAD", keep).is_err() || !keep {
                return;
            }
        }
    }
}

impl Response {
    pub fn with_body(mut self, body: Vec<u8>) -> Response {
        self.body = body;
        self
    }
}

struct Slot(Arc<Server>);

impl Drop for Slot {
    fn drop(&mut self) {
        self.0.connections.fetch_sub(1, Ordering::SeqCst);
    }
}

fn peer_closed(s: &TcpStream) -> bool {
    if s.set_nonblocking(true).is_err() {
        return false;
    }
    let mut b = [0u8; 1];
    let closed = matches!(s.peek(&mut b), Ok(0));
    let _ = s.set_nonblocking(false);
    closed
}

pub fn editor_url(port: u16) -> String {
    format!("http://127.0.0.1:{port}/")
}

pub fn listen() -> std::io::Result<(TcpListener, u16)> {
    let mut last = std::io::Error::from(std::io::ErrorKind::AddrInUse);
    let base = crate::app::DEFAULT_PORT;
    for p in base..base + 10 {
        match TcpListener::bind(("127.0.0.1", p)) {
            Ok(ln) => return Ok((ln, p)),
            Err(e) => last = e,
        }
    }
    Err(last)
}

#[cfg(test)]
mod tests;
