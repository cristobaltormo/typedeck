use std::collections::VecDeque;
use std::sync::Condvar;

use super::*;
use crate::capture::Event;
use crate::hid::MOD_GUI;
use crate::sync::{wait_timeout, wait_until};

#[derive(Default)]
struct Pipe {
    buf: Mutex<(VecDeque<u8>, bool)>,
    cv: Condvar,
}

impl Pipe {
    fn write(&self, b: &[u8]) {
        lock(&self.buf).0.extend(b);
        self.cv.notify_all();
    }

    fn read(&self, out: &mut [u8]) -> usize {
        let mut g = lock(&self.buf);
        loop {
            if !g.0.is_empty() {
                let n = out.len().min(g.0.len());
                for (i, b) in g.0.drain(..n).enumerate() {
                    out[i] = b;
                }
                return n;
            }
            if g.1 {
                return 0;
            }
            g = wait_timeout(&self.cv, g, Duration::from_millis(50));
        }
    }

    fn close(&self) {
        lock(&self.buf).1 = true;
        self.cv.notify_all();
    }
}

struct FakePort {
    to_prog: Arc<Pipe>,
    to_fw: Arc<Pipe>,
}

impl Port for FakePort {
    fn read(&self, buf: &mut [u8]) -> io::Result<usize> {
        Ok(self.to_prog.read(buf))
    }
    fn write_all(&self, b: &[u8]) -> io::Result<()> {
        self.to_fw.write(b);
        Ok(())
    }
    fn close(&self) {
        self.to_prog.close();
        self.to_fw.close();
    }
}

struct Firmware {
    to_prog: Arc<Pipe>,
    got: Mutex<Vec<String>>,
}

impl Firmware {
    fn say(&self, s: &str) {
        self.to_prog.write(format!("{s}\n").as_bytes());
    }

    fn commands(&self) -> Vec<String> {
        lock(&self.got).clone()
    }
}

struct FakeConn {
    fw: Option<Arc<Firmware>>,
    to_prog: Arc<Pipe>,
    to_fw: Arc<Pipe>,
}

impl Connector for FakeConn {
    fn open(&self) -> Result<(Arc<dyn Port>, String), OpenError> {
        if self.fw.is_none() {
            return Err(OpenError::NotFound);
        }
        Ok((Arc::new(FakePort { to_prog: self.to_prog.clone(), to_fw: self.to_fw.clone() }), "fake0".into()))
    }
}

