mod apps;
mod clipboard;
mod front;
mod input;
mod popup;
pub mod reg;
mod shell;

use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::atomic::{AtomicIsize, Ordering};
use std::sync::{Mutex, mpsc};
use std::time::Duration;

use windows_sys::Win32::Foundation::{CloseHandle, ERROR_ALREADY_EXISTS, GetLastError, HANDLE, WAIT_OBJECT_0};
use windows_sys::Win32::Foundation::{GENERIC_READ, GENERIC_WRITE, INVALID_HANDLE_VALUE};
use windows_sys::Win32::Globalization::{GetOEMCP, MultiByteToWideChar};
use windows_sys::Win32::Storage::FileSystem::{CreateFileW, FILE_SHARE_READ, FILE_SHARE_WRITE, OPEN_EXISTING};
use windows_sys::Win32::System::Console::{
    ATTACH_PARENT_PROCESS, AttachConsole, CTRL_CLOSE_EVENT, CTRL_LOGOFF_EVENT, CTRL_SHUTDOWN_EVENT, GetStdHandle, STD_ERROR_HANDLE, STD_OUTPUT_HANDLE,
    SetConsoleCtrlHandler, SetStdHandle,
};
use windows_sys::Win32::System::Diagnostics::Debug::MessageBeep;
use windows_sys::Win32::System::Diagnostics::ToolHelp::{CreateToolhelp32Snapshot, PROCESSENTRY32W, Process32FirstW, Process32NextW, TH32CS_SNAPPROCESS};
use windows_sys::Win32::System::Power::{ES_CONTINUOUS, ES_DISPLAY_REQUIRED, ES_SYSTEM_REQUIRED, SetSuspendState, SetThreadExecutionState};
use windows_sys::Win32::System::Registry::HKEY_CURRENT_USER;
use windows_sys::Win32::System::Shutdown::LockWorkStation;
use windows_sys::Win32::System::Threading::{CreateMutexW, GetCurrentProcessId, OpenProcess, PROCESS_SYNCHRONIZE, ReleaseMutex, WaitForSingleObject};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    FindWindowW, GetWindowThreadProcessId, HWND_BROADCAST, MB_ICONERROR, MB_SETFOREGROUND, MessageBoxW, PostMessageW, SMTO_ABORTIFHUNG, SendMessageTimeoutW,
    SendMessageW, WM_CLOSE, WM_SETTINGCHANGE, WM_SYSCOMMAND,
};

use super::{DoctorCheck, Platform, PopupOptions, ShellHooks, StopFn, SystemInfo, UNSUPPORTED, command, detached, env_var, have, quote_cmd_arg, run};
use crate::hid::{Layout, Stroke};
use reg::wide;

const PERSONALIZE: &str = r"Software\Microsoft\Windows\CurrentVersion\Themes\Personalize";
const RUN_KEY: &str = r"Software\Microsoft\Windows\CurrentVersion\Run";
const TASK: &str = "Typedeck";
const KEYBOARD_LAYOUTS: &str = r"SYSTEM\CurrentControlSet\Control\Keyboard Layouts";

static INSTANCE: AtomicIsize = AtomicIsize::new(0);
type CloseFn = Box<dyn Fn() + Send + Sync>;
static ON_CLOSE: Mutex<Option<CloseFn>> = Mutex::new(None);

pub struct Windows;

impl Windows {
    pub fn new() -> Windows {
        Windows
    }
}

pub fn decode_oem(b: &[u8]) -> Option<String> {
    unsafe {
        let cp = GetOEMCP();
        let n = MultiByteToWideChar(cp, 0, b.as_ptr(), b.len() as i32, std::ptr::null_mut(), 0);
        if n <= 0 {
            return None;
        }
        let mut u = vec![0u16; n as usize];
        MultiByteToWideChar(cp, 0, b.as_ptr(), b.len() as i32, u.as_mut_ptr(), n);
        Some(String::from_utf16_lossy(&u))
    }
}

