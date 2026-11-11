# Contributing

Bug reports with the output of `typedeck doctor` are the most useful thing you can send. Pull requests are welcome.

## Setup

Go 1.24 or newer, and nothing else for the program: it has no dependencies. Arduino CLI with the `arduino:avr` core and the
USB Host Shield Library 2.0 are needed to build the firmware.

```sh
make check      # gofmt, go vet for the three systems, race-tested unit tests, editor module tests
make e2e-linux  # end to end on Linux with a simulated board
make e2e-linux-capture  # software capture on Linux with uinput virtual keyboards; run it in a throwaway VM, it creates input devices
make selftest   # the editor in a headless Chromium (needs Chromium or Chrome)
make dist       # six binaries with SHA256 sums
make macos-helpers   # the two Swift helpers for macOS (needs a Mac with swiftc)
make release-files   # plus the archives and the .deb and .rpm packages (needs nfpm)
make screenshots  # regenerate docs/images
```

## Tests with hardware

They are not part of CI. `tests/e2e/hardware.py` runs against a real board on the machine it is plugged into, with Typedeck started
with `TYPEDECK_DEV=1`; set `TYPEDECK_EXPECT_VID` and `TYPEDECK_EXPECT_PID` to also check the keyboard behind the board.
`make e2e-windows` and `make e2e-linux-real` deploy a build to another machine over ssh and run everything there, typing only
into a window of their own:

| Variable | Used by | Meaning |
|---|---|---|
| `WIN_HOST` | e2e-windows | ssh alias of the Windows machine (default `windows`) |
| `WIN_DIR` | e2e-windows | folder where Typedeck is installed there, for example `C:\Users\me\typedeck` |
| `WIN_COM_PORT` | e2e-windows | serial port of the board, for example `COM3` |
| `LINUX_HOST` | e2e-linux-real | `user@host` of the Linux machine |
| `LINUX_KEY` | e2e-linux-real | ssh key (default `~/.ssh/id_ed25519`) |
| `LINUX_SUDO_FILE` | e2e-linux-real | optional file holding the sudo password; without it sudo must not ask |
| `LINUX_SERIAL` | e2e-linux-real | serial port of the board (default `/dev/ttyACM0`) |

`tests/e2e/linux_capture.py` creates virtual keyboards through `/dev/uinput`, so run it only in a throwaway virtual machine or as a
user of the `input` group on a machine where you accept that. `LINUX_CAPTURE_ROUNDS` (latency samples), `LINUX_CAPTURE_UNPRIV` (an
unprivileged user for the permission-error check, default `nobody`), `LINUX_CAPTURE_WIDEN_UINPUT=1` (briefly opens `/dev/uinput` to
check the second error) and `LINUX_CAPTURE_DISPLAY` (an X display with `xclip`, for the paste check) are optional.

The Linux machine needs `/tmp/tdenv.sh` exporting the session variables (`XDG_RUNTIME_DIR`, `DISPLAY` or `WAYLAND_DISPLAY`,
`DBUS_SESSION_BUS_ADDRESS`). `make flash` flashes through a Mac reachable as `MAC_HOST` (default `mac`).

## Rules

- Keep it dependency free. The binary is small and starts instantly, and that matters.
- Everything OS specific goes behind `internal/platform`. A new system means a new implementation of that interface and its tests.
- Macros in the gallery (`scripts/gen-packs.py`) use only each application's own default shortcuts, and only letters, digits, arrows
  and function keys so they work on any keyboard layout. Say in the pack notes when something needs setting up.
- `internal/packs/packs.json` and `docs/COMPATIBILITY.md` are generated; edit their sources and run the scripts.
- Say in the pull request which hardware tests you ran.
- Commit messages in English, imperative, with a body when the reason is not obvious.

## Releasing

1. Move the Unreleased section of `CHANGELOG.md` under a new `## X.Y.Z - date` heading.
2. Commit, then `git tag -s vX.Y.Z -m "Release X.Y.Z"` and push the tag.
3. The Release workflow tests, builds the six binaries, the archives and the .deb and .rpm packages with one checksum file and a
   provenance attestation, and publishes them with the section of the changelog as the notes.
