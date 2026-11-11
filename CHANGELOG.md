# Changelog

## 0.9.0 - 2026-11-11

### Windows
- A second copy no longer starts: it opens the editor of the running one. Opening `typedeck.exe` by hand opens the editor and puts an
  icon in the notification area (Open editor, status, Restart, Quit); a fatal error at start-up shows a message box.
- Orderly stop: `taskkill` without `/F`, signing out and shutting down release the board and save the statistics. `uninstall` stops
  the program that way (forcing it only if it does not answer in 8 seconds) and no longer depends on the language of Windows.
- Locking the screen or suspending releases the captured keys and they are captured again afterwards. Both go through one path (the
  tray window calls `Pause` and `Resume`; `Resume` also reinstalls the hook), and the second hidden window of the hook is gone.
- Software capture: if the hook cannot be reinstalled the session is declared dead and restarted instead of staying "alive" without a
  hook; a `Start` that times out cancels its thread, so no orphan hook is left; `AC Search` is a media key like on Linux. Serial port: concurrent readers are serialised and a write after `Close` fails
  cleanly.
- `typedeck install` makes the scheduled task from XML: no time limit, runs on battery, never starts two copies, restarts after a
  failure, for your own account and with limited rights; without permission to create tasks it uses the `Run` key.
- The release ships `typedeck-cli.exe` for terminals (output and exit code of `doctor`, `install`, `uninstall`), and the executables
  carry a manifest, version information and an icon.
- Serial port: overlapped I/O, so a pending read never delays a write; a short write is an error; only COM ports of the supported
  boards (USB vendor and product id) are opened.
- The typing history moves to `%LOCALAPPDATA%` and the log rotates at 1 MB instead of being truncated.
- Applications: opening one that is already running brings its window to the front (restoring it only if minimised) instead of
  starting another; Microsoft Store apps are listed and opened; closing and "is it in front" match the program, not the window title.
- Text: typed text no longer erases an image or files on the clipboard, is kept out of the clipboard history, and the clipboard is
  restored when the target program has read the text. Scan codes and extended keys are complete, the numeric keypad and international
  keys map, Alt Gr is kept and Ctrl or Alt from the layout are honoured. `cmd` output keeps its accents and `{clipboard}` in shell
  actions no longer drops characters.
- Layout detection follows the layout id and variants, and only Spanish (Spain) selects `es-win`.
- Popups replace the previous one, appear on the monitor in use and scale per monitor; notifications use the notification area icon
  instead of PowerShell; dark mode notifies running programs; sleep uses the system call.

### Software capture, all systems
- The consent is recorded on the installation (`capture-consent.json`, outside `config.json` and its backups) by the same call that
  turns the mode on. Restoring a backup, importing a configuration, `POST /api/config` and a `config.json` that says `software`
  without that record can no longer turn on the keyboard hook: they are normalised to `board` and the log and a notice say so.
  Configurations written by earlier builds of this branch fall back to the board once.
- With software capture, keys without a macro are reported to the program (and so reach the WebSocket and the history) only while the
  key assistant detects keys or when you turned the typing history on; an open editor no longer asks for every key, and the consent
  text says that the history records what you type in every application. With the board nothing changes. `SECURITY.md`, the FAQ and
  the READMEs state the real limit: the editor's port is shared by every local user, so do not use Typedeck on a shared computer.
- A failed start retries with a growing delay (4 s, doubling up to 2 minutes) instead of every 4 s. If switching to software capture
  fails, at start-up or when applying a configuration, the board stays active and a notice is shown; switching is serialised, and
  `Pause` and `Resume` reach both sources. State changes (connected, disconnected) are delivered in order with the key events.
- `capture.Source` is split into `Capturer` and `Injector`; what only the board has (`LayerSetter`, `Commander`, `RawDoer`,
  `SysReporter`) is separate. Injecting with the hook off says that capture is not running instead of "unsupported".
