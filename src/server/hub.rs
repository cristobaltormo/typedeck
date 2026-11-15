use std::io::{BufReader, Write};
use std::net::TcpStream;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use serde::Serialize;
use serde_json::{Value, json};

use super::{Request, Server, fail, http};
use crate::queue::Queue;
use crate::sync::{Flag, lock};
use crate::ws;

const MAX_CLIENTS: usize = 8;
const MAX_PAYLOAD: u64 = 8 << 10;
const READ_TIMEOUT: Duration = Duration::from_secs(70);
const PROTO_PREFIX: &str = "typedeck.";

pub struct Client {
    id: u64,
    out: Queue<Vec<u8>>,
    closed: Flag,
    editing: AtomicBool,
}

#[derive(Default)]
pub struct Hub {
    clients: Mutex<Vec<Arc<Client>>>,
    next: AtomicU64,
}

impl Hub {
    fn add(&self) -> Option<Arc<Client>> {
        let mut c = lock(&self.clients);
        if c.len() >= MAX_CLIENTS {
            return None;
        }
        let client =
            Arc::new(Client { id: self.next.fetch_add(1, Ordering::SeqCst), out: Queue::new(64), closed: Flag::new(), editing: AtomicBool::new(false) });
        c.push(client.clone());
        Some(client)
    }

    fn remove(&self, id: u64) {
        lock(&self.clients).retain(|c| c.id != id);
    }

    pub fn count(&self) -> usize {
        lock(&self.clients).len()
    }

    pub fn editing(&self) -> bool {
        lock(&self.clients).iter().any(|c| c.editing.load(Ordering::SeqCst))
    }

    pub fn broadcast(&self, v: &impl Serialize) {
        let Ok(b) = serde_json::to_vec(v) else { return };
        for c in lock(&self.clients).iter() {
            c.out.try_push(b.clone());
        }
    }
}

fn origin_ok(s: &Server, req: &Request) -> bool {
    let o = req.header("origin");
    o.is_empty() || o == format!("http://127.0.0.1:{}", s.port) || o == format!("http://localhost:{}", s.port)
}

fn token_ok(s: &Server, req: &Request) -> bool {
    req.header("sec-websocket-protocol")
        .split(',')
        .map(str::trim)
        .filter_map(|p| p.strip_prefix(PROTO_PREFIX))
        .any(|t| crate::crypto::constant_eq(t.as_bytes(), s.token.as_bytes()))
}

pub fn serve_ws(s: &Server, req: &Request, mut r: BufReader<TcpStream>, mut w: TcpStream) {
    let reject = |w: &mut TcpStream, code: u16, msg: &str| {
        let mut resp = fail(code, msg);
        super::security_headers(&mut resp);
        let _ = http::write_response(w, &resp, false, false);
    };
    if !s.host_ok(req) || !origin_ok(s, req) || !token_ok(s, req) {
        return reject(&mut w, 403, "token");
    }
    let key = req.header("sec-websocket-key");
    if !req.header("upgrade").eq_ignore_ascii_case("websocket") || key.is_empty() {
        return reject(&mut w, 400, "websocket");
    }
    let hello = format!(
        "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: {}\r\nSec-WebSocket-Protocol: {PROTO_PREFIX}{}\r\n\r\n",
        ws::accept_key(key),
        s.token
    );
    if w.write_all(hello.as_bytes()).is_err() {
        return;
    }
    let Some(client) = s.hub.add() else {
        let _ = ws::write_frame(&mut w, 8, &[0x03, 0xf1], None);
        return;
    };
    s.eng.set_editor_open(true);
    let (_, last) = s.eng.events_since(0, Duration::ZERO, &|| false);
    let writer = {
        let (c, mut w) = (client.clone(), w);
        let hello = serde_json::to_vec(&json!({"t": "hello", "last_id": last})).unwrap_or_default();
        crate::sync::spawn("ws-write", move || {
            let _ = w.set_write_timeout(Some(Duration::from_secs(5)));
            if ws::write_frame(&mut w, 1, &hello, None).is_err() {
                c.closed.set();
            }
            let mut idle = Duration::ZERO;
            while !c.closed.is_set() {
                match c.out.pop(Duration::from_millis(500)) {
                    Some(m) => {
                        idle = Duration::ZERO;
                        if ws::write_frame(&mut w, 1, &m, None).is_err() {
                            break;
                        }
                    }
                    None => {
                        idle += Duration::from_millis(500);
                        if idle >= Duration::from_secs(25) {
                            idle = Duration::ZERO;
                            if ws::write_frame(&mut w, 9, &[], None).is_err() {
                                break;
                            }
                        }
                    }
                }
            }
            c.closed.set();
            let _ = w.shutdown(std::net::Shutdown::Both);
        })
    };
    let _ = r.get_ref().set_read_timeout(Some(READ_TIMEOUT));
    while !client.closed.is_set() {
        let Ok(f) = ws::read_frame(&mut r, MAX_PAYLOAD, true) else { break };
        match f.op {
            8 => break,
            1 => {
                if let Ok(m) = serde_json::from_slice::<Value>(&f.payload)
                    && m["t"] == json!("editing")
                {
                    client.editing.store(m["on"].as_bool().unwrap_or(false), Ordering::SeqCst);
                    s.eng.set_editing(s.hub.editing());
                }
            }
            _ => {}
        }
    }
    client.closed.set();
    let _ = r.get_ref().shutdown(std::net::Shutdown::Both);
    if let Some(h) = writer {
        let _ = h.join();
    }
    s.hub.remove(client.id);
    s.eng.set_editor_open(s.hub.count() > 0);
    if client.editing.load(Ordering::SeqCst) {
        s.eng.set_editing(s.hub.editing());
    }
}
