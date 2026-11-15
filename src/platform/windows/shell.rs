use std::sync::atomic::{AtomicIsize, AtomicU32, Ordering};
use std::sync::{Mutex, OnceLock};

use windows_sys::Win32::Foundation::{HWND, LPARAM, LRESULT, POINT, WPARAM};
use windows_sys::Win32::System::LibraryLoader::GetModuleHandleW;
use windows_sys::Win32::System::RemoteDesktop::WTSRegisterSessionNotification;
use windows_sys::Win32::UI::Shell::{NIF_ICON, NIF_INFO, NIF_MESSAGE, NIF_TIP, NIM_ADD, NIM_DELETE, NIM_MODIFY, NOTIFYICONDATAW, Shell_NotifyIconW};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    AppendMenuW, CreatePopupMenu, CreateWindowExW, DefWindowProcW, DestroyMenu, DispatchMessageW, GetCursorPos, GetMessageW, GetSystemMetrics, IDI_APPLICATION,
    LoadIconW, LoadImageW, MF_GRAYED, MF_SEPARATOR, MSG, PostMessageW, PostQuitMessage, RegisterClassExW, RegisterWindowMessageW, SM_CXSMICON, SM_CYSMICON,
    SW_SHOWNA, SetForegroundWindow, SetTimer, ShowWindow, TPM_BOTTOMALIGN, TPM_RIGHTBUTTON, TrackPopupMenu, WM_CLOSE, WM_COMMAND, WM_DESTROY, WM_ENDSESSION,
    WM_POWERBROADCAST, WM_QUERYENDSESSION, WM_RENDERFORMAT, WM_TIMER, WM_WTSSESSION_CHANGE, WNDCLASSEXW, WS_EX_NOACTIVATE, WS_EX_TOOLWINDOW, WS_POPUP,
};

use super::clipboard::{CF_UNICODETEXT, render_paste};
use super::reg::wide;
use crate::platform::ShellHooks;
use crate::sync::lock;

const WM_TRAY: u32 = 0x8001;
const MENU_OPEN: usize = 1;
const MENU_RESTART: usize = 2;
const MENU_QUIT: usize = 3;

static HOOKS: OnceLock<ShellHooks> = OnceLock::new();
static HWND_SHELL: AtomicIsize = AtomicIsize::new(0);
static TASKBAR_MSG: AtomicU32 = AtomicU32::new(0);
static ICON: AtomicIsize = AtomicIsize::new(0);
static TIP: Mutex<String> = Mutex::new(String::new());

pub fn owner() -> isize {
    HWND_SHELL.load(Ordering::SeqCst)
}

pub fn class_name() -> &'static str {
    "TypedeckShell"
}

pub fn start(h: ShellHooks) {
    let _ = HOOKS.set(h);
    let _ = std::thread::Builder::new().name("shell".into()).spawn(shell_loop);
}

fn status_text() -> String {
    match HOOKS.get().and_then(|h| h.status.as_ref()).map(|f| f()) {
        Some(s) if !s.is_empty() => format!("Typedeck - {s}"),
        _ => "Typedeck".into(),
    }
}

fn copy_into(dst: &mut [u16], s: &str) {
    let w: Vec<u16> = s.encode_utf16().take(dst.len() - 1).collect();
    dst[..w.len()].copy_from_slice(&w);
    dst[w.len()] = 0;
}

fn tray_data(hwnd: HWND, flags: u32) -> NOTIFYICONDATAW {
    let mut d: NOTIFYICONDATAW = unsafe { std::mem::zeroed() };
    d.cbSize = std::mem::size_of::<NOTIFYICONDATAW>() as u32;
    d.hWnd = hwnd;
    d.uID = 1;
    d.uFlags = flags;
    d
}

fn add_tray(hwnd: HWND) {
    let mut d = tray_data(hwnd, NIF_MESSAGE | NIF_ICON | NIF_TIP);
    d.uCallbackMessage = WM_TRAY;
    d.hIcon = ICON.load(Ordering::SeqCst) as _;
    let tip = status_text();
    copy_into(&mut d.szTip, &tip);
    *lock(&TIP) = tip;
    unsafe {
        Shell_NotifyIconW(NIM_ADD, &d);
    }
}

fn refresh_tip(hwnd: HWND) {
    let tip = status_text();
    if *lock(&TIP) == tip {
        return;
    }
    let mut d = tray_data(hwnd, NIF_TIP);
    copy_into(&mut d.szTip, &tip);
    *lock(&TIP) = tip;
    unsafe {
        Shell_NotifyIconW(NIM_MODIFY, &d);
    }
}

pub fn balloon(title: &str, body: &str) -> bool {
    let hwnd = owner() as HWND;
    if hwnd.is_null() {
        return false;
    }
    let mut d = tray_data(hwnd, NIF_INFO);
    d.dwInfoFlags = 1;
    copy_into(&mut d.szInfoTitle, title);
    copy_into(&mut d.szInfo, body);
    unsafe { Shell_NotifyIconW(NIM_MODIFY, &d) != 0 }
}

fn show_menu(hwnd: HWND) {
    let menu = unsafe { CreatePopupMenu() };
    if menu.is_null() {
        return;
    }
    let add = |flags: u32, id: usize, text: &str| {
        let t = wide(text);
        unsafe {
            AppendMenuW(menu, flags, id, t.as_ptr());
        }
    };
    add(MF_GRAYED, 0, &status_text());
    add(MF_SEPARATOR, 0, "");
    add(0, MENU_OPEN, "Open editor");
    add(0, MENU_RESTART, "Restart");
    add(0, MENU_QUIT, "Quit");
    let mut pt = POINT { x: 0, y: 0 };
    unsafe {
        GetCursorPos(&mut pt);
        SetForegroundWindow(hwnd);
        TrackPopupMenu(menu, TPM_BOTTOMALIGN | TPM_RIGHTBUTTON, pt.x, pt.y, 0, hwnd, std::ptr::null());
        PostMessageW(hwnd, 0, 0, 0);
        DestroyMenu(menu);
    }
}

