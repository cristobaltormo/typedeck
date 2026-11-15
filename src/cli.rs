use std::path::PathBuf;
use std::sync::{Arc, Once};
use std::time::Duration;

use crate::app::{NAME, VERSION};
use crate::board::{Board, SerialConnector};
use crate::capture::{SOFTWARE, Selector, Source};
use crate::config::Paths;
use crate::engine::Engine;
use crate::server::{self, Server};
use crate::sync::Flag;
use crate::{actions, platform};

const HELP: &str = "usage: typedeck [command]
  (no command)  start the program and the editor at http://127.0.0.1:7788
  doctor        check that everything is in place and say what fails
  install       start it with your session (user service)
  uninstall     remove the start with the session (the configuration is kept)
  version       print the version";

pub fn main() {
    let args: Vec<String> = std::env::args().collect();
    let background = args.get(1).is_some_and(|a| a == "--background");
    if args.len() > 1 && !background {
        platform::attach_console();
        std::process::exit(run_command(&args[1]));
    }
    run_program(background);
}

pub fn run_command(name: &str) -> i32 {
    match name {
        "version" | "-v" | "--version" => println!("typedeck {VERSION}"),
        "doctor" => return crate::doctor::run(),
        "install" | "uninstall" => return service(name),
        "help" | "-h" | "--help" => println!("{HELP}"),
        _ => {
            eprintln!("unknown command: {name} (typedeck help)");
            return 2;
        }
    }
    0
}

fn capture_note(exe: &std::path::Path) -> Option<String> {
    if platform::get().name() != "darwin" {
        return None;
    }
    let helper = exe.parent().map(|d| d.join(crate::capture::HELPER_NAME)).unwrap_or_default();
    let name = crate::capture::HELPER_NAME;
    Some(if helper.exists() {
        format!(
            "Using Typedeck without a board runs {}; macOS asks you to allow it under Accessibility and Input Monitoring the first time you turn that on",
            helper.display()
        )
    } else {
        format!("{name} is not next to typedeck, so using Typedeck without a board will not work; the macOS archive includes it")
    })
}

fn service(what: &str) -> i32 {
    let res = if what == "install" {
        let exe = match std::env::current_exe().and_then(std::fs::canonicalize) {
            Ok(e) => e,
            Err(e) => {
                eprintln!("could not locate the executable: {e}");
                return 1;
            }
        };
        let exe = strip_verbatim(exe);
        platform::get().install_service(&exe).map(|m| match capture_note(&exe) {
            Some(n) => format!("{m}\n{n}"),
            None => m,
        })
    } else {
        platform::get().uninstall_service()
    };
    match res {
        Ok(msg) => {
            println!("{msg}");
            0
        }
        Err(e) => {
            eprintln!("error: {e}");
            1
        }
    }
}

fn strip_verbatim(p: PathBuf) -> PathBuf {
    let s = p.to_string_lossy();
    match s.strip_prefix(r"\\?\") {
        Some(rest) if !rest.starts_with("UNC\\") => PathBuf::from(rest),
        _ => p,
    }
}

fn log_to_file() {
    let Some(path) = platform::get().log_file() else { return };
    if std::fs::metadata(&path).is_ok_and(|m| m.len() > 1 << 20) {
        if platform::get().name() == "windows" {
            let mut old = path.clone().into_os_string();
            old.push(".1");
            let _ = std::fs::remove_file(&old);
            let _ = std::fs::rename(&path, &old);
        } else if let Ok(f) = std::fs::OpenOptions::new().write(true).open(&path) {
            let _ = f.set_len(0);
        }
    }
    if platform::get().name() == "windows" {
        if let Some(dir) = path.parent() {
            let _ = std::fs::create_dir_all(dir);
        }
        if let Ok(f) = std::fs::OpenOptions::new().create(true).append(true).open(&path) {
            crate::logging::to_file(f);
        }
    }
}

fn capture_status(src: &dyn Source) -> String {
    let st = src.status();
    match (src.kind() == SOFTWARE, st.connected) {
        (true, true) => "software capture active".into(),
        (true, false) => "software capture not running".into(),
        (false, false) => "board not connected".into(),
        (false, true) => format!("board connected on {}", st.port),
    }
}

fn fatal(msg: &str) -> ! {
    crate::log!("{msg}");
    platform::show_error(NAME, msg);
    std::process::exit(1);
}

#[cfg(unix)]
static SIGNAL_PIPE: std::sync::atomic::AtomicI32 = std::sync::atomic::AtomicI32::new(-1);

#[cfg(unix)]
extern "C" fn on_signal(_sig: libc::c_int) {
    let fd = SIGNAL_PIPE.load(std::sync::atomic::Ordering::SeqCst);
    if fd >= 0 {
        let b = [1u8];
        unsafe {
            libc::write(fd, b.as_ptr().cast(), 1);
        }
    }
}

