use std::time::{Duration, Instant, SystemTime};

use serde_json::Value;

use super::{Engine, Event, KeyState, State, hex_key, target_index};
use crate::actions;
use crate::config::{Action, KeyDef, Keys};
use crate::hid::Stroke;
use crate::timer;

fn modifier_of(kd: &KeyDef) -> Option<&Action> {
    kd.tap.as_ref().filter(|a| a.is_momentary_layer()).or_else(|| kd.hold.as_ref().filter(|a| a.is_momentary_layer()))
}

fn set_bit(m: &mut [u8; 32], u: u8) {
    m[usize::from(u >> 3)] |= 1 << (u & 7);
}

fn add_keys(m: &mut [u8; 32], keys: &Keys) {
    for (id, kd) in keys {
        if kd.has_action()
            && let Ok(u) = u8::from_str_radix(id, 16)
        {
            set_bit(m, u);
        }
    }
}

fn held_targets(st: &State) -> Vec<usize> {
    let mut out = Vec::new();
    let n = st.cfg.layers.len();
    let mut visit = |keys: &Keys| {
        for kd in keys.values() {
            if let Some(m) = modifier_of(kd) {
                let i = target_index(m.to.as_ref().unwrap_or(&Value::Null), st.layer, n);
                if i != st.layer && !out.contains(&i) {
                    out.push(i);
                }
            }
        }
    };
    visit(&st.cfg.global);
    if let Some(l) = st.cfg.layers.get(st.layer) {
        visit(&l.keys);
    }
    out
}

fn resolve(st: &State, id: &str) -> Option<KeyDef> {
    if let Some(kd) = st.cfg.layers.get(st.layer).and_then(|l| l.keys.get(id)).filter(|kd| kd.has_action()) {
        return Some(kd.clone());
    }
    st.cfg.global.get(id).filter(|kd| kd.has_action()).cloned()
}

impl Engine {
    pub(crate) fn plain_key(&self, u: u8, down: bool) {
        let now = SystemTime::now();
        if down {
            self.keylog.press(u, now);
        } else {
            self.keylog.release(u, now);
        }
        if let Some(f) = self.hooks.read().unwrap_or_else(|e| e.into_inner()).plain_key.as_ref() {
            f(u, down);
        }
    }

    pub fn mask_bytes(&self) -> [u8; 32] {
        let st = self.st();
        let mut m = [0u8; 32];
        if st.edit_until.is_some_and(|t| Instant::now() < t) {
            return m;
        }
        if st.learning {
            for u in 0x04..0xE0u8 {
                set_bit(&mut m, u);
            }
            return m;
        }
        add_keys(&mut m, &st.cfg.global);
        if let Some(l) = st.cfg.layers.get(st.layer) {
            add_keys(&mut m, &l.keys);
        }
        for i in held_targets(&st) {
            add_keys(&mut m, &st.cfg.layers[i].keys);
        }
        for (u, k) in &st.keys {
            if k.down && k.known {
                set_bit(&mut m, *u);
            }
        }
        m
    }

    fn pass(&self, u: u8, mods: u8) {
        self.inject(Stroke::new(u, mods), if u == 0x39 { 150 } else { 0 });
    }

    pub fn key_down(&self, u: u8, mods: u8) {
        self.keylog.press(u, SystemTime::now());
        let id = hex_key(u);
        let mut st = self.st();
        if st.learning {
            *st.seen.entry(id.clone()).or_insert(0) += 1;
            self.emit_locked(&mut st, Event { key: id, ..Event::kind("watch") });
            return;
        }
        for (k, other) in st.keys.iter_mut() {
            if *k != u && other.down && other.modifier {
                other.used = true;
            }
        }
        if st.keys.get(&u).is_some_and(|k| k.down && k.modifier) {
            return;
        }
        st.last_key = id.clone();
        st.last_ts = crate::clock::now_f64();
        self.emit_locked(&mut st, Event { key: id.clone(), ..Event::kind("down") });
        let kd = resolve(&st, &id);
        let known = kd.is_some();
        let kd = kd.unwrap_or_default();
        let pending = st.keys.get_mut(&u).and_then(|k| {
            if let Some(t) = k.hold_timer.take() {
                t.cancel();
            }
            k.pending.take()
        });
        let mut ks = KeyState::new(kd.clone(), known, mods, pending);
        if !known {
            ks.done = true;
            st.keys.insert(u, ks);
            drop(st);
            self.pass(u, mods);
            return;
        }
        if let Some(m) = modifier_of(&kd) {
            ks.modifier = true;
            ks.momentary = true;
            ks.prev_layer = st.layer;
            if let Some(t) = ks.pending.take() {
                t.cancel();
            }
            let to = m.to.clone().unwrap_or(Value::Null);
            st.keys.insert(u, ks);
            drop(st);
            self.goto_layer(&to, false, false);
            return;
        }
        if let Some(t) = ks.pending.take() {
            t.cancel();
            ks.done = true;
            st.keys.insert(u, ks);
            drop(st);
            self.fire(u, "double", kd.double.as_ref(), &kd);
            return;
        }
        match (kd.hold.is_some(), kd.double.is_some()) {
            (false, false) => {
                ks.done = true;
                st.keys.insert(u, ks);
                drop(st);
                self.fire(u, "tap", kd.tap.as_ref(), &kd);
            }
            (true, _) => {
                let me = self.me.clone();
                ks.hold_timer = Some(timer::after(Duration::from_millis(st.cfg.settings.hold_ms.max(0) as u64), move || {
                    if let Some(e) = me.upgrade() {
                        e.hold_elapsed(u);
                    }
                }));
                st.keys.insert(u, ks);
            }
            _ => {
                st.keys.insert(u, ks);
            }
        }
    }

