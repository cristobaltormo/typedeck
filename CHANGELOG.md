# Changelog

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

