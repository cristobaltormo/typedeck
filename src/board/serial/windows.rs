use std::io;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::time::Duration;

use windows_sys::Win32::Devices::Communication::{COMMTIMEOUTS, DCB, EscapeCommFunction, GetCommState, PurgeComm, SETDTR, SetCommState, SetCommTimeouts};
use windows_sys::Win32::Foundation::{
    CloseHandle, ERROR_IO_PENDING, ERROR_OPERATION_ABORTED, GENERIC_READ, GENERIC_WRITE, HANDLE, INVALID_HANDLE_VALUE, WAIT_TIMEOUT,
};
use windows_sys::Win32::Storage::FileSystem::{CreateFileW, FILE_ATTRIBUTE_NORMAL, FILE_FLAG_OVERLAPPED, OPEN_EXISTING, ReadFile, WriteFile};
use windows_sys::Win32::System::IO::{CancelIoEx, GetOverlappedResult, OVERLAPPED};
use windows_sys::Win32::System::Registry::HKEY_LOCAL_MACHINE;
use windows_sys::Win32::System::Threading::{CreateEventW, WaitForSingleObject};

use super::super::Port;
use crate::platform::reg;
use crate::sync::lock;

struct Event(HANDLE);

unsafe impl Send for Event {}
unsafe impl Sync for Event {}

impl Event {
    fn new() -> io::Result<Event> {
        let h = unsafe { CreateEventW(std::ptr::null(), 1, 0, std::ptr::null()) };
        if h.is_null() {
            return Err(io::Error::last_os_error());
        }
        Ok(Event(h))
    }
}

impl Drop for Event {
    fn drop(&mut self) {
        unsafe {
            CloseHandle(self.0);
        }
    }
}

pub struct WinPort {
    h: RwLock<HANDLE>,
    closed: AtomicBool,
    timeout_ms: AtomicU32,
    rmu: Mutex<Event>,
    wmu: Mutex<Event>,
}

unsafe impl Send for WinPort {}
unsafe impl Sync for WinPort {}

fn wide(s: &str) -> Vec<u16> {
    s.encode_utf16().chain(std::iter::once(0)).collect()
}

fn configure(h: HANDLE) -> io::Result<()> {
    let mut d: DCB = unsafe { std::mem::zeroed() };
    d.DCBlength = std::mem::size_of::<DCB>() as u32;
    if unsafe { GetCommState(h, &mut d) } == 0 {
        return Err(io::Error::last_os_error());
    }
    d.BaudRate = 115200;
    d.ByteSize = 8;
    d.Parity = 0;
    d.StopBits = 0;
    d._bitfield = 1 | 1 << 4 | 1 << 12;
    if unsafe { SetCommState(h, &d) } == 0 {
        return Err(io::Error::last_os_error());
    }
    let t = COMMTIMEOUTS {
        ReadIntervalTimeout: u32::MAX,
        ReadTotalTimeoutMultiplier: u32::MAX,
        ReadTotalTimeoutConstant: 100,
        WriteTotalTimeoutMultiplier: 0,
        WriteTotalTimeoutConstant: 1000,
    };
    unsafe {
        SetCommTimeouts(h, &t);
        EscapeCommFunction(h, SETDTR);
        PurgeComm(h, 0xF);
    }
    Ok(())
}

pub fn open_port(path: &str, timeout: Option<Duration>) -> io::Result<Arc<WinPort>> {
    let name = wide(&format!(r"\\.\{path}"));
    let h = unsafe {
        CreateFileW(
            name.as_ptr(),
            GENERIC_READ | GENERIC_WRITE,
            0,
            std::ptr::null(),
            OPEN_EXISTING,
            FILE_ATTRIBUTE_NORMAL | FILE_FLAG_OVERLAPPED,
            std::ptr::null_mut(),
        )
    };
    if h == INVALID_HANDLE_VALUE {
        return Err(io::Error::last_os_error());
    }
    if let Err(e) = configure(h) {
        unsafe {
            CloseHandle(h);
        }
        return Err(e);
    }
    let port = WinPort {
        h: RwLock::new(h),
        closed: AtomicBool::new(false),
        timeout_ms: AtomicU32::new(u32::MAX),
        rmu: Mutex::new(Event::new()?),
        wmu: Mutex::new(Event::new()?),
    };
    port.set_read_timeout(timeout);
    Ok(Arc::new(port))
}