fn pair() -> (FakeConn, Arc<Firmware>) {
    let (to_prog, to_fw) = (Arc::new(Pipe::default()), Arc::new(Pipe::default()));
    let fw = Arc::new(Firmware { to_prog: to_prog.clone(), got: Mutex::new(vec![]) });
    let (f2, rx) = (fw.clone(), to_fw.clone());
    std::thread::spawn(move || {
        let mut line = Vec::new();
        let mut buf = [0u8; 256];
        loop {
            let n = rx.read(&mut buf);
            if n == 0 {
                return;
            }
            for &b in &buf[..n] {
                if b != b'\n' {
                    line.push(b);
                    continue;
                }
                let l = String::from_utf8_lossy(&line).to_string();
                line.clear();
                lock(&f2.got).push(l.clone());
                match l.as_str() {
                    "WHO" => f2.say("TYPEDECK-FW 4"),
                    "INFO" => {
                        f2.say(r#"vid=258A pid=0016 bcd=0001 usb=0200 class=00 ep0=8 mfr="BY Tech" prod="Usb Gaming Keyboard" serial="" cfglen=59 ifaces=2 power=500 hidlen0=67 hidlen1=227"#);
                        f2.say("END");
                    }
                    l if l.starts_with("RDESC 0") => {
                        f2.say("05 01 09 06 A1 01 05 07 19 E0 29 E7 15 00 25 01 95 08 75 01 81 02 95 01 75 08 81 03 95 06 75 08 15 00 26 FF 00 05 07 19 00 2A FF 00 81 00 C0");
                        f2.say("END");
                    }
                    l if l.starts_with("RDESC") => {
                        f2.say("05 0C 09 01 A1 01 85 02 19 00 2A FF 02 15 00 26 FF 7F 95 01 75 10 81 00 C0");
                        f2.say("END");
                    }
                    "BOOTLOG" => f2.say("cold=1 recoveries=0 slow=0 first_seen_ds=12 this_boot=cold"),
                    "SYS" => f2.say("mcu=atmega32u4 f_cpu=16000000 board=leonardo fw=4 vcc_mv=5000 free_ram=900 max3421e_rev=3 uptime_s=10"),
                    "HB" => {}
                    l if ["MASK ", "KEY ", "CONS ", "WATCH ", "DARK ", "KEYS "].iter().any(|p| l.starts_with(p)) || l == "REBOOT" => f2.say("OK"),
                    _ => {}
                }
            }
        }
    });
    (FakeConn { fw: Some(fw.clone()), to_prog, to_fw }, fw)
}

#[test]
fn handshake_info_events_and_commands() {
    let (conn, fw) = pair();
    let b = Board::new(Box::new(conn));
    let events = Arc::new(Mutex::new(Vec::new()));
    let connected = Arc::new(Mutex::new(None::<Info>));
    let (e2, c2) = (events.clone(), connected.clone());
    b.bind(Handlers {
        on_event: Some(Arc::new(move |e| lock(&e2).push(e))),
        on_connected: Some(Arc::new(move |i: &Info| *lock(&c2) = Some(i.clone()))),
        mask: Some(Arc::new(|| {
            let mut m = [0u8; 32];
            m[0x5F >> 3] |= 1 << (0x5F & 7);
            m
        })),
        on_disconnect: None,
    });
    let b2 = b.clone();
    std::thread::spawn(move || b2.run());
    assert!(wait_until(Duration::from_secs(3), || lock(&connected).is_some()));
    let info = lock(&connected).clone().unwrap();
    assert_eq!(
        (info.firmware.as_str(), info.vid.as_str(), info.prod.as_str(), info.mfr.as_str(), info.power_ma),
        ("4", "258A", "Usb Gaming Keyboard", "BY Tech", 500)
    );
    assert_eq!(info.id(), "258A:0016");
    let kinds: Vec<&str> = info.reports.iter().map(|r| r.kind.as_str()).collect();
    assert!(kinds.contains(&"keyboard") && kinds.contains(&"consumer"));
    assert!(fw.commands().iter().any(|c| c == &format!("MASK {}80{}", "0".repeat(22), "0".repeat(40))), "{:?}", fw.commands());
    fw.say("D 5F 02");
    fw.say("U 5F");
    fw.say("W 04");
    fw.say("K 0");
    assert!(wait_until(Duration::from_secs(2), || lock(&events).len() == 4));
    let e = lock(&events).clone();
    assert_eq!(e[0], Event::new(b'D', 0x5F, 0x02));
    assert_eq!(e[1].kind, b'U');
    assert_eq!(e[2], Event::new(b'W', 0x04, 0));
    assert_eq!((e[3].kind, e[3].arg), (b'K', 0));
    b.tap(Stroke::new(0x04, MOD_GUI)).unwrap();
    b.tap_held(Stroke::new(0x39, 0), 150).unwrap();
    b.consumer(0xCD).unwrap();
    b.watch(true).unwrap();
    for (name, arg) in [("bootlog", 0), ("reboot", 0), ("dark", 7)] {
        b.command(name, arg).unwrap();
    }
    assert_eq!(b.sys().unwrap().board, "leonardo");
    let got = fw.commands().join("|");
    for want in ["KEY 08 04", "KEY 00 39 150", "CONS CD", "WATCH 1", "BOOTLOG", "REBOOT", "DARK 1"] {
        assert!(got.contains(want), "missing {want} in {got}");
    }
    assert!(b.command("rm -rf", 0).is_err());
    b.pause();
    let zero = format!("MASK {}", "0".repeat(64));
    assert!(wait_until(Duration::from_secs(1), || fw.commands().contains(&zero)));
    b.close();
}

#[test]
fn heartbeat_keeps_flowing() {
    let (conn, fw) = pair();
    let b = Board::new(Box::new(conn));
    let b2 = b.clone();
    std::thread::spawn(move || b2.run());
    assert!(wait_until(Duration::from_secs(4), || fw.commands().iter().filter(|c| *c == "HB").count() >= 2));
    b.close();
    let zero = format!("MASK {}", "0".repeat(64));
    assert!(wait_until(Duration::from_secs(1), || fw.commands().contains(&zero)));
}

#[test]
fn parse_event_rejects_garbage() {
    for l in ["", "D", "D ZZ", "D 123 00", "X 05", "OK", "TYPEDECK-FW 4", "K 2"] {
        assert!(parse_event(l).is_none(), "{l}");
    }
    assert_eq!(parse_event("D 5F"), Some(Event::new(b'D', 0x5F, 0)));
    assert_eq!(parse_event("D 5F 02 extra"), Some(Event::new(b'D', 0x5F, 2)));
}

#[test]
fn no_board_keeps_retrying_and_stops_on_close() {
    let b = Board::new(Box::new(FakeConn { fw: None, to_prog: Arc::new(Pipe::default()), to_fw: Arc::new(Pipe::default()) }));
    let b2 = b.clone();
    let h = std::thread::spawn(move || b2.run());
    std::thread::sleep(Duration::from_millis(50));
    assert!(!b.connected());
    assert_eq!(b.error(), "not_found");
    b.close();
    assert!(wait_until(Duration::from_secs(2), || h.is_finished()));
}

#[test]
fn key_values_and_info() {
    let kv = key_values(r#"vid=258a mfr="BY Tech" serial="" n=5 bad= x"#);
    assert_eq!(kv, vec![("vid".into(), "258a".into()), ("mfr".into(), "BY Tech".into()), ("serial".into(), String::new()), ("n".into(), "5".into())]);
    let info = parse_info(&["vid=258a pid=0016 hidlen1=227".into()]);
    assert_eq!((info.vid.as_str(), info.hid_len.clone()), ("258A", vec![0, 227]));
    assert_eq!(parse_info(&["NODEVICE".into()]), Info::default());
    assert_eq!(parse_info(&[]), Info::default());
    let s = parse_sys("mcu=atmega32u4 f_cpu=16000000 board=leonardo max3421e_rev=3");
    assert_eq!((s.f_cpu, s.max3421_rev.as_str()), (16_000_000, "3"));
}

#[test]
fn usb_ids() {
    assert_eq!(parse_usb_key("VID_2341&PID_8036"), Some(("2341".into(), "8036".into())));
    assert_eq!(parse_usb_key("vid_1b4f&pid_9206&mi_00"), Some(("1B4F".into(), "9206".into())));
    for bad in ["", "ROOT_HUB30", "VID_2341", "VID_2341-PID_8036", "VID_234é&PID_8036", "VID_2341&PID_8é", "VID_2341&PID_80"] {
        assert!(parse_usb_key(bad).is_none(), "{bad}");
    }
    let usb = vec![
        ("VID_2341&PID_8036&MI_00".to_string(), vec!["COM7".to_string()]),
        ("VID_0403&PID_6001".into(), vec!["COM3".into()]),
        ("VID_1B4F&PID_9206".into(), vec!["COM9".into(), "COM12".into()]),
        ("VID_2341&PID_8037".into(), vec!["COM5".into()]),
    ];
    let present: Vec<String> = ["COM3", "COM7", "com9", "COM4"].iter().map(|s| s.to_string()).collect();
    assert_eq!(board_ports(&usb, &present), vec!["COM7", "COM9"]);
    assert!(board_ports(&usb, &[]).is_empty());
}

#[test]
fn wildcards() {
    assert!(serial::wildcard("ttyACM*", "ttyACM0"));
    assert!(serial::wildcard("tty?CM*", "ttyACM12"));
    assert!(!serial::wildcard("ttyACM*", "ttyUSB0"));
    assert!(serial::wildcard("*", ""));
}