    fn hold_elapsed(&self, u: u8) {
        let mut st = self.st();
        let layer = st.layer;
        let Some(k) = st.keys.get_mut(&u).filter(|k| k.down && !k.hold_fired) else { return };
        k.hold_fired = true;
        let kd = k.kd.clone();
        if let Some(h) = kd.hold.as_ref().filter(|h| h.is_momentary_layer()) {
            k.momentary = true;
            k.prev_layer = layer;
            let to = h.to.clone().unwrap_or(Value::Null);
            drop(st);
            self.goto_layer(&to, false, false);
            return;
        }
        drop(st);
        self.fire(u, "hold", kd.hold.as_ref(), &kd);
    }

    pub fn key_up(&self, u: u8) {
        self.keylog.release(u, SystemTime::now());
        let mut st = self.st();
        self.emit_locked(&mut st, Event { key: hex_key(u), ..Event::kind("up") });
        let double_ms = st.cfg.settings.double_ms.max(0) as u64;
        let Some(k) = st.keys.get_mut(&u).filter(|k| k.down) else { return };
        k.down = false;
        if let Some(t) = k.hold_timer.take() {
            t.cancel();
        }
        if k.momentary {
            let (prev, kd, mods) = (k.prev_layer, k.kd.clone(), k.mods);
            let alone = k.modifier && !k.used;
            k.momentary = false;
            k.modifier = false;
            drop(st);
            self.goto_layer(&Value::from(prev as i64), false, false);
            if alone {
                match kd.tap.as_ref().filter(|t| !t.is_momentary_layer()) {
                    Some(t) => self.fire(u, "tap", Some(t), &kd),
                    None => self.pass(u, mods),
                }
            }
            return;
        }
        if k.done || k.hold_fired {
            return;
        }
        let kd = k.kd.clone();
        if kd.double.is_some() {
            let me = self.me.clone();
            k.pending = Some(timer::after(Duration::from_millis(double_ms), move || {
                if let Some(e) = me.upgrade() {
                    e.tap_after_wait(u);
                }
            }));
            return;
        }
        drop(st);
        self.tap(u, &kd);
    }

    fn tap_after_wait(&self, u: u8) {
        let mut st = self.st();
        let Some(k) = st.keys.get_mut(&u) else { return };
        if k.pending.take().is_none() {
            return;
        }
        let kd = k.kd.clone();
        drop(st);
        self.tap(u, &kd);
    }

    fn tap(&self, u: u8, kd: &KeyDef) {
        match &kd.tap {
            None => self.pass(u, 0),
            Some(a) => self.fire(u, "tap", Some(a), kd),
        }
    }

    fn fire(&self, u: u8, gesture: &'static str, a: Option<&Action>, kd: &KeyDef) {
        let Some(a) = a else { return };
        let mut label = kd.label.clone();
        if label.is_empty() {
            label = actions::describe(a);
        }
        if label.is_empty() {
            label = hex_key(u);
        }
        let key = (u, gesture);
        {
            let mut st = self.st();
            if st.last_fire.get(&key).is_some_and(|t| t.elapsed() < Duration::from_millis(60)) {
                return;
            }
            st.last_fire.insert(key, Instant::now());
            if a.confirm {
                let fresh = st.confirm.get(&key).is_none_or(|t| t.elapsed() > Duration::from_secs(3));
                if fresh {
                    st.confirm.insert(key, Instant::now());
                    let lang = st.cfg.settings.language.clone();
                    drop(st);
                    self.hud_async(super::tr_lang(&lang, "confirm", &[]), super::tr_lang(&lang, "press_again", std::slice::from_ref(&label)), 2.5, true);
                    return;
                }
                st.confirm.remove(&key);
            }
        }
        self.spawn_action(u, gesture, a.clone(), label);
    }
}
