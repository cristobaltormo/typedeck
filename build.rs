use std::env;
use std::fmt::Write as _;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

fn main() {
    let root = PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("manifest dir"));
    let out = PathBuf::from(env::var("OUT_DIR").expect("out dir"));
    let version = version(&root);
    println!("cargo:rustc-env=TYPEDECK_BUILD_VERSION={version}");
    embed_web(&root, &out);
    if env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("windows") {
        windows_resources(&root, &out, &version);
    }
}

fn git(root: &Path, args: &[&str]) -> Option<String> {
    let out = Command::new("git").args(args).current_dir(root).output().ok()?;
    let text = String::from_utf8(out.stdout).ok()?;
    let text = text.trim();
    (out.status.success() && !text.is_empty()).then(|| text.to_string())
}

fn own_repo(root: &Path) -> bool {
    let Some(top) = git(root, &["rev-parse", "--show-toplevel"]) else {
        return false;
    };
    let here = fs::canonicalize(root).ok();
    here.is_some() && fs::canonicalize(&top).ok() == here
}

fn version(root: &Path) -> String {
    println!("cargo:rerun-if-env-changed=TYPEDECK_VERSION");
    if let Ok(v) = env::var("TYPEDECK_VERSION")
        && !v.trim().is_empty()
    {
        return v.trim().to_string();
    }
    if own_repo(root) {
        if let Some(dir) = git(root, &["rev-parse", "--git-dir"]) {
            let dir = root.join(dir);
            for name in ["HEAD", "packed-refs", "refs/tags"] {
                let p = dir.join(name);
                if p.exists() {
                    println!("cargo:rerun-if-changed={}", p.display());
                }
            }
        }
        if let Some(v) = git(root, &["describe", "--tags", "--always"]) {
            return v;
        }
    }
    format!("v{}", env::var("CARGO_PKG_VERSION").unwrap_or_default())
}

fn walk(dir: &Path, base: &Path, out: &mut Vec<(String, PathBuf)>) {
    let Ok(entries) = fs::read_dir(dir) else {
        return;
    };
    let mut entries: Vec<_> = entries.flatten().map(|e| e.path()).collect();
    entries.sort();
    for p in entries {
        if p.is_dir() {
            walk(&p, base, out);
        } else if let Ok(rel) = p.strip_prefix(base) {
            let rel = rel.to_string_lossy().replace('\\', "/");
            out.push((rel, p));
        }
    }
}

fn content_type(path: &str) -> &'static str {
    let ext = path.rsplit('.').next().unwrap_or("");
    match ext {
        "html" => "text/html; charset=utf-8",
        "js" | "mjs" => "text/javascript; charset=utf-8",
        "css" => "text/css; charset=utf-8",
        "json" => "application/json",
        "svg" => "image/svg+xml",
        "png" => "image/png",
        "ico" => "image/vnd.microsoft.icon",
        "woff2" => "font/woff2",
        "woff" => "font/woff",
        "txt" => "text/plain; charset=utf-8",
        "webp" => "image/webp",
        _ => "application/octet-stream",
    }
}

fn fnv64(data: &[u8]) -> u64 {
    let mut h: u64 = 0xcbf29ce484222325;
    for b in data {
        h ^= u64::from(*b);
        h = h.wrapping_mul(0x100000001b3);
    }
    h
}

fn embed_web(root: &Path, out: &Path) {
    let ui = root.join("web").join("ui");
    println!("cargo:rerun-if-changed={}", ui.display());
    let mut files = Vec::new();
    walk(&ui, &ui, &mut files);
    let mut code = String::from("pub static ASSETS: &[Asset] = &[\n");
    for (rel, path) in &files {
        println!("cargo:rerun-if-changed={}", path.display());
        let data = fs::read(path).expect("read web asset");
        let _ = writeln!(
            code,
            "    Asset {{ path: {rel:?}, ctype: {:?}, etag: \"\\\"{:016x}\\\"\", data: include_bytes!({:?}) }},",
            content_type(rel),
            fnv64(&data),
            path.display().to_string()
        );
    }
    code.push_str("];\n");
    fs::write(out.join("assets.rs"), code).expect("write assets.rs");
}

struct Resource {
    kind: u16,
    id: u16,
    lang: u16,
    data: Vec<u8>,
}

