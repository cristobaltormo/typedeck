# Changelog

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

