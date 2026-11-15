use std::collections::HashMap;
use std::sync::atomic::{AtomicIsize, Ordering};
use std::sync::{Mutex, Once, mpsc};
use std::time::Duration;

use windows_sys::Win32::Foundation::{HWND, LPARAM, LRESULT, RECT, WPARAM};
use windows_sys::Win32::Graphics::Gdi::{
    BeginPaint, CreateFontW, CreatePen, CreateRoundRectRgn, CreateSolidBrush, DT_CENTER, DT_END_ELLIPSIS, DT_SINGLELINE, DT_VCENTER, DeleteObject, DrawTextW,
    Ellipse, EndPaint, FillRect, GetMonitorInfoW, HDC, MONITOR_DEFAULTTONEAREST, MONITORINFO, MonitorFromWindow, PAINTSTRUCT, PS_NULL, SelectObject, SetBkMode,
    SetTextColor, SetWindowRgn, TRANSPARENT,
};
use windows_sys::Win32::System::LibraryLoader::GetModuleHandleW;
use windows_sys::Win32::UI::HiDpi::{GetDpiForMonitor, GetDpiForWindow, MDT_EFFECTIVE_DPI};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    CreateWindowExW, DefWindowProcW, DestroyWindow, DispatchMessageW, GetClientRect, GetForegroundWindow, GetMessageW, GetSystemMetrics, IDC_ARROW, KillTimer,
    LWA_ALPHA, LoadCursorW, MSG, PostMessageW, PostQuitMessage, RegisterClassExW, SW_SHOWNOACTIVATE, SetLayeredWindowAttributes, SetTimer, ShowWindow,
    WM_CLOSE, WM_DESTROY, WM_PAINT, WM_TIMER, WNDCLASSEXW, WS_EX_LAYERED, WS_EX_NOACTIVATE, WS_EX_TOOLWINDOW, WS_EX_TOPMOST, WS_EX_TRANSPARENT, WS_POPUP,
};

use super::reg::wide;
use crate::platform::PopupOptions;
use crate::sync::lock;

static TEXTS: Mutex<Option<HashMap<isize, PopupOptions>>> = Mutex::new(None);
static CURRENT: AtomicIsize = AtomicIsize::new(0);
static REGISTER: Once = Once::new();
const CLASS: &str = "TypedeckPopup";

fn rgb(r: u8, g: u8, b: u8) -> u32 {
    u32::from(r) | u32::from(g) << 8 | u32::from(b) << 16
}

pub fn parse_hex(c: &str) -> Option<(u8, u8, u8)> {
    let c = c.strip_prefix('#').unwrap_or(c);
    if c.len() != 6 {
        return None;
    }
    let v = u32::from_str_radix(c, 16).ok()?;
    Some(((v >> 16) as u8, (v >> 8) as u8, v as u8))
}

fn window_scale(hwnd: HWND) -> f64 {
    let d = unsafe { GetDpiForWindow(hwnd) };
    if d >= 96 { f64::from(d) / 96.0 } else { 1.0 }
}

fn active_monitor() -> (RECT, f64) {
    unsafe {
        let mon = MonitorFromWindow(GetForegroundWindow(), MONITOR_DEFAULTTONEAREST);
        let mut mi: MONITORINFO = std::mem::zeroed();
        mi.cbSize = std::mem::size_of::<MONITORINFO>() as u32;
        if GetMonitorInfoW(mon, &mut mi) == 0 {
            return (RECT { left: 0, top: 0, right: GetSystemMetrics(0), bottom: GetSystemMetrics(1) }, 1.0);
        }
        let (mut dx, mut dy) = (0u32, 0u32);
        let scale = if GetDpiForMonitor(mon, MDT_EFFECTIVE_DPI, &mut dx, &mut dy) == 0 && dx >= 96 { f64::from(dx) / 96.0 } else { 1.0 };
        (mi.rcWork, scale)
    }
}

