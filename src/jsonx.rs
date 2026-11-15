use serde::Serializer;
use serde::de::DeserializeOwned;
use serde_json::{Map, Value};

pub fn strip_nulls(v: Value) -> Value {
    match v {
        Value::Object(m) => Value::Object(m.into_iter().filter(|(_, v)| !v.is_null()).map(|(k, v)| (k, strip_nulls(v))).collect()),
        Value::Array(a) => Value::Array(a.into_iter().map(strip_nulls).collect()),
        other => other,
    }
}

pub fn first_value(data: &[u8]) -> Result<Value, String> {
    match serde_json::Deserializer::from_slice(data).into_iter::<Value>().next() {
        Some(Ok(v)) => Ok(v),
        Some(Err(e)) => Err(e.to_string()),
        None => Err("EOF".into()),
    }
}

pub fn decode<T: DeserializeOwned>(data: &[u8]) -> Result<T, String> {
    let v = strip_nulls(first_value(data)?);
    serde_json::from_value(v).map_err(|e| e.to_string())
}

pub fn go_f64<S: Serializer>(v: &f64, s: S) -> Result<S::Ok, S::Error> {
    if v.is_finite() && v.fract() == 0.0 && v.abs() < 1e15 { s.serialize_i64(*v as i64) } else { s.serialize_f64(*v) }
}

pub fn is_zero_f64(v: &f64) -> bool {
    *v == 0.0
}

pub fn is_zero_i64(v: &i64) -> bool {
    *v == 0
}

pub fn is_zero_i32(v: &i32) -> bool {
    *v == 0
}

pub fn is_false(v: &bool) -> bool {
    !*v
}

pub struct Body {
    map: Map<String, Value>,
}

impl Body {
    pub fn parse(data: &[u8]) -> Result<Body, String> {
        match first_value(data)? {
            Value::Object(map) => Ok(Body { map }),
            Value::Null => Ok(Body { map: Map::new() }),
            _ => Err("json: not an object".into()),
        }
    }

    fn find(&self, name: &str) -> Option<&Value> {
        self.map.get(name).or_else(|| self.map.iter().find(|(k, _)| k.eq_ignore_ascii_case(name)).map(|(_, v)| v)).filter(|v| !v.is_null())
    }

    pub fn str(&self, name: &str) -> Result<String, String> {
        match self.find(name) {
            None => Ok(String::new()),
            Some(Value::String(s)) => Ok(s.clone()),
            Some(_) => Err(format!("json: {name} must be a string")),
        }
    }

    pub fn bool(&self, name: &str) -> Result<bool, String> {
        match self.find(name) {
            None => Ok(false),
            Some(Value::Bool(b)) => Ok(*b),
            Some(_) => Err(format!("json: {name} must be a boolean")),
        }
    }

    pub fn int(&self, name: &str) -> Result<i64, String> {
        match self.find(name) {
            None => Ok(0),
            Some(Value::Number(n)) => n.as_i64().ok_or_else(|| format!("json: {name} must be an integer")),
            Some(_) => Err(format!("json: {name} must be a number")),
        }
    }

    pub fn value(&self, name: &str) -> Option<&Value> {
        self.find(name)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn first_value_ignores_trailing_data() {
        assert_eq!(first_value(br#"{"a":1} trailing"#).unwrap(), serde_json::json!({"a":1}));
        assert!(first_value(b"").is_err());
        assert!(first_value(b"{").is_err());
    }

    #[test]
    fn nulls_are_like_absent_fields() {
        let v = strip_nulls(serde_json::json!({"a": null, "b": {"c": null, "d": 1}, "e": [null, 2]}));
        assert_eq!(v, serde_json::json!({"b": {"d": 1}, "e": [null, 2]}));
    }

    #[test]
    fn body_is_case_insensitive_and_typed() {
        let b = Body::parse(br#"{"Index": 2, "on": true, "name": "x", "bad": "1"}"#).unwrap();
        assert_eq!(b.int("index").unwrap(), 2);
        assert!(b.bool("on").unwrap());
        assert_eq!(b.str("name").unwrap(), "x");
        assert_eq!(b.str("missing").unwrap(), "");
        assert!(b.int("bad").is_err());
        assert!(Body::parse(b"[1]").is_err());
        assert!(Body::parse(br#"{"arg": 1.5}"#).unwrap().int("arg").is_err());
    }
}
