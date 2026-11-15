use std::time::Duration;

const TIMEOUT: Duration = Duration::from_secs(8);

fn content_type(body: &str) -> &'static str {
    let t = body.trim();
    if t.starts_with('{') || t.starts_with('[') { "application/json" } else { "text/plain" }
}

fn finish(status: u16, body: &[u8]) -> Result<String, String> {
    if status >= 400 {
        return Err(format!("HTTP {status}"));
    }
    let b = &body[..body.len().min(300)];
    Ok(format!("{status} {}", String::from_utf8_lossy(b).trim()))
}

pub fn request(method: &str, url: &str, body: &str, subst: impl Fn(&str) -> String) -> Result<String, String> {
    if !url.starts_with("http://") && !url.starts_with("https://") {
        return Err("only http or https".into());
    }
    let method = if method.is_empty() { "GET" } else { method };
    let body = if body.is_empty() { None } else { Some(subst(body)) };
    send(method, url, body.as_deref())
}

#[cfg(target_os = "linux")]
fn agent() -> &'static ureq::Agent {
    use std::sync::{Arc, OnceLock};
    use ureq::tls::{RootCerts, TlsConfig};
    static A: OnceLock<ureq::Agent> = OnceLock::new();
    A.get_or_init(|| {
        let mut certs = Vec::new();
        for f in [
            "/etc/ssl/certs/ca-certificates.crt",
            "/etc/pki/tls/certs/ca-bundle.crt",
            "/etc/ssl/ca-bundle.pem",
            "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
            "/etc/ssl/cert.pem",
        ] {
            if let Ok(data) = std::fs::read(std::env::var("SSL_CERT_FILE").unwrap_or_else(|_| f.to_string())) {
                for item in ureq::tls::parse_pem(&data).flatten() {
                    if let ureq::tls::PemItem::Certificate(c) = item {
                        certs.push(c);
                    }
                }
                break;
            }
        }
        let roots = if certs.is_empty() { RootCerts::WebPki } else { RootCerts::new_with_certs(&certs) };
        let provider = Arc::new(rustls::crypto::ring::default_provider());
        ureq::Agent::config_builder()
            .http_status_as_error(false)
            .timeout_global(Some(TIMEOUT))
            .user_agent(concat!("Typedeck/", env!("CARGO_PKG_VERSION")))
            .tls_config(TlsConfig::builder().root_certs(roots).unversioned_rustls_crypto_provider(provider).build())
            .max_idle_connections(0)
            .build()
            .new_agent()
    })
}

#[cfg(target_os = "linux")]
fn send(method: &str, url: &str, body: Option<&str>) -> Result<String, String> {
    use std::io::Read;
    let req = ureq::http::Request::builder().method(method).uri(url);
    let req = match body {
        Some(b) => req.header("Content-Type", content_type(b)).body(b.as_bytes().to_vec()),
        None => req.body(Vec::new()),
    }
    .map_err(|e| e.to_string())?;
    let resp = agent().run(req).map_err(|e| e.to_string())?;
    let status = resp.status().as_u16();
    let mut buf = Vec::new();
    let _ = resp.into_body().into_reader().take(300).read_to_end(&mut buf);
    finish(status, &buf)
}

// curl is part of every supported macOS and Windows release and uses the system certificate store.
#[cfg(not(target_os = "linux"))]
fn curl_args(method: &str, url: &str, body: Option<&str>, out: &std::path::Path) -> Vec<std::ffi::OsString> {
    let mut a: Vec<std::ffi::OsString> = Vec::new();
    for s in ["-q", "-sS", "-L", "--max-redirs", "10", "--max-time", "8", "--proto", "=http,https", "--proto-redir", "=http,https"] {
        a.push(s.into());
    }
    if method != "GET" && method != "POST" {
        a.extend(["-X".into(), method.into()]);
    }
    if let Some(b) = body {
        a.extend(["-H".into(), format!("Content-Type: {}", content_type(b)).into()]);
    }
    if body.is_some() || method == "POST" {
        a.extend(["--data-binary".into(), "@-".into()]);
    }
    if method == "GET" && body.is_some() {
        a.extend(["-X".into(), "GET".into()]);
    }
    a.extend(["-o".into(), out.as_os_str().to_owned(), "-w".into(), "%{http_code}".into(), "--".into(), url.into()]);
    a
}

