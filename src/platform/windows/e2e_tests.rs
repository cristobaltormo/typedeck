use std::path::PathBuf;
use std::time::Duration;

use windows_sys::Win32::System::DataExchange::{EmptyClipboard, IsClipboardFormatAvailable, RegisterClipboardFormatW, SetClipboardData};

use super::clipboard::{HISTORY, Open, alloc, exclude_from_history, set_text};
use super::front::{close_windows, top_windows, windows_of_image};
use super::reg::wide;
use super::*;

const CF_DIB: u32 = 8;

fn e2e_dir() -> Option<PathBuf> {
    std::env::var_os("TYPEDECK_E2E_DIR").map(PathBuf::from)
}

fn box_text(dir: &Path) -> String {
    std::fs::read_to_string(dir.join("testbox.txt")).unwrap_or_default().replace("\r\n", "\n").trim_start_matches('\u{feff}').to_string()
}

fn wait_box(dir: &Path, want: &str) -> String {
    let mut got = String::new();
    for _ in 0..20 {
        got = box_text(dir);
        if got == want {
            break;
        }
        std::thread::sleep(Duration::from_millis(250));
    }
    got
}

fn clear_box() {
    let _ = input::send_combo(&["ctrl"], u16::from(b'A'));
    let _ = input::send_combo(&[], 0x08);
    std::thread::sleep(Duration::from_millis(400));
}

fn has_format(f: u32) -> bool {
    let Some(_g) = Open::new(20) else { return false };
    unsafe { IsClipboardFormatAvailable(f) != 0 }
}

#[test]
#[ignore]
fn e2e_paste_text_restores_the_clipboard() {
    let Some(dir) = e2e_dir() else { return };
    clear_box();
    set_text("previous clipboard").unwrap();
    let want = "Hola, ¿qué tal? Ñandú\nsegunda línea";
    Windows.paste(want).unwrap();
    assert_eq!(wait_box(&dir, want), want);
    assert_eq!(Windows.clipboard(), "previous clipboard");
}

#[test]
#[ignore]
fn e2e_paste_keeps_an_image_on_the_clipboard() {
    let Some(dir) = e2e_dir() else { return };
    clear_box();
    let mut dib = vec![0u8; 56];
    dib[..4].copy_from_slice(&40u32.to_le_bytes());
    dib[4..8].copy_from_slice(&2u32.to_le_bytes());
    dib[8..12].copy_from_slice(&2u32.to_le_bytes());
    dib[12..14].copy_from_slice(&1u16.to_le_bytes());
    dib[14..16].copy_from_slice(&32u16.to_le_bytes());
    {
        let _g = Open::new(20).expect("clipboard");
        unsafe { EmptyClipboard() };
        let h = alloc(&dib).unwrap();
        assert!(!unsafe { SetClipboardData(CF_DIB, h) }.is_null());
    }
    let want = "typed without touching the clipboard";
    Windows.paste(want).unwrap();
    assert_eq!(wait_box(&dir, want), want);
    assert!(has_format(CF_DIB), "the image on the clipboard was lost");
}

#[test]
#[ignore]
fn e2e_paste_is_hidden_from_clipboard_history() {
    if e2e_dir().is_none() {
        return;
    }
    clear_box();
    {
        let _g = Open::new(20).expect("clipboard");
        unsafe { EmptyClipboard() };
        exclude_from_history();
    }
    for name in HISTORY {
        let n = wide(name);
        assert!(has_format(unsafe { RegisterClipboardFormatW(n.as_ptr()) }), "{name}");
    }
}

#[test]
#[ignore]
fn e2e_soft_hotkey_reaches_the_focused_window() {
    let Some(dir) = e2e_dir() else { return };
    clear_box();
    Windows.paste("abc").unwrap();
    wait_box(&dir, "abc");
    Windows.soft_hotkey("ctrl+a").unwrap();
    Windows.soft_hotkey("delete").unwrap();
    assert_eq!(wait_box(&dir, ""), "");
}

#[test]
#[ignore]
fn e2e_open_app_does_not_duplicate() {
    if e2e_dir().is_none() {
        return;
    }
    close_windows(&windows_of_image("charmap.exe"));
    std::thread::sleep(Duration::from_secs(1));
    for _ in 0..3 {
        Windows.open_app("charmap").unwrap();
        std::thread::sleep(Duration::from_millis(1500));
    }
    assert_eq!(windows_of_image("charmap.exe").len(), 1);
    Windows.quit_app("charmap").unwrap();
    std::thread::sleep(Duration::from_millis(1500));
    assert!(windows_of_image("charmap.exe").is_empty());
}

#[test]
#[ignore]
fn e2e_quit_app_ignores_titles_of_other_programs() {
    if e2e_dir().is_none() {
        return;
    }
    assert!(Windows.quit_app("test box").is_err());
    assert!(top_windows().iter().any(|w| w.title.contains("Typedeck test box")));
}

#[test]
#[ignore]
fn e2e_popup_replaces_the_previous_one() {
    if e2e_dir().is_none() {
        return;
    }
    for i in 0..3 {
        assert!(Windows.popup(&PopupOptions { title: format!("Layer {}", i + 1), seconds: 3.0, ..Default::default() }));
    }
    std::thread::sleep(Duration::from_millis(500));
    let n = super::front::all_windows_of_class("TypedeckPopup");
    assert_eq!(n, 1);
}

#[test]
#[ignore]
fn e2e_notify_balloon_needs_no_powershell() {
    if e2e_dir().is_none() {
        return;
    }
    start_shell(ShellHooks { status: Some(Box::new(|| "test".into())), ..Default::default() });
    std::thread::sleep(Duration::from_secs(1));
    assert!(shell::balloon("Typedeck", "balloon from the tray icon"));
}

#[test]
#[ignore]
fn e2e_system_layout_and_store_apps() {
    if e2e_dir().is_none() {
        return;
    }
    println!("layout: {:?}, HKL {:08X}", Windows.system_layout(), input::foreground_layout() as u32);
    for _ in 0..20 {
        if !apps::store_apps().is_empty() {
            break;
        }
        std::thread::sleep(Duration::from_secs(1));
    }
    assert!(!apps::store_apps().is_empty(), "no Microsoft Store apps found");
}
