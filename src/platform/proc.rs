use std::io::{Read, Write};
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use crate::sync::{Flag, lock};

const WAIT_DELAY: Duration = Duration::from_secs(2);

#[cfg(windows)]
pub fn hide_window(cmd: &mut Command) {
    use std::os::windows::process::CommandExt;
    cmd.creation_flags(0x08000000);
}

#[cfg(not(windows))]
pub fn hide_window(_cmd: &mut Command) {}

#[cfg(windows)]
fn decode_output(b: &[u8]) -> String {
    if let Ok(s) = std::str::from_utf8(b) {
        return s.to_string();
    }
    super::windows::decode_oem(b).unwrap_or_else(|| String::from_utf8_lossy(b).into_owned())
}

#[cfg(not(windows))]
fn decode_output(b: &[u8]) -> String {
    String::from_utf8_lossy(b).into_owned()
}

pub fn run(timeout: Duration, stdin: &str, name: &str, args: &[&str]) -> Result<String, String> {
    let mut cmd = super::command(name);
    cmd.args(args);
    run_command(cmd, timeout, stdin)
}

pub fn run_out(timeout: Duration, name: &str, args: &[&str]) -> String {
    run(timeout, "", name, args).unwrap_or_else(|e| e)
}

pub fn run_command(mut cmd: Command, timeout: Duration, stdin: &str) -> Result<String, String> {
    let (reader, writer) = std::io::pipe().map_err(|e| e.to_string())?;
    let writer2 = writer.try_clone().map_err(|e| e.to_string())?;
    cmd.stdout(writer).stderr(writer2);
    cmd.stdin(if stdin.is_empty() { Stdio::null() } else { Stdio::piped() });
    super::apply_env(&mut cmd);
    hide_window(&mut cmd);
    let mut child = cmd.spawn().map_err(|e| e.to_string())?;
    drop(cmd);
    if let Some(mut w) = child.stdin.take() {
        let data = stdin.as_bytes().to_vec();
        crate::sync::spawn("child-stdin", move || {
            let _ = w.write_all(&data);
        });
    }
    let shared: Arc<(Mutex<Vec<u8>>, Flag)> = Arc::new((Mutex::new(Vec::new()), Flag::new()));
    let sink = shared.clone();
    crate::sync::spawn("child-output", move || {
        let mut reader = reader;
        let mut chunk = [0u8; 4096];
        while let Ok(n) = reader.read(&mut chunk) {
            if n == 0 {
                break;
            }
            let mut buf = lock(&sink.0);
            if buf.len() < 1 << 20 {
                buf.extend_from_slice(&chunk[..n]);
            }
        }
        sink.1.set();
    });
    let deadline = Instant::now() + timeout;
    let mut timed_out = false;
    let status = loop {
        match child.try_wait() {
            Ok(Some(st)) => break Some(st),
            Ok(None) if Instant::now() >= deadline => {
                timed_out = true;
                let _ = child.kill();
                break child.wait().ok();
            }
            Ok(None) => std::thread::sleep(Duration::from_millis(5)),
            Err(_) => break None,
        }
    };
    shared.1.wait(WAIT_DELAY);
    let out = lock(&shared.0).clone();
    let out = decode_output(&out).trim().to_string();
    if timed_out {
        return Err(if out.is_empty() { format!("timed out ({}s)", timeout.as_secs()) } else { format!("{out} (timed out after {}s)", timeout.as_secs()) });
    }
    match status {
        Some(st) if st.success() => Ok(out),
        Some(st) => Err(if out.is_empty() { format!("exit status {}", st.code().unwrap_or(-1)) } else { out }),
        None => Err(if out.is_empty() { "the program could not be waited for".into() } else { out }),
    }
}

pub fn detached(mut cmd: Command) -> Result<(), String> {
    cmd.stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null());
    super::apply_env(&mut cmd);
    hide_window(&mut cmd);
    let child = cmd.spawn().map_err(|e| e.to_string())?;
    reap(child);
    Ok(())
}

pub fn launch(mut cmd: Command) -> Result<(), String> {
    cmd.stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null());
    super::apply_env(&mut cmd);
    hide_window(&mut cmd);
    let mut child = cmd.spawn().map_err(|e| e.to_string())?;
    let deadline = Instant::now() + Duration::from_secs(3);
    while Instant::now() < deadline {
        match child.try_wait() {
            Ok(Some(st)) if st.success() => return Ok(()),
            Ok(Some(st)) => return Err(format!("exit status {}", st.code().unwrap_or(-1))),
            Ok(None) => std::thread::sleep(Duration::from_millis(20)),
            Err(e) => return Err(e.to_string()),
        }
    }
    reap(child);
    Ok(())
}

#[cfg(unix)]
pub fn reap(child: Child) {
    use std::sync::{Condvar, Mutex, OnceLock};
    static Q: OnceLock<(Mutex<Vec<Child>>, Condvar)> = OnceLock::new();
    static STARTED: std::sync::Once = std::sync::Once::new();
    let q = Q.get_or_init(|| (Mutex::new(Vec::new()), Condvar::new()));
    crate::sync::lock(&q.0).push(child);
    q.1.notify_one();
    STARTED.call_once(|| {
        crate::sync::spawn("reaper", move || {
            let mut list = crate::sync::lock(&q.0);
            loop {
                list.retain_mut(|c| matches!(c.try_wait(), Ok(None)));
                let wait = if list.is_empty() { Duration::from_secs(3600) } else { Duration::from_secs(1) };
                list = crate::sync::wait_timeout(&q.1, list, wait);
            }
        });
    });
}

#[cfg(not(unix))]
pub fn reap(child: Child) {
    drop(child);
}
