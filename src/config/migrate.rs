use serde_json::{Map, Value, json};

use super::lenient::{self, Kind};
use super::{Action, Config, KeyDef, VERSION, is_key_id, validate};

const V2_NAMES: &[(&str, &str)] = &[
    ("NUMLOCK", "53"),
    ("KP/", "54"),
    ("KP*", "55"),
    ("KP-", "56"),
    ("KP+", "57"),
    ("KPENTER", "58"),
    ("KP1", "59"),
    ("KP2", "5A"),
    ("KP3", "5B"),
    ("KP4", "5C"),
    ("KP5", "5D"),
    ("KP6", "5E"),
    ("KP7", "5F"),
    ("KP8", "60"),
    ("KP9", "61"),
    ("KP0", "62"),
    ("KP.", "63"),
];

pub fn parse(data: &[u8]) -> Result<Config, String> {
    let probe: Value = serde_json::from_slice(data).map_err(|e| format!("invalid json: {e}"))?;
    let probe = match crate::jsonx::strip_nulls(probe) {
        Value::Object(m) => m,
        Value::Null => Map::new(),
        _ => return Err("invalid json: not an object".into()),
    };
    let ver = probe.get("version").and_then(Value::as_f64).unwrap_or(0.0);
    if ver >= 3.0 {
        let c: Config = lenient::from_value(Value::Object(probe), Kind::Config)?;
        return validate(&c);
    }
    if ver == 2.0 {
        return migrate_v2(probe);
    }
    migrate_v1(&probe)
}

fn migrate_v1(m: &Map<String, Value>) -> Result<Config, String> {
    let conv = |keys: Option<&Value>| -> Map<String, Value> {
        let mut out = Map::new();
        if let Some(Value::Object(km)) = keys {
            for (k, v) in km {
                let Value::Object(a) = v else { continue };
                if a.get("type").is_none() {
                    continue;
                }
                let mut a = a.clone();
                let mut kd = Map::new();
                if let Some(Value::String(l)) = a.get("label")
                    && !l.is_empty()
                {
                    kd.insert("label".into(), Value::String(l.clone()));
                }
                a.remove("label");
                kd.insert("tap".into(), Value::Object(a));
                out.insert(k.clone(), Value::Object(kd));
            }
        }
        out
    };
    let mut layers = Vec::new();
    if let Some(Value::Array(ls)) = m.get("layers") {
        for l in ls {
            let name = l.get("name").cloned().unwrap_or(Value::Null);
            layers.push(json!({"name": name, "keys": conv(l.get("keys"))}));
        }
    }
    let mut settings = json!({});
    if let Some(Value::Object(st)) = m.get("settings")
        && st.get("hud_on_action") == Some(&Value::Bool(true))
    {
        settings = json!({"hud": {"on_action": true}});
    }
    let mut v2 = Map::new();
    v2.insert("version".into(), json!(2));
    v2.insert("global".into(), Value::Object(conv(m.get("global"))));
    v2.insert("layers".into(), Value::Array(layers));
    v2.insert("settings".into(), settings);
    migrate_v2(v2)
}

fn rekey(keys: Option<&Value>) -> Map<String, Value> {
    let mut out = Map::new();
    if let Some(Value::Object(km)) = keys {
        for (k, v) in km {
            if let Some((_, id)) = V2_NAMES.iter().find(|(n, _)| n == k) {
                out.insert((*id).to_string(), v.clone());
            } else if is_key_id(k) {
                out.insert(k.to_ascii_uppercase(), v.clone());
            }
        }
    }
    out
}

fn migrate_v2(mut m: Map<String, Value>) -> Result<Config, String> {
    let global = rekey(m.get("global"));
    m.insert("global".into(), Value::Object(global));
    if let Some(Value::Array(ls)) = m.get_mut("layers") {
        for l in ls.iter_mut() {
            if let Value::Object(lm) = l {
                let keys = rekey(lm.get("keys"));
                lm.insert("keys".into(), Value::Object(keys));
            }
        }
    }
    m.insert("version".into(), json!(VERSION));
    let mut c: Config = lenient::from_value(Value::Object(m), Kind::Config)?;
    c.global.entry("53".into()).or_insert_with(|| KeyDef {
        tap: Some(Action { to: Some("next".into()), ..Action::new("layer") }),
        label: "Capa".into(),
        icon: "layers".into(),
        ..Default::default()
    });
    validate(&c)
}
