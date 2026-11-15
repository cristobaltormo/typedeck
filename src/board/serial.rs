use std::collections::HashMap;
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use super::{Connector, OpenError, Port};
use crate::sync::lock;

#[cfg(unix)]
pub use unix::open_port;
#[cfg(windows)]
pub use windows::open_port;

#[derive(Default)]
pub struct SerialConnector {
    rejected: Mutex<HashMap<String, Instant>>,
}

pub fn glob(pattern: &str) -> Vec<String> {
    let p = Path::new(pattern);
    let (Some(dir), Some(name)) = (p.parent(), p.file_name().and_then(|n| n.to_str())) else {
        return vec![];
    };
    if !name.contains(['*', '?']) {
        return if p.exists() { vec![pattern.to_string()] } else { vec![] };
    }
    let dir = if dir.as_os_str().is_empty() { Path::new(".") } else { dir };
    let mut out: Vec<String> = std::fs::read_dir(dir)
        .map(|rd| {
            rd.flatten().filter(|e| e.file_name().to_str().is_some_and(|n| wildcard(name, n))).map(|e| dir.join(e.file_name()).display().to_string()).collect()
        })
        .unwrap_or_default();
    out.sort();
    out
}

pub fn wildcard(pat: &str, s: &str) -> bool {
    let (p, s): (Vec<char>, Vec<char>) = (pat.chars().collect(), s.chars().collect());
    let (mut pi, mut si, mut star, mut mark) = (0, 0, None, 0);
    while si < s.len() {
        if pi < p.len() && (p[pi] == '?' || p[pi] == s[si]) {
            pi += 1;
            si += 1;
        } else if pi < p.len() && p[pi] == '*' {
            star = Some(pi);
            mark = si;
            pi += 1;
        } else if let Some(st) = star {
            pi = st + 1;
            mark += 1;
            si = mark;
        } else {
            return false;
        }
    }
    p[pi..].iter().all(|c| *c == '*')
}

fn candidates() -> Vec<String> {
    let Some(v) = std::env::var("TYPEDECK_PORT").ok().map(|v| v.trim().to_string()).filter(|v| !v.is_empty()) else {
        return system_ports();
    };
    let mut out = Vec::new();
    for p in v.split(',').map(str::trim).filter(|p| !p.is_empty()) {
        let m = glob(p);
        if m.is_empty() {
            out.push(p.to_string());
        } else {
            out.extend(m);
        }
    }
    out.sort();
    out
}

#[cfg(target_os = "linux")]
fn system_ports() -> Vec<String> {
    glob("/dev/ttyACM*")
}

#[cfg(target_os = "macos")]
fn system_ports() -> Vec<String> {
    glob("/dev/cu.usbmodem*")
}

#[cfg(windows)]
fn system_ports() -> Vec<String> {
    windows::system_ports()
}

enum Probe {
    No,
    Ok,
    Old,
}

fn probe(port: &dyn Port) -> Probe {
    std::thread::sleep(Duration::from_millis(300));
    if port.write_all(b"WHO\n").is_err() {
        return Probe::No;
    }
    let deadline = Instant::now() + Duration::from_millis(900);
    let mut line = Vec::new();
    let mut buf = [0u8; 256];
    while Instant::now() < deadline {
        let n = match port.read(&mut buf) {
            Ok(0) => return Probe::No,
            Ok(n) => n,
            Err(e) if e.kind() == std::io::ErrorKind::TimedOut => continue,
            Err(_) => return Probe::No,
        };
        for &b in &buf[..n] {
            if b != b'\n' {
                line.push(b);
                continue;
            }
            let l = String::from_utf8_lossy(&line).trim().to_string();
            line.clear();
            if l.starts_with(crate::app::FIRMWARE_ID) {
                return Probe::Ok;
            }
            if l.starts_with("NUMDECK-FW") || l.starts_with("LEONARDO-MACROS") {
                return Probe::Old;
            }
        }
    }
    Probe::No
}

