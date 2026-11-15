use serde::{Deserialize, Serialize};

use super::Paths;
use super::store::write_private;

pub const CAPTURE_CONSENT_VERSION: i64 = 1;

#[derive(Serialize, Deserialize)]
struct CaptureConsent {
    version: i64,
    at: String,
}

pub fn has_capture_consent(p: &Paths) -> bool {
    let Ok(b) = std::fs::read(p.capture_consent()) else {
        return false;
    };
    consent_ok(&b)
}

fn consent_ok(b: &[u8]) -> bool {
    use serde_json::Value;
    let Ok(Value::Object(m)) = serde_json::from_slice::<Value>(b) else { return false };
    let field = |name: &str| m.get(name).or_else(|| m.iter().find(|(k, _)| k.eq_ignore_ascii_case(name)).map(|(_, v)| v)).filter(|v| !v.is_null());
    if field("at").is_some_and(|v| !v.is_string()) {
        return false;
    }
    field("version").and_then(Value::as_i64) == Some(CAPTURE_CONSENT_VERSION)
}

pub fn record_capture_consent(p: &Paths) -> Result<(), String> {
    let at = crate::clock::utc(crate::clock::unix_secs(std::time::SystemTime::now())).rfc3339_utc();
    let b = serde_json::to_vec(&CaptureConsent { version: CAPTURE_CONSENT_VERSION, at }).map_err(|e| e.to_string())?;
    std::fs::create_dir_all(&p.dir).map_err(|e| e.to_string())?;
    write_private(&p.capture_consent(), &b).map_err(|e| e.to_string())
}

#[cfg(test)]
mod tests {
    #[test]
    fn consent_file_is_read_like_before() {
        for (data, want) in [
            (r#"{"version":1,"at":"2026-01-01T00:00:00Z"}"#, true),
            (r#"{"Version":1}"#, true),
            (r#"{"VERSION":1,"at":null}"#, true),
            (r#"{"version":2}"#, false),
            (r#"{"version":1.5}"#, false),
            (r#"{"version":"1"}"#, false),
            (r#"{"version":1,"at":5}"#, false),
            (r#"{"version":1} x"#, false),
            ("[]", false),
        ] {
            assert_eq!(super::consent_ok(data.as_bytes()), want, "{data}");
        }
    }
}