- Release: the macOS helpers keep their execute bit in the archives (artifacts lose it between jobs); `make archives` sets it and
  fails if a helper is not executable inside the archive, and the release fails if a helper is missing. The generated example
  macros use documentation addresses (`192.0.2.x`). The compiled `typedeck` binary that had been committed by mistake is gone and
  ignored.

### macOS
- The launch agent is now `io.github.cristobaltormo.typedeck`; `install` and `uninstall` remove the old `cc.cristobal.typedeck` one, so upgrading
  needs no manual step.
- Software capture on macOS: Typedeck works without the board. `typedeck-capture`, a small Swift helper that the program starts and
  watches, installs a CoreGraphics event tap, takes only the keys that have a macro and lets every other key through without
  reading it, and sends Typedeck's own shortcuts, text (Unicode), media keys and Caps Lock. It talks to the program through plain
  text lines on its standard input and output. It is off until you choose it, exactly like on Windows (`settings.capture`,
  `POST /api/capture`, the consent in the editor), and the permissions it needs, Accessibility and Input Monitoring, are asked for
  only then. If the program dies the helper sees its input close and exits, which removes the tap; if macOS disables a slow tap the
  helper releases what it held, turns it back on and tells the program. `typedeck doctor` and the editor say which permission is
  missing and where to grant it, and `typedeck install` tells you if the helper is not next to the program. Honest status: built and
  tested on an Apple Silicon Mac (key table, protocol, state machine with synthetic events, permission messages), but no real key has
  been swallowed or injected yet because that needs the permissions granted by hand. Caps Lock reaches the tap as a toggle and
  password fields are hidden by macOS from every tap.
- `make macos-helpers` builds `typedeck-capture` and the popup helper `typedeck-hud` as universal binaries signed ad hoc (no Apple
  developer account, no notarization); the CI builds and self tests them on a macOS runner, and the release puts both in the macOS
  archives. `typedeck-capture --check` and `--selftest` need no permission.
- Key compensation of the board's keyboard (modifier remap and the swapped ISO keys) is not applied to keys Typedeck sends in
  software capture mode, since it belongs to the board.

### Linux
- Software capture on Linux: Typedeck works without the board. The program opens the keyboards in `/dev/input` (every node with all
  the letter keys and no pointer axes), takes each one exclusively (`EVIOCGRAB`) and passes every key it does not take back through a
  virtual keyboard of its own (`/dev/uinput`), with modifiers, auto-repeat, media keys and the LEDs going the other way. It works
  under the compositor, so X11 and Wayland behave the same, and it needs no library and no cgo. It is off until you choose it,
  exactly like on Windows and macOS (`settings.capture`, `POST /api/capture`, the consent in the editor). Keyboards plugged in while
  it runs are picked up (inotify) and an unplugged one releases its held keys. Typedeck's own shortcuts, text and media keys go out
  through the same virtual keyboard; text the layout cannot type is pasted with Ctrl+V sent that way.
- Fail-open: the grab belongs to the file descriptor, so if the program dies, even with `SIGKILL`, the kernel gives the keyboards back
  at once (measured: 2 ms). `SIGTSTP` is ignored while capture runs so Ctrl+Z cannot freeze it. A process frozen but alive (`SIGSTOP`)
  does keep the keyboards until it resumes or is killed; that is documented, not fixed.
- Access: it needs your user in the `input` group and the `uinput` module. The `.deb` and `.rpm` packages now install a udev rule that
  gives `/dev/uinput` to that group, load the module at boot and at install, and nothing adds anybody to the group. `typedeck doctor`
  and the editor (Spanish and English) say which of `/dev/uinput`, the keyboards in `/dev/input` or the clipboard tool is missing and
  how to fix it.
- The evdev to HID usage table has its own tests, as do the ioctl numbers and the per-key decisions; `tests/e2e/linux_capture.py` runs
  the whole thing against uinput virtual keyboards (swallow, order, auto-repeat, gestures, layers, modifiers, injection, hotplug,
  LEDs, signals, latency, idle cost, permission errors). Honest status: tested on a Kali Linux virtual machine, with virtual keyboards
  and end to end with its emulated PS/2 keyboard and a real Xorg with libinput; not tried on a physical keyboard, on a Wayland
  compositor or next to a remapper such as keyd, and macros keep running on the lock screen because Typedeck does not detect it.
