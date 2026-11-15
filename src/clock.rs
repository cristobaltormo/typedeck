use std::time::{Duration, SystemTime, UNIX_EPOCH};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Local {
    pub year: i32,
    pub month: u32,
    pub day: u32,
    pub hour: u32,
    pub minute: u32,
    pub second: u32,
    pub offset_secs: i64,
}

pub fn unix_secs_f64(t: SystemTime) -> f64 {
    t.duration_since(UNIX_EPOCH).map(|d| d.as_secs_f64()).unwrap_or(0.0)
}

pub fn now_f64() -> f64 {
    unix_secs_f64(SystemTime::now())
}

pub fn unix_millis(t: SystemTime) -> i64 {
    t.duration_since(UNIX_EPOCH).map(|d| d.as_millis() as i64).unwrap_or(0)
}

pub fn now_millis() -> i64 {
    unix_millis(SystemTime::now())
}

pub fn unix_secs(t: SystemTime) -> i64 {
    t.duration_since(UNIX_EPOCH).map(|d| d.as_secs() as i64).unwrap_or(0)
}

pub fn from_unix_millis(ms: i64) -> SystemTime {
    UNIX_EPOCH + Duration::from_millis(ms.max(0) as u64)
}

pub fn civil_from_days(z: i64) -> (i32, u32, u32) {
    let z = z + 719468;
    let era = if z >= 0 { z } else { z - 146096 } / 146097;
    let doe = (z - era * 146097) as u64;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let y = yoe as i64 + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = (doy - (153 * mp + 2) / 5 + 1) as u32;
    let m = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
    ((if m <= 2 { y + 1 } else { y }) as i32, m, d)
}

pub fn utc(secs: i64) -> Local {
    let days = secs.div_euclid(86400);
    let rem = secs.rem_euclid(86400);
    let (year, month, day) = civil_from_days(days);
    Local { year, month, day, hour: (rem / 3600) as u32, minute: (rem % 3600 / 60) as u32, second: (rem % 60) as u32, offset_secs: 0 }
}

#[cfg(unix)]
pub fn offset_at(secs: i64) -> i64 {
    let t = secs as _;
    let mut tm: libc::tm = unsafe { std::mem::zeroed() };
    let ok = unsafe { !libc::localtime_r(&t, &mut tm).is_null() };
    if ok { tm.tm_gmtoff as i64 } else { 0 }
}

#[cfg(windows)]
pub fn offset_at(secs: i64) -> i64 {
    use windows_sys::Win32::Foundation::{FILETIME, SYSTEMTIME};
    use windows_sys::Win32::System::Time::{FileTimeToSystemTime, SystemTimeToTzSpecificLocalTime};
    let ticks = (secs + 11_644_473_600) as u64 * 10_000_000;
    let ft = FILETIME { dwLowDateTime: ticks as u32, dwHighDateTime: (ticks >> 32) as u32 };
    let mut st: SYSTEMTIME = unsafe { std::mem::zeroed() };
    let mut local: SYSTEMTIME = unsafe { std::mem::zeroed() };
    unsafe {
        if FileTimeToSystemTime(&ft, &mut st) == 0 || SystemTimeToTzSpecificLocalTime(std::ptr::null(), &st, &mut local) == 0 {
            return 0;
        }
    }
    let to_secs = |s: &SYSTEMTIME| {
        let days = days_from_civil(i32::from(s.wYear), u32::from(s.wMonth), u32::from(s.wDay));
        days * 86400 + i64::from(s.wHour) * 3600 + i64::from(s.wMinute) * 60 + i64::from(s.wSecond)
    };
    to_secs(&local) - to_secs(&st)
}

#[cfg(not(any(unix, windows)))]
pub fn offset_at(_secs: i64) -> i64 {
    0
}

pub fn days_from_civil(y: i32, m: u32, d: u32) -> i64 {
    let y = i64::from(y) - i64::from(m <= 2);
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = (y - era * 400) as u64;
    let mp = u64::from(if m > 2 { m - 3 } else { m + 9 });
    let doy = (153 * mp + 2) / 5 + u64::from(d) - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146097 + doe as i64 - 719468
}

pub fn local(t: SystemTime) -> Local {
    let secs = unix_secs(t);
    let off = offset_at(secs);
    let mut l = utc(secs + off);
    l.offset_secs = off;
    l
}

pub fn now_local() -> Local {
    local(SystemTime::now())
}

impl Local {
    pub fn date(&self) -> String {
        format!("{:04}-{:02}-{:02}", self.year, self.month, self.day)
    }

    pub fn hm(&self) -> String {
        format!("{:02}:{:02}", self.hour, self.minute)
    }

    pub fn hms(&self) -> String {
        format!("{:02}:{:02}:{:02}", self.hour, self.minute, self.second)
    }

    pub fn stamp(&self) -> String {
        format!("{:04}{:02}{:02}-{:02}{:02}{:02}", self.year, self.month, self.day, self.hour, self.minute, self.second)
    }

    pub fn rfc3339_utc(&self) -> String {
        format!("{}T{}Z", self.date(), self.hms())
    }

    pub fn minutes_of_day(&self) -> u32 {
        self.hour * 60 + self.minute
    }
}

pub fn local_midnight_millis(now: SystemTime) -> i64 {
    midnight_with(unix_secs(now), offset_at) * 1000
}

fn midnight_with(secs: i64, offset: impl Fn(i64) -> i64) -> i64 {
    let l = utc(secs + offset(secs));
    let wall = days_from_civil(l.year, l.month, l.day) * 86400;
    let mut t = wall - offset(secs);
    for _ in 0..3 {
        let next = wall - offset(t);
        if next == t {
            break;
        }
        t = next;
    }
    if t > secs { secs - (secs + offset(secs)).rem_euclid(86400) } else { t }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn civil_round_trip() {
        for days in [-1000i64, 0, 1, 365, 10957, 20000, 20407] {
            let (y, m, d) = civil_from_days(days);
            assert_eq!(days_from_civil(y, m, d), days);
        }
        assert_eq!(civil_from_days(0), (1970, 1, 1));
        assert_eq!(civil_from_days(20407), (2025, 11, 15));
    }

    #[test]
    fn midnight_survives_a_dst_change_earlier_that_day() {
        let change = 1_774_746_000;
        let off = |t: i64| if t >= change { 7200 } else { 3600 };
        let midnight = days_from_civil(2026, 3, 29) * 86400 - 3600;
        assert_eq!(midnight_with(change + 3600 * 10, off), midnight);
        assert_eq!(midnight_with(midnight, off), midnight);
        assert_eq!(midnight_with(midnight - 1, off), midnight - 86400);
        let back = 1_792_890_000;
        let off2 = |t: i64| if t >= back { 3600 } else { 7200 };
        let midnight2 = days_from_civil(2026, 10, 25) * 86400 - 7200;
        assert_eq!(midnight_with(back + 3600 * 20, off2), midnight2);
        assert_eq!(midnight_with(1_000_000, |_| 0), 1_000_000 - 1_000_000 % 86400);
    }

    #[test]
    fn formats() {
        let t = utc(1_763_164_800 + 3661);
        assert_eq!(t.date(), "2025-11-15");
        assert_eq!(t.hms(), "01:01:01");
        assert_eq!(t.stamp(), "20251115-010101");
        assert_eq!(t.rfc3339_utc(), "2025-11-15T01:01:01Z");
    }
}
