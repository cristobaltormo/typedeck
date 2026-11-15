use std::sync::{Arc, Mutex, RwLock, Weak};
use std::time::Duration;

use super::{BOARD, HandlerSlot, Handlers, Info, RawReply, SOFTWARE, Source, Status, Sys};
use crate::hid::Stroke;
use crate::sync::lock;

pub struct Selector {
    me: Weak<Selector>,
    board: Arc<dyn Source>,
    soft: Arc<dyn Source>,
    select_lock: Mutex<()>,
    active: RwLock<&'static str>,
    handlers: Arc<HandlerSlot>,
}

impl Selector {
    pub fn new(board: Arc<dyn Source>, soft: Arc<dyn Source>) -> Arc<Selector> {
        Arc::new_cyclic(|me| Selector {
            me: me.clone(),
            board,
            soft,
            select_lock: Mutex::new(()),
            active: RwLock::new(BOARD),
            handlers: Arc::new(HandlerSlot::default()),
        })
    }

    fn current(&self) -> &Arc<dyn Source> {
        if self.active_kind() == SOFTWARE { &self.soft } else { &self.board }
    }

    fn active_kind(&self) -> &'static str {
        *self.active.read().unwrap_or_else(|e| e.into_inner())
    }

    fn swap(&self, kind: &'static str) -> &'static str {
        let mut a = self.active.write().unwrap_or_else(|e| e.into_inner());
        std::mem::replace(&mut *a, kind)
    }

    fn handlers_for(&self, kind: &'static str) -> Handlers {
        let me = self.me.clone();
        let gate = move || me.upgrade().filter(|s| s.active_kind() == kind).map(|s| s.handlers.get());
        let g1 = gate.clone();
        let g2 = gate.clone();
        let g3 = gate.clone();
        let g4 = gate;
        Handlers {
            on_event: Some(Arc::new(move |ev| {
                if let Some(f) = g1().and_then(|h| h.on_event) {
                    f(ev);
                }
            })),
            mask: Some(Arc::new(move || g2().and_then(|h| h.mask).map(|f| f()).unwrap_or([0; 32]))),
            on_connected: Some(Arc::new(move |i: &Info| {
                if let Some(f) = g3().and_then(|h| h.on_connected) {
                    f(i);
                }
            })),
            on_disconnect: Some(Arc::new(move || {
                if let Some(f) = g4().and_then(|h| h.on_disconnect) {
                    f();
                }
            })),
        }
    }

    fn back_to_board(&self) {
        self.swap(BOARD);
        self.soft.disable();
        self.board.sync_mask(true);
        self.notify_current();
    }

    fn notify_current(&self) {
        let st = self.board.status();
        match (st.connected, st.info) {
            (true, Some(info)) => self.handlers.connected(&info),
            (false, _) => self.handlers.disconnected(),
            _ => {}
        }
    }
}

impl Source for Selector {
    fn kind(&self) -> &'static str {
        self.active_kind()
    }

    fn bind(&self, h: Handlers) {
        self.handlers.set(h);
        self.board.bind(self.handlers_for(BOARD));
        self.soft.bind(self.handlers_for(SOFTWARE));
    }

    fn run(&self) {
        let soft = self.soft.clone();
        crate::sync::spawn("software-capture", move || soft.run());
        self.board.run();
    }

    fn close(&self) {
        self.soft.close();
        self.board.close();
    }

    fn status(&self) -> Status {
        self.current().status()
    }

    fn connected(&self) -> bool {
        self.current().connected()
    }

    fn error(&self) -> String {
        self.current().error()
    }

    fn sync_mask(&self, force: bool) {
        self.current().sync_mask(force);
    }

    fn keys(&self, on: bool) -> Result<(), String> {
        self.current().keys(on)
    }

    fn pause(&self) {
        self.board.pause();
        self.soft.pause();
    }

    fn resume(&self) {
        self.board.resume();
        self.soft.resume();
    }

    fn tap_held(&self, s: Stroke, hold_ms: u32) -> Result<(), String> {
        self.current().tap_held(s, hold_ms)
    }

    fn consumer(&self, usage: u16) -> Result<(), String> {
        self.current().consumer(usage)
    }

    fn command(&self, name: &str, arg: i64) -> Result<Vec<String>, String> {
        self.current().command(name, arg)
    }

    fn raw(&self, cmd: &str, multi: bool, timeout: Duration) -> RawReply {
        self.current().raw(cmd, multi, timeout)
    }

    fn sys(&self) -> Result<Sys, String> {
        self.current().sys()
    }

    fn set_layer(&self, index: usize) {
        self.current().set_layer(index);
    }

    fn warnings(&self) -> Vec<String> {
        self.current().warnings()
    }

    fn type_text(&self, text: &str) -> Result<(), String> {
        self.current().type_text(text)
    }

    fn select(&self, kind: &str) -> Result<(), String> {
        let kind = match kind {
            BOARD => BOARD,
            SOFTWARE => SOFTWARE,
            _ => return Err("unknown capture mode".into()),
        };
        let _g = lock(&self.select_lock);
        if self.swap(kind) == kind {
            return Ok(());
        }
        if kind == BOARD {
            self.back_to_board();
            return Ok(());
        }
        self.board.sync_mask(true);
        let _ = self.board.keys(false);
        if let Err(e) = self.soft.enable() {
            self.back_to_board();
            return Err(e);
        }
        Ok(())
    }

    fn active(&self) -> &'static str {
        self.active_kind()
    }

    fn can_switch(&self) -> bool {
        true
    }
}