unsafe fn draw(hdc: HDC, text: &str, height: i32, weight: i32, color: u32, mut r: RECT) {
    let face = wide("Segoe UI");
    let t: Vec<u16> = text.encode_utf16().collect();
    unsafe {
        let font = CreateFontW(-height, 0, 0, 0, weight, 0, 0, 0, 1, 0, 0, 5, 0, face.as_ptr());
        let old = SelectObject(hdc, font);
        SetTextColor(hdc, color);
        DrawTextW(hdc, t.as_ptr(), t.len() as i32, &mut r, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_END_ELLIPSIS);
        SelectObject(hdc, old);
        DeleteObject(font);
    }
}

unsafe fn paint(hwnd: HWND) {
    let o = lock(&TEXTS).as_ref().and_then(|m| m.get(&(hwnd as isize)).cloned()).unwrap_or_default();
    let mut ps: PAINTSTRUCT = unsafe { std::mem::zeroed() };
    let hdc = unsafe { BeginPaint(hwnd, &mut ps) };
    let sc = window_scale(hwnd);
    let s = |v: f64| (v * sc) as i32;
    let mut rc = RECT { left: 0, top: 0, right: 0, bottom: 0 };
    unsafe {
        GetClientRect(hwnd, &mut rc);
        let bg = CreateSolidBrush(rgb(0x1c, 0x1f, 0x25));
        FillRect(hdc, &rc, bg);
        DeleteObject(bg);
    }
    let (ar, ag, ab) = parse_hex(&o.accent).unwrap_or((0x25, 0x63, 0xeb));
    let bar = RECT { left: 0, top: 0, right: s(6.0), bottom: rc.bottom };
    unsafe {
        let br = CreateSolidBrush(rgb(ar, ag, ab));
        FillRect(hdc, &bar, br);
        DeleteObject(br);
        SetBkMode(hdc, TRANSPARENT as i32);
    }
    let pad = s(16.0);
    let has_sub = !o.subtitle.trim().is_empty();
    let dots_h = if o.dots > 1 { s(20.0) } else { 0 };
    let mut title = RECT { left: pad + bar.right, top: s(8.0), right: rc.right - pad, bottom: s(8.0) + s(34.0) };
    if !has_sub {
        title.top = 0;
        title.bottom = rc.bottom - dots_h;
    }
    unsafe {
        draw(hdc, &o.title, s(22.0), 600, rgb(0xf5, 0xf6, 0xf7), title);
        if has_sub {
            draw(
                hdc,
                &o.subtitle,
                s(15.0),
                400,
                rgb(0x94, 0x9e, 0xa6),
                RECT { left: pad + bar.right, top: title.bottom, right: rc.right - pad, bottom: rc.bottom - dots_h - s(6.0) },
            );
        }
    }
    if o.dots > 1 {
        let (d, gap) = (s(8.0), s(8.0));
        let total = o.dots as i32 * d + (o.dots as i32 - 1) * gap;
        let mut x = (rc.right + bar.right) / 2 - total / 2;
        let y = rc.bottom - s(18.0);
        for i in 0..o.dots {
            let col = if i == o.active { rgb(ar, ag, ab) } else { rgb(0x4a, 0x52, 0x5a) };
            unsafe {
                let b = CreateSolidBrush(col);
                let pen = CreatePen(PS_NULL, 0, 0);
                let ob = SelectObject(hdc, b);
                let op = SelectObject(hdc, pen);
                Ellipse(hdc, x, y, x + d, y + d);
                SelectObject(hdc, ob);
                SelectObject(hdc, op);
                DeleteObject(b);
                DeleteObject(pen);
            }
            x += d + gap;
        }
    }
    unsafe {
        EndPaint(hwnd, &ps);
    }
}

