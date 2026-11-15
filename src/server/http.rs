use std::io::{self, BufRead, BufReader, Write};
use std::net::TcpStream;
use std::time::{Duration, Instant};

pub const MAX_BODY: usize = 2 << 20;
const MAX_HEADER: usize = 16 << 10;

pub struct Request {
    pub method: String,
    pub path: String,
    pub query: String,
    pub version: String,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
    pub closed: Box<dyn Fn() -> bool + Send + Sync>,
}

impl Request {
    pub fn new(method: &str, target: &str, host: &str) -> Request {
        let (path, query) = target.split_once('?').unwrap_or((target, ""));
        Request {
            method: method.into(),
            path: path.into(),
            query: query.into(),
            version: "HTTP/1.1".into(),
            headers: vec![("host".into(), host.into())],
            body: vec![],
            closed: Box::new(|| false),
        }
    }

    pub fn header(&self, name: &str) -> &str {
        self.headers.iter().find(|(k, _)| k.eq_ignore_ascii_case(name)).map(|(_, v)| v.as_str()).unwrap_or("")
    }

    pub fn host(&self) -> &str {
        self.header("host")
    }

    pub fn query_param(&self, name: &str) -> String {
        for pair in self.query.split('&') {
            let (k, v) = pair.split_once('=').unwrap_or((pair, ""));
            if percent_decode(k) == name {
                return percent_decode(&v.replace('+', " "));
            }
        }
        String::new()
    }

    pub fn wants_close(&self) -> bool {
        let c = self.header("connection").to_ascii_lowercase();
        c.contains("close") || (self.version == "HTTP/1.0" && !c.contains("keep-alive"))
    }
}

pub struct Response {
    pub status: u16,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}

impl Response {
    pub fn new(status: u16) -> Response {
        Response { status, headers: vec![], body: vec![] }
    }

    pub fn header(mut self, k: &str, v: &str) -> Response {
        self.set(k, v);
        self
    }

    pub fn set(&mut self, k: &str, v: &str) {
        match self.headers.iter_mut().find(|(n, _)| n.eq_ignore_ascii_case(k)) {
            Some(h) => h.1 = v.to_string(),
            None => self.headers.push((k.to_string(), v.to_string())),
        }
    }

    pub fn get(&self, k: &str) -> &str {
        self.headers.iter().find(|(n, _)| n.eq_ignore_ascii_case(k)).map(|(_, v)| v.as_str()).unwrap_or("")
    }

    pub fn text(&self) -> String {
        String::from_utf8_lossy(&self.body).into_owned()
    }
}

fn reason(status: u16) -> &'static str {
    match status {
        101 => "Switching Protocols",
        200 => "OK",
        304 => "Not Modified",
        400 => "Bad Request",
        403 => "Forbidden",
        404 => "Not Found",
        405 => "Method Not Allowed",
        411 => "Length Required",
        413 => "Payload Too Large",
        431 => "Request Header Fields Too Large",
        500 => "Internal Server Error",
        503 => "Service Unavailable",
        _ => "",
    }
}

pub fn percent_decode(s: &str) -> String {
    let b = s.as_bytes();
    let mut out = Vec::with_capacity(b.len());
    let mut i = 0;
    while i < b.len() {
        if b[i] == b'%'
            && i + 2 < b.len()
            && let (Some(h), Some(l)) = ((b[i + 1] as char).to_digit(16), (b[i + 2] as char).to_digit(16))
        {
            out.push((h * 16 + l) as u8);
            i += 3;
            continue;
        }
        out.push(b[i]);
        i += 1;
    }
    String::from_utf8_lossy(&out).into_owned()
}

pub fn clean_path(p: &str) -> String {
    let mut parts: Vec<&str> = Vec::new();
    for seg in p.split('/') {
        match seg {
            "" | "." => {}
            ".." => {
                parts.pop();
            }
            s => parts.push(s),
        }
    }
    parts.join("/")
}

fn fill(r: &mut BufReader<TcpStream>, deadline: Instant) -> io::Result<&[u8]> {
    if r.buffer().is_empty() {
        let left = deadline.saturating_duration_since(Instant::now());
        if left.is_zero() {
            return Err(io::Error::from(io::ErrorKind::TimedOut));
        }
        r.get_ref().set_read_timeout(Some(left.max(Duration::from_millis(1))))?;
    }
    r.fill_buf()
}

fn read_line_limited(r: &mut BufReader<TcpStream>, budget: &mut usize, deadline: Instant) -> io::Result<String> {
    let mut line = Vec::new();
    loop {
        let buf = fill(r, deadline)?;
        if buf.is_empty() {
            if line.is_empty() {
                return Err(io::Error::from(io::ErrorKind::UnexpectedEof));
            }
            break;
        }
        let (n, done) = match buf.iter().position(|b| *b == b'\n') {
            Some(i) => (i + 1, true),
            None => (buf.len(), false),
        };
        if n > *budget {
            return Err(io::Error::new(io::ErrorKind::InvalidData, "header too large"));
        }
        *budget -= n;
        line.extend_from_slice(&buf[..n]);
        r.consume(n);
        if done {
            break;
        }
    }
    Ok(String::from_utf8_lossy(&line).trim_end_matches(['\r', '\n']).to_string())
}

