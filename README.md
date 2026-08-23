# Typedeck

Turn any USB keyboard into a macro keyboard. A small Arduino Leonardo with a USB Host Shield sits between the keyboard and the
computer, passes everything through, and lets any key run its own actions: tap, hold, double tap, layers, app launchers, shell
commands, SSH, text snippets, media keys and more. The keyboard keeps working normally even when the software is not
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
- **No permissions to type.** Shortcuts, text and media keys leave the board as real USB keystrokes, so macOS's Accessibility
  permission is not needed. Remapped modifiers and ISO/ANSI quirks of a Mac are compensated.

## What you need

| Part | Notes |
|---|---|
| Arduino Leonardo (ATmega32U4) | tested with a Keystudio KS0248 |
| USB Host Shield 2.0 (MAX3421E) **with ICSP header** | tested with a Yanmis |
| A wired USB keyboard | up to 500 mA |
| macOS 12+ on Apple Silicon | the other systems are planned |

## Quick start

```sh
make flash      # compile and upload the firmware (needs arduino-cli and avrdude)
make install    # build and install the program as a login service on the Mac
open ~/Applications/Typedeck.app   # or http://127.0.0.1:7788
```

`typedeck doctor` checks that everything is in place.

## Features

- Tap, hold and double tap on every key, each with its own action. Hold can switch a layer only while it is held.
- Up to 9 layers, each with a color and an optional list of apps that activate it automatically.
- 13 action types: app (open, toggle, quit), link, shell command, SSH (result shown on screen), web request, shortcut, text (with
  `{date}`, `{time}`, `{clipboard}`), sequence, media, system, timer, layer, popup.
- A gallery of 38 packs in 11 categories, laid out on the keys your keyboard really has: OBS Studio, Discord, OBS and Discord,
  Zoom, Meet, Teams, Slack, Photoshop, Figma, Premiere, VS Code, Git, smart home, study and more.
- A compatibility page with your real setup and automatic checks, activity log and per-key usage, printable key sheet,
  diagnostics, command palette, undo and redo, automatic backups, import and export, English and Spanish.

## Documentation

[Compatibility](docs/COMPATIBILITY.md) · [Serial protocol](docs/PROTOCOL.md) · [Hardware](docs/HARDWARE.md) · [Security](docs/SECURITY.md)

## Limits

Six simultaneous keys (boot protocol, confirmed on the real keyboard) and no forwarding of the keyboard's built-in mouse or system
keys. macOS only for now.

## License

MIT, see [LICENSE](LICENSE).
