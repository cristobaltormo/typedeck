use std::time::Duration;

use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
    GetKeyboardLayout, INPUT, INPUT_0, INPUT_KEYBOARD, KEYBDINPUT, KEYEVENTF_EXTENDEDKEY, KEYEVENTF_KEYUP, KEYEVENTF_UNICODE, MapVirtualKeyW, SendInput,
    VkKeyScanExW, keybd_event,
};
use windows_sys::Win32::UI::WindowsAndMessaging::GetWindowThreadProcessId;

use super::front::foreground;
use crate::hid::{self, Stroke};
use crate::platform::{is_extended_vk, split_hotkey, vk_of};

pub fn vk_mod(m: &str) -> u16 {
    match m {
        "ctrl" => 0x11,
        "shift" => 0x10,
        "alt" => 0x12,
        "gui" => 0x5B,
        _ => 0xA5,
    }
}

pub fn key_event(vk: u16, up: bool) {
    let mut flags = if up { KEYEVENTF_KEYUP } else { 0 };
    if is_extended_vk(vk) {
        flags |= KEYEVENTF_EXTENDEDKEY;
    }
    let code = u32::from(vk & 0xFF);
    unsafe {
        let scan = MapVirtualKeyW(code, 0);
        keybd_event(code as u8, scan as u8, flags, 0);
    }
}

pub fn send_combo(mods: &[&str], vk: u16) -> Result<(), String> {
    if vk == 0 {
        return Err("key without a Windows equivalent".into());
    }
    for m in mods {
        key_event(vk_mod(m), false);
    }
    key_event(vk, false);
    std::thread::sleep(Duration::from_millis(10));
    key_event(vk, true);
    for m in mods.iter().rev() {
        key_event(vk_mod(m), true);
    }
    Ok(())
}

pub fn foreground_layout() -> isize {
    let (hwnd, _) = foreground();
    let tid = if hwnd.is_null() { 0 } else { unsafe { GetWindowThreadProcessId(hwnd, std::ptr::null_mut()) } };
    unsafe { GetKeyboardLayout(tid) as isize }
}

pub fn soft_hotkey(spec: &str) -> Result<(), String> {
    let (mut mods, key) = split_hotkey(spec)?;
    if let Some(u) = hid::key_usage(&key) {
        return send_combo(&mods, vk_of(u));
    }
    let mut chars = key.chars();
    let (Some(c), None) = (chars.next(), chars.next()) else { return Err(format!("unknown key: {key}")) };
    let mut buf = [0u16; 2];
    if c.encode_utf16(&mut buf).len() != 1 {
        return Err(format!("unknown key: {key}"));
    }
    let v = unsafe { VkKeyScanExW(buf[0], foreground_layout() as _) };
    if v == -1 {
        return Err(format!("unknown key: {key}"));
    }
    let v = v as u16;
    for (bit, m) in [(0x100, "shift"), (0x200, "ctrl"), (0x400, "alt")] {
        if v & bit != 0 && !mods.contains(&m) {
            mods.push(m);
        }
    }
    send_combo(&mods, v & 0xFF)
}

pub fn soft_stroke(st: Stroke) -> Result<(), String> {
    let mods: Vec<&str> = [(hid::MOD_CTRL, "ctrl"), (hid::MOD_SHIFT, "shift"), (hid::MOD_ALT, "alt"), (hid::MOD_GUI, "gui"), (hid::MOD_ALTGR, "altgr")]
        .iter()
        .filter(|(b, _)| st.mods & b != 0)
        .map(|(_, n)| *n)
        .collect();
    send_combo(&mods, vk_of(st.usage))
}

fn unicode_unit(unit: u16) {
    let mk = |flags| INPUT { r#type: INPUT_KEYBOARD, Anonymous: INPUT_0 { ki: KEYBDINPUT { wVk: 0, wScan: unit, dwFlags: flags, time: 0, dwExtraInfo: 0 } } };
    let inputs = [mk(KEYEVENTF_UNICODE), mk(KEYEVENTF_UNICODE | KEYEVENTF_KEYUP)];
    unsafe {
        SendInput(2, inputs.as_ptr(), std::mem::size_of::<INPUT>() as i32);
    }
}

pub fn type_unicode(text: &str) -> Result<(), String> {
    for c in text.chars() {
        match c {
            '\r' => {}
            '\n' => send_combo(&[], 0x0D)?,
            '\t' => send_combo(&[], 0x09)?,
            c => {
                let mut buf = [0u16; 2];
                for u in c.encode_utf16(&mut buf) {
                    unicode_unit(*u);
                }
            }
        }
        std::thread::sleep(Duration::from_millis(2));
    }
    Ok(())
}

pub fn soft_media(cmd: &str, step: i64) -> Result<String, String> {
    let vk = match cmd {
        "playpause" => 0xB3,
        "next" => 0xB0,
        "prev" => 0xB1,
        "volup" => 0xAF,
        "voldown" => 0xAE,
        "mute" => 0xAD,
        _ => return Err(format!("unknown media command: {cmd}")),
    };
    let n = if cmd == "volup" || cmd == "voldown" { (step / 2).max(1) } else { 1 };
    for _ in 0..n {
        key_event(vk, false);
        key_event(vk, true);
    }
    Ok(String::new())
}
