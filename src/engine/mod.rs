mod keys;
mod stats;

use std::collections::{BTreeMap, HashMap, VecDeque};
use std::sync::{Arc, Condvar, Mutex, MutexGuard, RwLock, Weak};
use std::time::{Duration, Instant};

use serde::Serialize;
use serde_json::Value;

pub use stats::Stats;

use crate::actions::{self, ActResult, Env};
use crate::capture::{BOARD, Event as CaptureEvent, Handlers, Info, SOFTWARE, Source};
use crate::config::{self, Action, Config, Paths, Settings};
use crate::hid::Stroke;
use crate::jsonx::{is_false, is_zero_i64};
use crate::keylog::Log;
use crate::platform::{self, StopFn, SystemInfo};
use crate::sync::{Flag, Signal, lock, wait_timeout};
use crate::timer::{self, Timer};

const MAX_EVENTS: usize = 400;
const MAX_ACTIONS: usize = 8;
const STALE_INJECT: Duration = Duration::from_millis(500);

fn is_empty(s: &str) -> bool {
    s.is_empty()
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
pub struct Event {
    pub id: i64,
    #[serde(serialize_with = "crate::jsonx::go_f64")]
    pub ts: f64,
    pub kind: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub key: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub gesture: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub layer: Option<i64>,
    #[serde(skip_serializing_if = "is_empty")]
    pub layer_name: String,
    #[serde(skip_serializing_if = "is_empty")]
    pub name: String,
    #[serde(skip_serializing_if = "is_false")]
    pub auto: bool,
    #[serde(skip_serializing_if = "is_empty")]
    pub label: String,
    #[serde(rename = "type", skip_serializing_if = "is_empty")]
    pub action_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ok: Option<bool>,
    #[serde(skip_serializing_if = "is_zero_i64")]
    pub ms: i64,
    #[serde(skip_serializing_if = "is_empty")]
    pub output: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub names: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub connected: Option<bool>,
    #[serde(skip_serializing_if = "is_empty")]
    pub port: String,
}

impl Event {
    fn kind(kind: &str) -> Event {
        Event { kind: kind.into(), ..Default::default() }
    }
}

#[derive(Clone, Debug, Serialize)]
pub struct EngineStatus {
    pub layer: usize,
    pub manual_layer: usize,
    #[serde(skip_serializing_if = "is_empty")]
    pub last: String,
    pub last_ts: f64,
    pub front: Vec<String>,
    pub uptime: i64,
    pub auto_active: bool,
    pub last_id: i64,
    pub learning: bool,
}

pub(crate) struct KeyState {
    kd: config::KeyDef,
    known: bool,
    down: bool,
    done: bool,
    hold_fired: bool,
    mods: u8,
    hold_timer: Option<Timer>,
    pending: Option<Timer>,
    momentary: bool,
    modifier: bool,
    used: bool,
    prev_layer: usize,
    at: Instant,
}

impl KeyState {
    fn new(kd: config::KeyDef, known: bool, mods: u8, pending: Option<Timer>) -> KeyState {
        KeyState {
            kd,
            known,
            down: true,
            done: false,
            hold_fired: false,
            mods,
            hold_timer: None,
            pending,
            momentary: false,
            modifier: false,
            used: false,
            prev_layer: 0,
            at: Instant::now(),
        }
    }
}

pub(crate) struct State {
    cfg: Config,
    layer: usize,
    manual_layer: usize,
    toggle_back: usize,
    auto_active: bool,
    events: VecDeque<Event>,
    next_id: i64,
    keys: HashMap<u8, KeyState>,
    confirm: HashMap<(u8, &'static str), Instant>,
    last_fire: HashMap<(u8, &'static str), Instant>,
    stats: Stats,
    stats_timer: Option<Timer>,
    front: Vec<String>,
    asn: String,
    last_key: String,
    last_ts: f64,
    stop_awake: Option<StopFn>,
    learning: bool,
    seen: BTreeMap<String, i64>,
    edit_until: Option<Instant>,
    kbd_timer: Option<Timer>,
    kbd_gen: u64,
    kbd_lost: bool,
    editor_open: bool,
    pending_notice: bool,
}

type ExecFn = Box<dyn Fn(&Action, bool) -> ActResult + Send + Sync>;
type HudFn = Box<dyn Fn(&str, &str, f64, bool) + Send + Sync>;
type InjectFn = Box<dyn Fn(Stroke, u32) + Send + Sync>;
type PlainKeyFn = Box<dyn Fn(u8, bool) + Send + Sync>;

#[derive(Default)]
struct Hooks {
    exec: Option<ExecFn>,
    hud: Option<HudFn>,
    inject: Option<InjectFn>,
    plain_key: Option<PlainKeyFn>,
}

#[derive(Default)]
struct SyncRequest {
    mask: bool,
    keys: bool,
    layer: Option<usize>,
}

pub struct Engine {
    me: Weak<Engine>,
    paths: Paths,
    st: Mutex<State>,
    events_cv: Condvar,
    keylog: Arc<Log>,
    src: Option<Arc<dyn Source>>,
    hooks: RwLock<Hooks>,
    wake: Signal,
    started: Instant,
    actions: (Mutex<usize>, Condvar),
    sync: Arc<(Mutex<SyncRequest>, Condvar)>,
    inject_q: Arc<crate::queue::Queue<(Stroke, u32, Instant)>>,
    kbd_lost_after: Mutex<Duration>,
    cfg_write: Mutex<()>,
}

const STRINGS: &[(&str, &str, &str)] = &[
    ("layer_of", "Capa {} de {}", "Layer {} of {}"),
    ("confirm", "Confirmar", "Confirm"),
    ("press_again", "Pulsa otra vez: {}", "Press again: {}"),
    ("error", "Error", "Error"),
    ("caff_on", "Mac despierto", "Mac awake"),
    ("caff_off", "Mac puede dormir", "Mac can sleep"),
    ("timer_start", "Temporizador", "Timer"),
    ("timer_end", "Tiempo cumplido", "Time is up"),
    ("kbd_lost", "Teclado no detectado", "Keyboard not detected"),
    ("kbd_lost_sub", "Desenchufa el teclado del shield y vuelve a enchufarlo", "Unplug the keyboard from the shield and plug it back in"),
    ("kbd_back", "Teclado conectado", "Keyboard connected"),
    ("hist_on", "Historial de tecleo activado", "Typing history on"),
    ("hist_off", "Historial de tecleo desactivado", "Typing history off"),
    ("capture_failed", "Captura por software no disponible", "Software capture unavailable"),
    ("capture_failed_sub", "Se usa la placa; revisa los permisos en el editor", "Using the board; check the permissions in the editor"),
    ("capture_consent_lost", "Captura por software desactivada", "Software capture turned off"),
    (
        "capture_consent_lost_sub",
        "Esta instalación nunca la ha aceptado: actívala desde el editor",
        "This installation never agreed to it: turn it on from the editor",
    ),
];

fn tr_lang(lang: &str, key: &str, args: &[String]) -> String {
    let Some((_, es, en)) = STRINGS.iter().find(|(k, _, _)| *k == key) else { return key.into() };
    let mut f = if lang == "es" { (*es).to_string() } else { (*en).to_string() };
    for a in args {
        f = f.replacen("{}", a, 1);
    }
    f
}

pub fn hex_key(u: u8) -> String {
    format!("{u:02X}")
}

fn clip(s: &str, n: usize) -> String {
    s.chars().take(n).collect()
}

pub fn target_index(target: &Value, current: usize, n: usize) -> usize {
    if n == 0 {
        return 0;
    }
    match target {
        Value::String(s) if s == "prev" => (current + n - 1) % n,
        Value::String(_) => (current + 1) % n,
        Value::Number(num) => {
            let v = num.as_i64().unwrap_or_else(|| num.as_f64().map(|f| f.trunc() as i64).unwrap_or(0));
            v.clamp(0, n as i64 - 1) as usize
        }
        _ => current.min(n - 1),
    }
}

impl Engine {
    pub fn new(paths: Paths, src: Option<Arc<dyn Source>>) -> Result<Arc<Engine>, String> {
        let (cfg, note) = config::load_with_note(&paths)?;
        if !note.is_empty() {
            crate::log!("{note}");
        }
        let keylog = Log::new(paths.keystrokes());
        let e = Arc::new_cyclic(|me| Engine {
            me: me.clone(),
            paths: paths.clone(),
            st: Mutex::new(State {
                cfg: cfg.clone(),
                layer: 0,
                manual_layer: 0,
                toggle_back: 0,
                auto_active: false,
                events: VecDeque::new(),
                next_id: 1,
                keys: HashMap::new(),
                confirm: HashMap::new(),
                last_fire: HashMap::new(),
                stats: Stats::load(&paths.stats()),
                stats_timer: None,
                front: vec![],
                asn: String::new(),
                last_key: String::new(),
                last_ts: 0.0,
                stop_awake: None,
                learning: false,
                seen: BTreeMap::new(),
                edit_until: None,
                kbd_timer: None,
                kbd_gen: 0,
                kbd_lost: false,
                editor_open: false,
                pending_notice: false,
            }),
            events_cv: Condvar::new(),
            keylog,
            src: src.clone(),
            hooks: RwLock::new(Hooks::default()),
            wake: Signal::new(),
            started: Instant::now(),
            actions: (Mutex::new(0), Condvar::new()),
            sync: Arc::new((Mutex::new(SyncRequest::default()), Condvar::new())),
            inject_q: Arc::new(crate::queue::Queue::new(256)),
            kbd_lost_after: Mutex::new(Duration::from_secs(10)),
            cfg_write: Mutex::new(()),
        });
        let (guarded, lost) = e.guard_capture(cfg.clone());
        if lost {
            lock(&e.st).cfg = guarded.clone();
            lock(&e.st).pending_notice = true;
            let _ = config::save(&paths, &guarded, false, false);
        }
        let s = e.settings();
        e.keylog.set_enabled(s.key_history);
        e.keylog.set_retention(Duration::from_secs(s.key_history_days.max(0) as u64 * 86400));
        e.start_workers();
        if let Some(src) = &src {
            src.bind(e.handlers());
        }
        Ok(e)
    }

    fn start_workers(&self) {
        let (sync, me) = (self.sync.clone(), self.me.clone());
        crate::sync::spawn("engine-sync", move || {
            loop {
                let req = {
                    let mut g = lock(&sync.0);
                    while !g.mask && !g.keys && g.layer.is_none() {
                        g = wait_timeout(&sync.1, g, Duration::from_secs(3600));
                        if me.strong_count() == 0 {
                            return;
                        }
                    }
                    std::mem::take(&mut *g)
                };
                let Some(e) = me.upgrade() else { return };
                let Some(src) = e.src.clone() else { continue };
                if req.mask {
                    src.sync_mask(false);
                }
                if req.keys {
                    let _ = src.keys(e.wants_all_keys());
                }
                if let Some(l) = req.layer {
                    src.set_layer(l);
                }
            }
        });
        let (q, me) = (self.inject_q.clone(), self.me.clone());
        crate::sync::spawn("engine-inject", move || {
            loop {
                let item = q.pop(Duration::from_secs(5));
                let Some(e) = me.upgrade() else { return };
                if let (Some((s, ms, at)), Some(src)) = (item, e.src.clone())
                    && at.elapsed() < STALE_INJECT
                {
                    let _ = src.tap_held(s, ms);
                }
            }
        });
    }

    fn st(&self) -> MutexGuard<'_, State> {
        lock(&self.st)
    }

    pub fn set_test_hooks(&self, exec: Option<ExecFn>, hud: Option<HudFn>, inject: Option<InjectFn>) {
        let mut h = self.hooks.write().unwrap_or_else(|e| e.into_inner());
        if exec.is_some() {
            h.exec = exec;
        }
        if hud.is_some() {
            h.hud = hud;
        }
        if inject.is_some() {
            h.inject = inject;
        }
    }

    pub fn set_plain_key(&self, f: PlainKeyFn) {
        self.hooks.write().unwrap_or_else(|e| e.into_inner()).plain_key = Some(f);
    }

    pub fn set_kbd_lost_after(&self, d: Duration) {
        *lock(&self.kbd_lost_after) = d;
    }

    fn exec(&self, a: &Action, capture: bool) -> ActResult {
        if let Some(f) = self.hooks.read().unwrap_or_else(|e| e.into_inner()).exec.as_ref() {
            return f(a, capture);
        }
        actions::execute(a, self, capture)
    }

    pub(crate) fn hud_fn(&self, title: &str, sub: &str, seconds: f64, force: bool) {
        if let Some(f) = self.hooks.read().unwrap_or_else(|e| e.into_inner()).hud.as_ref() {
            return f(title, sub, seconds, force);
        }
        self.show_hud(title, sub, seconds, force);
    }

    fn hud_async(&self, title: String, sub: String, seconds: f64, force: bool) {
        let me = self.me.clone();
        crate::sync::spawn("hud", move || {
            if let Some(e) = me.upgrade() {
                e.hud_fn(&title, &sub, seconds, force);
            }
        });
    }

    pub(crate) fn inject(&self, s: Stroke, hold_ms: u32) {
        if let Some(f) = self.hooks.read().unwrap_or_else(|e| e.into_inner()).inject.as_ref() {
            return f(s, hold_ms);
        }
        if self.src.is_some() {
            self.inject_q.try_push((s, hold_ms, Instant::now()));
        }
    }

    fn tr(&self, key: &str, args: &[String]) -> String {
        let lang = self.st().cfg.settings.language.clone();
        tr_lang(&lang, key, args)
    }

    fn handlers(&self) -> Handlers {
        let (a, b, c, d) = (self.me.clone(), self.me.clone(), self.me.clone(), self.me.clone());
        Handlers {
            on_event: Some(Arc::new(move |ev| {
                if let Some(e) = a.upgrade() {
                    e.handle_event(ev);
                }
            })),
            mask: Some(Arc::new(move || b.upgrade().map(|e| e.mask_bytes()).unwrap_or([0; 32]))),
            on_connected: Some(Arc::new(move |i: &Info| {
                if let Some(e) = c.upgrade() {
                    e.on_board_connected(&i.firmware);
                    e.keyboard_changed(i.present);
                }
            })),
            on_disconnect: Some(Arc::new(move || {
                if let Some(e) = d.upgrade() {
                    e.on_board_disconnected();
                }
            })),
        }
    }

    pub fn config(&self) -> Config {
        self.st().cfg.clone()
    }

    pub fn paths(&self) -> &Paths {
        &self.paths
    }

    pub fn keylog(&self) -> &Arc<Log> {
        &self.keylog
    }

    pub fn source(&self) -> Option<&Arc<dyn Source>> {
        self.src.as_ref()
    }

    pub fn set_config(&self, c: Config) -> Result<Config, String> {
        let _w = lock(&self.cfg_write);
        self.set_config_locked(c, "")
    }

    pub fn set_config_keep_capture(&self, mut c: Config, origin: &str) -> Result<Config, String> {
        let _w = lock(&self.cfg_write);
        c.settings.capture = self.config().settings.capture;
        self.set_config_locked(c, origin)
    }

    pub fn update_config(&self, f: impl FnOnce(&mut Config)) -> Result<Config, String> {
        let _w = lock(&self.cfg_write);
        let mut c = self.config();
        f(&mut c);
        self.set_config_locked(c, "")
    }

    fn set_config_locked(&self, c: Config, origin: &str) -> Result<Config, String> {
        let (c, _) = self.guard_capture(c);
        config::save(&self.paths, &c, true, false)?;
        let loaded = config::load(&self.paths)?;
        Ok(self.apply_config(loaded, origin))
    }

    pub fn apply_restored(&self, c: Config) -> Config {
        let _w = lock(&self.cfg_write);
        self.apply_config(c, "")
    }

    fn apply_config(&self, c: Config, origin: &str) -> Config {
        let (c, lost) = self.guard_capture(c);
        if lost {
            let _ = config::save(&self.paths, &c, false, false);
            self.hud_async(self.tr("capture_consent_lost", &[]), self.tr("capture_consent_lost_sub", &[]), 8.0, true);
        }
        let mode = c.settings.capture.clone();
        {
            let mut st = self.st();
            st.cfg = c.clone();
            self.keylog.set_enabled(c.settings.key_history);
            self.keylog.set_retention(Duration::from_secs(c.settings.key_history_days.max(0) as u64 * 86400));
            st.asn.clear();
            let n = c.layers.len().max(1);
            st.layer = st.layer.min(n - 1);
            st.manual_layer = st.manual_layer.min(n - 1);
            let origin: String = origin.chars().filter(|c| c.is_ascii_alphanumeric() || *c == '-').take(40).collect();
            self.emit_locked(&mut st, Event { name: origin, ..Event::kind("config") });
        }
        self.wake.notify();
        self.apply_capture(&mode);
        self.sync_mask_async();
        self.sync_keys_async();
        c
    }

    pub fn set_editor_open(&self, open: bool) {
        let changed = {
            let mut st = self.st();
            let changed = st.editor_open != open;
            st.editor_open = open;
            changed
        };
        if changed {
            self.sync_keys_async();
        }
    }

    fn software_mode(&self) -> bool {
        self.src.as_ref().is_some_and(|s| s.kind() == SOFTWARE)
    }

    fn wants_all_keys(&self) -> bool {
        let software = self.software_mode();
        let st = self.st();
        if software { st.learning || st.cfg.settings.key_history } else { st.editor_open || st.cfg.settings.key_history }
    }

    pub(crate) fn sync_mask_async(&self) {
        lock(&self.sync.0).mask = true;
        self.sync.1.notify_one();
    }

    fn sync_keys_async(&self) {
        lock(&self.sync.0).keys = true;
        self.sync.1.notify_one();
    }

    fn set_layer_async(&self, i: usize) {
        lock(&self.sync.0).layer = Some(i);
        self.sync.1.notify_one();
    }

    pub fn run(&self, stop: &Flag) {
        crate::sync::fault_point("engine");
        if std::mem::take(&mut self.st().pending_notice) {
            self.hud_async(self.tr("capture_consent_lost", &[]), self.tr("capture_consent_lost_sub", &[]), 8.0, true);
        }
        let mode = self.settings().capture;
        self.apply_capture(&mode);
        while !stop.is_set() {
            if self.wants_front_app() {
                self.check_front_app();
                self.wake.wait(Duration::from_millis(1500));
            } else {
                self.wake.wait(Duration::from_secs(3600));
            }
        }
        self.save_stats();
        self.keylog.flush();
    }

    pub fn stop(&self) {
        self.wake.notify();
        self.save_stats();
        self.keylog.flush();
    }

    pub fn handle_event(&self, ev: CaptureEvent) {
        crate::sync::fault_point("event");
        match ev.kind {
            b'D' => self.key_down(ev.usage, ev.mods),
            b'U' => self.key_up(ev.usage),
            b'P' | b'R' => self.plain_key(ev.usage, ev.kind == b'P'),
            b'W' => {
                let mut st = self.st();
                if st.learning {
                    *st.seen.entry(hex_key(ev.usage)).or_insert(0) += 1;
                    self.emit_locked(&mut st, Event { key: hex_key(ev.usage), ..Event::kind("watch") });
                }
            }
            b'K' => {
                let c = ev.arg == 1;
                self.emit(Event { connected: Some(c), ..Event::kind("keyboard") });
                self.keyboard_changed(c);
            }
            _ => {}
        }
    }

    pub fn keyboard_changed(&self, present: bool) {
        let (notify, back) = {
            let mut st = self.st();
            if let Some(t) = st.kbd_timer.take() {
                t.cancel();
            }
            let back = present && st.kbd_lost;
            if present {
                st.kbd_lost = false;
            } else {
                let me = self.me.clone();
                st.kbd_gen += 1;
                let generation = st.kbd_gen;
                st.kbd_timer = Some(timer::after(*lock(&self.kbd_lost_after), move || {
                    let Some(e) = me.upgrade() else { return };
                    let on = {
                        let mut st = e.st();
                        if st.kbd_gen != generation || st.kbd_timer.as_ref().is_none_or(Timer::is_cancelled) {
                            return;
                        }
                        st.kbd_lost = true;
                        st.kbd_timer = None;
                        st.cfg.settings.notify_keyboard
                    };
                    if on {
                        e.hud_async(e.tr("kbd_lost", &[]), e.tr("kbd_lost_sub", &[]), 5.0, true);
                    }
                }));
            }
            (st.cfg.settings.notify_keyboard, back)
        };
        if back && notify {
            self.hud_fn(&self.tr("kbd_back", &[]), "", 2.0, true);
        }
    }

    pub fn status(&self) -> EngineStatus {
        let st = self.st();
        EngineStatus {
            layer: st.layer,
            manual_layer: st.manual_layer,
            last: st.last_key.clone(),
            last_ts: st.last_ts,
            front: st.front.clone(),
            uptime: self.started.elapsed().as_secs() as i64,
            auto_active: st.auto_active,
            last_id: st.next_id - 1,
            learning: st.learning,
        }
    }

    pub fn start_learn(&self) -> Result<(), String> {
        if !self.src.as_ref().is_some_and(|s| s.connected()) {
            return Err("keyboard capture is not active".into());
        }
        {
            let mut st = self.st();
            st.learning = true;
            st.seen.clear();
        }
        self.sync_mask_async();
        self.sync_keys_async();
        Ok(())
    }

    pub fn stop_learn(&self) {
        self.st().learning = false;
        self.sync_mask_async();
        self.sync_keys_async();
    }

    pub fn set_learning_for_test(&self, on: bool) {
        let mut st = self.st();
        st.learning = on;
        if on {
            st.seen.clear();
        }
    }

    pub fn seen(&self) -> BTreeMap<String, i64> {
        self.st().seen.clone()
    }

    fn on_board_connected(&self, port: &str) {
        self.emit(Event { connected: Some(true), port: port.into(), name: self.capture_mode().into(), ..Event::kind("board") });
        self.sync_keys_async();
    }

    fn on_board_disconnected(&self) {
        let restore = {
            let mut st = self.st();
            if let Some(t) = st.kbd_timer.take() {
                t.cancel();
            }
            let restore = st.keys.values().filter(|k| k.down && k.momentary).min_by_key(|k| k.at).map(|k| k.prev_layer);
            for k in st.keys.values_mut() {
                if let Some(t) = k.hold_timer.take() {
                    t.cancel();
                }
                k.down = false;
                k.momentary = false;
                k.modifier = false;
            }
            restore
        };
        if let Some(prev) = restore {
            self.goto_layer(&Value::from(prev as i64), false, false);
        }
        self.emit(Event { connected: Some(false), name: self.capture_mode().into(), ..Event::kind("board") });
    }

    fn emit_locked(&self, st: &mut State, mut ev: Event) {
        ev.id = st.next_id;
        ev.ts = crate::clock::now_f64();
        st.next_id += 1;
        st.events.push_back(ev);
        while st.events.len() > MAX_EVENTS {
            st.events.pop_front();
        }
        self.events_cv.notify_all();
    }

    fn emit(&self, ev: Event) {
        let mut st = self.st();
        self.emit_locked(&mut st, ev);
    }

    pub fn events_since(&self, id: i64, wait: Duration, cancelled: &dyn Fn() -> bool) -> (Vec<Event>, i64) {
        let deadline = Instant::now() + wait;
        let mut st = self.st();
        loop {
            let out: Vec<Event> = st.events.iter().filter(|e| e.id > id).cloned().collect();
            let last = st.next_id - 1;
            if !out.is_empty() || wait.is_zero() {
                return (out, last);
            }
            let now = Instant::now();
            if now >= deadline || cancelled() {
                return (vec![], last);
            }
            st = wait_timeout(&self.events_cv, st, (deadline - now).min(Duration::from_millis(500)));
        }
    }

    pub fn system(&self) -> SystemInfo {
        actions::system_info(self)
    }

    fn accent_locked(st: &State, layer: usize) -> String {
        match st.cfg.layers.get(layer) {
            Some(l) if !l.color.is_empty() => l.color.clone(),
            _ => st.cfg.settings.accent.clone(),
        }
    }

    fn show_hud(&self, title: &str, sub: &str, seconds: f64, force: bool) {
        let (h, layers, cur, accent) = {
            let st = self.st();
            (st.cfg.settings.hud.clone(), st.cfg.layers.len(), st.layer, Engine::accent_locked(&st, st.layer))
        };
        if !h.enabled && !force {
            return;
        }
        let seconds = if seconds <= 0.0 { h.seconds } else { seconds };
        let (sub, dots) = match sub.strip_suffix("\0dots") {
            Some(s) => (s.to_string(), layers),
            None => (sub.to_string(), 0),
        };
        crate::hud::show(crate::hud::Options { title: title.into(), subtitle: sub, seconds, position: h.position, accent, dots, active: cur, sound: h.sound });
    }

    pub fn editing(&self) -> bool {
        self.st().edit_until.is_some_and(|t| Instant::now() < t)
    }

    pub fn set_editing(&self, on: bool) {
        let hold = Duration::from_secs(8);
        let was = {
            let mut st = self.st();
            let was = st.edit_until.is_some_and(|t| Instant::now() < t);
            if on {
                st.edit_until = Some(Instant::now() + hold);
                let me = self.me.clone();
                timer::after(hold + Duration::from_millis(50), move || {
                    if let Some(e) = me.upgrade() {
                        e.sync_mask_async();
                    }
                });
            } else {
                st.edit_until = None;
            }
            was
        };
        if was != on {
            self.sync_mask_async();
        }
    }

    pub fn capture_mode(&self) -> &'static str {
        match &self.src {
            Some(s) if s.can_switch() => s.active(),
            _ => BOARD,
        }
    }

    fn guard_capture(&self, mut c: Config) -> (Config, bool) {
        if c.settings.capture != SOFTWARE || config::has_capture_consent(&self.paths) {
            return (c, false);
        }
        c.settings.capture = BOARD.into();
        crate::log!("capture: the configuration asks for software capture but this installation never agreed to it; using the board");
        (c, true)
    }

    fn apply_capture(&self, kind: &str) {
        let Some(src) = self.src.as_ref().filter(|s| s.can_switch()) else { return };
        if src.active() == kind {
            return;
        }
        if let Err(e) = src.select(kind) {
            crate::log!("capture mode {kind}: {e}");
            let _ = src.select(BOARD);
            self.hud_async(self.tr("capture_failed", &[]), self.tr("capture_failed_sub", &[]), 6.0, true);
        }
    }

    pub fn set_capture(&self, kind: &str, consent: bool) -> Result<(), String> {
        let src = self.src.as_ref().filter(|s| s.can_switch()).ok_or("this build cannot switch the capture mode")?;
        if kind == SOFTWARE {
            if !consent {
                return Err("software capture needs explicit consent".into());
            }
            config::record_capture_consent(&self.paths)?;
        }
        let prev = src.active();
        if let Err(e) = src.select(kind) {
            let _ = src.select(prev);
            return Err(e);
        }
        if let Err(e) = self.update_config(|cfg| cfg.settings.capture = kind.into()) {
            let _ = src.select(prev);
            return Err(e);
        }
        self.sync_mask_async();
        self.sync_keys_async();
        Ok(())
    }

    pub fn set_key_history(&self, mode: &str) -> bool {
        let mut on = false;
        let res = self.update_config(|cfg| {
            on = match mode {
                "on" => true,
                "off" => false,
                _ => !cfg.settings.key_history,
            };
            cfg.settings.key_history = on;
        });
        if res.is_err() {
            return !on;
        }
        self.emit(Event { name: "key_history".into(), ok: Some(on), ..Event::kind("settings") });
        self.hud_fn(&self.tr(if on { "hist_on" } else { "hist_off" }, &[]), "", 1.6, true);
        on
    }

    pub fn toggle_caffeinate(&self) -> bool {
        let mut st = self.st();
        let lang = st.cfg.settings.language.clone();
        if let Some(stop) = st.stop_awake.take() {
            stop();
            drop(st);
            self.hud_async(tr_lang(&lang, "caff_off", &[]), String::new(), 0.0, true);
            return false;
        }
        match platform::get().keep_awake() {
            Ok(stop) => {
                st.stop_awake = Some(stop);
                drop(st);
                self.hud_async(tr_lang(&lang, "caff_on", &[]), String::new(), 0.0, true);
                true
            }
            Err(_) => false,
        }
    }

    pub fn start_timer(&self, minutes: f64, label: &str) {
        let mut sub = format!("{} min", actions::fmt_minutes(minutes));
        if !label.is_empty() {
            sub.push_str(" - ");
            sub.push_str(label);
        }
        self.hud_async(self.tr("timer_start", &[]), sub, 0.0, true);
        let (me, label) = (self.me.clone(), label.to_string());
        timer::after(Duration::from_secs_f64(minutes * 60.0), move || {
            let Some(e) = me.upgrade() else { return };
            e.hud_async(e.tr("timer_end", &[]), label, 3.0, true);
            if platform::get().name() == "darwin" {
                let mut c = platform::command("afplay");
                c.arg("/System/Library/Sounds/Glass.aiff");
                let _ = platform::detached(c);
            }
        });
    }

    pub fn hud(&self, title: &str, subtitle: &str, force: bool) {
        self.hud_fn(title, subtitle, 0.0, force);
    }

    pub fn test_action(&self, a: &Action) -> ActResult {
        let r = self.exec(a, true);
        self.emit(Event { action_type: a.kind.clone(), ok: Some(r.ok), ms: r.ms, output: r.output.clone(), ..Event::kind("test") });
        r
    }

    fn acquire_action_slot(&self) -> bool {
        let deadline = Instant::now() + Duration::from_secs(3);
        let mut n = lock(&self.actions.0);
        while *n >= MAX_ACTIONS {
            let now = Instant::now();
            if now >= deadline {
                return false;
            }
            n = wait_timeout(&self.actions.1, n, deadline - now);
        }
        *n += 1;
        true
    }

    fn release_action_slot(&self) {
        *lock(&self.actions.0) -= 1;
        self.actions.1.notify_one();
    }

    fn run_action(&self, u: u8, gesture: &'static str, a: Action, label: String) {
        if !self.acquire_action_slot() {
            self.hud_fn(&self.tr("error", &[]), "too many actions at once", 2.0, true);
            return;
        }
        let layer_name = {
            let st = self.st();
            st.cfg.layers.get(st.layer.min(st.cfg.layers.len().saturating_sub(1))).map(|l| l.name.clone()).unwrap_or_default()
        };
        let shows_output =
            ((a.kind == "shell" || a.kind == "ssh") && (a.show_output == Some(true) || (a.kind == "ssh" && a.show_output.is_none()))) || a.kind == "http";
        let res = crate::sync::catching(|| self.exec(&a, shows_output)).unwrap_or_else(|_| ActResult {
            ok: false,
            output: "the action failed unexpectedly".into(),
            ms: 0,
        });
        self.release_action_slot();
        self.emit(Event {
            key: hex_key(u),
            gesture: gesture.into(),
            layer_name: layer_name.clone(),
            label: label.clone(),
            action_type: a.kind.clone(),
            ok: Some(res.ok),
            ms: res.ms,
            output: res.output.clone(),
            ..Event::kind("exec")
        });
        self.count(&hex_key(u), &layer_name, &a.kind);
        let on_action = self.st().cfg.settings.hud.on_action;
        if !res.ok {
            let msg = if res.output.is_empty() { label } else { res.output };
            self.hud_fn(&self.tr("error", &[]), &clip(&msg, 60), 2.2, true);
        } else if shows_output {
            let first = clip(res.output.split('\n').next().unwrap_or(""), 60);
            self.hud_fn(&label, &first, 2.4, true);
        } else if on_action && a.kind != "layer" && a.kind != "hud" {
            self.hud_fn(&label, &clip(&res.output, 40), 0.0, false);
        }
    }

    pub fn wants_front_app(&self) -> bool {
        let st = self.st();
        st.cfg.settings.auto_layer && st.cfg.layers.iter().any(|l| !l.auto_apps.is_empty())
    }

    fn check_front_app(&self) {
        let asn = actions::front_id();
        {
            let mut st = self.st();
            if asn == st.asn {
                return;
            }
            st.asn = asn.clone();
        }
        let names = actions::front_names(&asn);
        let (target, announce) = {
            let mut st = self.st();
            st.front = names.clone();
            self.emit_locked(&mut st, Event { names: names.clone(), ..Event::kind("front") });
            let low: Vec<String> = names.iter().map(|n| n.to_lowercase()).collect();
            let found = st.cfg.layers.iter().position(|l| l.auto_apps.iter().any(|a| low.contains(&a.to_lowercase())));
            let announce = st.cfg.settings.hud.on_auto;
            let target = match found {
                Some(m) if m != st.layer => {
                    st.auto_active = true;
                    Some(m)
                }
                None if st.auto_active => {
                    st.auto_active = false;
                    (st.layer != st.manual_layer).then_some(st.manual_layer)
                }
                _ => None,
            };
            (target, announce)
        };
        if let Some(t) = target {
            self.goto_layer(&Value::from(t as i64), false, announce);
        }
    }

    pub fn goto(&self, target: &Value) {
        self.goto_layer(target, true, true);
    }

    pub fn toggle_layer(&self, target: &Value) {
        let back = {
            let mut st = self.st();
            let n = st.cfg.layers.len();
            let idx = target_index(target, st.layer, n);
            if st.layer == idx {
                let b = st.toggle_back;
                if b == idx || b >= n { 0 } else { b }
            } else {
                st.toggle_back = st.layer;
                idx
            }
        };
        self.goto_layer(&Value::from(back as i64), true, true);
    }

    pub(crate) fn goto_layer(&self, target: &Value, manual: bool, announce: bool) {
        let (idx, n, name) = {
            let mut st = self.st();
            let n = st.cfg.layers.len();
            if n == 0 {
                return;
            }
            let idx = target_index(target, st.layer, n);
            st.layer = idx;
            if manual {
                st.manual_layer = idx;
                st.auto_active = false;
            }
            let name = st.cfg.layers[idx].name.clone();
            self.emit_locked(&mut st, Event { layer: Some(idx as i64), name: name.clone(), auto: !manual, ..Event::kind("layer") });
            (idx, n, name)
        };
        self.set_layer_async(idx);
        self.sync_mask_async();
        if announce {
            self.hud_fn(&name, &format!("{}\0dots", self.tr("layer_of", &[(idx + 1).to_string(), n.to_string()])), 0.0, false);
        }
    }

    pub fn layer(&self) -> usize {
        self.st().layer
    }

    pub fn layer_name(&self) -> String {
        let st = self.st();
        st.cfg.layers.get(st.layer).map(|l| l.name.clone()).unwrap_or_default()
    }

    pub fn settings(&self) -> Settings {
        self.st().cfg.settings.clone()
    }

    fn spawn_action(&self, u: u8, gesture: &'static str, a: Action, label: String) {
        let me = self.me.clone();
        crate::sync::spawn("action", move || {
            if let Some(e) = me.upgrade() {
                e.run_action(u, gesture, a, label);
            }
        });
    }
}

impl Env for Engine {
    fn settings(&self) -> Settings {
        Engine::settings(self)
    }
    fn hud(&self, title: &str, subtitle: &str, force: bool) {
        Engine::hud(self, title, subtitle, force);
    }
    fn goto(&self, target: &Value) {
        Engine::goto(self, target);
    }
    fn toggle_layer(&self, target: &Value) {
        Engine::toggle_layer(self, target);
    }
    fn toggle_caffeinate(&self) -> bool {
        Engine::toggle_caffeinate(self)
    }
    fn start_timer(&self, minutes: f64, label: &str) {
        Engine::start_timer(self, minutes, label);
    }
    fn hardware(&self) -> Option<Arc<dyn Source>> {
        self.src.clone()
    }
    fn layer(&self) -> usize {
        Engine::layer(self)
    }
    fn layer_name(&self) -> String {
        Engine::layer_name(self)
    }
    fn set_key_history(&self, mode: &str) -> bool {
        Engine::set_key_history(self, mode)
    }
}

#[cfg(test)]
mod tests;