fn le16(b: &[u8], at: usize) -> u16 {
    u16::from_le_bytes([b[at], b[at + 1]])
}

fn le32(b: &[u8], at: usize) -> u32 {
    u32::from_le_bytes([b[at], b[at + 1], b[at + 2], b[at + 3]])
}

fn split_icon(ico: &[u8]) -> (Vec<Resource>, Vec<u8>) {
    assert!(ico.len() >= 6 && le16(ico, 2) == 1, "not an .ico file");
    let n = usize::from(le16(ico, 4));
    let mut group = Vec::new();
    for v in [0u16, 1, n as u16] {
        group.extend_from_slice(&v.to_le_bytes());
    }
    let mut icons = Vec::new();
    for i in 0..n {
        let e = 6 + 16 * i;
        assert!(ico.len() >= e + 16, "truncated .ico file");
        let size = le32(ico, e + 8) as usize;
        let off = le32(ico, e + 12) as usize;
        assert!(off + size <= ico.len(), "truncated .ico file");
        let id = (i + 1) as u16;
        icons.push(Resource { kind: 3, id, lang: 0, data: ico[off..off + size].to_vec() });
        group.extend_from_slice(&ico[e..e + 4]);
        group.extend_from_slice(&1u16.to_le_bytes());
        group.extend_from_slice(&ico[e + 6..e + 8]);
        group.extend_from_slice(&(size as u32).to_le_bytes());
        group.extend_from_slice(&id.to_le_bytes());
    }
    (icons, group)
}

fn numeric_version(v: &str) -> [u16; 4] {
    let v = v.strip_prefix('v').unwrap_or(v);
    let mut out = [0u16; 4];
    for (i, part) in v.split(['.', '-', '+']).take(3).enumerate() {
        let digits: String = part.chars().take_while(char::is_ascii_digit).collect();
        out[i] = digits.parse().unwrap_or(0);
    }
    out
}

fn pad4(b: &mut Vec<u8>) {
    while !b.len().is_multiple_of(4) {
        b.push(0);
    }
}

fn utf16z(s: &str) -> Vec<u8> {
    s.encode_utf16().chain(std::iter::once(0)).flat_map(u16::to_le_bytes).collect()
}

fn block(key: &str, value_len: u16, kind: u16, value: &[u8], children: &[Vec<u8>]) -> Vec<u8> {
    let mut b = vec![0, 0];
    b.extend_from_slice(&value_len.to_le_bytes());
    b.extend_from_slice(&kind.to_le_bytes());
    b.extend_from_slice(&utf16z(key));
    pad4(&mut b);
    b.extend_from_slice(value);
    for c in children {
        pad4(&mut b);
        b.extend_from_slice(c);
    }
    let len = b.len() as u16;
    b[..2].copy_from_slice(&len.to_le_bytes());
    b
}

fn version_info(version: &str) -> Vec<u8> {
    let v = numeric_version(version);
    let numeric = format!("{}.{}.{}.{}", v[0], v[1], v[2], v[3]);
    let text = if version.is_empty() { numeric } else { version.to_string() };
    let fields = [
        ("CompanyName", "Cristobal Tormo"),
        ("FileDescription", "Typedeck, a macro keyboard companion"),
        ("FileVersion", text.as_str()),
        ("InternalName", "typedeck"),
        ("LegalCopyright", "Copyright 2026 Cristobal Tormo. MIT License."),
        ("OriginalFilename", "typedeck.exe"),
        ("ProductName", "Typedeck"),
        ("ProductVersion", text.as_str()),
    ];
    let strings: Vec<Vec<u8>> = fields
        .iter()
        .map(|(k, val)| {
            let val = utf16z(val);
            let mut s = block(k, (val.len() / 2) as u16, 1, &val, &[]);
            pad4(&mut s);
            s
        })
        .collect();
    let table = block("040904B0", 0, 1, &[], &strings);
    let string_info = block("StringFileInfo", 0, 1, &[], &[table]);
    let translation = 0x04B0_0409u32.to_le_bytes();
    let var_info = block("VarFileInfo", 0, 1, &[], &[block("Translation", 4, 0, &translation, &[])]);
    let ms = u32::from(v[0]) << 16 | u32::from(v[1]);
    let ls = u32::from(v[2]) << 16 | u32::from(v[3]);
    let fixed: Vec<u8> = [0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x40004, 1, 0, 0, 0].iter().flat_map(|x: &u32| x.to_le_bytes()).collect();
    block("VS_VERSION_INFO", 52, 0, &fixed, &[string_info, var_info])
}