fn local_data_dir() -> PathBuf {
    let base = env_var("LOCALAPPDATA").map(PathBuf::from).unwrap_or_else(|| PathBuf::from(env_var("USERPROFILE").unwrap_or_default()).join(r"AppData\Local"));
    base.join("typedeck")
}

fn config_dir() -> PathBuf {
    let base = env_var("APPDATA").map(PathBuf::from).unwrap_or_else(|| PathBuf::from(env_var("USERPROFILE").unwrap_or_default()).join(r"AppData\Roaming"));
    base.join("typedeck")
}

pub fn private_dir(dir: &Path) -> PathBuf {
    if dir != config_dir() {
        return dir.to_path_buf();
    }
    let d = local_data_dir();
    let _ = std::fs::create_dir_all(&d);
    d
}

pub fn migrate_private(dir: &Path, name: &str) {
    let dst = private_dir(dir).join(name);
    let src = dir.join(name);
    if dst == src || dst.exists() || !src.exists() {
        return;
    }
    if let Some(p) = dst.parent() {
        let _ = std::fs::create_dir_all(p);
    }
    if std::fs::rename(&src, &dst).is_err()
        && let Ok(b) = std::fs::read(&src)
        && crate::config::write_private_file(&dst, &b).is_ok()
    {
        let _ = std::fs::remove_file(&src);
    }
}

fn fnv32a(s: &str) -> u32 {
    let mut h: u32 = 0x811c9dc5;
    for b in s.as_bytes() {
        h ^= u32::from(*b);
        h = h.wrapping_mul(0x01000193);
    }
    h
}

fn instance_key() -> String {
    format!("{:08x}", fnv32a(&config_dir().display().to_string().to_lowercase()))
}

pub fn shell_title() -> String {
    format!("Typedeck {}", instance_key())
}

pub fn acquire_instance() -> bool {
    let name = wide(&format!(r"Local\Typedeck-{}", instance_key()));
    let h = unsafe { CreateMutexW(std::ptr::null(), 0, name.as_ptr()) };
    if h.is_null() {
        return true;
    }
    if unsafe { GetLastError() } == ERROR_ALREADY_EXISTS {
        unsafe { CloseHandle(h) };
        return false;
    }
    INSTANCE.store(h as isize, Ordering::SeqCst);
    true
}

pub fn release_instance() {
    let h = INSTANCE.swap(0, Ordering::SeqCst) as HANDLE;
    if !h.is_null() {
        unsafe {
            ReleaseMutex(h);
            CloseHandle(h);
        }
    }
}

fn parent_image() -> String {
    let snap = unsafe { CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0) };
    if snap.is_null() || snap as isize == -1 {
        return String::new();
    }
    let me = unsafe { GetCurrentProcessId() };
    let mut e: PROCESSENTRY32W = unsafe { std::mem::zeroed() };
    e.dwSize = std::mem::size_of::<PROCESSENTRY32W>() as u32;
    let mut parent = 0;
    let mut ok = unsafe { Process32FirstW(snap, &mut e) } != 0;
    while ok {
        if e.th32ProcessID == me {
            parent = e.th32ParentProcessID;
            break;
        }
        ok = unsafe { Process32NextW(snap, &mut e) } != 0;
    }
    unsafe { CloseHandle(snap) };
    if parent == 0 { String::new() } else { front::process_image(parent) }
}

pub fn started_by_user() -> bool {
    Path::new(&parent_image()).file_name().is_some_and(|n| n.to_string_lossy().eq_ignore_ascii_case("explorer.exe"))
}

pub fn show_error(title: &str, text: &str) {
    let (t, x) = (wide(title), wide(text));
    unsafe {
        MessageBoxW(std::ptr::null_mut(), x.as_ptr(), t.as_ptr(), MB_ICONERROR | MB_SETFOREGROUND);
    }
}