impl Connector for SerialConnector {
    fn open(&self) -> Result<(Arc<dyn Port>, String), OpenError> {
        let mut outdated = None;
        for path in candidates() {
            if lock(&self.rejected).get(&path).is_some_and(|t| t.elapsed() < Duration::from_secs(5)) {
                continue;
            }
            let Ok(port) = open_port(&path, Some(Duration::from_millis(100))) else { continue };
            match probe(port.as_ref()) {
                Probe::Ok => {
                    port.set_read_timeout(None);
                    return Ok((port, path));
                }
                Probe::Old => outdated = Some(path.clone()),
                Probe::No => {}
            }
            port.close();
            lock(&self.rejected).insert(path, Instant::now());
        }
        match outdated {
            Some(p) => Err(OpenError::Outdated(p)),
            None => Err(OpenError::NotFound),
        }
    }
}

#[cfg(unix)]
mod unix {
    use std::ffi::CString;
    use std::io;
    use std::os::fd::{AsRawFd, FromRawFd, OwnedFd};
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, AtomicI32, Ordering};
    use std::time::Duration;

    use super::super::Port;

    pub struct UnixPort {
        fd: OwnedFd,
        stop: [OwnedFd; 2],
        closed: AtomicBool,
        timeout_ms: AtomicI32,
    }

    impl UnixPort {
        pub fn set_read_timeout(&self, d: Option<Duration>) {
            self.timeout_ms.store(d.map_or(-1, |d| d.as_millis() as i32), Ordering::SeqCst);
        }
    }

    fn set_raw(fd: i32) -> io::Result<()> {
        let mut t: libc::termios = unsafe { std::mem::zeroed() };
        if unsafe { libc::tcgetattr(fd, &mut t) } != 0 {
            return Err(io::Error::last_os_error());
        }
        t.c_iflag &= !(libc::IGNBRK | libc::BRKINT | libc::PARMRK | libc::ISTRIP | libc::INLCR | libc::IGNCR | libc::ICRNL | libc::IXON);
        t.c_oflag &= !libc::OPOST;
        t.c_lflag &= !(libc::ECHO | libc::ECHONL | libc::ICANON | libc::ISIG | libc::IEXTEN);
        t.c_cflag &= !(libc::CSIZE | libc::PARENB);
        t.c_cflag |= libc::CS8 | libc::CREAD | libc::CLOCAL;
        t.c_cc[libc::VMIN] = 1;
        t.c_cc[libc::VTIME] = 0;
        unsafe {
            libc::cfsetispeed(&mut t, libc::B115200);
            libc::cfsetospeed(&mut t, libc::B115200);
        }
        if unsafe { libc::tcsetattr(fd, libc::TCSANOW, &t) } != 0 {
            return Err(io::Error::last_os_error());
        }
        Ok(())
    }

    pub fn open_port(path: &str, timeout: Option<Duration>) -> io::Result<Arc<UnixPortHandle>> {
        let c = CString::new(path).map_err(|_| io::Error::from(io::ErrorKind::InvalidInput))?;
        let fd = unsafe { libc::open(c.as_ptr(), libc::O_RDWR | libc::O_NOCTTY | libc::O_CLOEXEC | libc::O_NONBLOCK) };
        if fd < 0 {
            return Err(io::Error::last_os_error());
        }
        let fd = unsafe { OwnedFd::from_raw_fd(fd) };
        set_raw(fd.as_raw_fd())?;
        let stop = crate::platform::cloexec_pipe()?;
        let port = UnixPort { fd, stop, closed: AtomicBool::new(false), timeout_ms: AtomicI32::new(-1) };
        port.set_read_timeout(timeout);
        Ok(Arc::new(UnixPortHandle(port)))
    }

    #[cfg(not(target_os = "macos"))]
    fn wait(fd: i32, stop: i32, write: bool, timeout_ms: i32) -> io::Result<(bool, bool)> {
        let mut fds = [
            libc::pollfd { fd, events: if write { libc::POLLOUT } else { libc::POLLIN }, revents: 0 },
            libc::pollfd { fd: stop, events: libc::POLLIN, revents: 0 },
        ];
        if unsafe { libc::poll(fds.as_mut_ptr(), 2, timeout_ms) } < 0 {
            return Err(io::Error::last_os_error());
        }
        Ok((fds[0].revents != 0, fds[1].revents != 0))
    }

    // poll() does not work on macOS character devices, so the serial port waits with select().
    #[cfg(target_os = "macos")]
    fn wait(fd: i32, stop: i32, write: bool, timeout_ms: i32) -> io::Result<(bool, bool)> {
        unsafe {
            let mut rd: libc::fd_set = std::mem::zeroed();
            let mut wr: libc::fd_set = std::mem::zeroed();
            libc::FD_ZERO(&mut rd);
            libc::FD_ZERO(&mut wr);
            libc::FD_SET(stop, &mut rd);
            if write {
                libc::FD_SET(fd, &mut wr);
            } else {
                libc::FD_SET(fd, &mut rd);
            }
            let mut tv = libc::timeval { tv_sec: (timeout_ms / 1000) as _, tv_usec: ((timeout_ms % 1000) * 1000) as _ };
            let tvp = if timeout_ms < 0 { std::ptr::null_mut() } else { &mut tv as *mut libc::timeval };
            if libc::select(fd.max(stop) + 1, &mut rd, &mut wr, std::ptr::null_mut(), tvp) < 0 {
                return Err(io::Error::last_os_error());
            }
            let ready = if write { libc::FD_ISSET(fd, &wr) } else { libc::FD_ISSET(fd, &rd) };
            Ok((ready, libc::FD_ISSET(stop, &rd)))
        }
    }

    pub struct UnixPortHandle(UnixPort);

    impl UnixPortHandle {
        pub fn set_read_timeout(&self, d: Option<Duration>) {
            self.0.set_read_timeout(d);
        }
    }

    impl Port for UnixPortHandle {
        fn read(&self, buf: &mut [u8]) -> io::Result<usize> {
            let p = &self.0;
            loop {
                if p.closed.load(Ordering::SeqCst) {
                    return Ok(0);
                }
                let (ready, stop) = match wait(p.fd.as_raw_fd(), p.stop[0].as_raw_fd(), false, p.timeout_ms.load(Ordering::SeqCst)) {
                    Err(e) if e.kind() == io::ErrorKind::Interrupted => continue,
                    r => r?,
                };
                if stop {
                    return Ok(0);
                }
                if !ready {
                    return Err(io::Error::from(io::ErrorKind::TimedOut));
                }
                let n = unsafe { libc::read(p.fd.as_raw_fd(), buf.as_mut_ptr().cast(), buf.len()) };
                if n < 0 {
                    let e = io::Error::last_os_error();
                    if e.kind() == io::ErrorKind::WouldBlock || e.kind() == io::ErrorKind::Interrupted {
                        continue;
                    }
                    return Err(e);
                }
                return Ok(n as usize);
            }
        }

        fn write_all(&self, mut b: &[u8]) -> io::Result<()> {
            let p = &self.0;
            while !b.is_empty() {
                if p.closed.load(Ordering::SeqCst) {
                    return Err(io::Error::from(io::ErrorKind::BrokenPipe));
                }
                let n = unsafe { libc::write(p.fd.as_raw_fd(), b.as_ptr().cast(), b.len()) };
                if n < 0 {
                    let e = io::Error::last_os_error();
                    if e.kind() == io::ErrorKind::WouldBlock || e.kind() == io::ErrorKind::Interrupted {
                        match wait(p.fd.as_raw_fd(), p.stop[0].as_raw_fd(), true, 1000) {
                            Ok((true, false)) => continue,
                            Err(e) if e.kind() == io::ErrorKind::Interrupted => continue,
                            Ok((_, true)) => return Err(io::Error::from(io::ErrorKind::BrokenPipe)),
                            _ => return Err(io::Error::from(io::ErrorKind::TimedOut)),
                        }
                    }
                    return Err(e);
                }
                b = &b[n as usize..];
            }
            Ok(())
        }

        fn close(&self) {
            if !self.0.closed.swap(true, Ordering::SeqCst) {
                let one = [1u8];
                unsafe {
                    libc::write(self.0.stop[1].as_raw_fd(), one.as_ptr().cast(), 1);
                }
            }
        }
    }
}

#[cfg(windows)]
mod windows;