fn resources(root: &Path, version: &str) -> Vec<Resource> {
    let dir = root.join("packaging").join("windows");
    let manifest = dir.join("typedeck.manifest");
    let icon = dir.join("typedeck.ico");
    println!("cargo:rerun-if-changed={}", manifest.display());
    println!("cargo:rerun-if-changed={}", icon.display());
    let manifest = fs::read(manifest).expect("read the Windows manifest");
    let ico = fs::read(icon).expect("read the Windows icon");
    let (icons, group) = split_icon(&ico);
    let mut res = vec![
        Resource { kind: 24, id: 1, lang: 0, data: manifest },
        Resource { kind: 14, id: 1, lang: 0, data: group },
        Resource { kind: 16, id: 1, lang: 0x0409, data: version_info(version) },
    ];
    res.extend(icons);
    res.sort_by_key(|r| (r.kind, r.id, r.lang));
    res
}

fn res_file(res: &[Resource]) -> Vec<u8> {
    let mut out = Vec::new();
    let entry = |out: &mut Vec<u8>, kind: u16, id: u16, lang: u16, data: &[u8]| {
        let mut h = Vec::new();
        h.extend_from_slice(&(data.len() as u32).to_le_bytes());
        h.extend_from_slice(&32u32.to_le_bytes());
        for v in [0xFFFFu16, kind, 0xFFFF, id] {
            h.extend_from_slice(&v.to_le_bytes());
        }
        h.extend_from_slice(&0u32.to_le_bytes());
        h.extend_from_slice(&if kind == 0 { 0u16 } else { 0x1030u16 }.to_le_bytes());
        h.extend_from_slice(&lang.to_le_bytes());
        h.extend_from_slice(&0u32.to_le_bytes());
        h.extend_from_slice(&0u32.to_le_bytes());
        out.extend_from_slice(&h);
        out.extend_from_slice(data);
        pad4(out);
    };
    entry(&mut out, 0, 0, 0, &[]);
    for r in res {
        entry(&mut out, r.kind, r.id, r.lang, &r.data);
    }
    out
}

