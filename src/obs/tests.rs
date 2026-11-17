use std::io::{BufRead, BufReader};
use std::net::TcpListener;
use std::sync::{Arc, Mutex};

use super::*;
use crate::sync::lock;

struct FakeObs {
    port: i64,
    scene: Arc<Mutex<String>>,
}

fn reply(w: &mut TcpStream, v: Value) {
    ws::write_frame(w, 1, &serde_json::to_vec(&v).unwrap(), None).unwrap();
}

fn read_client(r: &mut BufReader<TcpStream>) -> Option<Value> {
    let f = ws::read_frame(r, 1 << 20, true).ok()?;
    serde_json::from_slice(&f.payload).ok()
}

fn start(password: &str) -> FakeObs {
    let ln = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = i64::from(ln.local_addr().unwrap().port());
    let scene = Arc::new(Mutex::new("Juego".to_string()));
    let (pw, sc) = (password.to_string(), scene.clone());
    std::thread::spawn(move || {
        for c in ln.incoming().flatten() {
            let (pw, sc) = (pw.clone(), sc.clone());
            std::thread::spawn(move || serve(c, &pw, &sc));
        }
    });
    FakeObs { port, scene }
}

fn serve(c: TcpStream, password: &str, scene: &Mutex<String>) {
    let mut w = c.try_clone().unwrap();
    let mut r = BufReader::new(c);
    let mut key = String::new();
    loop {
        let mut line = String::new();
        if r.read_line(&mut line).unwrap_or(0) == 0 {
            return;
        }
        if line.trim().is_empty() {
            break;
        }
        if let Some(v) = line.strip_prefix("Sec-WebSocket-Key:") {
            key = v.trim().to_string();
        }
    }
    let _ =
        write!(w, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: {}\r\n\r\n", ws::accept_key(&key));
    let mut hello = json!({"rpcVersion": 1, "obsWebSocketVersion": "5.5.0"});
    if !password.is_empty() {
        hello["authentication"] = json!({"challenge": "chal", "salt": "salt"});
    }
    reply(&mut w, json!({"op": 0, "d": hello}));
    let Some(id) = read_client(&mut r) else { return };
    if !password.is_empty() {
        let secret = crypto::base64(&crypto::sha256(format!("{password}salt").as_bytes()));
        let want = crypto::base64(&crypto::sha256(format!("{secret}chal").as_bytes()));
        if id["d"]["authentication"] != json!(want) {
            return;
        }
    }
    reply(&mut w, json!({"op": 2, "d": {"negotiatedRpcVersion": 1}}));
    let mut muted = false;
    while let Some(m) = read_client(&mut r) {
        let typ = m["d"]["requestType"].as_str().unwrap_or("").to_string();
        let resp = match typ.as_str() {
            "GetSceneList" => {
                json!({"currentProgramSceneName": *lock(scene), "scenes": [{"sceneName": "Final"}, {"sceneName": "Charla"}, {"sceneName": "Juego"}]})
            }
            "SetCurrentProgramScene" => {
                *lock(scene) = m["d"]["requestData"]["sceneName"].as_str().unwrap_or("").to_string();
                json!({})
            }
            "ToggleStream" | "ToggleRecord" => json!({"outputActive": true}),
            "ToggleInputMute" => {
                muted = !muted;
                json!({"inputMuted": muted})
            }
            "GetSpecialInputs" => json!({"mic1": "Mic/Aux", "desktop1": "Escritorio"}),
            "GetVersion" => json!({"obsVersion": "32.2.2"}),
            "GetInputList" => json!({"inputs": [{"inputName": "Mic/Aux", "inputKind": "pulse_input_capture"}, {"inputName": "Cam", "inputKind": "v4l2"}]}),
            _ => json!({}),
        };
        reply(&mut w, json!({"op": 7, "d": {"requestType": typ, "requestId": typ, "requestStatus": {"result": true, "code": 100}, "responseData": resp}}));
    }
}

fn cfg(port: i64, password: &str) -> Config {
    Config { host: String::new(), port, password: password.into() }
}

#[test]
fn run_without_password() {
    let f = start("");
    let c = cfg(f.port, "");
    assert_eq!(run(&c, "stream", "").unwrap(), "live");
    assert_eq!(run(&c, "scene", "Charla").unwrap(), "Charla");
    assert_eq!(*lock(&f.scene), "Charla");
    assert!(run(&c, "mute", "Mic").unwrap().contains("muted"));
}

#[test]
fn scene_step_wraps() {
    let f = start("");
    let c = cfg(f.port, "");
    for want in ["Charla", "Final", "Juego"] {
        assert_eq!(run(&c, "scene_next", "").unwrap(), want);
    }
    assert_eq!(run(&c, "scene_prev", "").unwrap(), "Final");
}

#[test]
fn password() {
    let f = start("secreto");
    assert!(run(&cfg(f.port, "secreto"), "record", "").is_ok());
    assert!(run(&cfg(f.port, "mala"), "record", "").is_err());
    assert!(run(&cfg(f.port, ""), "record", "").unwrap_err().contains("password"));
}

#[test]
fn probe_and_errors() {
    let f = start("");
    let c = cfg(f.port, "");
    let info = probe(&c).unwrap();
    assert_eq!(info.version, "32.2.2");
    assert_eq!(info.scenes.as_deref(), Some(&["Juego".to_string(), "Charla".into(), "Final".into()][..]));
    assert_eq!(info.inputs, Some(vec!["Mic/Aux".to_string()]));
    assert!(run(&c, "scene", "").is_err());
    assert_eq!(run(&c, "scene", "#2").unwrap(), "Charla");
    assert!(run(&c, "scene", "#9").is_err());
    assert!(run(&c, "mute", "@desktop").unwrap().starts_with("Escritorio"));
    assert!(run(&c, "mute", "").unwrap().starts_with("Mic/Aux"));
    assert!(run(&c, "nope", "").is_err());
    assert!(active(&c, "stream").is_ok());
    let err = run(&cfg(1, ""), "stream", "").unwrap_err();
    assert!(err.starts_with("could not connect to OBS") && err.contains("WebSocket"), "{err}");
    assert_eq!(host_port("::1", 0), "[::1]:4455");
}
