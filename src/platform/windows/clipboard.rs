use std::sync::Mutex;
use std::time::Duration;

use windows_sys::Win32::Foundation::{HANDLE, HWND};
use windows_sys::Win32::System::DataExchange::{
    CloseClipboard, EmptyClipboard, EnumClipboardFormats, GetClipboardData, OpenClipboard, RegisterClipboardFormatW, SetClipboardData,
};
use windows_sys::Win32::System::Memory::{GMEM_MOVEABLE, GlobalAlloc, GlobalLock, GlobalSize, GlobalUnlock};

use super::input::{send_combo, type_unicode};
use super::reg::wide;
use super::shell::owner;
use crate::sync::{Flag, lock};

const CF_TEXT: u32 = 1;
const CF_OEMTEXT: u32 = 7;
pub const CF_UNICODETEXT: u32 = 13;
const CF_LOCALE: u32 = 16;
pub(super) const HISTORY: [&str; 3] = ["ExcludeClipboardContentFromMonitorProcessing", "CanIncludeInClipboardHistory", "CanUploadToCloudClipboard"];

struct Pending {
    text: String,
    done: std::sync::Arc<Flag>,
}

static PENDING: Mutex<Option<Pending>> = Mutex::new(None);

pub(super) struct Open;

impl Open {
    pub(super) fn new(tries: u32) -> Option<Open> {
        let o = owner() as HWND;
        for _ in 0..tries {
            if unsafe { OpenClipboard(o) } != 0 {
                return Some(Open);
            }
            std::thread::sleep(Duration::from_millis(15));
        }
        None
    }
}

impl Drop for Open {
    fn drop(&mut self) {
        unsafe {
            CloseClipboard();
        }
    }
}

fn is_plain_text() -> bool {
    let mut f = 0u32;
    loop {
        f = unsafe { EnumClipboardFormats(f) };
        match f {
            0 => return true,
            CF_TEXT | CF_OEMTEXT | CF_UNICODETEXT | CF_LOCALE => {}
            _ => return false,
        }
    }
}

fn read_unicode() -> String {
    let h = unsafe { GetClipboardData(CF_UNICODETEXT) };
    if h.is_null() {
        return String::new();
    }
    let size = unsafe { GlobalSize(h) };
    let p = unsafe { GlobalLock(h) } as *const u16;
    if p.is_null() {
        return String::new();
    }
    let units = unsafe { std::slice::from_raw_parts(p, size / 2) };
    let end = units.iter().position(|c| *c == 0).unwrap_or(units.len());
    let s = String::from_utf16_lossy(&units[..end]);
    unsafe {
        GlobalUnlock(h);
    }
    s
}

pub fn clipboard() -> String {
    match Open::new(10) {
        Some(_g) => read_unicode(),
        None => String::new(),
    }
}

pub(super) fn alloc(data: &[u8]) -> Result<HANDLE, String> {
    let h = unsafe { GlobalAlloc(GMEM_MOVEABLE, data.len()) };
    if h.is_null() {
        return Err("out of memory for the clipboard".into());
    }
    let p = unsafe { GlobalLock(h) } as *mut u8;
    if p.is_null() {
        return Err("could not allocate the clipboard".into());
    }
    unsafe {
        std::ptr::copy_nonoverlapping(data.as_ptr(), p, data.len());
        GlobalUnlock(h);
    }
    Ok(h)
}

fn put_unicode(text: &str) -> Result<(), String> {
    let bytes: Vec<u8> = wide(text).into_iter().flat_map(u16::to_le_bytes).collect();
    let h = alloc(&bytes)?;
    if unsafe { SetClipboardData(CF_UNICODETEXT, h) }.is_null() {
        return Err("could not write to the clipboard".into());
    }
    Ok(())
}

pub(super) fn exclude_from_history() {
    for name in HISTORY {
        let n = wide(name);
        let id = unsafe { RegisterClipboardFormatW(n.as_ptr()) };
        if id != 0
            && let Ok(h) = alloc(&[0u8; 4])
        {
            unsafe {
                SetClipboardData(id, h);
            }
        }
    }
}

pub(super) fn set_text(text: &str) -> Result<(), String> {
    let _g = Open::new(20).ok_or("could not open the clipboard")?;
    unsafe {
        EmptyClipboard();
    }
    if text.is_empty() {
        return Ok(());
    }
    put_unicode(text)?;
    exclude_from_history();
    Ok(())
}

pub fn render_paste() {
    let p = lock(&PENDING).as_ref().map(|p| (p.text.clone(), p.done.clone()));
    if let Some((text, done)) = p {
        let _ = put_unicode(&text);
        done.set();
    }
}

fn snapshot() -> Option<(String, bool)> {
    let _g = Open::new(20)?;
    if !is_plain_text() {
        return Some((String::new(), false));
    }
    Some((read_unicode(), true))
}

pub fn paste(text: &str) -> Result<(), String> {
    let snap = snapshot();
    let Some((old, true)) = snap.clone().filter(|_| owner() != 0) else {
        if let Some((_, false)) = snap {
            return type_unicode(text);
        }
        let old = snap.map(|s| s.0).unwrap_or_default();
        set_text(text)?;
        let r = send_combo(&["ctrl"], u16::from(b'V'));
        std::thread::sleep(Duration::from_millis(300));
        let _ = set_text(&old);
        return r;
    };
    let done = std::sync::Arc::new(Flag::new());
    *lock(&PENDING) = Some(Pending { text: text.to_string(), done: done.clone() });
    let r = (|| {
        {
            let _g = Open::new(20).ok_or("could not open the clipboard")?;
            unsafe {
                EmptyClipboard();
                SetClipboardData(CF_UNICODETEXT, std::ptr::null_mut());
            }
            exclude_from_history();
        }
        send_combo(&["ctrl"], u16::from(b'V'))?;
        if done.wait(Duration::from_secs(2)) {
            std::thread::sleep(Duration::from_millis(150));
        }
        Ok::<(), String>(())
    })();
    *lock(&PENDING) = None;
    r?;
    set_text(&old)
}
