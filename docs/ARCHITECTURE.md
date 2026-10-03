# Architecture

```
 USB keyboard ──► USB Host Shield (MAX3421E) ──► Arduino Leonardo ──► computer (USB)
                                                      │  ▲
                                          CDC serial  │  │  events / commands
                                                      ▼  │
                                               typedeck (Go)  ◄── editor (127.0.0.1:7788)
                                               actions, layers, gestures, stats
```

## Who does what

| Part | Language | Job | Why there |
|---|---|---|---|
| `firmware/` | C++ (Arduino) | Reads the keyboard, forwards it to the computer, captures keys that have a macro, injects keystrokes | This is the part that has to be instant: about 130 µs per report, never through the computer |
| `cmd/`, `internal/` | Go | Gestures, layers, actions, API, statistics | One static binary, no dependencies, about 11 MB resident, 0 % CPU when idle |
| `web/ui/` | HTML, CSS and JS, no dependencies | The editor | About 100 KB on first load, no framework, nothing fetched from the network |
| `macos/hud/` | Swift | Floating popup on macOS | A native window that only exists while it is shown |

Typing normally never depends on Typedeck: the board forwards the keyboard on its own, even when the program is not running.

## Packages

| Package | Responsibility |
|---|---|
| `internal/board` | Serial link to the board: detection by `WHO`, heartbeat, capture mask, events, `INFO`, `SYS`, `BUS`. Native serial ports for macOS, Linux and Windows |
| `internal/engine` | Gestures, layers, per-application layers, debounce, statistics |
| `internal/actions` | Runs the actions. Sends keys through the board and delegates everything OS-specific to `platform` |
| `internal/platform` | One implementation per OS (macOS, Linux, Windows): launch and quit apps, front window, software keys, clipboard, sleep, lock, notifications, floating popup, auto-start service. macOS key compensation lives here |
| `internal/obs` | A minimal RFC 6455 WebSocket client and the obs-websocket 5 requests, no dependencies |
| `internal/layouts` | Physical layouts (100, 80, 75, 65, 60 %, ISO and ANSI, numpad) and the algorithm that guesses which one a keyboard has from the keys observed |
| `internal/kbdb` | Brand and model: the name the keyboard reports, vendor by VID, verified models, form factor from name keywords |
| `internal/setup` | Setup summary, layout choice and compatibility checks (`/api/setup`) |
| `internal/packs` | Gallery packs, laid out on the keys the keyboard really has, adapted per OS |
| `internal/config` | Schema v3, migration from v1 and v2, validation, backups and recovery |
| `internal/hid` | USB key tables, text layouts (US, Spanish for macOS, Linux and Windows) and a report descriptor parser |
| `internal/server` | Loopback HTTP server: static editor and JSON API, a WebSocket for live events, the editing flag and macro file review |
| `internal/keylog` | The optional typing history: key, time and duration in a local file, retention, summary and the text rebuilt from the presses |

## How the layout is decided

1. What the user picked for that keyboard (VID:PID).
2. A verified model in `internal/kbdb/db.json` (needs exact vendor and product: generic controller PIDs are reused).
3. The form factor in the keyboard name ("TKL", "60 %", "75 %"...).
4. ISO or ANSI from the keyboard type the OS records, when it records one.

The key assistant watches what you press and scores the 11 layouts with the harmonic mean of precision and coverage. When ISO
and ANSI cannot be told apart it says which keys to press (`<` and `ç`). Many ISO controllers send `0x31` for the key next to
Enter instead of `0x32`; that is accounted for.

## Capture and fail-open

1. Typedeck computes the keys that have a macro on the active layer (plus global ones) and sends `MASK <256 bits>`.
2. The board removes those keys from the report it forwards and sends `D <usage> <mods>` and `U <usage>` instead.
3. Typedeck sends `HB` every second. With no heartbeat for 5 s the board stops capturing and every key types its own
   character again. A crashed program can never leave the keyboard unusable.
4. A layer change sends a new mask (one 66-byte line).
5. A key that only has "hold" or "double" forwards the original key on a short press, so it never goes dead.

## Gestures

Tap runs on key down when there is no hold or double; with hold, on release before the threshold; with double, after the double
tap window. A `layer` action with `momentary` activates the layer while the key is held, and also the moment another captured key goes down; for that
the capture mask includes the keys of the layers a held key can reach, and a captured key with no action in the layer in force is typed back.

## The live connection

The editor opens a WebSocket (`/api/ws`) that carries events, key presses for the on-screen keyboard and one message back: whether
a text field of the editor has focus. While it does, the program releases the capture mask, so typing the letter of a key that has a
macro types it instead of running the macro; the mask comes back when the field loses focus, when the window does, or when the
connection closes. The board reports every other key (`KEYS 1`) only while an editor is connected or the history is on.

## Typing through the board

`KEY <mods> <usage>` taps a key and `CONS <usage>` a media key. Typedeck turns text into keystrokes with the layout of the
computer (US, and Spanish for macOS, Linux or Windows) and the OS layer adapts them:

- **macOS:** per-device modifier remapping (`com.apple.keyboard.modifiermapping.*`) and the keyboard type (`com.apple.keyboardtype`)
  that crosses the `<` and `º` keys when a board registered as ANSI carries an ISO layout.
- **Windows and Linux:** Spanish text uses Alt Gr (right Alt); on Windows `Alt Gr+4` is a dead key, so `~` takes a space after it.
- Layouts where letters move (AZERTY, Dvorak...) never type through the board: text falls back to the clipboard.

## Platform layer

`platform.Current` is chosen at build time (`darwin.go`, `linux.go`, `windows.go`). Everything the rest of the program needs
from the OS goes through that interface, so adding a system means implementing it once. Linux discovers applications from
`.desktop` files and the focused window from Hyprland, Sway or X11 (xprop, then xdotool); Windows uses user32 and kernel32
directly (no PowerShell), a scheduled task for auto-start, and its own popup window instead of system notifications.

## Security

See [SECURITY.md](SECURITY.md).
