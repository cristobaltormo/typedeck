use std::collections::VecDeque;
use std::sync::{Condvar, Mutex};
use std::time::Duration;

use crate::sync::{lock, wait_timeout};

pub struct Queue<T> {
    items: Mutex<VecDeque<T>>,
    cv: Condvar,
    cap: usize,
}

impl<T> Queue<T> {
    pub fn new(cap: usize) -> Self {
        Self { items: Mutex::new(VecDeque::new()), cv: Condvar::new(), cap }
    }

    pub fn try_push(&self, v: T) -> bool {
        let mut q = lock(&self.items);
        if q.len() >= self.cap {
            return false;
        }
        q.push_back(v);
        drop(q);
        self.cv.notify_one();
        true
    }

    pub fn push(&self, v: T) {
        lock(&self.items).push_back(v);
        self.cv.notify_one();
    }

    pub fn push_dropping_oldest(&self, v: T) {
        let mut q = lock(&self.items);
        if q.len() >= self.cap {
            q.pop_front();
        }
        q.push_back(v);
        drop(q);
        self.cv.notify_one();
    }

    pub fn pop(&self, timeout: Duration) -> Option<T> {
        let mut q = lock(&self.items);
        if let Some(v) = q.pop_front() {
            return Some(v);
        }
        q = wait_timeout(&self.cv, q, timeout);
        q.pop_front()
    }

    pub fn try_pop(&self) -> Option<T> {
        lock(&self.items).pop_front()
    }

    pub fn clear(&self) {
        lock(&self.items).clear();
    }

    pub fn len(&self) -> usize {
        lock(&self.items).len()
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    pub fn wake(&self) {
        self.cv.notify_all();
    }
}