fn shutdown_and_exit() -> ! {
    if let Some(f) = HOOKS.get().and_then(|h| h.shutdown.as_ref()) {
        f();
    }
    std::process::exit(0);
}

fn restart() {
    let Ok(exe) = std::env::current_exe() else { return };
    if let Some(f) = HOOKS.get().and_then(|h| h.shutdown.as_ref()) {
        f();
    }
    super::release_instance();
    let mut c = crate::platform::command(&exe.display().to_string());
    c.arg("--background");
    let _ = crate::platform::detached(c);
    std::process::exit(0);
}

fn on_command(id: usize) {
    match id & 0xFFFF {
        MENU_OPEN => {
            if HOOKS.get().is_some_and(|h| h.open_editor.is_some()) {
                crate::sync::spawn("open-editor", || {
                    if let Some(f) = HOOKS.get().and_then(|h| h.open_editor.as_ref()) {
                        f();
                    }
                });
            }
        }
        MENU_RESTART => restart(),
        MENU_QUIT => shutdown_and_exit(),
        _ => {}
    }
}

fn release() {
    if let Some(f) = HOOKS.get().and_then(|h| h.release.as_ref()) {
        f();
    }
}

fn reclaim() {
    if let Some(f) = HOOKS.get().and_then(|h| h.reclaim.as_ref()) {
        f();
    }
}

unsafe extern "system" fn proc(hwnd: HWND, m: u32, wp: WPARAM, lp: LPARAM) -> LRESULT {
    match m {
        WM_CLOSE => shutdown_and_exit(),
        WM_QUERYENDSESSION => return 1,
        WM_ENDSESSION => {
            if wp != 0 {
                shutdown_and_exit();
            }
            return 0;
        }
        WM_COMMAND => {
            on_command(wp);
            return 0;
        }
        WM_RENDERFORMAT => {
            if wp as u32 == CF_UNICODETEXT {
                render_paste();
            }
            return 0;
        }
        WM_WTSSESSION_CHANGE => {
            match wp {
                7 | 2 | 4 => release(),
                8 | 1 | 3 => reclaim(),
                _ => {}
            }
            return 0;
        }
        WM_POWERBROADCAST => {
            match wp {
                0x4 => release(),
                0x7 | 0x12 => reclaim(),
                _ => {}
            }
            return 1;
        }
        WM_TIMER => {
            refresh_tip(hwnd);
            return 0;
        }
        WM_TRAY => {
            match (lp & 0xFFFF) as u32 {
                0x0205 => show_menu(hwnd),
                0x0203 => on_command(MENU_OPEN),
                _ => {}
            }
            return 0;
        }
        WM_DESTROY => {
            let d = tray_data(hwnd, 0);
            unsafe {
                Shell_NotifyIconW(NIM_DELETE, &d);
                PostQuitMessage(0);
            }
            return 0;
        }
        _ => {}
    }
    let tb = TASKBAR_MSG.load(Ordering::SeqCst);
    if tb != 0 && m == tb {
        add_tray(hwnd);
        return 0;
    }
    unsafe { DefWindowProcW(hwnd, m, wp, lp) }
}

fn shell_loop() {
    let inst = unsafe { GetModuleHandleW(std::ptr::null()) };
    let class = wide(class_name());
    let mut wc: WNDCLASSEXW = unsafe { std::mem::zeroed() };
    wc.cbSize = std::mem::size_of::<WNDCLASSEXW>() as u32;
    wc.lpfnWndProc = Some(proc);
    wc.hInstance = inst;
    wc.lpszClassName = class.as_ptr();
    let title = wide(&super::shell_title());
    let hwnd = unsafe {
        RegisterClassExW(&wc);
        CreateWindowExW(
            WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE,
            class.as_ptr(),
            title.as_ptr(),
            WS_POPUP,
            0,
            0,
            0,
            0,
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            inst,
            std::ptr::null(),
        )
    };
    if hwnd.is_null() {
        return;
    }
    let tb = wide("TaskbarCreated");
    unsafe {
        ShowWindow(hwnd, SW_SHOWNA);
        TASKBAR_MSG.store(RegisterWindowMessageW(tb.as_ptr()), Ordering::SeqCst);
        WTSRegisterSessionNotification(hwnd, 0);
        let (cx, cy) = (GetSystemMetrics(SM_CXSMICON), GetSystemMetrics(SM_CYSMICON));
        let mut icon = LoadImageW(inst, 1 as _, 1, cx, cy, 0);
        if icon.is_null() {
            icon = LoadIconW(std::ptr::null_mut(), IDI_APPLICATION);
        }
        ICON.store(icon as isize, Ordering::SeqCst);
    }
    HWND_SHELL.store(hwnd as isize, Ordering::SeqCst);
    add_tray(hwnd);
    unsafe {
        SetTimer(hwnd, 1, 5000, None);
    }
    let mut m: MSG = unsafe { std::mem::zeroed() };
    while unsafe { GetMessageW(&mut m, std::ptr::null_mut(), 0, 0) } > 0 {
        unsafe {
            DispatchMessageW(&m);
        }
    }
}
