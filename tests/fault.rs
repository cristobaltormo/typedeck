use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

fn run_with_fault(fault: &str) -> Option<i32> {
    let home = PathBuf::from(env!("CARGO_TARGET_TMPDIR")).join(format!("fault-{fault}-{}", std::process::id()));
    std::fs::create_dir_all(&home).unwrap();
    let mut child = Command::new(env!("CARGO_BIN_EXE_typedeck"))
        .env("TYPEDECK_FAULT", fault)
        .env("TYPEDECK_PORT", home.join("no-such-port"))
        .env("HOME", &home)
        .env("XDG_CONFIG_HOME", home.join("cfg"))
        .env("APPDATA", home.join("roaming"))
        .env("LOCALAPPDATA", home.join("local"))
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let deadline = Instant::now() + Duration::from_secs(15);
    let code = loop {
        if let Some(st) = child.try_wait().unwrap() {
            break st.code();
        }
        if Instant::now() > deadline {
            let _ = child.kill();
            let _ = child.wait();
            break None;
        }
        std::thread::sleep(Duration::from_millis(50));
    };
    let _ = std::fs::remove_dir_all(&home);
    code
}

#[cfg(debug_assertions)]
#[test]
fn a_panic_in_a_worker_thread_ends_the_process() {
    assert_eq!(run_with_fault("engine"), Some(70));
}
