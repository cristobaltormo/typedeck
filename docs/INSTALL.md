# Install and set up

The quick start is in the [README](../README.md#quick-start). This page has every platform in full.

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

The `typedeck-cli-windows-amd64.exe` next to it is the same program for terminals (see Set it up below); the zip archives have both.

There are builds for Linux (amd64, arm64), macOS (Intel and Apple silicon) and Windows (amd64 and arm64), each with a checksum in
`SHA256SUMS` and a build provenance attestation (`gh attestation verify <file> --repo cristobaltormo/typedeck`). To build it yourself you need Go 1.24 or newer:

```sh
go install github.com/cristobaltormo/typedeck/cmd/typedeck@latest
```

Flash the board once. Each release attaches the ready-made firmware, `typedeck-firmware-<number>.hex`, and `arduino-cli` uploads it on
Windows, Linux and macOS alike:

```sh
arduino-cli core install arduino:avr
arduino-cli upload --fqbn arduino:avr:leonardo --port <port> --input-file typedeck-firmware-15.hex
```

Stop Typedeck first and see [HARDWARE.md](HARDWARE.md#flashing) for the port names and the steps. To compile the firmware
yourself it needs the two libraries it uses ([firmware/README.md](../firmware/README.md)).

## Set it up

```sh
typedeck doctor     # checks the configuration, the OS tools, the port, the board and the keyboard
typedeck            # starts the program; the editor is at http://127.0.0.1:7788
typedeck install    # start with your session (typedeck uninstall removes it)
```

- **macOS:** `typedeck install` creates a launch agent that starts it when you sign in. The on-screen popup is a small Swift helper:
  `swiftc -O macos/hud/hud.swift -o typedeck-hud`, placed next to the `typedeck` binary. The macOS archive already contains both
  helpers (`typedeck-hud` and `typedeck-capture`, universal and ad-hoc signed); `make macos-helpers` builds them on a Mac with the
  Xcode command line tools. Archives downloaded with a browser carry a quarantine mark that blocks unsigned helpers:
  `xattr -dr com.apple.quarantine <folder>` once. Software capture asks for **Accessibility** and **Input Monitoring** for
  `typedeck-capture` the first time you turn it on (never before); if you miss the prompt, add it by hand in System Settings, Privacy
  & Security, with the `+` button, choosing the file next to `typedeck`. `typedeck doctor` says which one is missing.
- **Linux:** copy `packaging/linux/70-typedeck.rules` to `/etc/udev/rules.d/` so your user can open the board without the `dialout`
  group. Install `xdg-utils` and `libnotify-bin`; `xdotool` (X11) or `wtype` (Wayland) only matter when the board is not connected.
  Logs: `journalctl --user -u typedeck`.
- **Windows:** double click `typedeck.exe` to start it: it opens the editor in your browser and puts an icon in the notification area
  (Open editor, status, Restart, Quit). Opening it again only opens the editor of the copy that is already running.
  `typedeck.exe install` makes it start when you sign in, with a scheduled task that has no time limit, also runs on battery and
  starts no second copy; if Windows does not let a standard user create it, it falls back to the `Run` key of your account.
  `typedeck.exe uninstall` stops it in an orderly way and removes both. The archive also has `typedeck-cli.exe`, the same program built
  for terminals: use it for `doctor`, `install` and `uninstall` from cmd or PowerShell so the output is shown in place and the exit code
  is reliable. Logs: `%LOCALAPPDATA%\typedeck\typedeck.log`. Locking the screen or suspending releases the captured keys and they are
  captured again when you return. The program is not code signed (a certificate costs money), so see
  [Windows blocks the executable](TROUBLESHOOTING.md#windows-blocks-the-executable).

Set `TYPEDECK_PORT=/dev/ttyACM1` (or `COM5`) when the board is on an unusual port. The first screen of the editor walks you through
the keyboard it found and its layout.