pub fn attach_console() {
    let usable = |h: HANDLE| !h.is_null() && h != INVALID_HANDLE_VALUE;
    unsafe {
        if usable(GetStdHandle(STD_OUTPUT_HANDLE)) || AttachConsole(ATTACH_PARENT_PROCESS) == 0 {
            return;
        }
        let name = wide("CONOUT$");
        let h = CreateFileW(
            name.as_ptr(),
            GENERIC_READ | GENERIC_WRITE,
            FILE_SHARE_READ | FILE_SHARE_WRITE,
            std::ptr::null(),
            OPEN_EXISTING,
            0,
            std::ptr::null_mut(),
        );
        if !usable(h) {
            return;
        }
        for which in [STD_OUTPUT_HANDLE, STD_ERROR_HANDLE] {
            if !usable(GetStdHandle(which)) {
                SetStdHandle(which, h);
            }
        }
    }
}

const CLOSE_GRACE: Duration = Duration::from_secs(4);

unsafe extern "system" fn ctrl_handler(ctrl: u32) -> i32 {
    if let Some(f) = crate::sync::lock(&ON_CLOSE).as_ref() {
        f();
    }
    if matches!(ctrl, CTRL_CLOSE_EVENT | CTRL_LOGOFF_EVENT | CTRL_SHUTDOWN_EVENT) {
        std::thread::sleep(CLOSE_GRACE);
    }
    1
}

pub fn on_console_close(f: CloseFn) {
    *crate::sync::lock(&ON_CLOSE) = Some(f);
    unsafe {
        SetConsoleCtrlHandler(Some(ctrl_handler), 1);
    }
}

pub fn start_shell(h: ShellHooks) {
    shell::start(h);
}

fn stop_running() -> bool {
    let (class, title) = (wide(shell::class_name()), wide(&shell_title()));
    let hwnd = unsafe { FindWindowW(class.as_ptr(), title.as_ptr()) };
    if hwnd.is_null() {
        return true;
    }
    let mut pid = 0u32;
    unsafe { GetWindowThreadProcessId(hwnd, &mut pid) };
    let h = unsafe { OpenProcess(PROCESS_SYNCHRONIZE, 0, pid) };
    if h.is_null() {
        return true;
    }
    unsafe { PostMessageW(hwnd, WM_CLOSE, 0, 0) };
    let r = unsafe { WaitForSingleObject(h, 8000) };
    unsafe { CloseHandle(h) };
    r == WAIT_OBJECT_0
}

fn klid_by_layout_id(id: u16, lang: u16) -> String {
    let want = format!("{id:X}");
    let suffix = format!("{lang:04X}");
    for klid in reg::sub_keys(windows_sys::Win32::System::Registry::HKEY_LOCAL_MACHINE, KEYBOARD_LAYOUTS) {
        if klid.len() != 8 || !klid[4..].eq_ignore_ascii_case(&suffix) {
            continue;
        }
        if let Some(v) = reg::string(windows_sys::Win32::System::Registry::HKEY_LOCAL_MACHINE, &format!(r"{KEYBOARD_LAYOUTS}\{klid}"), "Layout Id")
            && v.trim_start_matches('0').eq_ignore_ascii_case(&want)
        {
            return klid;
        }
    }
    String::new()
}

fn gui_executable(exe: &Path) -> PathBuf {
    if exe.file_name().is_some_and(|n| n.to_string_lossy().eq_ignore_ascii_case("typedeck-cli.exe"))
        && let Some(gui) = exe.parent().map(|d| d.join("typedeck.exe")).filter(|g| g.exists())
    {
        return gui;
    }
    exe.to_path_buf()
}

fn task_exists() -> bool {
    run(Duration::from_secs(10), "", "schtasks", &["/Query", "/TN", TASK]).is_ok()
}

fn autostart_installed() -> (bool, &'static str) {
    if task_exists() {
        return (true, "scheduled task");
    }
    if reg::string(HKEY_CURRENT_USER, RUN_KEY, TASK).is_some() {
        return (true, "Run registry key");
    }
    (false, "")
}

