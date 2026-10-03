const MODS = new Set(["Shift", "Control", "Alt", "Meta", "AltGraph", "CapsLock"]);
const SPECIAL = {
  Enter: "enter", NumpadEnter: "enter", Escape: "esc", Tab: "tab", Space: "space", Backspace: "backspace", Delete: "forwarddelete",
  ArrowUp: "up", ArrowDown: "down", ArrowLeft: "left", ArrowRight: "right", Home: "home", End: "end", PageUp: "pageup", PageDown: "pagedown",
  Insert: "insert", PrintScreen: "printscreen", Pause: "pause", Minus: "minus", Equal: "equals", Comma: "comma", Period: "period", Slash: "slash",
};

export const MIN_WAIT = 250;
export const TEXT_GAP = 1200;

function codeName(e) {
  if (/^Key[A-Z]$/.test(e.code)) return e.code.slice(3).toLowerCase();
  if (/^Digit\d$/.test(e.code)) return e.code.slice(5);
  if (/^F\d{1,2}$/.test(e.code)) return e.code.toLowerCase();
  if (SPECIAL[e.code]) return SPECIAL[e.code];
  return e.key && e.key.length === 1 ? e.key.toLowerCase() : null;
}

export function modPrefix(e) {
  return [e.ctrlKey && "ctrl", e.altKey && "alt", e.shiftKey && "shift", e.metaKey && "cmd"].filter(Boolean);
}

export function createRecorder({ timings = true, max = 100 } = {}) {
  const steps = [];
  let last = null, full = false;

  const roundMs = (ms) => Math.min(60000, Math.max(50, Math.round(ms / 50) * 50));
  const push = (s) => { if (steps.length >= max) { full = true; return false; } steps.push(s); return true; };

  function gapBefore(now, glue) {
    if (last === null) return;
    const gap = now - last;
    const limit = glue ? TEXT_GAP : MIN_WAIT;
    if (timings && gap >= limit && steps.length) push({ type: "wait", ms: roundMs(gap) });
  }

  function key(e, now = e.timeStamp) {
    if (MODS.has(e.key) || e.key === "Dead" || e.key === "Process" || e.repeat) return false;
    const plain = !e.ctrlKey && !e.altKey && !e.metaKey;
    const tail = steps[steps.length - 1];
    if (plain && e.key.length === 1) {
      const glue = tail && tail.type === "text";
      gapBefore(now, glue);
      const cur = steps[steps.length - 1];
      if (cur && cur.type === "text") cur.text += e.key;
      else push({ type: "text", text: e.key });
    } else {
      const name = codeName(e);
      if (!name) return false;
      gapBefore(now, false);
      push({ type: "hotkey", keys: [...modPrefix(e), name].join("+") });
    }
    last = now;
    return true;
  }

  return { key, steps, get full() { return full; }, setTimings(v) { timings = v; } };
}

export function describeStep(s) {
  if (s.type === "text") return s.text;
  if (s.type === "wait") return `${s.ms} ms`;
  return s.keys;
}