impl WinPort {
    pub fn set_read_timeout(&self, d: Option<Duration>) {
        self.timeout_ms.store(d.map_or(u32::MAX, |d| d.as_millis() as u32), Ordering::SeqCst);
    }

    fn io(&self, ev: &Event, write: bool, buf: *mut u8, len: u32) -> io::Result<usize> {
        let h = *self.h.read().unwrap_or_else(|e| e.into_inner());
        if self.closed.load(Ordering::SeqCst) {
            return Err(io::Error::from(io::ErrorKind::BrokenPipe));
        }
        let mut ov: OVERLAPPED = unsafe { std::mem::zeroed() };
        ov.hEvent = ev.0;
        let ok = unsafe { if write { WriteFile(h, buf, len, std::ptr::null_mut(), &mut ov) } else { ReadFile(h, buf, len, std::ptr::null_mut(), &mut ov) } };
        if ok == 0 {
            let e = io::Error::last_os_error();
            if e.raw_os_error() != Some(ERROR_IO_PENDING as i32) {
                return Err(e);
            }
            if !write && unsafe { WaitForSingleObject(ev.0, 200) } == WAIT_TIMEOUT {
                unsafe { CancelIoEx(h, &ov) };
            }
        }
        let mut n = 0u32;
        if unsafe { GetOverlappedResult(h, &ov, &mut n, 1) } == 0 {
            let e = io::Error::last_os_error();
            if e.raw_os_error() == Some(ERROR_OPERATION_ABORTED as i32) && !self.closed.load(Ordering::SeqCst) {
                return Ok(0);
            }
            return Err(e);
        }
        Ok(n as usize)
    }
}

impl Port for WinPort {
    fn read(&self, buf: &mut [u8]) -> io::Result<usize> {
        let ev = lock(&self.rmu);
        let limit = self.timeout_ms.load(Ordering::SeqCst);
        let mut waited = 0u32;
        loop {
            if self.closed.load(Ordering::SeqCst) {
                return Ok(0);
            }
            let n = self.io(&ev, false, buf.as_mut_ptr(), buf.len().min(4096) as u32)?;
            if self.closed.load(Ordering::SeqCst) {
                return Ok(0);
            }
            if n > 0 {
                return Ok(n);
            }
            waited = waited.saturating_add(150);
            if limit != u32::MAX && waited >= limit {
                return Err(io::Error::from(io::ErrorKind::TimedOut));
            }
        }
    }

    fn write_all(&self, b: &[u8]) -> io::Result<()> {
        let ev = lock(&self.wmu);
        let mut data = b.to_vec();
        let n = self.io(&ev, true, data.as_mut_ptr(), data.len() as u32)?;
        if n < data.len() {
            return Err(io::Error::from(io::ErrorKind::WriteZero));
        }
        Ok(())
    }

    fn close(&self) {
        if self.closed.swap(true, Ordering::SeqCst) {
            return;
        }
        let h = *self.h.read().unwrap_or_else(|e| e.into_inner());
        unsafe {
            CancelIoEx(h, std::ptr::null());
        }
        let _r = lock(&self.rmu);
        let _w = lock(&self.wmu);
        let mut g = self.h.write().unwrap_or_else(|e| e.into_inner());
        unsafe {
            CloseHandle(*g);
        }
        *g = INVALID_HANDLE_VALUE;
    }
}

const SERIALCOMM: &str = r"HARDWARE\DEVICEMAP\SERIALCOMM";
const USB_ENUM: &str = r"SYSTEM\CurrentControlSet\Enum\USB";

pub fn system_ports() -> Vec<String> {
    let present: Vec<String> = reg::string_values(HKEY_LOCAL_MACHINE, SERIALCOMM).into_iter().map(|(_, v)| v).collect();
    let mut usb = Vec::new();
    for dev in reg::sub_keys(HKEY_LOCAL_MACHINE, USB_ENUM) {
        if !super::super::is_board_key(&dev) {
            continue;
        }
        let mut ports = Vec::new();
        for inst in reg::sub_keys(HKEY_LOCAL_MACHINE, &format!(r"{USB_ENUM}\{dev}")) {
            if let Some(p) = reg::string(HKEY_LOCAL_MACHINE, &format!(r"{USB_ENUM}\{dev}\{inst}\Device Parameters"), "PortName") {
                ports.push(p);
            }
        }
        usb.push((dev, ports));
    }
    super::super::board_ports(&usb, &present)
}
