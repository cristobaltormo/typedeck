use std::path::Path;

use windows_sys::Win32::Foundation::{CloseHandle, HWND, LPARAM, TRUE};
use windows_sys::Win32::Graphics::Dwm::DwmGetWindowAttribute;
use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_QUERY_LIMITED_INFORMATION, QueryFullProcessImageNameW};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    EnumWindows, GW_OWNER, GWL_EXSTYLE, GetClassNameW, GetForegroundWindow, GetWindow, GetWindowLongW, GetWindowTextW, GetWindowThreadProcessId, IsIconic,
    IsWindowVisible, PostMessageW, SW_MINIMIZE, SW_RESTORE, SetForegroundWindow, ShowWindow, WM_CLOSE, WS_EX_TOOLWINDOW,
};

use super::input::key_event;

#[derive(Clone, Debug)]
pub struct TopWindow {
    pub hwnd: isize,
    pub pid: u32,
    pub title: String,
    pub class: String,
}

const BROWSERS: &[&str] = &["chrome.exe", "msedge.exe", "firefox.exe", "brave.exe", "opera.exe", "vivaldi.exe", "arc.exe", "librewolf.exe"];
const SHELL_CLASSES: &[&str] = &["Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd"];

fn window_text(hwnd: HWND) -> String {
    let mut buf = [0u16; 512];
    let n = unsafe { GetWindowTextW(hwnd, buf.as_mut_ptr(), buf.len() as i32) };
    String::from_utf16_lossy(&buf[..n.max(0) as usize])
}

fn window_class(hwnd: HWND) -> String {
    let mut buf = [0u16; 256];
    let n = unsafe { GetClassNameW(hwnd, buf.as_mut_ptr(), buf.len() as i32) };
    String::from_utf16_lossy(&buf[..n.max(0) as usize])
}

fn is_cloaked(hwnd: HWND) -> bool {
    let mut cloaked = 0u32;
    let r = unsafe { DwmGetWindowAttribute(hwnd, 14, (&mut cloaked as *mut u32).cast(), 4) };
    r == 0 && cloaked != 0
}

fn is_app_window(hwnd: HWND) -> bool {
    unsafe {
        IsWindowVisible(hwnd) != 0
            && GetWindow(hwnd, GW_OWNER).is_null()
            && (GetWindowLongW(hwnd, GWL_EXSTYLE) as u32 & WS_EX_TOOLWINDOW) == 0
            && !is_cloaked(hwnd)
    }
}

unsafe extern "system" fn visit(hwnd: HWND, lparam: LPARAM) -> i32 {
    let out = unsafe { &mut *(lparam as *mut Vec<TopWindow>) };
    if is_app_window(hwnd) {
        let title = window_text(hwnd);
        let class = window_class(hwnd);
        if !title.is_empty() && !SHELL_CLASSES.contains(&class.as_str()) {
            let mut pid = 0u32;
            unsafe {
                GetWindowThreadProcessId(hwnd, &mut pid);
            }
            out.push(TopWindow { hwnd: hwnd as isize, pid, title, class });
        }
    }
    TRUE
}

pub fn top_windows() -> Vec<TopWindow> {
    let mut out: Vec<TopWindow> = Vec::new();
    unsafe {
        EnumWindows(Some(visit), (&mut out as *mut Vec<TopWindow>) as LPARAM);
    }
    out
}

pub fn process_image(pid: u32) -> String {
    let h = unsafe { OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid) };
    if h.is_null() {
        return String::new();
    }
    let mut buf = [0u16; 1024];
    let mut size = buf.len() as u32;
    let ok = unsafe { QueryFullProcessImageNameW(h, 0, buf.as_mut_ptr(), &mut size) } != 0;
    unsafe {
        CloseHandle(h);
    }
    if ok { String::from_utf16_lossy(&buf[..size as usize]) } else { String::new() }
}

fn base_name(p: &str) -> String {
    Path::new(p).file_name().map(|n| n.to_string_lossy().to_lowercase()).unwrap_or_default()
}

pub fn windows_of_image(exe: &str) -> Vec<TopWindow> {
    let want = exe.to_lowercase();
    let mut cache: Vec<(u32, String)> = Vec::new();
    top_windows()
        .into_iter()
        .filter(|w| {
            let img = match cache.iter().find(|(p, _)| *p == w.pid) {
                Some((_, i)) => i.clone(),
                None => {
                    let i = base_name(&process_image(w.pid));
                    cache.push((w.pid, i.clone()));
                    i
                }
            };
            img == want
        })
        .collect()
}

pub fn close_windows(ws: &[TopWindow]) {
    for w in ws {
        unsafe {
            PostMessageW(w.hwnd as HWND, WM_CLOSE, 0, 0);
        }
    }
}

pub fn bring_to_front(hwnd: isize) -> bool {
    let h = hwnd as HWND;
    unsafe {
        if IsIconic(h) != 0 {
            ShowWindow(h, SW_RESTORE);
        }
    }
    key_event(0x12, false);
    key_event(0x12, true);
    unsafe { SetForegroundWindow(h) != 0 }
}

pub fn focus_editor() -> bool {
    top_windows()
        .into_iter()
        .find(|w| w.title.contains("Typedeck") && BROWSERS.contains(&base_name(&process_image(w.pid)).as_str()))
        .is_some_and(|w| bring_to_front(w.hwnd))
}

pub fn foreground() -> (HWND, u32) {
    let hwnd = unsafe { GetForegroundWindow() };
    let mut pid = 0u32;
    if !hwnd.is_null() {
        unsafe {
            GetWindowThreadProcessId(hwnd, &mut pid);
        }
    }
    (hwnd, pid)
}

pub fn hide_front() -> Result<(), String> {
    let (hwnd, _) = foreground();
    if hwnd.is_null() {
        return Err(crate::platform::UNSUPPORTED.into());
    }
    unsafe {
        ShowWindow(hwnd, SW_MINIMIZE);
    }
    Ok(())
}

pub fn front_id() -> String {
    let (hwnd, pid) = foreground();
    if hwnd.is_null() {
        return String::new();
    }
    format!("{:x}:{pid}", hwnd as usize)
}

#[cfg(test)]
unsafe extern "system" fn count_class(hwnd: HWND, lparam: LPARAM) -> i32 {
    let (want, n) = unsafe { &mut *(lparam as *mut (String, usize)) };
    if window_class(hwnd) == *want {
        *n += 1;
    }
    TRUE
}

#[cfg(test)]
pub fn all_windows_of_class(class: &str) -> usize {
    let mut state = (class.to_string(), 0usize);
    unsafe {
        EnumWindows(Some(count_class), (&mut state as *mut (String, usize)) as LPARAM);
    }
    state.1
}
