# Typedeck

English | [Español](README.es.md)

[![CI](https://github.com/cristobaltormo/typedeck/actions/workflows/ci.yml/badge.svg)](https://github.com/cristobaltormo/typedeck/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cristobaltormo/typedeck?sort=semver)](https://github.com/cristobaltormo/typedeck/releases)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platforms](https://img.shields.io/badge/macOS%20%7C%20Linux%20%7C%20Windows-supported-lightgrey)](docs/COMPATIBILITY.md)

Turn the USB keyboard you already own into a macro keyboard. A small Arduino Leonardo with a USB Host Shield sits between the
keyboard and the computer, passes every key through untouched, and lets any key run its own actions: tap, hold or double tap,
layers, app launchers, shell commands, SSH, text snippets, media keys, OBS Studio and more.

Typing never depends on the software. If the program is not running, or crashes, the board keeps forwarding the keyboard on its
own and every key types its own character.

Typedeck is a single static Go binary with no dependencies. Idle it uses about 11 MB of RAM and no CPU, and forwarding a key
takes about 0.13 ms inside the board. It runs on macOS, Linux and Windows, and its tests run on the three on every push.

![The Typedeck editor with a keyboard, layers and the action of the selected key](docs/images/editor.png)

Move a macro to another key by dragging it; Alt copies instead of moving.

![Dragging a macro to another key](docs/images/drag.gif)

## What it looks like

The editor opens in your browser, on your own computer. Pick a key on the drawing of your real keyboard and give it an action.

| | |
|---|---|
| ![A sequence of steps with a recorder](docs/images/sequence.png) **Sequences** with a recorder, conditions and editable pauses | ![The gallery of ready-made packs](docs/images/gallery.png) **A gallery** of packs laid out on the keys your keyboard really has |
| ![What you type, rebuilt live in a text box](docs/images/history.png) **Typing history**, optional and local, with what you type shown live | ![Light theme](docs/images/editor-light.png) **Light and dark**, English and Spanish |

## How it works

![Keyboard, shield, Leonardo, computer and the Typedeck program](docs/images/architecture.svg)

The board takes the keyboard on the shield, forwards its reports to the computer as a normal USB keyboard, and removes only the keys
that have a macro, sending them to the program over a serial line instead. The program decides what each gesture does. A heartbeat
every second keeps capture on; without it, the board stops capturing after 5 seconds. More in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## What you need

| Part | Notes |
|---|---|
| Arduino Leonardo (ATmega32U4) | tested with a Keystudio KS0248 |
| USB Host Shield 2.0 (MAX3421E) **with an ICSP header** | tested with a Yanmis |
| A wired USB keyboard | up to 500 mA |
| macOS 12 or newer, Linux (X11 or Wayland) or Windows 10 and 11 | see [compatibility](docs/COMPATIBILITY.md) for what is tested where |

Stack the shield on the Leonardo, plug the keyboard into the shield and the Leonardo into the computer. Behind a KVM switch or a
hub, read [docs/HARDWARE.md](docs/HARDWARE.md) first.

## Install

On Debian, Ubuntu and Fedora, download the `.deb` or `.rpm` for your architecture from the
[releases page](https://github.com/cristobaltormo/typedeck/releases). It installs the program, the udev rule that lets your user open the
board, and the firmware sketch:

```sh
sudo apt install ./typedeck_<version>_amd64.deb      # or: sudo dnf install ./typedeck-<version>-1.x86_64.rpm
```

Everywhere else, download the archive or the plain binary for your machine from the same page:

```sh
curl -L -o typedeck https://github.com/cristobaltormo/typedeck/releases/latest/download/typedeck-linux-amd64
chmod +x typedeck
```

On Windows, in PowerShell:

```powershell
Invoke-WebRequest https://github.com/cristobaltormo/typedeck/releases/latest/download/typedeck-windows-amd64.exe -OutFile typedeck.exe
```

There are builds for Linux (amd64, arm64), macOS (Intel and Apple silicon) and Windows (amd64 and arm64), each with a checksum in
`SHA256SUMS` and a build provenance attestation (`gh attestation verify <file> --repo cristobaltormo/typedeck`). To build it yourself you need Go 1.24 or newer:

```sh
go install github.com/cristobaltormo/typedeck/cmd/typedeck@latest
```

Flash the board once. Each release attaches the ready-made firmware, `typedeck-firmware-<number>.hex`, and `arduino-cli` uploads it on
Windows, Linux and macOS alike:

```sh
arduino-cli core install arduino:avr
arduino-cli upload --fqbn arduino:avr:leonardo --port <port> --input-file typedeck-firmware-14.hex
```

Stop Typedeck first and see [docs/HARDWARE.md](docs/HARDWARE.md#flashing) for the port names and the steps. To compile the firmware
yourself it needs the two libraries it uses ([firmware/README.md](firmware/README.md)).

## Set it up

```sh
typedeck doctor     # checks the configuration, the OS tools, the port, the board and the keyboard
typedeck            # starts the program; the editor is at http://127.0.0.1:7788
typedeck install    # start with your session (typedeck uninstall removes it)
```

- **macOS:** `make install` builds and installs a login service and the popup, and puts `Typedeck.app` in `~/Applications`.
- **Linux:** copy `packaging/linux/70-typedeck.rules` to `/etc/udev/rules.d/` so your user can open the board without the `dialout`
  group. Install `xdg-utils` and `libnotify-bin`; `xdotool` (X11) or `wtype` (Wayland) only matter when the board is not connected.
  Logs: `journalctl --user -u typedeck`.
- **Windows:** `typedeck.exe install` creates a scheduled task that starts it when you sign in. Windows 11 with Smart App Control on
  blocks unsigned programs; turn it off (Windows Security, App and browser control) or build the program yourself.

Set `TYPEDECK_PORT=/dev/ttyACM1` (or `COM5`) when the board is on an unusual port. The first screen of the editor walks you through
the keyboard it found and its layout.

## What it can do

- **Gestures and layers.** Tap, hold and double tap on every key, each with its own action. Up to nine layers with a color, switched
  by a key, momentarily while a key is held, or automatically when an application comes to the front.
- **Fifteen kinds of action.** Application (open, toggle, quit), link, command, SSH, web request, shortcut, text, sequence,
  condition, media, system, typing history, OBS Studio, timer, layer and popup. The full list is in [docs/MACROS.md](docs/MACROS.md).
- **Record, then edit.** Type a sequence and Typedeck turns it into steps, with your pauses as wait steps you can retime, reorder
  and duplicate.
- **Conditions.** Run one list of steps or another depending on the front application, the active layer, the time of day, the
  system, whether OBS is live or recording, or what the clipboard holds.
- **Share macros as files.** Export a layer or everything; importing shows every command and web request in a file before anything
  is added. [examples/](examples) has packs to start from.
- **A gallery of 38 packs** for OBS Studio, Discord, Zoom, Meet, Teams, Slack, Photoshop, Figma, Premiere, VS Code, Git, smart
  home, study and more, laid out on the keys your keyboard has.
- **The whole keyboard, not a separate pad.** It detects the keyboard (brand, model, USB details) and draws it with its real format,
  100 to 60 percent, ISO or ANSI. A key assistant works the layout out when the keyboard does not say.
- **No permissions to type.** Shortcuts, text and media keys leave the board as real USB keystrokes: no Accessibility permission on
  macOS, no `xdotool` on Linux. Remapped modifiers and ISO/ANSI quirks on a
  Mac are compensated, and Spanish text uses the right layout and Alt Gr on each system.
- **OBS Studio without hotkeys.** Scenes, going live, recording, muting the mic or saving the replay through OBS's own WebSocket
  server.
- **Live editor.** Every key lights up on the drawing as you press it, a popup warns you when the board stops seeing the keyboard,
  and the key that opens the editor focuses the tab that is already open.
- **Typing history, optional.** Off by default. A page with what you type shown as text, totals, your most used keys and the latest
  presses, stored in a local file with the oldest entries deleted automatically and a switch you can put on a key. It never leaves
  your computer.
- **Everything around it.** Compatibility page with automatic checks, activity log and per-key usage, searchable and printable key
  sheet, diagnostics, command palette, undo and redo, automatic backups.

## Safe by design

- Keys are only taken from the keyboard while the program is alive. If it stops, every key types again within 5 seconds.
- The editor listens only on `127.0.0.1`, every request needs a token generated on each start, and a macro file you import is
  validated and shown to you before it is added. [docs/SECURITY.md](docs/SECURITY.md) has the model, and [SECURITY.md](SECURITY.md)
  how to report a problem.
- Typedeck makes no network connection of its own, has no telemetry and loads nothing from the internet.
- Without the optional typing history, the program only sees the keys that have a macro.

## Performance

| What | Result |
|---|---|
| Forwarding one keyboard report inside the board | about 0.13 ms |
| Program memory when idle | about 11 MB, 0 % CPU |
| First load of the editor | about 100 KB, no framework |

Details and how to measure your own setup are in [docs/PERFORMANCE.md](docs/PERFORMANCE.md).

## Documentation

[Macros](docs/MACROS.md) · [FAQ](docs/FAQ.md) · [Compatibility](docs/COMPATIBILITY.md) · [Architecture](docs/ARCHITECTURE.md) ·
[Serial protocol](docs/PROTOCOL.md) · [Hardware and power](docs/HARDWARE.md) · [Security](docs/SECURITY.md) ·
[Performance](docs/PERFORMANCE.md) · [Troubleshooting](docs/TROUBLESHOOTING.md) · [Changelog](CHANGELOG.md) ·
[Contributing](CONTRIBUTING.md)

## Limits

Six simultaneous keys (boot protocol, confirmed on the real keyboard) and no forwarding of the keyboard's built-in mouse or system
keys. Behind a KVM switch the keyboard can go missing after a power cut; the firmware recovers it by itself in under a minute and
[Hardware](docs/HARDWARE.md) explains how to avoid it.

## Contributing

Bug reports with the output of `typedeck doctor` are the most useful thing you can send; pull requests are welcome.
[CONTRIBUTING.md](CONTRIBUTING.md) has the setup and the rules, and everything is checked with `make check`.

## Code signing policy

Free code signing provided by [SignPath.io](https://signpath.io), certificate by [SignPath Foundation](https://signpath.org). Until the
project is approved, the Windows builds in the [releases](https://github.com/cristobaltormo/typedeck/releases) are unsigned.

- Only binaries built by this repository's GitHub Actions workflow from its own source code are signed; every release is built from a
  version tag and carries a build provenance attestation.
- Roles: author, reviewer and approver is [Cristóbal Tormo](https://github.com/cristobaltormo), the maintainer. Each signing request
  is approved by hand.
- Privacy: Typedeck makes no network connection of its own and sends no data anywhere; the editor listens on `127.0.0.1` only.

## License

The program, the editor and the documentation are MIT licensed, see [LICENSE](LICENSE). The firmware in `firmware/` is built with
the USB Host Shield Library 2.0 and is therefore GPL-2.0, see [NOTICE](NOTICE).