fn token_user() -> (Option<String>, String) {
    use windows_sys::Win32::Foundation::{HLOCAL, LocalFree};
    use windows_sys::Win32::Security::Authorization::ConvertSidToStringSidW;
    use windows_sys::Win32::Security::{GetTokenInformation, LookupAccountSidW, SID_NAME_USE, TOKEN_QUERY, TOKEN_USER, TokenUser};
    use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};
    unsafe {
        let mut token: HANDLE = std::ptr::null_mut();
        if OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) == 0 {
            return (None, String::new());
        }
        let mut buf = vec![0u64; 64];
        let mut len = 0u32;
        let ok = GetTokenInformation(token, TokenUser, buf.as_mut_ptr().cast(), (buf.len() * 8) as u32, &mut len);
        CloseHandle(token);
        if ok == 0 {
            return (None, String::new());
        }
        let sid = (*(buf.as_ptr() as *const TOKEN_USER)).User.Sid;
        let mut text: *mut u16 = std::ptr::null_mut();
        let sid_text = if ConvertSidToStringSidW(sid, &mut text) != 0 && !text.is_null() {
            let n = (0..).take_while(|&i| *text.add(i) != 0).count();
            let s = String::from_utf16_lossy(std::slice::from_raw_parts(text, n));
            LocalFree(text as HLOCAL);
            s
        } else {
            String::new()
        };
        let (mut name, mut domain) = ([0u16; 256], [0u16; 256]);
        let (mut name_len, mut domain_len) = (name.len() as u32, domain.len() as u32);
        let mut kind: SID_NAME_USE = 0;
        if LookupAccountSidW(std::ptr::null(), sid, name.as_mut_ptr(), &mut name_len, domain.as_mut_ptr(), &mut domain_len, &mut kind) == 0 {
            return (None, sid_text);
        }
        let name = String::from_utf16_lossy(&name[..name_len as usize]);
        let domain = String::from_utf16_lossy(&domain[..domain_len as usize]);
        (Some(if domain.is_empty() { name } else { format!(r"{domain}\{name}") }), sid_text)
    }
}

fn user_name(account: Option<String>) -> String {
    if let Some(u) = account {
        return u;
    }
    match (env_var("USERDOMAIN"), env_var("USERNAME")) {
        (Some(d), Some(u)) => format!(r"{d}\{u}"),
        (None, Some(u)) => u,
        _ => String::new(),
    }
}

fn create_task(exe: &Path) -> Result<(), String> {
    let (account, sid) = token_user();
    let xml = super::utf16_with_bom(&super::task_xml(&user_name(account), &sid, &exe.display().to_string()));
    let mut n = [0u8; 8];
    let _ = crate::crypto::random(&mut n);
    let path = std::env::temp_dir().join(format!("typedeck-task-{}.xml", crate::crypto::hex(&n)));
    std::fs::write(&path, xml).map_err(|e| e.to_string())?;
    let r = run(Duration::from_secs(20), "", "schtasks", &["/Create", "/TN", TASK, "/XML", &path.display().to_string(), "/F"]);
    let _ = std::fs::remove_file(&path);
    r.map(|_| ())
}