fn read_exact(r: &mut BufReader<TcpStream>, out: &mut [u8], deadline: Instant) -> io::Result<()> {
    let mut done = 0;
    while done < out.len() {
        let buf = fill(r, deadline)?;
        if buf.is_empty() {
            return Err(io::Error::from(io::ErrorKind::UnexpectedEof));
        }
        let n = buf.len().min(out.len() - done);
        out[done..done + n].copy_from_slice(&buf[..n]);
        r.consume(n);
        done += n;
    }
    Ok(())
}

#[derive(Debug, PartialEq, Eq)]
pub enum ReadError {
    Closed,
    Bad(u16),
}

fn closed_or(code: u16) -> impl Fn(io::Error) -> ReadError {
    move |e| if e.kind() == io::ErrorKind::InvalidData { ReadError::Bad(code) } else { ReadError::Closed }
}

pub fn read_request(r: &mut BufReader<TcpStream>, header_time: Duration, body_time: Duration) -> Result<Request, ReadError> {
    let deadline = Instant::now() + header_time;
    let mut budget = MAX_HEADER;
    let mut first = String::new();
    while first.is_empty() {
        first = read_line_limited(r, &mut budget, deadline).map_err(closed_or(431))?;
    }
    let mut parts = first.split_whitespace();
    let (Some(method), Some(target), Some(version), None) = (parts.next(), parts.next(), parts.next(), parts.next()) else {
        return Err(ReadError::Bad(400));
    };
    let mut headers: Vec<(String, String)> = Vec::new();
    loop {
        let line = read_line_limited(r, &mut budget, deadline).map_err(closed_or(431))?;
        if line.is_empty() {
            break;
        }
        let Some((k, v)) = line.split_once(':') else { return Err(ReadError::Bad(400)) };
        let k = k.trim().to_ascii_lowercase();
        if k.is_empty() || ((k == "host" || k == "content-length") && headers.iter().any(|(n, _)| *n == k)) {
            return Err(ReadError::Bad(400));
        }
        headers.push((k, v.trim().to_string()));
    }
    let target = target.strip_prefix("http://").and_then(|t| t.find('/').map(|i| &t[i..])).unwrap_or(target);
    let (path, query) = target.split_once('?').unwrap_or((target, ""));
    let mut req =
        Request { method: method.into(), path: path.into(), query: query.into(), version: version.into(), headers, body: vec![], closed: Box::new(|| false) };
    let chunked = req.header("transfer-encoding").to_ascii_lowercase().contains("chunked");
    let length = req.header("content-length").to_string();
    let deadline = Instant::now() + body_time;
    if chunked {
        if !length.is_empty() {
            return Err(ReadError::Bad(400));
        }
        req.body = read_chunked(r, deadline)?;
    } else if !length.is_empty() {
        let n: usize = length.parse().map_err(|_| ReadError::Bad(400))?;
        if n > MAX_BODY {
            return Err(ReadError::Bad(413));
        }
        let mut body = vec![0u8; n];
        read_exact(r, &mut body, deadline).map_err(|_| ReadError::Closed)?;
        req.body = body;
    }
    Ok(req)
}

fn read_chunked(r: &mut BufReader<TcpStream>, deadline: Instant) -> Result<Vec<u8>, ReadError> {
    let mut body = Vec::new();
    loop {
        let mut budget = MAX_HEADER;
        let line = read_line_limited(r, &mut budget, deadline).map_err(closed_or(400))?;
        let size = usize::from_str_radix(line.split(';').next().unwrap_or("").trim(), 16).map_err(|_| ReadError::Bad(400))?;
        if size == 0 {
            while !read_line_limited(r, &mut budget, deadline).map_err(closed_or(431))?.is_empty() {}
            return Ok(body);
        }
        let end = body.len().checked_add(size).filter(|e| *e <= MAX_BODY).ok_or(ReadError::Bad(413))?;
        let start = body.len();
        body.resize(end, 0);
        read_exact(r, &mut body[start..], deadline).map_err(|_| ReadError::Closed)?;
        read_line_limited(r, &mut budget, deadline).map_err(closed_or(400))?;
    }
}

