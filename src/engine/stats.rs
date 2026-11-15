use std::collections::BTreeMap;
use std::path::Path;
use std::time::Duration;

use serde::{Deserialize, Serialize};

use super::Engine;
use crate::timer;

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Stats {
    pub since: String,
    pub total: i64,
    pub keys: BTreeMap<String, i64>,
    pub layers: BTreeMap<String, i64>,
    pub types: BTreeMap<String, i64>,
    pub days: BTreeMap<String, i64>,
}

impl Stats {
    pub fn fresh() -> Stats {
        Stats { since: crate::clock::now_local().date(), ..Default::default() }
    }

    pub fn load(path: &Path) -> Stats {
        let Ok(b) = std::fs::read(path) else { return Stats::fresh() };
        match serde_json::from_slice::<serde_json::Value>(&b) {
            Ok(v) if v.get("keys").is_some_and(|k| k.is_object()) => serde_json::from_value(crate::jsonx::strip_nulls(v)).unwrap_or_else(|_| Stats::fresh()),
            _ => Stats::fresh(),
        }
    }
}

impl Engine {
    pub(crate) fn count(&self, key: &str, layer: &str, kind: &str) {
        let mut st = self.st();
        let s = &mut st.stats;
        s.total += 1;
        *s.keys.entry(key.into()).or_insert(0) += 1;
        *s.layers.entry(layer.into()).or_insert(0) += 1;
        *s.types.entry(kind.into()).or_insert(0) += 1;
        *s.days.entry(crate::clock::now_local().date()).or_insert(0) += 1;
        while s.days.len() > 60 {
            let Some(oldest) = s.days.keys().next().cloned() else { break };
            s.days.remove(&oldest);
        }
        if st.stats_timer.is_none() {
            let me = self.me.clone();
            st.stats_timer = Some(timer::after(Duration::from_secs(30), move || {
                crate::sync::background(move || {
                    if let Some(e) = me.upgrade() {
                        e.save_stats();
                    }
                });
            }));
        }
    }

    pub fn save_stats(&self) {
        let data = {
            let mut st = self.st();
            st.stats_timer = None;
            serde_json::to_vec(&st.stats).unwrap_or_default()
        };
        let _ = std::fs::create_dir_all(&self.paths.dir);
        let _ = crate::config::write_atomic(&self.paths.stats(), &data);
    }

    pub fn stats(&self) -> Stats {
        self.st().stats.clone()
    }

    pub fn reset_stats(&self) {
        self.st().stats = Stats::fresh();
        self.save_stats();
    }
}