unsafe extern "system" fn proc(hwnd: HWND, m: u32, wp: WPARAM, lp: LPARAM) -> LRESULT {
    match m {
        WM_PAINT => {
            unsafe { paint(hwnd) };
            0
        }
        WM_TIMER => {
            unsafe { DestroyWindow(hwnd) };
            0
        }
        WM_DESTROY => {
            unsafe { KillTimer(hwnd, 1) };
            if let Some(map) = lock(&TEXTS).as_mut() {
                map.remove(&(hwnd as isize));
            }
            let _ = CURRENT.compare_exchange(hwnd as isize, 0, Ordering::SeqCst, Ordering::SeqCst);
            unsafe { PostQuitMessage(0) };
            0
        }
        _ => unsafe { DefWindowProcW(hwnd, m, wp, lp) },
    }
}

fn popup_rect(o: &PopupOptions) -> (i32, i32, i32, i32) {
    let (work, sc) = active_monitor();
    let s = |v: f64| (v * sc) as i32;
    let w = s(440.0);
    let mut h = if o.subtitle.trim().is_empty() { s(70.0) } else { s(96.0) };
    if o.dots > 1 {
        h += s(18.0);
    }
    let x = work.left + (work.right - work.left - w) / 2;
    let y = match o.position.as_str() {
        "top" => work.top + s(70.0),
        "center" => work.top + (work.bottom - work.top - h) / 2,
        _ => work.bottom - h - s(110.0),
    };
    (x, y, w, h)
}

fn show(o: PopupOptions, ready: mpsc::Sender<bool>) {
    let class = wide(CLASS);
    REGISTER.call_once(|| unsafe {
        let mut wc: WNDCLASSEXW = std::mem::zeroed();
        wc.cbSize = std::mem::size_of::<WNDCLASSEXW>() as u32;
        wc.lpfnWndProc = Some(proc);
        wc.hInstance = GetModuleHandleW(std::ptr::null());
        wc.hCursor = LoadCursorW(std::ptr::null_mut(), IDC_ARROW);
        wc.lpszClassName = class.as_ptr();
        RegisterClassExW(&wc);
    });
    let (x, y, w, h) = popup_rect(&o);
    let secs = if o.seconds < 0.6 { 1.2 } else { o.seconds };
    let hwnd = unsafe {
        CreateWindowExW(
            WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE | WS_EX_LAYERED | WS_EX_TRANSPARENT,
            class.as_ptr(),
            std::ptr::null(),
            WS_POPUP,
            x,
            y,
            w,
            h,
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            GetModuleHandleW(std::ptr::null()),
            std::ptr::null(),
        )
    };
    if hwnd.is_null() {
        let _ = ready.send(false);
        return;
    }
    lock(&TEXTS).get_or_insert_with(HashMap::new).insert(hwnd as isize, o);
    let prev = CURRENT.swap(hwnd as isize, Ordering::SeqCst);
    unsafe {
        if prev != 0 {
            PostMessageW(prev as HWND, WM_CLOSE, 0, 0);
        }
        SetLayeredWindowAttributes(hwnd, 0, 240, LWA_ALPHA);
        let r = (18.0 * window_scale(hwnd)) as i32;
        SetWindowRgn(hwnd, CreateRoundRectRgn(0, 0, w, h, r, r), 1);
        SetTimer(hwnd, 1, (secs * 1000.0) as u32, None);
        ShowWindow(hwnd, SW_SHOWNOACTIVATE);
    }
    let _ = ready.send(true);
    let mut m: MSG = unsafe { std::mem::zeroed() };
    while unsafe { GetMessageW(&mut m, std::ptr::null_mut(), 0, 0) } > 0 {
        unsafe { DispatchMessageW(&m) };
    }
}

pub fn popup(o: &PopupOptions) -> bool {
    if o.title.trim().is_empty() && o.subtitle.trim().is_empty() {
        return true;
    }
    let (tx, rx) = mpsc::channel();
    let o = o.clone();
    if std::thread::Builder::new().name("popup".into()).spawn(move || show(o, tx)).is_err() {
        return false;
    }
    rx.recv_timeout(Duration::from_secs(2)).unwrap_or(false)
}
