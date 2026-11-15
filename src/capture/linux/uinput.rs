use std::io;
use std::os::fd::{AsRawFd, OwnedFd};
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

use super::sys::*;
use crate::sync::lock;

pub const UINPUT_PATHS: [&str; 2] = ["/dev/uinput", "/dev/input/uinput"];

pub fn open_uinput() -> io::Result<OwnedFd> {
    let mut first: Option<io::Error> = None;
    for p in UINPUT_PATHS {
        match open(Path::new(p)) {
            Ok(fd) => return Ok(fd),
            Err(e) => {
                if first.as_ref().is_none_or(|f| f.kind() == io::ErrorKind::NotFound) {
                    first = Some(e);
                }
            }
        }
    }
    Err(first.unwrap_or_else(|| io::Error::from(io::ErrorKind::NotFound)))
}

pub fn virtual_keys() -> Vec<u16> {
    VIRTUAL_KEY_RANGES.iter().flat_map(|&(a, b)| a..=b).collect()
}

pub type LedFn = Arc<dyn Fn(u16, i32) + Send + Sync>;

pub struct VirtualKeyboard {
    fd: Arc<OwnedFd>,
    write: Mutex<()>,
    stop: Arc<StopPipe>,
    leds: Mutex<Option<JoinHandle<()>>>,
}

fn create(fd: i32) -> io::Result<()> {
    for ev in [EV_KEY, EV_MSC, EV_LED, EV_SYN] {
        ioctl_val(fd, UI_SET_EVBIT, libc::c_ulong::from(ev))?;
    }
    ioctl_val(fd, UI_SET_MSCBIT, libc::c_ulong::from(MSC_SCAN))?;
    for code in virtual_keys() {
        ioctl_val(fd, UI_SET_KEYBIT, libc::c_ulong::from(code))?;
    }
    for led in 0..=LED_MAX {
        ioctl_val(fd, UI_SET_LEDBIT, libc::c_ulong::from(led))?;
    }
    let mut phys = VIRTUAL_PHYS.as_bytes().to_vec();
    phys.push(0);
    ioctl_ptr(fd, UI_SET_PHYS, phys.as_mut_ptr())?;
    let mut setup = UinputSetup { bustype: 0x06, vendor: 0, product: 0, version: 0, name: [0; 80], ff_effects_max: 0 };
    let name = b"Typedeck virtual keyboard";
    setup.name[..name.len()].copy_from_slice(name);
    ioctl_ptr(fd, UI_DEV_SETUP, &mut setup)?;
    ioctl_val(fd, UI_DEV_CREATE, 0)
}

impl VirtualKeyboard {
    pub fn new(on_led: LedFn) -> io::Result<VirtualKeyboard> {
        let fd = open_uinput()?;
        create(fd.as_raw_fd())?;
        let fd = Arc::new(fd);
        let stop = Arc::new(StopPipe::new()?);
        let (f2, s2) = (fd.clone(), stop.clone());
        let leds = crate::sync::spawn("uinput-leds", move || {
            let mut buf = [InputEvent::syn(); 16];
            loop {
                let Some(ready) = wait_readable(&[f2.as_raw_fd(), s2.fd()], -1) else { return };
                if ready[1] {
                    return;
                }
                match read_events(f2.as_raw_fd(), &mut buf) {
                    Ok(n) => {
                        for ev in &buf[..n] {
                            if ev.kind == EV_LED {
                                on_led(ev.code, ev.value);
                            }
                        }
                    }
                    Err(e) if e.kind() == io::ErrorKind::WouldBlock => {}
                    Err(_) => return,
                }
            }
        });
        Ok(VirtualKeyboard { fd, write: Mutex::new(()), stop, leds: Mutex::new(leds) })
    }

    pub fn write(&self, ev: &[InputEvent]) -> io::Result<()> {
        let _g = lock(&self.write);
        write_events(self.fd.as_raw_fd(), ev)
    }

    pub fn keys(&self, codes: &[u16], value: i32) -> io::Result<()> {
        let mut ev: Vec<InputEvent> = codes.iter().map(|&c| InputEvent::key(c, value)).collect();
        ev.push(InputEvent::syn());
        self.write(&ev)
    }
}

impl Drop for VirtualKeyboard {
    fn drop(&mut self) {
        self.stop.fire();
        if let Some(h) = lock(&self.leds).take()
            && h.thread().id() != std::thread::current().id()
        {
            let _ = h.join();
        }
        let _ = ioctl_val(self.fd.as_raw_fd(), UI_DEV_DESTROY, 0);
    }
}