impl Platform for Windows {
    fn name(&self) -> &'static str {
        "windows"
    }

    fn version(&self) -> String {
        let Ok(out) = run(Duration::from_secs(5), "", "cmd", &["/C", "ver"]) else { return String::new() };
        match out.split_once("[Version ") {
            Some((_, v)) => v.trim_end_matches(']').to_string(),
            None => out.trim().to_string(),
        }
    }

    fn open_url(&self, url: &str) -> Result<(), String> {
        if !url.starts_with("http://") && !url.starts_with("https://") && !url.starts_with("mailto:") {
            return Err("only http, https or mailto links can be opened".into());
        }
        let mut c = command("rundll32.exe");
        c.args(["url.dll,FileProtocolHandler", url]);
        detached(c)
    }

    fn focus_editor(&self, _url: &str) -> bool {
        front::focus_editor()
    }

    fn open_app(&self, name: &str) -> Result<(), String> {
        apps::open_app(name)
    }

    fn quit_app(&self, name: &str) -> Result<(), String> {
        apps::quit_app(name)
    }

    fn hide_app(&self, _name: &str) -> Result<(), String> {
        front::hide_front()
    }

    fn front_id(&self) -> String {
        front::front_id()
    }

    fn front_names(&self, _id: &str) -> Vec<String> {
        apps::front_names()
    }

    fn installed_apps(&self) -> Vec<String> {
        apps::shortcuts().into_iter().map(|s| s.name).collect()
    }

    fn soft_hotkey(&self, spec: &str) -> Result<(), String> {
        input::soft_hotkey(spec)
    }

    fn soft_stroke(&self, st: Stroke) -> Result<(), String> {
        input::soft_stroke(st)
    }

    fn paste(&self, text: &str) -> Result<(), String> {
        clipboard::paste(text)
    }

    fn clipboard(&self) -> String {
        clipboard::clipboard()
    }

    fn soft_media(&self, cmd: &str, step: i64) -> Result<String, String> {
        input::soft_media(cmd, step)
    }

    fn system(&self, cmd: &str) -> Result<(), String> {
        let last = || Err(std::io::Error::last_os_error().to_string());
        match cmd {
            "lock" => {
                if unsafe { LockWorkStation() } == 0 {
                    return last();
                }
                Ok(())
            }
            "sleep" => {
                if !unsafe { SetSuspendState(false, false, false) } {
                    return last();
                }
                Ok(())
            }
            "sleepdisplay" => {
                unsafe { SendMessageW(HWND_BROADCAST, WM_SYSCOMMAND, 0xF170, 2) };
                Ok(())
            }
            "screensaver" => {
                unsafe { SendMessageW(HWND_BROADCAST, WM_SYSCOMMAND, 0xF140, 0) };
                Ok(())
            }
            "screenshot" => input::send_combo(&["gui", "shift"], u16::from(b'S')),
            "darkmode" => {
                let light = reg::dword(HKEY_CURRENT_USER, PERSONALIZE, "AppsUseLightTheme");
                let next = u32::from(light == Some(0));
                for v in ["AppsUseLightTheme", "SystemUsesLightTheme"] {
                    reg::set_dword(HKEY_CURRENT_USER, PERSONALIZE, v, next)?;
                }
                let s = wide("ImmersiveColorSet");
                let mut result = 0usize;
                unsafe { SendMessageTimeoutW(HWND_BROADCAST, WM_SETTINGCHANGE, 0, s.as_ptr() as isize, SMTO_ABORTIFHUNG, 3000, &mut result) };
                Ok(())
            }
            _ => Err(UNSUPPORTED.into()),
        }
    }

    fn lock_stroke(&self) -> Option<Stroke> {
        None
    }

    fn keep_awake(&self) -> Result<StopFn, String> {
        let (tx, rx) = mpsc::channel::<()>();
        let (ready_tx, ready_rx) = mpsc::channel::<()>();
        std::thread::Builder::new()
            .name("keep-awake".into())
            .spawn(move || {
                unsafe { SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED | ES_DISPLAY_REQUIRED) };
                let _ = ready_tx.send(());
                let _ = rx.recv();
                unsafe { SetThreadExecutionState(ES_CONTINUOUS) };
            })
            .map_err(|e| e.to_string())?;
        let _ = ready_rx.recv_timeout(Duration::from_secs(2));
        Ok(Box::new(move || drop(tx)))
    }

    fn notify(&self, title: &str, body: &str, sound: bool) {
        if !title.is_empty() || !body.is_empty() {
            shell::balloon(title, body);
        }
        if sound {
            unsafe { MessageBeep(0) };
        }
    }

    fn popup(&self, o: &PopupOptions) -> bool {
        popup::popup(o)
    }

    fn system_layout(&self) -> String {
        let hkl = input::foreground_layout() as u32;
        super::layout_from_klid(&super::klid_from_hkl(hkl, Some(&klid_by_layout_id))).to_string()
    }

    fn adapt_stroke(&self, st: Stroke, _layout: &Layout) -> Stroke {
        st
    }

    fn info(&self, l: &Layout) -> SystemInfo {
        SystemInfo { typing_layout: l.name.into(), ..Default::default() }
    }

    fn shell(&self, cmd: &str) -> Command {
        use std::os::windows::process::CommandExt;
        let mut c = command("cmd.exe");
        c.raw_arg(format!("/S /C \"{cmd}\""));
        c
    }

    fn quote_arg(&self, s: &str) -> String {
        quote_cmd_arg(s)
    }

    fn log_file(&self) -> Option<PathBuf> {
        Some(local_data_dir().join("typedeck.log"))
    }

    fn config_dir(&self) -> PathBuf {
        config_dir()
    }

    fn doctor(&self) -> Vec<DoctorCheck> {
        let (installed, how) = autostart_installed();
        vec![
            DoctorCheck::new(have("taskkill"), "taskkill available (closing applications)", "taskkill is missing: applications cannot be closed"),
            DoctorCheck::new(have("schtasks"), "schtasks available (start with the session)", "schtasks is missing: typedeck install will not work"),
            DoctorCheck::new(
                have("powershell.exe"),
                "PowerShell available (Microsoft Store apps in the list)",
                "optional: PowerShell is missing: Microsoft Store apps will not be listed",
            ),
            DoctorCheck::new(installed, format!("starts with your session ({how})"), "optional: not set to start with your session (run typedeck install)"),
        ]
    }

    fn install_service(&self, exe: &Path) -> Result<String, String> {
        let exe = gui_executable(exe);
        if create_task(&exe).is_ok() {
            let _ = reg::delete_value(HKEY_CURRENT_USER, RUN_KEY, TASK);
            let _ = run(Duration::from_secs(15), "", "schtasks", &["/Run", "/TN", TASK]);
            return Ok(format!("Scheduled task \"{TASK}\" created: it starts when you sign in (and is already running)"));
        }
        reg::set_string(HKEY_CURRENT_USER, RUN_KEY, TASK, &format!("\"{}\" --background", exe.display()))
            .map_err(|e| format!("could not create the scheduled task or the Run entry: {e}"))?;
        let mut c = command(&exe.display().to_string());
        c.arg("--background");
        let _ = detached(c);
        Ok("Windows did not allow a scheduled task, so Typedeck was added to your Run entries: it starts when you sign in (and is already running)".into())
    }

    fn uninstall_service(&self) -> Result<String, String> {
        if task_exists() {
            run(Duration::from_secs(15), "", "schtasks", &["/Delete", "/TN", TASK, "/F"]).map_err(|e| format!("schtasks: {e}"))?;
        }
        reg::delete_value(HKEY_CURRENT_USER, RUN_KEY, TASK).map_err(|e| format!("could not remove the Run entry: {e}"))?;
        if !stop_running() {
            let _ = run(Duration::from_secs(10), "", "taskkill", &["/F", "/IM", "typedeck.exe"]);
        }
        Ok("Start with your session removed and Typedeck stopped (the configuration is kept)".into())
    }
}

