use std::path::PathBuf;
use std::sync::OnceLock;

use crate::platform::{self, PopupOptions};

pub fn binary() -> Option<&'static PathBuf> {
    static B: OnceLock<Option<PathBuf>> = OnceLock::new();
    B.get_or_init(|| std::env::current_exe().ok().and_then(|e| e.parent().map(|d| d.join("typedeck-hud")))).as_ref()
}

pub struct Options {
    pub title: String,
    pub subtitle: String,
    pub seconds: f64,
    pub position: String,
    pub accent: String,
    pub dots: usize,
    pub active: usize,
    pub sound: bool,
}

pub fn show(o: Options) {
    let p = platform::get();
    if let Some(bin) = binary().filter(|b| b.is_file()) {
        let mut c = platform::command(&bin.display().to_string());
        c.args([
            "--title",
            &o.title,
            "--sub",
            &o.subtitle,
            "--sec",
            &format!("{:.1}", o.seconds),
            "--pos",
            &o.position,
            "--accent",
            &o.accent,
            "--dots",
            &o.dots.to_string(),
            "--active",
            &o.active.to_string(),
        ]);
        let _ = platform::detached(c);
        if o.sound && !o.title.trim().is_empty() {
            p.notify("", "", true);
        }
        return;
    }
    let popup = PopupOptions {
        title: o.title.clone(),
        subtitle: o.subtitle.clone(),
        seconds: o.seconds,
        position: o.position,
        accent: o.accent,
        dots: o.dots,
        active: o.active,
    };
    if p.popup(&popup) {
        if o.sound && !o.title.trim().is_empty() {
            p.notify("", "", true);
        }
        return;
    }
    let body = if o.subtitle.is_empty() { o.title.clone() } else { o.subtitle.clone() };
    p.notify(&o.title, &body, o.sound);
}
