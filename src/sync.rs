use std::cell::Cell;
use std::sync::{Condvar, Mutex, MutexGuard};
use std::time::{Duration, Instant};

pub fn lock<T>(m: &Mutex<T>) -> MutexGuard<'_, T> {
    m.lock().unwrap_or_else(|e| e.into_inner())
}

pub fn wait_timeout<'a, T>(cv: &Condvar, g: MutexGuard<'a, T>, d: Duration) -> MutexGuard<'a, T> {
    match cv.wait_timeout(g, d) {
        Ok((g, _)) => g,
        Err(e) => e.into_inner().0,
    }
}

#[derive(Default)]
pub struct Flag {
    set: Mutex<bool>,
    cv: Condvar,
}

impl Flag {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn set(&self) {
        *lock(&self.set) = true;
        self.cv.notify_all();
    }

    pub fn is_set(&self) -> bool {
        *lock(&self.set)
    }

    pub fn wait(&self, d: Duration) -> bool {
        let deadline = Instant::now() + d;
        let mut g = lock(&self.set);
        while !*g {
            let now = Instant::now();
            if now >= deadline {
                return false;
            }
            g = wait_timeout(&self.cv, g, deadline - now);
        }
        true
    }
}

#[derive(Default)]
pub struct Signal {
    pending: Mutex<bool>,
    cv: Condvar,
}

impl Signal {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn notify(&self) {
        *lock(&self.pending) = true;
        self.cv.notify_all();
    }

    pub fn wait(&self, d: Duration) -> bool {
        let deadline = Instant::now() + d;
        let mut g = lock(&self.pending);
        while !*g {
            let now = Instant::now();
            if now >= deadline {
                return false;
            }
            g = wait_timeout(&self.cv, g, deadline - now);
        }
        *g = false;
        true
    }

    pub fn take(&self) -> bool {
        std::mem::take(&mut *lock(&self.pending))
    }
}

pub fn spawn<F>(name: &str, f: F) -> Option<std::thread::JoinHandle<()>>
where
    F: FnOnce() + Send + 'static,
{
    std::thread::Builder::new().name(name.to_string()).stack_size(512 * 1024).spawn(f).ok()
}

pub fn wait_until(timeout: Duration, mut cond: impl FnMut() -> bool) -> bool {
    let deadline = Instant::now() + timeout;
    loop {
        if cond() {
            return true;
        }
        if Instant::now() >= deadline {
            return false;
        }
        std::thread::sleep(Duration::from_millis(10));
    }
}

thread_local! {
    static CATCHING: Cell<u32> = const { Cell::new(0) };
}

pub fn catching<T>(f: impl FnOnce() -> T) -> std::thread::Result<T> {
    CATCHING.with(|c| c.set(c.get() + 1));
    let r = std::panic::catch_unwind(std::panic::AssertUnwindSafe(f));
    CATCHING.with(|c| c.set(c.get() - 1));
    r
}

pub fn inside_catch() -> bool {
    CATCHING.with(|c| c.get() > 0)
}

pub const PANIC_EXIT: i32 = 70;

pub fn install_panic_policy() {
    std::panic::set_hook(Box::new(|info| {
        crate::log!("internal error: {info}");
        if !inside_catch() {
            std::process::exit(PANIC_EXIT);
        }
    }));
}

#[cfg(debug_assertions)]
pub fn fault_point(name: &str) {
    if std::env::var("TYPEDECK_FAULT").is_ok_and(|v| v == name) {
        panic!("injected fault in {name}");
    }
}

#[cfg(not(debug_assertions))]
pub fn fault_point(_name: &str) {}

type Job = Box<dyn FnOnce() + Send>;

pub fn background<F: FnOnce() + Send + 'static>(f: F) {
    static Q: std::sync::OnceLock<std::sync::Arc<crate::queue::Queue<Job>>> = std::sync::OnceLock::new();
    let q = Q.get_or_init(|| {
        let q = std::sync::Arc::new(crate::queue::Queue::new(usize::MAX));
        let q2 = q.clone();
        spawn("background", move || {
            loop {
                if let Some(job) = q2.pop(Duration::from_secs(3600))
                    && catching(job).is_err()
                {
                    crate::log!("a background job panicked");
                }
            }
        });
        q
    });
    q.push(Box::new(f));
}
