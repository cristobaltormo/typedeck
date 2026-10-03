# Typedeck

Turn any USB keyboard into a macro keyboard. A small Arduino Leonardo with a USB Host Shield sits between the keyboard and the
computer, passes everything through, and lets any key run its own actions: tap, hold, double tap, layers, app launchers, shell
commands, SSH, text snippets, media keys, OBS Studio and more. The keyboard keeps working normally even when the software is not
running.

[Leer en español](README.es.md)

```
 USB keyboard ──► USB Host Shield ──► Arduino Leonardo ──► computer
                                           │  serial
                                           ▼
                                   typedeck (Go) + editor on 127.0.0.1:7788
```

## Why

- **The whole keyboard, not a separate pad.** It detects the keyboard you plug in (brand, model, USB details, what it sends) and
  draws it in the editor with its real format: 100, 80, 75, 65 or 60 percent, ISO or ANSI. A key assistant works the layout out
  when the keyboard does not say.
- **Fast and light.** Key forwarding happens in the board (about 0.13 ms). The computer program is one static Go binary using
  about 11 MB and no CPU when idle; the editor is about 100 KB.
- **Safe by design.** Keys are only taken away from the keyboard while the program is alive. If it stops, every key types again
  within 5 seconds.
- **macOS, Linux and Windows.** One binary per system from the same code. The board is a plain USB keyboard, so it works anywhere;
  the program adapts what depends on the OS.
- **No permissions to type.** Shortcuts, text and media keys leave the board as real USB keystrokes: no Accessibility permission on
  macOS, no `xdotool` on Linux. Remapped modifiers and ISO/ANSI quirks on a Mac are compensated, and Spanish text uses the right
  layout and Alt Gr on each system.
- **OBS Studio without hotkeys.** Switch scene, go live, record, mute the mic or save the replay through OBS's own WebSocket
  server: nothing to assign, and it works the same everywhere.

## What you need

| Part | Notes |
|---|---|
| Arduino Leonardo (ATmega32U4) | tested with a Keystudio KS0248 |
| USB Host Shield 2.0 (MAX3421E) **with ICSP header** | tested with a Yanmis |
| A wired USB keyboard | up to 500 mA |
| macOS 12+, Linux (X11 or Wayland) or Windows 10/11 | see [compatibility](docs/COMPATIBILITY.md) for what is tested where |

## Quick start

Flash the board once (needs `arduino-cli` and `avrdude`), then run the program on the computer the board is plugged into.

```sh
make flash                  # compile and upload the firmware
make dist                   # binaries for macOS, Linux and Windows in dist/
dist/typedeck-linux-amd64 install     # start with your session (also: typedeck uninstall)
```

Then open <http://127.0.0.1:7788>. `typedeck doctor` checks that everything is in place.

- **macOS:** `make install` builds and installs a login service and the popup, and puts `Typedeck.app` in `~/Applications`.
- **Linux:** copy `packaging/linux/99-typedeck.rules` to `/etc/udev/rules.d/` so your user can open the board without the
  `dialout` group. Install `xdg-utils` and `libnotify-bin`; `xdotool` (X11) or `wtype` (Wayland) only matter when the board is not
  connected. Logs: `journalctl --user -u typedeck`.
- **Windows:** `typedeck.exe install` creates a scheduled task that starts it when you sign in. Logs are in
  `%LOCALAPPDATA%\typedeck`. Windows 11 with Smart App Control on blocks unsigned programs; turn it off (Windows Security, App and
  browser control) or build the program yourself.

Set `TYPEDECK_PORT=/dev/ttyACM1` (or `COM5`) if the board is on an unusual port.

## Features

- Tap, hold and double tap on every key, each with its own action. Hold can switch a layer only while it is held.
- Up to 9 layers, each with a color and an optional list of apps that activate it automatically.
- 15 action types: app (open, toggle, quit), link, shell command, SSH (result shown on screen), web request, shortcut, text (with
  `{date}`, `{time}`, `{clipboard}`), sequence, condition, media, system, OBS Studio, timer, layer, popup.
- Record a sequence by typing it, then edit, reorder and time the steps. A condition runs one list of steps or another depending on
  the front app, the active layer, the time of day, the system, whether OBS is live or recording, or what the clipboard holds.
- Share macros as files: export a layer or everything, and review any commands or web requests in a file before importing it.
- The editor follows the board live over a WebSocket, releases key capture while you type in a field of the editor, and pops up
  a notice when the keyboard stops being seen.
- A gallery of 38 packs in 11 categories, laid out on the keys your keyboard really has: OBS Studio, Discord, OBS and Discord,
  Zoom, Meet, Teams, Slack, Photoshop, Figma, Premiere, VS Code, Git, smart home, study and more. Some are macOS-only and are not
  offered elsewhere; on Windows and Linux Cmd becomes Ctrl.
- Every key lights up in the editor as you press it. An optional, local typing history (key, time, duration) can be switched on in the
  settings; it is a file on your computer, with a list of the latest entries and a button that deletes everything.
- A compatibility page with your real setup and automatic checks, activity log and per-key usage, printable and searchable key sheet,
  diagnostics, command palette, undo and redo, automatic backups, import and export, English and Spanish.

## Documentation

[Compatibility](docs/COMPATIBILITY.md) · [Architecture](docs/ARCHITECTURE.md) · [Serial protocol](docs/PROTOCOL.md) ·
[Hardware and power](docs/HARDWARE.md) · [Security](docs/SECURITY.md) · [Troubleshooting](docs/TROUBLESHOOTING.md) ·
[Changelog](CHANGELOG.md) · [Contributing](CONTRIBUTING.md)

## Limits

Six simultaneous keys (boot protocol, confirmed on the real keyboard) and no forwarding of the keyboard's built-in mouse or system
keys. Behind a KVM switch the keyboard can go missing after a power cut; the firmware recovers it by itself in under a minute and
[Hardware](docs/HARDWARE.md) explains how to avoid it.

## License

MIT, see [LICENSE](LICENSE).