// A handler that only writes to a pipe keeps the signal mask untouched, so programs started by actions can still be stopped.
#[cfg(unix)]
fn watch_signals() -> i32 {
    use std::os::fd::IntoRawFd;
    let Ok([rd, wr]) = platform::cloexec_pipe() else { return -1 };
    let (rd, wr) = (rd.into_raw_fd(), wr.into_raw_fd());
    unsafe {
        SIGNAL_PIPE.store(wr, std::sync::atomic::Ordering::SeqCst);
        let handler = on_signal as extern "C" fn(libc::c_int) as libc::sighandler_t;
        libc::signal(libc::SIGINT, handler);
        libc::signal(libc::SIGTERM, handler);
    }
    rd
}

#[cfg(unix)]
fn wait_for_signal(fd: i32) {
    let mut b = [0u8; 1];
    loop {
        let r = unsafe { libc::read(fd, b.as_mut_ptr().cast(), 1) };
        if r == 1 || (r < 0 && std::io::Error::last_os_error().kind() != std::io::ErrorKind::Interrupted) {
            return;
        }
    }
}

#[cfg(windows)]
fn wait_for_signal(quit: &'static Flag) {
    platform::on_console_close(Box::new(move || quit.set()));
    while !quit.wait(Duration::from_secs(3600)) {}
}

fn run_program(background: bool) {
    #[cfg(unix)]
    let signals = watch_signals();
    let manual = !background && platform::started_by_user();
    if !platform::acquire_instance() {
        platform::attach_console();
        let url = crate::doctor::editor_url();
        eprintln!("Typedeck is already running, its editor is at {url}");
        if manual {
            let _ = platform::get().open_url(&url);
        }
        std::process::exit(0);
    }
    log_to_file();
    crate::sync::install_panic_policy();
    let paths = Paths::default_paths();
    let board = Board::with_log(Box::new(SerialConnector::default()), Box::new(|m| crate::log!("{m}")));
    let src = Selector::new(board, crate::capture::new_software());
    let source: Arc<dyn Source> = src.clone();
    let eng = Engine::new(paths.clone(), Some(source.clone())).unwrap_or_else(|e| fatal(&format!("could not load the configuration: {e}")));
    let (ln, port) = server::listen().unwrap_or_else(|e| fatal(&format!("no free port from {} on: {e}", crate::app::DEFAULT_PORT)));
    let srv = Server::new(eng.clone(), source.clone(), paths.clone(), port).unwrap_or_else(|e| fatal(&format!("could not start the editor: {e}")));
    let focus = Arc::downgrade(&srv);
    actions::set_editor(Box::new(move || server::editor_url(port)), Box::new(move || focus.upgrade().is_some_and(|s| s.focus_editor())));
    let _ = std::fs::write(paths.port_file(), port.to_string());
    crate::log!("Typedeck {VERSION}, editor at {}", server::editor_url(port));

    let stop: &'static Flag = Box::leak(Box::new(Flag::new()));
    let e2 = eng.clone();
    crate::sync::spawn("engine", move || e2.run(stop));
    let s2 = source.clone();
    crate::sync::spawn("capture", move || s2.run());
    let srv2 = srv.clone();
    crate::sync::spawn("editor", move || srv2.serve(ln));

    let once: &'static Once = Box::leak(Box::new(Once::new()));
    let (eng_s, src_s) = (eng.clone(), source.clone());
    let shutdown: Arc<dyn Fn() + Send + Sync> = Arc::new(move || {
        once.call_once(|| {
            crate::log!("shutting down");
            stop.set();
            eng_s.stop();
            src_s.close();
            std::thread::sleep(Duration::from_millis(200));
            crate::log!("stopped");
        });
    });
    let (st_src, rel_src, rec_src, sd) = (source.clone(), source.clone(), source.clone(), shutdown.clone());
    platform::start_shell(platform::ShellHooks {
        status: Some(Box::new(move || capture_status(st_src.as_ref()))),
        open_editor: Some(Box::new(move || {
            let _ = platform::get().open_url(&server::editor_url(port));
        })),
        shutdown: Some(Box::new(move || sd())),
        release: Some(Box::new(move || {
            crate::log!("session locked or suspended, releasing the keys");
            rel_src.pause();
        })),
        reclaim: Some(Box::new(move || {
            crate::log!("session back, capturing again");
            rec_src.resume();
        })),
    });
    if manual {
        crate::sync::spawn("open-editor", move || {
            let _ = platform::get().open_url(&server::editor_url(port));
        });
    }
    #[cfg(unix)]
    wait_for_signal(signals);
    #[cfg(windows)]
    wait_for_signal(Box::leak(Box::new(Flag::new())));
    shutdown();
    drop(src);
    std::process::exit(0);
}