- The virtual keyboard now declares keyboard keys 1 to 255 and 0x160 to 0x2BF, so keys such as `KEY_FN` are no longer lost; the button
  ranges stay out because they would make the kernel and udev classify it as a mouse or a joystick. The access to `/dev/input` is checked
  before the virtual keyboard is created, so a failing start no longer leaves a new virtual keyboard every few seconds. A keyboard that
  cannot be grabbed (`EBUSY`) is not counted as captured and shows up in `typedeck doctor` and as a warning in the editor.

### Added
- Software capture on Windows: Typedeck works without the board. A global keyboard hook takes the keys that have a macro and lets every
  other key through without reading it, and gestures, layers, actions, the editor and the key assistant work as with the board. It is
  off until you choose it: the editor offers **Use without a board** when no board shows up, asks for consent with a plain statement of
  what it does and does not do, and Settings has the switch to go back. The new setting is `settings.capture` (`board` or `software`,
  default `board`; it is separate from `settings.input`, which only decides how typed output is sent) and the endpoint is
  `POST /api/capture`; importing a configuration cannot turn it on. If the program is killed the hook goes with it and every key types
  again. `typedeck doctor` checks the hook, the session and whether it runs elevated.
- `internal/capture`: the interface the engine, the server and `main` depend on instead of the board, the hook state machine, the
  scancode and HID usage tables and the Windows backend. Porting software capture to another system means implementing `Backend`.
- Firmware 15: `HOLD <usage>` and `RELEASE [usage]` keep a key down for longer than `KEY` allows (8 s deadline, re-armed by each `HOLD`, released when
  the heartbeat stops), and `HELD` lists what is held. Re-flash with `scripts/flash.sh`; nothing else changes.
- Every release attaches the compiled firmware (`typedeck-firmware-<number>.hex`, with its checksum and attestation), and
  `docs/HARDWARE.md` explains how to flash it with `arduino-cli` on Windows, Linux and macOS, with no Mac and nothing to compile.

## 0.8.2 - 2026-10-03

### Fixed
- A layer key (Caps Lock) pressed on its own, whether tapped, held or double tapped, types its key again: Caps Lock toggles. Pressed with another
  key it no longer does, and the other key runs its macro from the layer. macOS ignores a Caps Lock press as short as the board's normal tap, so
  the firmware (now 14) takes an optional hold time in `KEY <mods> <usage> [ms]` and Caps Lock is sent held for 150 ms. Re-flash the board with
  `scripts/flash.sh` for it; with firmware 13 everything else works and Caps Lock alone still toggles on Windows and Linux.
- Key names were cut at the bottom on small windows. The icon shrinks or goes away first, and when even the name does not fit it is left out
  (the tooltip still shows it).

### Changed
- Shorter help texts in the action forms and the key panel.

## 0.8.1 - 2026-10-03

### Fixed
- A layer key (Caps Lock, for example) is now a true modifier: the layer turns on the instant the key goes down, with no hold time, and turns
  off when it is released, so Caps Lock + W is as immediate as Cmd + C. It works from the Tap or the Hold tab. Pressed alone and short it does
  nothing, unless the key has its own tap action, which then runs. A repeated key-down from the keyboard can no longer strand the layer.
- Names on the keys were cut ("Google Chrome", "Downloads", "Pomodoro"): the text now shrinks to fit the key and the icon makes room when the
  name takes two lines. Layer actions show the layer's name instead of the word "layer".
- The key detection assistant took an ISO keyboard whose Ç key sends 0x31 for ANSI unless you also pressed the < > key. It now treats that key as
  the ISO one, and when both layouts fit it asks which one you have (tall Enter or flat) instead of choosing for you.
- A key that comes from All layers showed an empty "No action" panel in a layer; it now shows what it does.

