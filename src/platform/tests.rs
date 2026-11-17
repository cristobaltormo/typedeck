#[cfg(unix)]
use std::time::Duration;

use super::*;

#[test]
fn quote_cmd_arg_cases() {
    for (input, want) in [
        ("hello", r#"^"hello^""#),
        ("a b", r#"^"a b^""#),
        ("a&b|c", r#"^"a^&b^|c^""#),
        (r#"say "hi""#, r#"^"say \^"hi\^"^""#),
        (r"dir\", r#"^"dir\\^""#),
        ("100%", r#"^"100%^^""#),
        ("line1\nline2", r#"^"line1 line2^""#),
        ("café ñ", "^\"café ñ^\""),
        ("(x) <y> ^z! 'q'", r#"^"^(x^) ^<y^> ^^z^! 'q'^""#),
    ] {
        assert_eq!(quote_cmd_arg(input), want, "{input}");
    }
    let got = quote_cmd_arg(r#"a&b|c<d>e^f"g%h"#);
    assert!("abcdefgh".chars().all(|c| got.contains(c)));
}

#[test]
fn layout_from_hkl() {
    let by_id = |id: u16, lang: u16| -> String {
        match (id, lang) {
            (0x0002, 0x0409) => "00020409".into(),
            (0x00AE, 0x040A) => "0001040A".into(),
            _ => String::new(),
        }
    };
    for (hkl, want) in [
        (0x040A040Au32, "es-win"),
        (0x04090409, "us"),
        (0x08090809, "qwerty"),
        (0x080A080A, "qwerty"),
        (0x040A080A, "es-win"),
        (0xF0020409, ""),
        (0xF0AE040A, ""),
        (0x00000407, ""),
        (0x04070407, ""),
        (0x0C0A0C0A, "qwerty"),
    ] {
        assert_eq!(layout_from_klid(&klid_from_hkl(hkl, Some(&by_id))), want, "{hkl:08X}");
    }
    let found = |_: u16, _: u16| "found".to_string();
    assert_eq!(klid_from_hkl(0xF0AE040A, Some(&found)), "found");
    assert_eq!(klid_from_hkl(0x040A040A, None), "0000040A");
}

#[test]
fn virtual_keys() {
    for (u, want) in [
        (0x04u8, u16::from(b'A')),
        (0x1D, u16::from(b'Z')),
        (0x1E, u16::from(b'1')),
        (0x27, u16::from(b'0')),
        (0x3A, 0x70),
        (0x45, 0x7B),
        (0x68, 0x7C),
        (0x28, 0x0D),
        (0x29, 0x1B),
        (0x2C, 0x20),
        (0x4F, 0x27),
        (0x52, 0x26),
        (0x54, 0x6F),
        (0x55, 0x6A),
        (0x56, 0x6D),
        (0x57, 0x6B),
        (0x58, 0x0D | vk::VK_EXTENDED),
        (0x59, 0x61),
        (0x61, 0x69),
        (0x62, 0x60),
        (0x63, 0x6E),
        (0x32, 0xDC),
        (0x64, 0xE2),
        (0x65, 0x5D),
        (0xE0, 0xA2),
        (0xE6, 0xA5),
        (0xE3, 0x5B),
    ] {
        assert_eq!(vk_of(u), want, "{u:02X}");
    }
    assert_eq!(vk_of(0xFF), 0);
    for v in [0x21u16, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x2C, 0x2D, 0x2E, 0x5B, 0x5C, 0x5D, 0x6F, 0x90, 0xA3, 0xA5, 0xAD, 0xB3, 0x0D | vk::VK_EXTENDED]
    {
        assert!(is_extended_vk(v), "{v:02X}");
    }
    for v in [u16::from(b'A'), u16::from(b'1'), 0x0D, 0x20, 0x70, 0x6A, 0x60, 0xA0, 0xA2] {
        assert!(!is_extended_vk(v), "{v:02X}");
    }
}

fn build_lnk(with_list: bool, ansi: &str, unicode: &str, suffix: &str) -> Vec<u8> {
    let mut data = vec![0u8; 0x4C];
    data[..4].copy_from_slice(&0x4Cu32.to_le_bytes());
    let flags = 0x2u32 | u32::from(with_list);
    data[0x14..0x18].copy_from_slice(&flags.to_le_bytes());
    if with_list {
        data.extend_from_slice(&[3, 0, 1, 2, 3]);
    }
    let header = if unicode.is_empty() { 0x1C } else { 0x24 };
    let mut info = vec![0u8; header];
    let put = |info: &mut Vec<u8>, at: usize, v: u32| info[at..at + 4].copy_from_slice(&v.to_le_bytes());
    put(&mut info, 4, header as u32);
    put(&mut info, 8, 1);
    let n = info.len() as u32;
    put(&mut info, 0x10, n);
    info.extend_from_slice(ansi.as_bytes());
    info.push(0);
    let n = info.len() as u32;
    put(&mut info, 0x18, n);
    info.extend_from_slice(suffix.as_bytes());
    info.push(0);
    if !unicode.is_empty() {
        let n = info.len() as u32;
        put(&mut info, 0x1C, n);
        for c in unicode.encode_utf16().chain(std::iter::once(0)) {
            info.extend_from_slice(&c.to_le_bytes());
        }
    }
    let n = info.len() as u32;
    put(&mut info, 0, n);
    data.extend(info);
    data
}

#[test]
fn lnk_targets() {
    assert_eq!(lnk_target(&build_lnk(true, r"C:\Program Files\App\app.exe", "", "")), r"C:\Program Files\App\app.exe");
    assert_eq!(lnk_target(&build_lnk(true, r"C:\x.exe", r"C:\Programas\Café\caf.exe", "")), r"C:\Programas\Café\caf.exe");
    assert_eq!(lnk_target(&build_lnk(false, r"C:\a\b.exe", "", "")), r"C:\a\b.exe");
    assert_eq!(lnk_target(&build_lnk(true, r"C:\a", "", "b.exe")), r"C:\ab.exe");
    assert_eq!(lnk_target(b"not a shortcut"), "");
    assert_eq!(lnk_target(&[]), "");
    let full = build_lnk(true, r"C:\a\b.exe", "", "");
    for n in 0..full.len() {
        lnk_target(&full[..n]);
    }
}

#[test]
fn task_xml_has_no_limits() {
    let x = task_xml(r"PC\ana & co", "S-1-5-21-1-2-3-1001", r"C:\Program Files\Typedeck\typedeck.exe");
    for want in [
        "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
        "<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>",
        "<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>",
        "<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>",
        "<Count>3</Count>",
        r"<UserId>PC\ana &amp; co</UserId>",
        "<RunLevel>LeastPrivilege</RunLevel>",
        r"<Command>C:\Program Files\Typedeck\typedeck.exe</Command>",
        "<Arguments>--background</Arguments>",
    ] {
        assert!(x.contains(want), "{want}");
    }
    assert_eq!(x.matches(r"<UserId>PC\ana &amp; co</UserId>").count(), 2);
    assert_eq!(utf16_with_bom("aé"), vec![0xFF, 0xFE, b'a', 0, 0xE9, 0]);
    assert!(x.contains("<SecurityDescriptor>D:(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;S-1-5-21-1-2-3-1001)</SecurityDescriptor>"));
    for bad in ["", "S-1-", "S-1-5)(A;;FA;;;WD", "x"] {
        let y = task_xml("u", bad, "e");
        assert!(!y.contains("SecurityDescriptor") && !y.contains("{SDDL}"), "{bad}");
    }
}

#[test]
fn split_hotkeys() {
    assert_eq!(split_hotkey("Cmd+Shift+4").unwrap(), (vec!["gui", "shift"], "4".to_string()));
    assert_eq!(split_hotkey("ctrl+alt+plus").unwrap(), (vec!["ctrl", "alt"], "+".to_string()));
    for bad in ["", "cmd+", "hyper+x"] {
        assert!(split_hotkey(bad).is_err(), "{bad}");
    }
}

#[cfg(unix)]
#[test]
fn run_timeout_and_output() {
    assert_eq!(run(Duration::from_secs(2), "", "echo", &["hola"]).unwrap(), "hola");
    assert_eq!(run(Duration::from_secs(2), "abc", "cat", &[]).unwrap(), "abc");
    let err = run(Duration::from_secs(2), "", "sh", &["-c", "echo fallo >&2; exit 3"]).unwrap_err();
    assert_eq!(err, "fallo");
    let t0 = std::time::Instant::now();
    let err = run(Duration::from_millis(300), "", "sleep", &["5"]).unwrap_err();
    assert!(err.contains("timed out"), "{err}");
    assert!(t0.elapsed() < Duration::from_secs(3));
    let t0 = std::time::Instant::now();
    let out = run(Duration::from_secs(3), "", "sh", &["-c", "sleep 10 & echo listo"]).unwrap();
    assert_eq!(out, "listo");
    assert!(t0.elapsed() < Duration::from_secs(4), "a grandchild that keeps the output open must not hang the run");
    assert!(run(Duration::from_secs(1), "", "no-such-program-typedeck", &[]).is_err());
}

#[test]
fn quoting() {
    assert_eq!(unix_quote("it's"), r"'it'\''s'");
    assert_eq!(applescript_quote("a \"b\" \\ c"), r#""a \"b\" \\ c""#);
}

#[test]
fn the_environment_overlay_reaches_children_without_touching_the_process() {
    set_env("TYPEDECK_OVERLAY_TEST", "yes");
    assert_eq!(env_var("TYPEDECK_OVERLAY_TEST").as_deref(), Some("yes"));
    assert!(std::env::var("TYPEDECK_OVERLAY_TEST").is_err());
    #[cfg(unix)]
    assert_eq!(run(Duration::from_secs(2), "", "sh", &["-c", "echo $TYPEDECK_OVERLAY_TEST"]).unwrap(), "yes");
}
