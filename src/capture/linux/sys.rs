use std::ffi::CString;
use std::io;
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd, RawFd};
use std::path::Path;

pub const EV_SYN: u16 = 0x00;
pub const EV_KEY: u16 = 0x01;
pub const EV_REL: u16 = 0x02;
pub const EV_ABS: u16 = 0x03;
pub const EV_MSC: u16 = 0x04;
pub const EV_LED: u16 = 0x11;
pub const SYN_REPORT: u16 = 0;
pub const SYN_DROPPED: u16 = 3;
pub const MSC_SCAN: u16 = 4;
pub const KEY_MAX: usize = 0x2FF;
pub const LED_MAX: u16 = 0x0F;
pub const VIRTUAL_PHYS: &str = "typedeck-virtual";
pub const VIRTUAL_KEY_RANGES: [(u16, u16); 2] = [(1, 0xFF), (0x160, 0x2BF)];

const IOC_WRITE: u64 = 1;
const IOC_READ: u64 = 2;

pub const fn ioc(dir: u64, kind: u8, nr: u64, size: u64) -> u64 {
    dir << 30 | size << 16 | (kind as u64) << 8 | nr
}

pub const fn eviocgbit(ev: u64, size: u64) -> u64 {
    ioc(IOC_READ, b'E', 0x20 + ev, size)
}

#[cfg(test)]
pub const fn eviocgname(size: u64) -> u64 {
    ioc(IOC_READ, b'E', 0x06, size)
}

pub const fn eviocgphys(size: u64) -> u64 {
    ioc(IOC_READ, b'E', 0x07, size)
}

pub const fn eviocgkey(size: u64) -> u64 {
    ioc(IOC_READ, b'E', 0x18, size)
}

pub const EVIOCGRAB: u64 = ioc(IOC_WRITE, b'E', 0x90, 4);
pub const UI_DEV_CREATE: u64 = ioc(0, b'U', 1, 0);
pub const UI_DEV_DESTROY: u64 = ioc(0, b'U', 2, 0);
pub const SETUP_SIZE: u64 = 92;
pub const UI_DEV_SETUP: u64 = ioc(IOC_WRITE, b'U', 3, SETUP_SIZE);
pub const UI_SET_EVBIT: u64 = ioc(IOC_WRITE, b'U', 100, 4);
pub const UI_SET_KEYBIT: u64 = ioc(IOC_WRITE, b'U', 101, 4);
pub const UI_SET_MSCBIT: u64 = ioc(IOC_WRITE, b'U', 104, 4);
pub const UI_SET_LEDBIT: u64 = ioc(IOC_WRITE, b'U', 105, 4);
pub const UI_SET_PHYS: u64 = ioc(IOC_WRITE, b'U', 108, std::mem::size_of::<usize>() as u64);

#[repr(C)]
#[derive(Clone, Copy)]
pub struct InputEvent {
    pub time: libc::timeval,
    pub kind: u16,
    pub code: u16,
    pub value: i32,
}

impl InputEvent {
    pub fn new(kind: u16, code: u16, value: i32) -> InputEvent {
        InputEvent { time: libc::timeval { tv_sec: 0, tv_usec: 0 }, kind, code, value }
    }

    pub fn key(code: u16, value: i32) -> InputEvent {
        InputEvent::new(EV_KEY, code, value)
    }

    pub fn syn() -> InputEvent {
        InputEvent::new(EV_SYN, SYN_REPORT, 0)
    }
}

impl std::fmt::Debug for InputEvent {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "({} {} {})", self.kind, self.code, self.value)
    }
}

pub const EVENT_SIZE: usize = std::mem::size_of::<InputEvent>();

#[repr(C)]
pub struct UinputSetup {
    pub bustype: u16,
    pub vendor: u16,
    pub product: u16,
    pub version: u16,
    pub name: [u8; 80],
    pub ff_effects_max: u32,
}

pub fn open(path: &Path) -> io::Result<OwnedFd> {
    let c = CString::new(path.as_os_str().as_encoded_bytes()).map_err(|_| io::Error::from(io::ErrorKind::InvalidInput))?;
    let fd = unsafe { libc::open(c.as_ptr(), libc::O_RDWR | libc::O_NONBLOCK | libc::O_CLOEXEC) };
    if fd < 0 {
        return Err(io::Error::last_os_error());
    }
    Ok(unsafe { OwnedFd::from_raw_fd(fd) })
}

pub fn ioctl_ptr<T>(fd: RawFd, req: u64, arg: *mut T) -> io::Result<()> {
    let r = unsafe { libc::ioctl(fd, req as _, arg) };
    if r < 0 { Err(io::Error::last_os_error()) } else { Ok(()) }
}