#[cfg(not(target_os = "linux"))]
fn send(method: &str, url: &str, body: Option<&str>) -> Result<String, String> {
    use std::io::Read;
    use std::sync::atomic::{AtomicU32, Ordering};
    static SEQ: AtomicU32 = AtomicU32::new(0);
    let name = format!("typedeck-http-{}-{}-{}", std::process::id(), SEQ.fetch_add(1, Ordering::Relaxed), crate::clock::now_millis());
    let path = std::env::temp_dir().join(name);
    std::fs::OpenOptions::new().write(true).create_new(true).open(&path).map_err(|e| e.to_string())?;
    struct Remove<'a>(&'a std::path::Path);
    impl Drop for Remove<'_> {
        fn drop(&mut self) {
            let _ = std::fs::remove_file(self.0);
        }
    }
    let _cleanup = Remove(&path);
    let mut c = crate::platform::command(if cfg!(windows) { "curl.exe" } else { "curl" });
    c.args(curl_args(method, url, body, &path));
    let out = crate::platform::run_command(c, TIMEOUT + Duration::from_secs(2), body.unwrap_or(""))?;
    let status: u16 = out.trim().parse().map_err(|_| out.clone())?;
    let mut buf = Vec::new();
    if let Ok(f) = std::fs::File::open(&path) {
        let _ = f.take(300).read_to_end(&mut buf);
    }
    finish(status, &buf)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{BufRead, BufReader, Write};
    use std::net::TcpListener;

    fn server(status: &str, body: &'static str) -> String {
        let ln = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = ln.local_addr().unwrap();
        let status = status.to_string();
        std::thread::spawn(move || {
            if let Some(Ok(mut c)) = ln.incoming().next() {
                let mut r = BufReader::new(c.try_clone().unwrap());
                let mut len = 0usize;
                loop {
                    let mut l = String::new();
                    r.read_line(&mut l).unwrap();
                    if l.trim().is_empty() {
                        break;
                    }
                    if let Some(v) = l.to_lowercase().strip_prefix("content-length:") {
                        len = v.trim().parse().unwrap_or(0);
                    }
                }
                let mut b = vec![0u8; len];
                std::io::Read::read_exact(&mut r, &mut b).unwrap();
                let _ = write!(c, "HTTP/1.1 {status}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}", body.len());
            }
        });
        format!("http://{addr}/hook")
    }

    #[test]
    fn requests() {
        if cfg!(not(target_os = "linux")) && !crate::platform::have("curl") {
            return;
        }
        assert_eq!(request("POST", &server("200 OK", " ok "), "{\"a\":1}", |s| s.to_string()).unwrap(), "200 ok");
        assert_eq!(request("", &server("404 Not Found", "x"), "", |s| s.to_string()).unwrap_err(), "HTTP 404");
        assert!(request("GET", "ftp://x", "", |s| s.to_string()).is_err());
        assert_eq!(content_type(" [1]"), "application/json");
        assert_eq!(content_type("hi"), "text/plain");
    }

    #[cfg(not(target_os = "linux"))]
    #[test]
    fn curl_arguments() {
        let p = std::path::Path::new("out");
        let args = |m: &str, b: Option<&str>| curl_args(m, "http://x/", b, p).into_iter().map(|s| s.into_string().unwrap()).collect::<Vec<_>>().join(" ");
        let common = "-q -sS -L --max-redirs 10 --max-time 8 --proto =http,https --proto-redir =http,https";
        let tail = "-o out -w %{http_code} -- http://x/";
        assert_eq!(args("GET", None), format!("{common} {tail}"));
        assert_eq!(args("POST", None), format!("{common} --data-binary @- {tail}"));
        assert_eq!(args("POST", Some("{}")), format!("{common} -H Content-Type: application/json --data-binary @- {tail}"));
        assert_eq!(args("PUT", Some("a")), format!("{common} -X PUT -H Content-Type: text/plain --data-binary @- {tail}"));
        assert_eq!(args("DELETE", None), format!("{common} -X DELETE {tail}"));
        assert_eq!(args("GET", Some("a")), format!("{common} -H Content-Type: text/plain --data-binary @- -X GET {tail}"));
    }
}
