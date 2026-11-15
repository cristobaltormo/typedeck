use std::fs::File;
use std::io::Write;
use std::sync::Mutex;

use crate::sync::lock;

static SINK: Mutex<Option<File>> = Mutex::new(None);

pub fn to_file(f: File) {
    *lock(&SINK) = Some(f);
}

pub fn write(msg: &str) {
    let line = format!("{} {}\n", crate::clock::now_local().hms(), msg);
    let mut sink = lock(&SINK);
    match sink.as_mut() {
        Some(f) => {
            let _ = f.write_all(line.as_bytes());
        }
        None => {
            let _ = std::io::stderr().write_all(line.as_bytes());
        }
    }
}

#[macro_export]
macro_rules! log {
    ($($arg:tt)*) => {
        $crate::logging::write(&format!($($arg)*))
    };
}
