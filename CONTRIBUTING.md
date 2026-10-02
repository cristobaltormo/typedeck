# Contributing

Bug reports with the output of `typedeck doctor` are the most useful thing you can send. Pull requests are welcome.

## Setup

Go 1.24 or newer, and nothing else for the program: it has no dependencies. Arduino CLI with the `arduino:avr` core and the
USB Host Shield Library 2.0 are needed to build the firmware.

```sh
make check      # gofmt, go vet for the three systems, race-tested unit tests
make e2e-linux  # end to end on Linux with a simulated board
make dist       # six binaries with SHA256 sums
```

## Rules

- Keep it dependency free. The binary is small and starts instantly, and that matters.
- Everything OS specific goes behind `internal/platform`. A new system means a new implementation of that interface and its tests.
- Macros in the gallery (`scripts/gen-packs.py`) use only each application's own default shortcuts, and only letters, digits, arrows
  and function keys so they work on any keyboard layout. Say in the pack notes when something needs setting up.
- `internal/packs/packs.json` and `docs/COMPATIBILITY.md` are generated; edit their sources and run the scripts.
- Tests that need hardware are in `tests/e2e/` and are not part of CI; say what you ran in the pull request.
- Commit messages in English, imperative, with a body when the reason is not obvious.
