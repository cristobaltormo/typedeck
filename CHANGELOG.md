# Changelog

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

