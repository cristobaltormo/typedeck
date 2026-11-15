mod consent;
pub mod lenient;
mod migrate;
mod store;
mod validate;

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::jsonx::{go_f64, is_false, is_zero_f64, is_zero_i64};

pub use consent::{CAPTURE_CONSENT_VERSION, has_capture_consent, record_capture_consent};
pub use migrate::parse;
pub use store::{BackupInfo, Paths, list_backups, load, load_with_note, restore_backup, save, write_atomic, write_private as write_private_file};
pub use validate::{MAX_LAYERS, MAX_STEPS, is_backup_name, is_hex_color, is_key_id, truncate_bytes, validate};

pub const VERSION: i64 = 3;

fn is_empty_str(s: &str) -> bool {
    s.is_empty()
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Action {
    #[serde(rename = "type")]
    pub kind: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub app: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub mode: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub url: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub cmd: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub show_output: Option<bool>,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub host: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub method: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub body: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub keys: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub text: String,
    #[serde(skip_serializing_if = "is_zero_f64", serialize_with = "go_f64")]
    pub minutes: f64,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub label: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub to: Option<Value>,
    #[serde(skip_serializing_if = "is_false")]
    pub momentary: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub toggle: bool,
    #[serde(skip_serializing_if = "is_zero_i64")]
    pub ms: i64,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub target: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub steps: Vec<Action>,
    #[serde(rename = "else", skip_serializing_if = "Vec::is_empty")]
    pub otherwise: Vec<Action>,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub cond: String,
    #[serde(skip_serializing_if = "is_false")]
    pub not: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub confirm: bool,
}

impl Action {
    pub fn new(kind: &str) -> Self {
        Action { kind: kind.into(), ..Default::default() }
    }

    pub fn is_momentary_layer(&self) -> bool {
        self.kind == "layer" && self.momentary
    }
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct KeyDef {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tap: Option<Action>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hold: Option<Action>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub double: Option<Action>,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub label: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub icon: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub color: String,
}

impl KeyDef {
    pub fn is_empty(&self) -> bool {
        !self.has_action() && self.label.is_empty() && self.icon.is_empty() && self.color.is_empty()
    }

    pub fn has_action(&self) -> bool {
        self.tap.is_some() || self.hold.is_some() || self.double.is_some()
    }

    pub fn tap(a: Action) -> Self {
        KeyDef { tap: Some(a), ..Default::default() }
    }
}

pub type Keys = BTreeMap<String, KeyDef>;

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Layer {
    pub name: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub color: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub icon: String,
    pub auto_apps: Vec<String>,
    pub keys: Keys,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Hud {
    pub enabled: bool,
    pub position: String,
    #[serde(serialize_with = "go_f64")]
    pub seconds: f64,
    pub on_action: bool,
    pub on_auto: bool,
    pub sound: bool,
}

impl Default for Hud {
    fn default() -> Self {
        Hud { enabled: true, position: "bottom".into(), seconds: 1.2, on_action: false, on_auto: true, sound: false }
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct Obs {
    pub host: String,
    pub port: i64,
    pub password: String,
}

impl Default for Obs {
    fn default() -> Self {
        Obs { host: "127.0.0.1".into(), port: 4455, password: String::new() }
    }
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Settings {
    pub language: String,
    pub theme: String,
    pub accent: String,
    pub density: String,
    pub keysize: i64,
    pub hud: Hud,
    pub hold_ms: i64,
    pub double_ms: i64,
    pub auto_layer: bool,
    pub volume_step: i64,
    pub input: String,
    pub capture: String,
    pub typing_layout: String,
    pub obs: Obs,
    pub notify_keyboard: bool,
    pub key_history: bool,
    pub key_history_days: i64,
}

impl Default for Settings {
    fn default() -> Self {
        Settings {
            language: "en".into(),
            theme: "auto".into(),
            accent: "#2563eb".into(),
            density: "comfortable".into(),
            keysize: 88,
            hud: Hud::default(),
            hold_ms: 450,
            double_ms: 280,
            auto_layer: true,
            volume_step: 6,
            input: "auto".into(),
            capture: "board".into(),
            typing_layout: "auto".into(),
            obs: Obs::default(),
            notify_keyboard: true,
            key_history: false,
            key_history_days: 30,
        }
    }
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct Keyboard {
    pub layout: String,
    #[serde(skip_serializing_if = "is_empty_str")]
    pub name: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Config {
    pub version: i64,
    pub settings: Settings,
    pub global: Keys,
    pub layers: Vec<Layer>,
    pub keyboards: BTreeMap<String, Keyboard>,
}

impl Config {
    pub fn initial() -> Config {
        let mut global = Keys::new();
        global.insert(
            "53".into(),
            KeyDef { tap: Some(Action { to: Some("next".into()), ..Action::new("layer") }), label: "Capa".into(), icon: "layers".into(), ..Default::default() },
        );
        global.insert(
            "58".into(),
            KeyDef { tap: Some(Action { url: "{editor}".into(), ..Action::new("url") }), label: "Editor".into(), icon: "sliders".into(), ..Default::default() },
        );
        let layer =
            |name: &str, color: &str, icon: &str| Layer { name: name.into(), color: color.into(), icon: icon.into(), auto_apps: vec![], keys: Keys::new() };
        Config {
            version: VERSION,
            settings: Settings::default(),
            global,
            layers: vec![layer("Layer 1", "#2563eb", "keyboard"), layer("Layer 2", "#8b5cf6", "bolt")],
            keyboards: BTreeMap::new(),
        }
    }

    pub fn key_count(&self) -> usize {
        self.global.len() + self.layers.iter().map(|l| l.keys.len()).sum::<usize>()
    }
}

#[cfg(test)]
mod tests;
