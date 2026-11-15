use std::cmp::Ordering as CmpOrdering;
use std::collections::BinaryHeap;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Condvar, Mutex, OnceLock};
use std::time::{Duration, Instant};

use crate::sync::{lock, wait_timeout};

type Job = Box<dyn FnOnce() + Send>;

struct Entry {
    at: Instant,
    seq: u64,
    cancelled: Arc<AtomicBool>,
    job: Job,
}

impl PartialEq for Entry {
    fn eq(&self, other: &Self) -> bool {
        self.at == other.at && self.seq == other.seq
    }
}

impl Eq for Entry {}

impl PartialOrd for Entry {
    fn partial_cmp(&self, other: &Self) -> Option<CmpOrdering> {
        Some(self.cmp(other))
    }
}

impl Ord for Entry {
    fn cmp(&self, other: &Self) -> CmpOrdering {
        other.at.cmp(&self.at).then_with(|| other.seq.cmp(&self.seq))
    }
}

struct Wheel {
    heap: Mutex<BinaryHeap<Entry>>,
    cv: Condvar,
    seq: AtomicU64,
}

fn wheel() -> &'static Wheel {
    static W: OnceLock<&'static Wheel> = OnceLock::new();
    W.get_or_init(|| {
        let w: &'static Wheel = Box::leak(Box::new(Wheel { heap: Mutex::new(BinaryHeap::new()), cv: Condvar::new(), seq: AtomicU64::new(0) }));
        let _ = std::thread::Builder::new().name("timers".into()).stack_size(512 * 1024).spawn(move || run(w));
        w
    })
}

fn run(w: &'static Wheel) {
    let mut heap = lock(&w.heap);
    loop {
        let now = Instant::now();
        match heap.peek() {
            None => heap = wait_timeout(&w.cv, heap, Duration::from_secs(3600)),
            Some(e) if e.at > now => {
                let d = e.at - now;
                heap = wait_timeout(&w.cv, heap, d);
            }
            Some(_) => {
                let Some(e) = heap.pop() else { continue };
                drop(heap);
                if !e.cancelled.load(Ordering::SeqCst) {
                    let job = e.job;
                    if crate::sync::catching(job).is_err() {
                        crate::log!("a timer callback panicked");
                    }
                }
                heap = lock(&w.heap);
            }
        }
    }
}

#[derive(Clone, Debug, Default)]
pub struct Timer {
    cancelled: Arc<AtomicBool>,
}

impl Timer {
    pub fn cancel(&self) {
        self.cancelled.store(true, Ordering::SeqCst);
    }

    pub fn is_cancelled(&self) -> bool {
        self.cancelled.load(Ordering::SeqCst)
    }
}

pub fn after<F: FnOnce() + Send + 'static>(d: Duration, f: F) -> Timer {
    let w = wheel();
    let cancelled = Arc::new(AtomicBool::new(false));
    let seq = w.seq.fetch_add(1, Ordering::Relaxed);
    lock(&w.heap).push(Entry { at: Instant::now() + d, seq, cancelled: cancelled.clone(), job: Box::new(f) });
    w.cv.notify_one();
    Timer { cancelled }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::AtomicUsize;

    #[test]
    fn fires_in_order_and_cancels() {
        let order = Arc::new(Mutex::new(Vec::new()));
        let o1 = order.clone();
        let o2 = order.clone();
        let o3 = order.clone();
        after(Duration::from_millis(60), move || lock(&o1).push(2));
        after(Duration::from_millis(20), move || lock(&o2).push(1));
        let t = after(Duration::from_millis(40), move || lock(&o3).push(9));
        t.cancel();
        std::thread::sleep(Duration::from_millis(150));
        assert_eq!(*lock(&order), vec![1, 2]);
    }

    #[test]
    fn a_panicking_callback_does_not_stop_the_wheel() {
        let n = Arc::new(AtomicUsize::new(0));
        after(Duration::from_millis(5), || panic!("boom"));
        let n2 = n.clone();
        after(Duration::from_millis(30), move || {
            n2.fetch_add(1, Ordering::SeqCst);
        });
        std::thread::sleep(Duration::from_millis(120));
        assert_eq!(n.load(Ordering::SeqCst), 1);
    }
}
