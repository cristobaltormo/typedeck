use std::io::{BufRead, BufReader, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::time::Duration;

use serde::Serialize;
use serde_json::{Map, Value, json};

use crate::crypto;
use crate::ws;

#[derive(Clone, Debug, Default)]
pub struct Config {
    pub host: String,
    pub port: i64,
    pub password: String,
}

#[derive(Clone, Debug, Default, Serialize, PartialEq, Eq)]
pub struct Info {
    pub version: String,
    pub scenes: Option<Vec<String>>,
    pub inputs: Option<Vec<String>>,
}

struct Session {
    r: BufReader<TcpStream>,
    w: TcpStream,
}

enum CallError {
    Io(String),
    Request { code: i64, comment: String },
}

impl CallError {
    fn message(self) -> String {
        match self {
            CallError::Io(e) => e,
            CallError::Request { comment, code } if comment.is_empty() => format!("OBS could not do it (code {code})"),
            CallError::Request { comment, .. } => comment,
        }
    }
}

pub fn host_port(host: &str, port: i64) -> String {
    let host = if host.is_empty() { "127.0.0.1" } else { host };
    let port = if port == 0 { 4455 } else { port };
    if host.contains(':') && !host.starts_with('[') { format!("[{host}]:{port}") } else { format!("{host}:{port}") }
}

fn simplify(e: &std::io::Error) -> String {
    let s = e.to_string();
    let s = s.rsplit(": ").next().unwrap_or(&s).to_string();
    s.split(" (os error").next().unwrap_or(&s).to_lowercase()
}

fn dial(addr: &str, timeout: Duration) -> std::io::Result<Session> {
    let mut last = std::io::Error::from(std::io::ErrorKind::NotFound);
    let mut stream = None;
    for a in addr.to_socket_addrs()? {
        match TcpStream::connect_timeout(&a, timeout) {
            Ok(s) => {
                stream = Some(s);
                break;
            }
            Err(e) => last = e,
        }
    }
    let s = stream.ok_or(last)?;
    s.set_read_timeout(Some(timeout))?;
    s.set_write_timeout(Some(timeout))?;
    let mut key = [0u8; 16];
    crypto::random(&mut key)?;
    let key = crypto::base64(&key);
    let mut w = s.try_clone()?;
    write!(
        w,
        "GET / HTTP/1.1\r\nHost: {addr}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: obswebsocket.json\r\n\r\n"
    )?;
    let mut r = BufReader::new(s);
    let mut status = String::new();
    r.read_line(&mut status)?;
    let code = status.split_whitespace().nth(1).unwrap_or("0").to_string();
    let mut accept = String::new();
    loop {
        let mut line = String::new();
        if r.read_line(&mut line)? == 0 {
            break;
        }
        let line = line.trim_end();
        if line.is_empty() {
            break;
        }
        if let Some((k, v)) = line.split_once(':')
            && k.trim().eq_ignore_ascii_case("sec-websocket-accept")
        {
            accept = v.trim().to_string();
        }
    }
    if code != "101" || accept != ws::accept_key(&key) {
        return Err(std::io::Error::other(format!("the server did not accept the WebSocket (HTTP {code})")));
    }
    Ok(Session { r, w })
}

impl Session {
    fn read_text(&mut self) -> std::io::Result<Vec<u8>> {
        let mut msg = Vec::new();
        loop {
            let f = ws::read_frame(&mut self.r, 1 << 20, false)?;
            match f.op {
                0x8 => return Err(std::io::Error::from(std::io::ErrorKind::UnexpectedEof)),
                0x9 => ws::write_frame(&mut self.w, 0xA, &f.payload, Some(mask()))?,
                0xA => {}
                0x0 | 0x1 => {
                    msg.extend_from_slice(&f.payload);
                    if f.fin {
                        return Ok(msg);
                    }
                }
                op if op == 0x2 && f.fin => {}
                op => return Err(std::io::Error::other(format!("unexpected WebSocket frame ({op})"))),
            }
        }
    }

    fn send(&mut self, op: i64, d: Value) -> Result<(), String> {
        let b = serde_json::to_vec(&json!({"op": op, "d": d})).map_err(|e| e.to_string())?;
        ws::write_frame(&mut self.w, 1, &b, Some(mask())).map_err(|e| e.to_string())
    }

    fn call(&mut self, typ: &str, data: Option<Value>) -> Result<Map<String, Value>, CallError> {
        let mut req = json!({"requestType": typ, "requestId": typ});
        if let Some(d) = data {
            req["requestData"] = d;
        }
        self.send(6, req).map_err(CallError::Io)?;
        loop {
            let raw = self.read_text().map_err(|e| CallError::Io(e.to_string()))?;
            let Ok(m) = serde_json::from_slice::<Value>(&raw) else { continue };
            if m["op"] != json!(7) || m["d"]["requestId"] != json!(typ) {
                continue;
            }
            let status = &m["d"]["requestStatus"];
            if status["result"] != json!(true) {
                return Err(CallError::Request { code: status["code"].as_i64().unwrap_or(0), comment: status["comment"].as_str().unwrap_or("").to_string() });
            }
            return Ok(m["d"]["responseData"].as_object().cloned().unwrap_or_default());
        }
    }

    fn call_s(&mut self, typ: &str, data: Option<Value>) -> Result<Map<String, Value>, String> {
        self.call(typ, data).map_err(CallError::message)
    }
}

fn mask() -> [u8; 4] {
    crypto::random_u32().to_le_bytes()
}

fn connect(cfg: &Config) -> Result<Session, String> {
    let mut s = dial(&host_port(&cfg.host, cfg.port), Duration::from_secs(3))
        .map_err(|e| format!("could not connect to OBS ({}). Turn on the WebSocket server in Tools, WebSocket Server Settings", simplify(&e)))?;
    let raw = s.read_text().map_err(|e| e.to_string())?;
    let hello: Value = serde_json::from_slice(&raw).map_err(|_| "invalid answer from OBS".to_string())?;
    let mut id = json!({"rpcVersion": 1});
    if let Some(auth) = hello["d"].get("authentication").filter(|a| a.is_object()) {
        if cfg.password.is_empty() {
            return Err("OBS asks for a password: enter it in Settings".into());
        }
        let salt = auth["salt"].as_str().unwrap_or("");
        let challenge = auth["challenge"].as_str().unwrap_or("");
        let secret = crypto::base64(&crypto::sha256(format!("{}{salt}", cfg.password).as_bytes()));
        id["authentication"] = json!(crypto::base64(&crypto::sha256(format!("{secret}{challenge}").as_bytes())));
    }
    s.send(1, id)?;
    loop {
        match s.read_text() {
            Ok(raw) => {
                if serde_json::from_slice::<Value>(&raw).is_ok_and(|m| m["op"] == json!(2)) {
                    let _ = s.w.set_read_timeout(Some(Duration::from_secs(5)));
                    return Ok(s);
                }
            }
            Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof || e.kind() == std::io::ErrorKind::ConnectionReset => {
                return Err("OBS rejected the password".into());
            }
            Err(e) => return Err(e.to_string()),
        }
    }
}

fn on_off(v: Option<&Value>, on: &str, off: &str) -> String {
    if v.and_then(Value::as_bool) == Some(true) { on.into() } else { off.into() }
}

fn ignore_active(r: Result<Map<String, Value>, CallError>) -> Result<(), String> {
    match r {
        Ok(_) | Err(CallError::Request { code: 500 | 501, .. }) => Ok(()),
        Err(e) => Err(e.message()),
    }
}

fn scenes(s: &mut Session) -> Result<(Vec<String>, String), String> {
    let r = s.call_s("GetSceneList", None)?;
    let list = r.get("scenes").and_then(Value::as_array).cloned().unwrap_or_default();
    let cur = r.get("currentProgramSceneName").and_then(Value::as_str).unwrap_or("").to_string();
    if list.is_empty() {
        return Err("OBS has no scenes".into());
    }
    let names = list.iter().rev().map(|m| m["sceneName"].as_str().unwrap_or("").to_string()).collect();
    Ok((names, cur))
}

fn input_name(s: &mut Session, target: &str) -> Result<String, String> {
    let target = target.trim();
    if !target.is_empty() && !target.starts_with('@') {
        return Ok(target.into());
    }
    let key = match target {
        "" | "@mic" => "mic1",
        "@desktop" => "desktop1",
        _ => return Err(format!("unknown source: {target}")),
    };
    let r = s.call_s("GetSpecialInputs", None)?;
    match r.get(key).and_then(Value::as_str) {
        Some(n) if !n.is_empty() => Ok(n.into()),
        _ => Err("OBS has no such audio source configured".into()),
    }
}

pub fn run(cfg: &Config, cmd: &str, target: &str) -> Result<String, String> {
    let mut s = connect(cfg)?;
    match cmd {
        "scene" => {
            let mut target = target.trim().to_string();
            if target.is_empty() {
                return Err("the scene name is missing".into());
            }
            if let Some(n) = target.strip_prefix('#').and_then(|n| n.parse::<i64>().ok()) {
                let (names, _) = scenes(&mut s)?;
                if n < 1 || n as usize > names.len() {
                    return Err(format!("OBS only has {} scenes", names.len()));
                }
                target = names[n as usize - 1].clone();
            }
            s.call_s("SetCurrentProgramScene", Some(json!({"sceneName": target})))?;
            Ok(target)
        }
        "scene_next" | "scene_prev" => {
            let (names, cur) = scenes(&mut s)?;
            let mut idx = names.iter().rposition(|n| *n == cur).unwrap_or(0);
            idx = if cmd == "scene_next" { (idx + 1) % names.len() } else { (idx + names.len() - 1) % names.len() };
            s.call_s("SetCurrentProgramScene", Some(json!({"sceneName": names[idx]})))?;
            Ok(names[idx].clone())
        }
        "stream" => s.call_s("ToggleStream", None).map(|r| on_off(r.get("outputActive"), "live", "stream stopped")),
        "stream_start" => ignore_active(s.call("StartStream", None)).map(|_| "live".into()),
        "stream_stop" => ignore_active(s.call("StopStream", None)).map(|_| "stream stopped".into()),
        "record" => s.call_s("ToggleRecord", None).map(|r| on_off(r.get("outputActive"), "recording", "recording stopped")),
        "record_pause" => s.call_s("ToggleRecordPause", None).map(|_| String::new()),
        "mute" => {
            let name = input_name(&mut s, target)?;
            let r = s.call_s("ToggleInputMute", Some(json!({"inputName": name})))?;
            Ok(format!("{name}: {}", on_off(r.get("inputMuted"), "muted", "unmuted")))
        }
        "replay_save" => s.call_s("SaveReplayBuffer", None).map(|_| "replay saved".into()),
        "virtualcam" => s.call_s("ToggleVirtualCam", None).map(|r| on_off(r.get("outputActive"), "virtual camera on", "virtual camera off")),
        "studio" => {
            let r = s.call_s("GetStudioModeEnabled", None)?;
            let on = r.get("studioModeEnabled").and_then(Value::as_bool).unwrap_or(false);
            s.call_s("SetStudioModeEnabled", Some(json!({"studioModeEnabled": !on})))?;
            Ok(if on { "studio mode off" } else { "studio mode on" }.into())
        }
        "studio_transition" => s.call_s("TriggerStudioModeTransition", None).map(|_| String::new()),
        _ => Err(format!("unknown OBS command: {cmd}")),
    }
}

pub fn active(cfg: &Config, what: &str) -> Result<bool, String> {
    let mut s = connect(cfg)?;
    let r = s.call_s(if what == "record" { "GetRecordStatus" } else { "GetStreamStatus" }, None)?;
    Ok(r.get("outputActive").and_then(Value::as_bool).unwrap_or(false))
}

pub fn probe(cfg: &Config) -> Result<Info, String> {
    let mut s = connect(cfg)?;
    let mut info = Info::default();
    if let Ok(r) = s.call_s("GetVersion", None) {
        info.version = r.get("obsVersion").and_then(Value::as_str).unwrap_or("").into();
    }
    if let Ok(r) = s.call_s("GetSceneList", None) {
        let names: Vec<String> = r
            .get("scenes")
            .and_then(Value::as_array)
            .into_iter()
            .flatten()
            .rev()
            .filter_map(|m| m["sceneName"].as_str())
            .filter(|n| !n.is_empty())
            .map(String::from)
            .collect();
        if !names.is_empty() {
            info.scenes = Some(names);
        }
    }
    if let Ok(r) = s.call_s("GetInputList", None) {
        let audio = ["audio", "wasapi", "pulse", "coreaudio", "alsa"];
        let names: Vec<String> = r
            .get("inputs")
            .and_then(Value::as_array)
            .into_iter()
            .flatten()
            .filter(|m| m["inputKind"].as_str().is_some_and(|k| audio.iter().any(|a| k.contains(a))))
            .filter_map(|m| m["inputName"].as_str())
            .filter(|n| !n.is_empty())
            .map(String::from)
            .collect();
        if !names.is_empty() {
            info.inputs = Some(names);
        }
    }
    Ok(info)
}

#[cfg(test)]
mod tests;
