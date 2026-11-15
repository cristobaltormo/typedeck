use std::io::{self, Read, Write};

pub const GUID: &str = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11";

pub fn accept_key(key: &str) -> String {
    crate::crypto::base64(&crate::crypto::sha1(format!("{key}{GUID}").as_bytes()))
}

pub fn write_frame(w: &mut impl Write, op: u8, payload: &[u8], mask: Option<[u8; 4]>) -> io::Result<()> {
    let mut out = Vec::with_capacity(payload.len() + 14);
    out.push(0x80 | op);
    let mbit = if mask.is_some() { 0x80 } else { 0 };
    let n = payload.len();
    if n < 126 {
        out.push(mbit | n as u8);
    } else if n < 1 << 16 {
        out.push(mbit | 126);
        out.extend_from_slice(&(n as u16).to_be_bytes());
    } else {
        out.push(mbit | 127);
        out.extend_from_slice(&(n as u64).to_be_bytes());
    }
    match mask {
        Some(m) => {
            out.extend_from_slice(&m);
            out.extend(payload.iter().enumerate().map(|(i, b)| b ^ m[i % 4]));
        }
        None => out.extend_from_slice(payload),
    }
    w.write_all(&out)?;
    w.flush()
}

pub struct Frame {
    pub fin: bool,
    pub op: u8,
    pub payload: Vec<u8>,
}

pub fn read_frame(r: &mut impl Read, max: u64, require_mask: bool) -> io::Result<Frame> {
    let mut h = [0u8; 2];
    r.read_exact(&mut h)?;
    let (fin, op, masked) = (h[0] & 0x80 != 0, h[0] & 0x0F, h[1] & 0x80 != 0);
    if require_mask && (!fin || h[0] & 0x70 != 0 || !masked) {
        return Err(io::Error::new(io::ErrorKind::InvalidData, "unsupported frame"));
    }
    let mut n = u64::from(h[1] & 0x7F);
    if n == 126 {
        let mut b = [0u8; 2];
        r.read_exact(&mut b)?;
        n = u64::from(u16::from_be_bytes(b));
    } else if n == 127 {
        let mut b = [0u8; 8];
        r.read_exact(&mut b)?;
        n = u64::from_be_bytes(b);
    }
    if n > max {
        return Err(io::Error::new(io::ErrorKind::InvalidData, "frame too large"));
    }
    let mut mask = [0u8; 4];
    if masked {
        r.read_exact(&mut mask)?;
    }
    let mut payload = vec![0u8; n as usize];
    r.read_exact(&mut payload)?;
    if masked {
        for (i, b) in payload.iter_mut().enumerate() {
            *b ^= mask[i % 4];
        }
    }
    Ok(Frame { fin, op, payload })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frames_round_trip() {
        for size in [0usize, 5, 125, 126, 300, 70000] {
            let data: Vec<u8> = (0..size).map(|i| i as u8).collect();
            for mask in [None, Some([1, 2, 3, 4])] {
                let mut buf = Vec::new();
                write_frame(&mut buf, 1, &data, mask).unwrap();
                let f = read_frame(&mut buf.as_slice(), 1 << 20, false).unwrap();
                assert!(f.fin);
                assert_eq!((f.op, f.payload.len()), (1, size));
                assert_eq!(f.payload, data);
            }
        }
        let mut buf = Vec::new();
        write_frame(&mut buf, 1, b"x", None).unwrap();
        assert!(read_frame(&mut buf.as_slice(), 10, true).is_err());
        let mut big = Vec::new();
        write_frame(&mut big, 1, &[0; 100], Some([0; 4])).unwrap();
        assert!(read_frame(&mut big.as_slice(), 10, true).is_err());
        assert_eq!(accept_key("dGhlIHNhbXBsZSBub25jZQ=="), "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=");
    }
}
