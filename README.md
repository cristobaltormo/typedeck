<div align="center">

<img src="docs/images/logo.svg" alt="" width="84">

# Typedeck

**Turn the keyboard you already own into a macro keyboard.**

Every key can do more than type: tap, hold or double tap, layers, app launchers, shell commands, SSH, text snippets, media keys, OBS Studio and more. With a small Arduino board, or with no hardware at all.

[![CI](https://github.com/cristobaltormo/typedeck/actions/workflows/ci.yml/badge.svg)](https://github.com/cristobaltormo/typedeck/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cristobaltormo/typedeck?sort=semver)](https://github.com/cristobaltormo/typedeck/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platforms](https://img.shields.io/badge/macOS%20%7C%20Linux%20%7C%20Windows-supported-lightgrey)](docs/COMPATIBILITY.md)

[Quick start](#quick-start) · [Documentation](#documentation) · [Releases](https://github.com/cristobaltormo/typedeck/releases) · [Changelog](CHANGELOG.md) · [Español](README.es.md)

<br>

<img src="docs/images/editor.png" alt="The Typedeck editor with a keyboard, layers and the action of the selected key" width="92%">

</div>

<br>

## Why

Macro pads are a second device on your desk, and most keyboard tools only work on one operating system. Typedeck puts the macros on the keyboard you already type on, and it is built so that typing never depends on it.

- **Your own keyboard.** A small Arduino Leonardo with a USB Host Shield sits between the keyboard and the computer, passes every key through untouched and lets any key run its own actions. Or skip the hardware: on Windows, macOS and Linux Typedeck can take the keys [in software](#without-a-board) when you turn that on.
- **Typing is safe.** If the program is not running, or crashes, the board keeps forwarding the keyboard on its own and every key types its own character. With software capture the hook disappears with the process.
- **Gestures and layers.** Tap, hold and double tap on every key, each with its own action. Up to nine layers with a colour, switched by a key, while a key is held, or automatically when an application comes to the front.
- **Fifteen kinds of action.** Application, link, command, SSH, web request, shortcut, text, sequence, condition, media, system, typing history, OBS Studio, timer, layer and popup. The list is in [docs/MACROS.md](docs/MACROS.md).
- **Record, then edit.** Type a sequence and Typedeck turns it into steps, with your pauses as wait steps you can retime, reorder and duplicate.
- **A gallery of 38 packs** for OBS Studio, Discord, Zoom, Meet, Teams, Slack, Photoshop, Figma, Premiere, VS Code, Git, smart home, study and more, laid out on the keys your keyboard really has.
- **Light on resources.** One static Go binary with no dependencies: about 11 MB of RAM and no CPU when idle, and about 0.13 ms to forward a key inside the board.
- **Private by design.** The editor listens only on `127.0.0.1`, Typedeck makes no network connection of its own, has no telemetry and loads nothing from the internet. See [security](docs/SECURITY.md).
- **macOS, Linux and Windows.** The tests run on the three on every push.

## Screenshots

Move a macro to another key by dragging it; Alt copies instead of moving.

<p align="center">
  <img src="docs/images/drag.gif" alt="Dragging a macro to another key" width="92%">
</p>

<table>
  <tr>
    <td width="50%"><img src="docs/images/sequence.png" alt="A sequence of steps with a recorder"><br><b>Sequences</b> with a recorder, conditions and editable pauses</td>
    <td width="50%"><img src="docs/images/gallery.png" alt="The gallery of ready-made packs"><br><b>A gallery</b> of packs laid out on the keys your keyboard really has</td>
  </tr>
  <tr>
    <td><img src="docs/images/history.png" alt="What you type, rebuilt live in a text box"><br><b>Typing history</b>, optional and local, with what you type shown live</td>
    <td><img src="docs/images/editor-light.png" alt="The light theme"><br><b>Light and dark</b>, English and Spanish</td>
  </tr>
  <tr>
    <td colspan="2"><img src="docs/images/palette.png" alt="The command palette"><br><b>Command palette</b> to jump to any key, action or setting</td>
  </tr>
</table>

## How it works

<p align="center">
  <img src="docs/images/architecture.svg" alt="Keyboard, shield, Leonardo, computer and the Typedeck program" width="92%">
</p>

The board takes the keyboard on the shield, forwards its reports to the computer as a normal USB keyboard, and removes only the keys that have a macro, sending them to the program over a serial line instead. The program decides what each gesture does. A heartbeat every second keeps capture on; without it the board stops capturing after 5 seconds. More in [architecture](docs/ARCHITECTURE.md).

### Without a board

Software capture replaces the board and is off until you choose it: the editor offers **Use without a board** when none shows up, and Settings has a switch to go back. Gestures, layers, actions and the editor are the same.

| System | How it takes the keys |
|---|---|
| Windows | a system-wide keyboard hook inside the program |
| macOS | `typedeck-capture`, a small Swift helper the program starts and watches, with an event tap |
| Linux | the kernel's input devices: it grabs the real keyboards (`EVIOCGRAB`) and re-emits what it does not take through a virtual keyboard (`/dev/uinput`), so X11 and Wayland behave the same |

It takes only the keys that have a macro on the active layer and lets every other key through without the program reading it, unless you are detecting keys or you turned the optional typing history on. A hook sees every key, so the editor asks for your consent first, says what it does and does not do, and records it on that installation: a restored backup or an imported file cannot turn it on. If the program dies, every key types its own character again at once. It is a trade: no hardware to build, a few limits the board does not have (see [Limits](#limits)). Do not use it on a computer shared with other users.

## How it compares

Typedeck does one thing: macros on the keyboard you have, on every desktop. The others are the right choice in their own cases.

| | Typedeck | QMK or VIA keyboards | Stream Deck | AutoHotkey, Hammerspoon |
|---|---|---|---|---|
| Works with the keyboard you own | yes | no, needs a compatible keyboard flashed | no, it is its own pad | yes |
| Hardware to buy | none, or an Arduino and a shield | a QMK keyboard | the pad | none |
| macOS, Linux and Windows | all three | any, it runs in the keyboard | Windows and macOS | one each |
| Visual editor | yes, in the browser | VIA | yes | no, scripts |
| Typing safe if the program dies | yes | yes | yes | depends on the script |

## What you need

| Part | Notes |
|---|---|
| Arduino Leonardo (ATmega32U4) | tested with a Keystudio KS0248 |
| USB Host Shield 2.0 (MAX3421E) **with an ICSP header** | tested with a Yanmis |
| A wired USB keyboard | up to 500 mA |
| macOS 12 or newer, Linux (X11 or Wayland) or Windows 10 and 11 | see [compatibility](docs/COMPATIBILITY.md) for what is tested where |

Stack the shield on the Leonardo, plug the keyboard into the shield and the Leonardo into the computer. Behind a KVM switch or a hub, read [hardware](docs/HARDWARE.md) first. Without the board, any keyboard works, wired, wireless or the laptop's own.

## Quick start

### 1. Install

On Debian, Ubuntu and Fedora, download the `.deb` or `.rpm` for your architecture from the [releases page](https://github.com/cristobaltormo/typedeck/releases):

```sh
sudo apt install ./typedeck_<version>_amd64.deb      # or: sudo dnf install ./typedeck-<version>-1.x86_64.rpm
```

On macOS and elsewhere on Linux, download the archive or the plain binary:

```sh
curl -L -o typedeck https://github.com/cristobaltormo/typedeck/releases/latest/download/typedeck-linux-amd64
chmod +x typedeck
```

On Windows, in PowerShell:

```powershell
Invoke-WebRequest https://github.com/cristobaltormo/typedeck/releases/latest/download/typedeck-windows-amd64.exe -OutFile typedeck.exe
```

Builds exist for Linux (amd64, arm64), macOS (Intel and Apple silicon) and Windows (amd64 and arm64), each with a checksum in `SHA256SUMS` and a build provenance attestation. Or build it: `go install github.com/cristobaltormo/typedeck/cmd/typedeck@latest`.

### 2. Flash the board (once, skip it without a board)

Each release attaches the ready-made firmware and `arduino-cli` uploads it on every system:

```sh
arduino-cli core install arduino:avr
arduino-cli upload --fqbn arduino:avr:leonardo --port <port> --input-file typedeck-firmware-15.hex
```

### 3. Run it

```sh
typedeck doctor     # checks the configuration, the OS tools, the port, the board and the keyboard
typedeck            # starts the program; the editor is at http://127.0.0.1:7788
typedeck install    # start with your session (typedeck uninstall removes it)
```

The first screen of the editor walks you through the keyboard it found and its layout. Platform details, permissions on macOS, the `input` group on Linux, the notification icon on Windows and everything else are in [docs/INSTALL.md](docs/INSTALL.md).

## Safe by design

- Keys are only taken from the keyboard while the program is alive. If it stops, every key types again within 5 seconds with the board, and at once with software capture.
- The editor listens only on `127.0.0.1`, every request needs a token generated on each start, and a macro file you import is validated and shown to you before it is added. [docs/SECURITY.md](docs/SECURITY.md) has the model, and [SECURITY.md](SECURITY.md) how to report a problem.
- Without the optional typing history, the program only sees the keys that have a macro.

## Performance

| What | Result |
|---|---|
| Forwarding one keyboard report inside the board | about 0.13 ms |
| Program memory when idle | about 11 MB, 0 % CPU |
| First load of the editor | about 100 KB, no framework |

How to measure your own setup is in [docs/PERFORMANCE.md](docs/PERFORMANCE.md).

## Limits

- **Board:** six simultaneous keys, and no forwarding of the keyboard's built-in mouse or system keys.
- **Windows without a board:** windows running as administrator and the UAC screen are out of reach of a normal user's hook, and the anti-cheat of some games treats any global hook as suspicious. Use the board for those games.
- **macOS without a board:** password fields (secure input) are hidden from every keyboard tool, the helper needs Accessibility and Input Monitoring, and the keyboard is not seen while the screen is locked.
- **Linux without a board:** it needs the `input` group, it works below the desktop so it also sees the lock screen, and if the program freezes instead of dying the grabbed keyboards wait for it.

Every limit with its reason and what to do about it is in [docs/LIMITS.md](docs/LIMITS.md).

## Documentation

| | |
|---|---|
| [Install and set up](docs/INSTALL.md) | every platform, permissions, starting with your session |
| [Macros](docs/MACROS.md) | every kind of action, gestures, layers and conditions |
| [Hardware](docs/HARDWARE.md) | the board, the shield, power, KVM switches, flashing |
| [Compatibility](docs/COMPATIBILITY.md) | what is tested where |
| [Limits](docs/LIMITS.md) | what each capture mode cannot do |
| [Security](docs/SECURITY.md) | the model, what is protected and what is not |
| [Architecture](docs/ARCHITECTURE.md) and [serial protocol](docs/PROTOCOL.md) | how it works inside |
| [Performance](docs/PERFORMANCE.md) | numbers and how to measure them |
| [Troubleshooting](docs/TROUBLESHOOTING.md) and [FAQ](docs/FAQ.md) | when something does not work |
| [Examples](examples) | macro packs to import |

## Contributing

Bug reports with the output of `typedeck doctor` are the most useful thing you can send; pull requests are welcome. See [CONTRIBUTING](CONTRIBUTING.md) and the [code of conduct](CODE_OF_CONDUCT.md). To report a vulnerability, read the [security policy](SECURITY.md). Everything is checked with:

```sh
make check
```

## License

The program, the editor and the documentation are MIT licensed, see [LICENSE](LICENSE). The firmware in `firmware/` is built with the USB Host Shield Library 2.0 and is therefore GPL-2.0, see [NOTICE](NOTICE).
