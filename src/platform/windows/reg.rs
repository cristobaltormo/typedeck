use windows_sys::Win32::Foundation::ERROR_SUCCESS;
use windows_sys::Win32::System::Registry::{
    HKEY, KEY_READ, KEY_WRITE, REG_DWORD, REG_OPTION_NON_VOLATILE, REG_SZ, RegCloseKey, RegCreateKeyExW, RegDeleteValueW, RegEnumKeyExW, RegEnumValueW,
    RegOpenKeyExW, RegQueryValueExW, RegSetValueExW,
};

pub fn wide(s: &str) -> Vec<u16> {
    s.encode_utf16().chain(std::iter::once(0)).collect()
}

fn from_wide(b: &[u16]) -> String {
    let end = b.iter().position(|c| *c == 0).unwrap_or(b.len());
    String::from_utf16_lossy(&b[..end])
}

struct Key(HKEY);

impl Drop for Key {
    fn drop(&mut self) {
        unsafe {
            RegCloseKey(self.0);
        }
    }
}

fn open(root: HKEY, path: &str, access: u32) -> Option<Key> {
    let mut k: HKEY = std::ptr::null_mut();
    let p = wide(path);
    (unsafe { RegOpenKeyExW(root, p.as_ptr(), 0, access, &mut k) } == ERROR_SUCCESS).then_some(Key(k))
}

fn query(root: HKEY, path: &str, name: &str) -> Option<(Vec<u8>, u32)> {
    let k = open(root, path, KEY_READ)?;
    let n = wide(name);
    let (mut typ, mut size) = (0u32, 0u32);
    if unsafe { RegQueryValueExW(k.0, n.as_ptr(), std::ptr::null(), &mut typ, std::ptr::null_mut(), &mut size) } != ERROR_SUCCESS || size == 0 {
        return None;
    }
    let mut buf = vec![0u8; size as usize];
    if unsafe { RegQueryValueExW(k.0, n.as_ptr(), std::ptr::null(), &mut typ, buf.as_mut_ptr(), &mut size) } != ERROR_SUCCESS {
        return None;
    }
    buf.truncate(size as usize);
    Some((buf, typ))
}

fn u16s(b: &[u8]) -> Vec<u16> {
    b.as_chunks::<2>().0.iter().map(|c| u16::from_le_bytes(*c)).collect()
}

pub fn string(root: HKEY, path: &str, name: &str) -> Option<String> {
    let (b, t) = query(root, path, name)?;
    (t == REG_SZ && b.len() >= 2).then(|| from_wide(&u16s(&b)))
}

pub fn dword(root: HKEY, path: &str, name: &str) -> Option<u32> {
    let (b, t) = query(root, path, name)?;
    (t == REG_DWORD && b.len() >= 4).then(|| u32::from_le_bytes([b[0], b[1], b[2], b[3]]))
}

pub fn sub_keys(root: HKEY, path: &str) -> Vec<String> {
    let Some(k) = open(root, path, KEY_READ) else { return vec![] };
    let mut out = Vec::new();
    for i in 0.. {
        let mut buf = [0u16; 256];
        let mut len = buf.len() as u32;
        let r =
            unsafe { RegEnumKeyExW(k.0, i, buf.as_mut_ptr(), &mut len, std::ptr::null(), std::ptr::null_mut(), std::ptr::null_mut(), std::ptr::null_mut()) };
        if r != ERROR_SUCCESS {
            break;
        }
        out.push(from_wide(&buf[..len as usize]));
    }
    out
}

pub fn string_values(root: HKEY, path: &str) -> Vec<(String, String)> {
    let Some(k) = open(root, path, KEY_READ) else { return vec![] };
    let mut out = Vec::new();
    for i in 0.. {
        let mut name = [0u16; 512];
        let mut nlen = name.len() as u32;
        let mut data = [0u8; 2048];
        let mut dlen = data.len() as u32;
        let mut typ = 0u32;
        let r = unsafe { RegEnumValueW(k.0, i, name.as_mut_ptr(), &mut nlen, std::ptr::null(), &mut typ, data.as_mut_ptr(), &mut dlen) };
        if r != ERROR_SUCCESS {
            break;
        }
        if typ == REG_SZ {
            out.push((from_wide(&name[..nlen as usize]), from_wide(&u16s(&data[..dlen as usize]))));
        }
    }
    out
}

fn create(root: HKEY, path: &str) -> Result<Key, String> {
    let mut k: HKEY = std::ptr::null_mut();
    let p = wide(path);
    let r =
        unsafe { RegCreateKeyExW(root, p.as_ptr(), 0, std::ptr::null(), REG_OPTION_NON_VOLATILE, KEY_WRITE, std::ptr::null(), &mut k, std::ptr::null_mut()) };
    if r != ERROR_SUCCESS {
        return Err(std::io::Error::from_raw_os_error(r as i32).to_string());
    }
    Ok(Key(k))
}

fn set(root: HKEY, path: &str, name: &str, typ: u32, data: &[u8]) -> Result<(), String> {
    let k = create(root, path)?;
    let n = wide(name);
    let r = unsafe { RegSetValueExW(k.0, n.as_ptr(), 0, typ, data.as_ptr(), data.len() as u32) };
    if r != ERROR_SUCCESS {
        return Err(std::io::Error::from_raw_os_error(r as i32).to_string());
    }
    Ok(())
}

pub fn set_string(root: HKEY, path: &str, name: &str, value: &str) -> Result<(), String> {
    let data: Vec<u8> = wide(value).into_iter().flat_map(u16::to_le_bytes).collect();
    set(root, path, name, REG_SZ, &data)
}

pub fn set_dword(root: HKEY, path: &str, name: &str, value: u32) -> Result<(), String> {
    set(root, path, name, REG_DWORD, &value.to_le_bytes())
}

pub fn delete_value(root: HKEY, path: &str, name: &str) -> Result<(), String> {
    let Some(k) = open(root, path, KEY_WRITE) else { return Ok(()) };
    let n = wide(name);
    let r = unsafe { RegDeleteValueW(k.0, n.as_ptr()) };
    if r != ERROR_SUCCESS && r != 2 {
        return Err(std::io::Error::from_raw_os_error(r as i32).to_string());
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use windows_sys::Win32::System::Registry::HKEY_CURRENT_USER;

    #[test]
    fn round_trip() {
        let path = r"Software\TypedeckTest";
        set_string(HKEY_CURRENT_USER, path, "s", "héllo").unwrap();
        set_dword(HKEY_CURRENT_USER, path, "d", 7).unwrap();
        assert_eq!(string(HKEY_CURRENT_USER, path, "s").as_deref(), Some("héllo"));
        assert_eq!(dword(HKEY_CURRENT_USER, path, "d"), Some(7));
        assert!(string_values(HKEY_CURRENT_USER, path).iter().any(|(n, v)| n == "s" && v == "héllo"));
        delete_value(HKEY_CURRENT_USER, path, "s").unwrap();
        delete_value(HKEY_CURRENT_USER, path, "d").unwrap();
        delete_value(HKEY_CURRENT_USER, path, "missing").unwrap();
        assert!(string(HKEY_CURRENT_USER, path, "s").is_none());
        assert!(sub_keys(HKEY_CURRENT_USER, r"Software\NoSuchTypedeckKey").is_empty());
    }
}