fn coff_object(res: &[Resource], machine: u16, reloc_type: u16) -> Vec<u8> {
    let mut types: Vec<u16> = res.iter().map(|r| r.kind).collect();
    types.dedup();
    let names_of = |t: u16| -> Vec<u16> {
        let mut ids: Vec<u16> = res.iter().filter(|r| r.kind == t).map(|r| r.id).collect();
        ids.dedup();
        ids
    };
    let dir = |n: usize| 16 + 8 * n;
    let mut offset = dir(types.len());
    let mut type_dirs = Vec::new();
    for &t in &types {
        type_dirs.push(offset);
        offset += dir(names_of(t).len());
    }
    let mut name_dirs = Vec::new();
    for &t in &types {
        for id in names_of(t) {
            name_dirs.push((t, id, offset));
            let langs = res.iter().filter(|r| r.kind == t && r.id == id).count();
            offset += dir(langs);
        }
    }
    let entries_at = offset;
    let data_at = entries_at + 16 * res.len();
    let mut buf = Vec::new();
    let header = |buf: &mut Vec<u8>, n: usize| {
        for v in [0u32, 0, 0, (n as u32) << 16] {
            buf.extend_from_slice(&v.to_le_bytes());
        }
    };
    header(&mut buf, types.len());
    for (i, &t) in types.iter().enumerate() {
        buf.extend_from_slice(&u32::from(t).to_le_bytes());
        buf.extend_from_slice(&(type_dirs[i] as u32 | 0x8000_0000).to_le_bytes());
    }
    for &t in &types {
        let ids = names_of(t);
        header(&mut buf, ids.len());
        for id in ids {
            let at = name_dirs.iter().find(|(nt, nid, _)| *nt == t && *nid == id).map(|x| x.2).unwrap_or(0);
            buf.extend_from_slice(&u32::from(id).to_le_bytes());
            buf.extend_from_slice(&(at as u32 | 0x8000_0000).to_le_bytes());
        }
    }
    let mut leaf = 0usize;
    for &(t, id, _) in &name_dirs {
        let langs: Vec<&Resource> = res.iter().filter(|r| r.kind == t && r.id == id).collect();
        header(&mut buf, langs.len());
        for r in langs {
            buf.extend_from_slice(&u32::from(r.lang).to_le_bytes());
            buf.extend_from_slice(&((entries_at + 16 * leaf) as u32).to_le_bytes());
            leaf += 1;
        }
    }
    let mut relocs = Vec::new();
    let mut pos = data_at;
    for &(t, id, _) in &name_dirs {
        for r in res.iter().filter(|r| r.kind == t && r.id == id) {
            relocs.push(buf.len() as u32);
            for v in [pos as u32, r.data.len() as u32, 0, 0] {
                buf.extend_from_slice(&v.to_le_bytes());
            }
            pos += (r.data.len() + 3) & !3;
        }
    }
    for &(t, id, _) in &name_dirs {
        for r in res.iter().filter(|r| r.kind == t && r.id == id) {
            buf.extend_from_slice(&r.data);
            pad4(&mut buf);
        }
    }
    let raw_ptr = 20 + 40;
    let reloc_ptr = raw_ptr + buf.len();
    let sym_ptr = reloc_ptr + 10 * relocs.len();
    let mut o = Vec::new();
    o.extend_from_slice(&machine.to_le_bytes());
    o.extend_from_slice(&1u16.to_le_bytes());
    o.extend_from_slice(&0u32.to_le_bytes());
    o.extend_from_slice(&(sym_ptr as u32).to_le_bytes());
    o.extend_from_slice(&2u32.to_le_bytes());
    o.extend_from_slice(&0u16.to_le_bytes());
    o.extend_from_slice(&0u16.to_le_bytes());
    let name = *b".rsrc\0\0\0";
    o.extend_from_slice(&name);
    for v in [0u32, 0, buf.len() as u32, raw_ptr as u32, reloc_ptr as u32, 0] {
        o.extend_from_slice(&v.to_le_bytes());
    }
    o.extend_from_slice(&(relocs.len() as u16).to_le_bytes());
    o.extend_from_slice(&0u16.to_le_bytes());
    o.extend_from_slice(&0x4000_0040u32.to_le_bytes());
    o.extend_from_slice(&buf);
    for r in &relocs {
        o.extend_from_slice(&r.to_le_bytes());
        o.extend_from_slice(&0u32.to_le_bytes());
        o.extend_from_slice(&reloc_type.to_le_bytes());
    }
    o.extend_from_slice(&name);
    o.extend_from_slice(&0u32.to_le_bytes());
    o.extend_from_slice(&1i16.to_le_bytes());
    o.extend_from_slice(&0u16.to_le_bytes());
    o.push(3);
    o.push(1);
    o.extend_from_slice(&(buf.len() as u32).to_le_bytes());
    o.extend_from_slice(&(relocs.len() as u16).to_le_bytes());
    o.extend_from_slice(&0u16.to_le_bytes());
    o.extend_from_slice(&0u32.to_le_bytes());
    o.extend_from_slice(&1u16.to_le_bytes());
    o.extend_from_slice(&[0u8; 4]);
    o.extend_from_slice(&4u32.to_le_bytes());
    o
}

fn windows_resources(root: &Path, out: &Path, version: &str) {
    let res = resources(root, version);
    let arch = env::var("CARGO_CFG_TARGET_ARCH").unwrap_or_default();
    let msvc = env::var("CARGO_CFG_TARGET_ENV").as_deref() == Ok("msvc");
    let path = if msvc {
        let p = out.join("typedeck.res");
        fs::write(&p, res_file(&res)).expect("write typedeck.res");
        p
    } else {
        let (machine, reloc) = match arch.as_str() {
            "aarch64" => (0xAA64, 2),
            "x86" => (0x014C, 7),
            _ => (0x8664, 3),
        };
        let p = out.join("typedeck-resources.o");
        fs::write(&p, coff_object(&res, machine, reloc)).expect("write the resource object");
        p
    };
    println!("cargo:rustc-link-arg-bins={}", path.display());
}
