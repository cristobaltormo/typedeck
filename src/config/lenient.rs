use serde::de::DeserializeOwned;
use serde_json::{Map, Value};

#[derive(Clone, Copy)]
pub enum Kind {
    Config,
    Settings,
    Hud,
    Obs,
    KeyDef,
    Action,
    Layer,
    Keyboard,
    Pack,
    PackFile,
}

#[derive(Clone, Copy)]
pub enum Node {
    Any,
    Str,
    Obj(Kind),
    Arr(&'static Node),
    Map(&'static Node),
}

const ACTION: Node = Node::Obj(Kind::Action);
const KEYDEF: Node = Node::Obj(Kind::KeyDef);
const KEYS: Node = Node::Map(&KEYDEF);
const LAYERS: Node = Node::Arr(&Node::Obj(Kind::Layer));

fn fields(kind: Kind) -> &'static [(&'static str, Node)] {
    use Node::{Any, Arr, Map, Obj, Str};
    match kind {
        Kind::Config => &[("version", Any), ("settings", Obj(Kind::Settings)), ("global", KEYS), ("layers", LAYERS), ("keyboards", Map(&Obj(Kind::Keyboard)))],
        Kind::Settings => &[
            ("language", Any),
            ("theme", Any),
            ("accent", Any),
            ("density", Any),
            ("keysize", Any),
            ("hud", Obj(Kind::Hud)),
            ("hold_ms", Any),
            ("double_ms", Any),
            ("auto_layer", Any),
            ("volume_step", Any),
            ("input", Any),
            ("capture", Any),
            ("typing_layout", Any),
            ("obs", Obj(Kind::Obs)),
            ("notify_keyboard", Any),
            ("key_history", Any),
            ("key_history_days", Any),
        ],
        Kind::Hud => &[("enabled", Any), ("position", Any), ("seconds", Any), ("on_action", Any), ("on_auto", Any), ("sound", Any)],
        Kind::Obs => &[("host", Any), ("port", Any), ("password", Any)],
        Kind::KeyDef => &[("tap", ACTION), ("hold", ACTION), ("double", ACTION), ("label", Any), ("icon", Any), ("color", Any)],
        Kind::Action => &[
            ("type", Any),
            ("app", Any),
            ("mode", Any),
            ("url", Any),
            ("cmd", Any),
            ("show_output", Any),
            ("host", Any),
            ("method", Any),
            ("body", Any),
            ("keys", Any),
            ("text", Any),
            ("minutes", Any),
            ("label", Any),
            ("to", Any),
            ("momentary", Any),
            ("toggle", Any),
            ("ms", Any),
            ("target", Any),
            ("steps", Arr(&ACTION)),
            ("else", Arr(&ACTION)),
            ("cond", Any),
            ("not", Any),
            ("confirm", Any),
        ],
        Kind::Layer => &[("name", Any), ("color", Any), ("icon", Any), ("auto_apps", Arr(&Str)), ("keys", KEYS)],
        Kind::Keyboard => &[("layout", Any), ("name", Any)],
        Kind::Pack => &[("typedeck_macros", Any), ("name", Any), ("author", Any), ("layers", LAYERS), ("global", KEYS)],
        Kind::PackFile => &[("pack", Obj(Kind::Pack))],
    }
}

fn zero(node: Node) -> Value {
    match node {
        Node::Str => Value::String(String::new()),
        Node::Obj(_) | Node::Map(_) => Value::Object(Map::new()),
        Node::Arr(_) => Value::Array(Vec::new()),
        Node::Any => Value::Null,
    }
}

pub fn normalize(v: Value, node: Node) -> Value {
    match (node, v) {
        (Node::Obj(kind), Value::Object(m)) => {
            let fs = fields(kind);
            let mut out = Map::new();
            let mut exact = Vec::new();
            for (k, val) in m {
                if val.is_null() {
                    continue;
                }
                let found =
                    fs.iter().find(|(n, _)| *n == k).map(|f| (f, true)).or_else(|| fs.iter().find(|(n, _)| n.eq_ignore_ascii_case(&k)).map(|f| (f, false)));
                let Some(((name, child), is_exact)) = found else { continue };
                if !is_exact && exact.contains(name) {
                    continue;
                }
                if is_exact {
                    exact.push(*name);
                }
                out.insert((*name).to_string(), normalize(val, *child));
            }
            Value::Object(out)
        }
        (Node::Obj(_), Value::Null) => Value::Object(Map::new()),
        (Node::Arr(elem), Value::Array(a)) => Value::Array(a.into_iter().map(|x| if x.is_null() { zero(*elem) } else { normalize(x, *elem) }).collect()),
        (Node::Map(elem), Value::Object(m)) => {
            Value::Object(m.into_iter().map(|(k, x)| (k, if x.is_null() { zero(*elem) } else { normalize(x, *elem) })).collect())
        }
        (Node::Any, v) => crate::jsonx::strip_nulls(v),
        (_, v) => v,
    }
}

pub fn from_value<T: DeserializeOwned>(v: Value, kind: Kind) -> Result<T, String> {
    serde_json::from_value(normalize(v, Node::Obj(kind))).map_err(|e| e.to_string())
}

pub fn decode<T: DeserializeOwned>(data: &[u8], kind: Kind) -> Result<T, String> {
    from_value(crate::jsonx::first_value(data)?, kind)
}