pub fn ioctl_val(fd: RawFd, req: u64, v: libc::c_ulong) -> io::Result<()> {
    let r = unsafe { libc::ioctl(fd, req as _, v) };
    if r < 0 { Err(io::Error::last_os_error()) } else { Ok(()) }
}

pub fn ioctl_string(fd: RawFd, req: u64) -> String {
    let mut buf = [0u8; 256];
    if ioctl_ptr(fd, req, buf.as_mut_ptr()).is_err() {
        return String::new();
    }
    let end = buf.iter().position(|b| *b == 0).unwrap_or(buf.len());
    String::from_utf8_lossy(&buf[..end]).into_owned()
}

pub fn bit_set(bits: &[u8], n: usize) -> bool {
    n / 8 < bits.len() && bits[n / 8] >> (n % 8) & 1 == 1
}

pub fn event_bits(fd: RawFd, ev: u64, size: usize) -> Vec<u8> {
    let mut bits = vec![0u8; size];
    if ioctl_ptr(fd, eviocgbit(ev, size as u64), bits.as_mut_ptr()).is_err() {
        return Vec::new();
    }
    bits
}

pub fn pressed_keys(fd: RawFd) -> io::Result<Vec<u8>> {
    let mut bits = vec![0u8; KEY_MAX / 8 + 1];
    ioctl_ptr(fd, eviocgkey(bits.len() as u64), bits.as_mut_ptr())?;
    Ok(bits)
}

pub fn is_keyboard(fd: RawFd) -> bool {
    let types = event_bits(fd, 0, 4);
    if !bit_set(&types, usize::from(EV_KEY)) || bit_set(&types, usize::from(EV_REL)) || bit_set(&types, usize::from(EV_ABS)) {
        return false;
    }
    let keys = event_bits(fd, u64::from(EV_KEY), KEY_MAX / 8 + 1);
    if !crate::capture::evdev_table().list[..26].iter().all(|(_, code)| bit_set(&keys, usize::from(*code))) {
        return false;
    }
    !ioctl_string(fd, eviocgphys(256)).starts_with(VIRTUAL_PHYS)
}

pub fn read_events(fd: RawFd, buf: &mut [InputEvent]) -> io::Result<usize> {
    let n = unsafe { libc::read(fd, buf.as_mut_ptr().cast(), std::mem::size_of_val(buf)) };
    if n < 0 {
        return Err(io::Error::last_os_error());
    }
    if n == 0 {
        return Err(io::Error::from(io::ErrorKind::UnexpectedEof));
    }
    Ok(n as usize / EVENT_SIZE)
}

pub fn write_events(fd: RawFd, ev: &[InputEvent]) -> io::Result<()> {
    if ev.is_empty() {
        return Ok(());
    }
    let len = std::mem::size_of_val(ev);
    let n = unsafe { libc::write(fd, ev.as_ptr().cast(), len) };
    if n < 0 {
        return Err(io::Error::last_os_error());
    }
    if n as usize != len {
        return Err(io::Error::from(io::ErrorKind::WriteZero));
    }
    Ok(())
}

pub struct StopPipe {
    rd: OwnedFd,
    wr: OwnedFd,
}

impl StopPipe {
    pub fn new() -> io::Result<StopPipe> {
        let mut fds = [0 as RawFd; 2];
        if unsafe { libc::pipe2(fds.as_mut_ptr(), libc::O_CLOEXEC | libc::O_NONBLOCK) } < 0 {
            return Err(io::Error::last_os_error());
        }
        Ok(unsafe { StopPipe { rd: OwnedFd::from_raw_fd(fds[0]), wr: OwnedFd::from_raw_fd(fds[1]) } })
    }

    pub fn fire(&self) {
        let b = [1u8];
        unsafe {
            libc::write(self.wr.as_raw_fd(), b.as_ptr().cast(), 1);
        }
    }

    pub fn fd(&self) -> RawFd {
        self.rd.as_raw_fd()
    }
}

pub fn wait_readable(fds: &[RawFd], timeout_ms: i32) -> Option<Vec<bool>> {
    let mut p: Vec<libc::pollfd> = fds.iter().map(|&fd| libc::pollfd { fd, events: libc::POLLIN, revents: 0 }).collect();
    loop {
        let r = unsafe { libc::poll(p.as_mut_ptr(), p.len() as libc::nfds_t, timeout_ms) };
        if r < 0 {
            if io::Error::last_os_error().kind() == io::ErrorKind::Interrupted {
                continue;
            }
            return None;
        }
        return Some(p.iter().map(|x| x.revents != 0).collect());
    }
}

pub fn ignore_tstp(on: bool) {
    unsafe {
        libc::signal(libc::SIGTSTP, if on { libc::SIG_IGN } else { libc::SIG_DFL });
    }
}