## 0.8.0 - 2026-10-03

### Fixed
- Holding a key that turns on a layer (Caps Lock, for example) and pressing another key at once now works: the layer switches the moment the
  second key goes down instead of after the hold time, and the keys of that layer are already captured, so Caps Lock + W no longer types a W.
  A key captured in the held layer that has no action there is typed normally.
- A "next layer" action on a key that only exists on one layer worked once and then vanished with the layer. The editor now warns about it and
  moves the key to All layers with one click.
- Holding a layer key no longer pops up the layer notice on every press; only the layer bar and the board show the change.

### Added
- Layer actions can toggle: one press goes to a chosen layer and the next one returns to where you were.
- Every action type has its own form: only the options that make sense for it (the double-press confirmation for risky ones, a test button
  where a test is safe, a countdown for actions that type, the shortcut examples of your operating system, a description of each mode).

## 0.7.1 - 2026-10-03

### Fixed
- Dragging a macro to another key uses pointer events instead of the browser's drag and drop, which did not start on every browser.
- The key detection assistant noted the ISO key next to Enter (Ç) only when the keyboard sent it as 0x32; keyboards that send 0x31 now show it as seen.

### Changed
- While the key detection assistant or "Pick the key I press" is on, every key is captured: the keyboard types nothing into the computer, so no
  stray press reaches an application. Typing in a field of the editor still works.
- "Follow my keyboard" is now "Pick the key I press" and explains itself under the switch.

## 0.7.0 - 2026-10-03

### Added
- Drag a macro from one key to another in the editor: onto an empty key it moves, onto a key with a macro the two swap, and holding Alt (Option) copies.
- The gallery previews zoom on the keys a pack uses, list its macros under the drawing, and open large when clicked.

### Changed
- The keyboard drawing and the editor chrome cannot be selected as text, and Ctrl+C and Ctrl+V no longer copy and paste a key's macro (the copy
  and paste buttons of the key panel still do).

## 0.6.1 - 2026-10-03

### Fixed
- Linux packages: the udev rule is installed as `70-typedeck.rules`. As `99-` it sorted after `73-seat-late.rules`, so the logged-in user never
  got access to the board and `typedeck doctor` could not open it.
- Spanish PC layout: the euro sign is typed with AltGr+E (AltGr+5 gave a half sign on Ubuntu).
- A port that failed the board handshake is tried again after 5 seconds instead of 30, so a board that keeps restarting while it looks for the
  keyboard is found.
- `typedeck doctor` shows what is optional (reading the active window on Wayland, software keys) as notes instead of failures.

## 0.6.0 - 2026-10-03

### Added
- Firmware 11: `BOOTLOG` (persistent boot statistics), `REBOOT` and `DARK`; TX/RX LEDs off by default.
- A restart-board button in the "keyboard not seen" notice and in the diagnostics page, and a clearer message about what to do.

- The editor releases key capture while a text field has focus, so typing the letter of a key that has a macro no longer runs it.
- A WebSocket between the editor and the program: events arrive live (the long poll stays as a fallback).
- A popup when the board has not seen the keyboard for 10 seconds, and another when it returns (can be turned off in settings).
- The key that opens the editor focuses the tab that is already open instead of opening another one (Chromium browsers and Safari on
  macOS, Hyprland, Sway and X11 on Linux, any window titled Typedeck on Windows).
- Condition actions (front app, layer, time, system, OBS live or recording, clipboard) with an otherwise branch; sequences of up to 100 steps.
- A recorder that turns typed text and shortcuts into sequence steps, with the pauses as editable wait steps, and a duplicate-step button.
- Export a layer or all macros to a file, and import one with a review that lists its commands and web requests.
- Search in the key sheet, and every macro key in the command palette.
- Every key press is shown in the editor, not only the keys with a macro (firmware 12, `KEYS 0|1`, sent only while an editor is open).
- An optional typing history, off by default: key, time and duration of each press, kept in `keystrokes.jsonl` in the configuration folder
  (mode 0600, capped at 4 MB, nothing leaves the computer), with its own History page in the menu: a switch, totals, most used keys, the latest presses (live, filterable) and a delete-all button.