#[cfg(test)]
mod e2e_tests;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn store_apps_are_parsed() {
        let apps = apps::parse_store_apps("Calculator|Microsoft.WindowsCalculator_8wekyb3d8bbwe!App\nUninstall Foo|X!Y\nNotepad|notepad.exe\n|x!y\n");
        assert_eq!(apps.len(), 1);
        assert_eq!(apps[0].name, "Calculator");
    }

    #[test]
    fn task_user_comes_from_the_token() {
        let (account, sid) = token_user();
        let u = account.expect("account of the process token");
        assert!(sid.starts_with("S-1-5-"), "{sid}");
        let (_, name) = u.rsplit_once('\\').unwrap_or(("", &u));
        assert!(!name.is_empty(), "{u}");
        assert_ne!(u.split('\\').next(), Some("WORKGROUP"), "{u}");
        assert_eq!(user_name(Some(u.clone())), u);
    }

    #[test]
    fn single_instance() {
        assert!(acquire_instance());
        assert!(!acquire_instance());
        release_instance();
    }

    #[test]
    fn quoting_through_cmd() {
        let p = Windows::new();
        let out = crate::platform::run_command(p.shell(&format!("echo {}", p.quote_arg("a&b"))), Duration::from_secs(5), "").unwrap();
        assert!(out.contains("a&b"), "{out}");
    }
}