pub fn write_response(w: &mut impl Write, resp: &Response, head_only: bool, keep_alive: bool) -> io::Result<()> {
    let mut out = format!("HTTP/1.1 {} {}\r\n", resp.status, reason(resp.status));
    for (k, v) in &resp.headers {
        if !k.eq_ignore_ascii_case("content-length") {
            out.push_str(&format!("{k}: {v}\r\n"));
        }
    }
    if resp.status != 304 {
        out.push_str(&format!("Content-Length: {}\r\n", resp.body.len()));
    }
    out.push_str(if keep_alive { "Connection: keep-alive\r\n\r\n" } else { "Connection: close\r\n\r\n" });
    let mut bytes = out.into_bytes();
    if !head_only && resp.status != 304 {
        bytes.extend_from_slice(&resp.body);
    }
    w.write_all(&bytes)?;
    w.flush()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::TcpListener;

    fn parse(raw: &[u8], close: bool, header_time: Duration) -> Result<Request, ReadError> {
        let ln = TcpListener::bind("127.0.0.1:0").unwrap();
        let mut c = TcpStream::connect(ln.local_addr().unwrap()).unwrap();
        let (srv, _) = ln.accept().unwrap();
        c.write_all(raw).unwrap();
        if close {
            c.shutdown(std::net::Shutdown::Write).unwrap();
        }
        let r = read_request(&mut BufReader::new(srv), header_time, Duration::from_millis(500));
        drop(c);
        r
    }

    fn ok(raw: &str) -> Request {
        parse(raw.as_bytes(), true, Duration::from_secs(2)).unwrap_or_else(|e| panic!("{raw}: {e:?}"))
    }

    fn bad(raw: &[u8]) -> ReadError {
        match parse(raw, true, Duration::from_secs(2)) {
            Ok(r) => panic!("accepted {} {}", r.method, r.path),
            Err(e) => e,
        }
    }

    #[test]
    fn requests_are_parsed_with_limits() {
        let r = ok("POST /api/x?a=1 HTTP/1.1\r\nHost: h\r\nContent-Length: 3\r\n\r\nabc");
        assert_eq!((r.method.as_str(), r.path.as_str(), r.query.as_str(), r.body.as_slice()), ("POST", "/api/x", "a=1", &b"abc"[..]));
        let r = ok("POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n3;x=1\r\nabc\r\n2\r\nde\r\n0\r\n\r\n");
        assert_eq!(r.body, b"abcde");
        assert_eq!(bad(format!("POST / HTTP/1.1\r\nContent-Length: {}\r\n\r\n", MAX_BODY + 1).as_bytes()), ReadError::Bad(413));
        assert_eq!(bad(b"POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\nffffffffffffffff\r\n"), ReadError::Bad(413));
        assert_eq!(bad(b"POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n"), ReadError::Bad(400));
        let big = format!("GET / HTTP/1.1\r\nX: {}\r\n\r\n", "a".repeat(MAX_HEADER));
        assert_eq!(bad(big.as_bytes()), ReadError::Bad(431));
        assert_eq!(bad(b"GET / HTTP/1.1\r\nno colon here\r\n\r\n"), ReadError::Bad(400));
        assert_eq!(bad(b"GET / HTTP/1.1\r\nHost: a\r\nHost: b\r\n\r\n"), ReadError::Bad(400));
        assert_eq!(bad(b"POST / HTTP/1.1\r\nContent-Length: 1\r\nContent-Length: 1\r\n\r\na"), ReadError::Bad(400));
        assert_eq!(bad(b"POST / HTTP/1.1\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n"), ReadError::Bad(400));
        assert_eq!(bad(b"POST / HTTP/1.1\r\nContent-Length: x\r\n\r\n"), ReadError::Bad(400));
        assert_eq!(bad(b"POST / HTTP/1.1\r\nContent-Length: 10\r\n\r\nabc"), ReadError::Closed);
        assert_eq!(bad(b""), ReadError::Closed);
    }

    #[test]
    fn a_slow_client_hits_the_total_header_deadline() {
        let ln = TcpListener::bind("127.0.0.1:0").unwrap();
        let mut c = TcpStream::connect(ln.local_addr().unwrap()).unwrap();
        let (srv, _) = ln.accept().unwrap();
        let writer = std::thread::spawn(move || {
            for b in b"GET / HTTP/1.1\r\nHost: x\r\n" {
                if c.write_all(&[*b]).is_err() {
                    return;
                }
                std::thread::sleep(Duration::from_millis(40));
            }
        });
        let t0 = Instant::now();
        let r = read_request(&mut BufReader::new(srv), Duration::from_millis(300), Duration::from_millis(300));
        assert_eq!(r.err(), Some(ReadError::Closed));
        assert!(t0.elapsed() < Duration::from_millis(900), "{:?}", t0.elapsed());
        let _ = writer.join();
    }

    #[test]
    fn paths_and_queries() {
        assert_eq!(clean_path("/"), "");
        assert_eq!(clean_path("/js/../../go.mod"), "go.mod");
        assert_eq!(clean_path(&percent_decode("/%2e%2e/go.mod")), "go.mod");
        assert_eq!(clean_path("/js/./app.js"), "js/app.js");
        assert_eq!(percent_decode("a%20b%zz%4"), "a b%zz%4");
        let r = Request::new("GET", "/api/events?since=12&wait=0.5&x=a+b", "h");
        assert_eq!(
            (r.query_param("since").as_str(), r.query_param("wait").as_str(), r.query_param("x").as_str(), r.query_param("none").as_str()),
            ("12", "0.5", "a b", "")
        );
    }
}