- The typing history deletes the oldest entries on its own (1, 7, 30, 90 or 365 days, 30 by default), has its own page with what you type shown live as text, and
  a new action turns it on or off from a key.
- Community files for GitHub (security policy, code of conduct, issue and pull request templates, Dependabot, CodeQL), examples, screenshots,
  a macro guide, an FAQ and performance notes, and CI jobs for the editor self test, the firmware build and the generated files.

### Fixed
- The key that opens the editor only focuses windows of a browser, so a folder or a terminal that happens to be called Typedeck is left alone.

### Changed
- English is the default language of the program and the editor; a language already chosen is kept. Errors, logs, the doctor and help output,
  the tests and the diagnostic fields of the serial protocol (firmware 13) are in English too.
- The macOS service runs at interactive instead of background priority: opening the editor from its key takes about 150 ms instead of
  over 500, and every action that starts a program is faster.

### Removed
- The firmware's automatic keyboard power cycles: the shield's VBUS switch does not cut the keyboard, so they did nothing.

## 0.5.0 - 2026-10-02

### Added
- Windows 10 and 11: native COM port, active window, keys, clipboard, Start menu applications, a scheduled task to start with the
  session and its own popup window, tested on a real Windows 11 with the board.
- A Spanish layout for Windows, where `Alt Gr+4` is a dead key, next to the Linux one.
- Firmware 10: a `BUS` command with the USB line state, and recovery of the keyboard after a power cut by restarting through the
  bootloader (it can take 20 to 40 seconds); retries a failed shield initialisation instead of stopping.
- Hardware notes on power and KVM switches, troubleshooting, contributing guide, architecture notes and a generated compatibility
  table.
- Continuous integration for Linux, macOS and Windows, and release builds for six targets.

### Changed
- The Discord pack drops the upload shortcut, which did not answer in the real application.
- Closing an application matches its window title with WM_CLOSE, so Store applications close too.
- New per-application rules are evaluated against the window that is already in front.

## 0.4.0 - 2026-09-11

### Added
- Linux (X11 and Wayland): serial port discovery, applications from `.desktop` files, the focused window on X11, Sway and Hyprland,
  software keys with `xdotool` or `wtype`, clipboard, sleep, lock, notifications and a systemd user service
  (`typedeck install`).
- `TYPEDECK_PORT` to force the serial port on any system.
- A Spanish layout for Linux (Alt Gr), per-OS pack variants (Cmd becomes Ctrl, Zoom has its own) and packs that only make sense on
  macOS are hidden elsewhere.
- Tests on Linux with a simulated board and with a virtual X11, and a udev rule so the user can open the board.

## 0.3.0 - 2026-08-30

### Added
- OBS Studio control over its WebSocket server: scenes (by name or by number), next and previous scene, streaming, recording, mute
  for a source, replay buffer, virtual camera and studio mode. A settings section tests the connection.
- The OBS packs use it instead of hotkeys, so there is nothing to assign inside OBS.

### Changed
- Configuration files are written with mode 0600 and the editor export leaves the OBS password empty.

## 0.2.0 - 2026-08-23

### Added
- The editor: the keyboard drawn from its real geometry, a key editor with tap, hold and double tap, the gallery, activity and a
  printable key sheet, diagnostics, compatibility, settings, a command palette, undo and redo, and English and Spanish.
- A self test for the editor and end-to-end tests against the real board on a Mac.
- Installation as a macOS login service.

## 0.1.0 - 2026-08-03

### Added
- Firmware that forwards a USB keyboard through the Leonardo and captures the keys that have a macro, failing open when the program
  stops sending heartbeats.
- The Go program: serial link to the board, gesture engine with layers and per-application layers, actions, floating popup, key
  layouts from 100 to 60 percent, keyboard identification, configuration with backups and migration, the gallery packs and the
  compatibility checks, behind a local API.

