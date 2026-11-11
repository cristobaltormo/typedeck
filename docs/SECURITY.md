# Security

Typedeck can run commands, so the editor is locked down:

- It listens only on `127.0.0.1:7788` (the next nine ports if that one is busy).
- Every API request needs an `X-Token` header, a random key generated on each start and only delivered in the start page. A
  web page from another origin cannot read it (same-origin policy) or send the header without permission.
- The `Host` header is checked: a request under another name (DNS rebinding) is rejected with 403.
- The live connection (`/api/ws`) applies the same rules: exact `Host`, `Origin` when the browser sends one, and the token as the
  WebSocket subprotocol, never in the URL. At most 8 clients, frames capped at 8 KB, and it only carries events and the editing flag.
  Key capture is released while the editor reports a text field in focus, and again as soon as that connection closes or goes
  quiet for 8 seconds.
- Restrictive content policy (`default-src 'self'`), nothing loaded from outside, `X-Frame-Options: DENY`.
- The whole configuration is validated before it is saved: closed set of action types, bounded lengths, sequences of at most 100
  steps and conditions nested no deeper than one level inside a sequence.
- A macro file is validated like the configuration before it is offered for import, and the import dialog lists every shell command,
  SSH command and web request in it. Nothing from a file runs until its key is pressed.
- `{clipboard}` is quoted when used inside commands, per OS (single quotes on macOS and Linux, double quotes with `"`, `%` and `^`
  removed on Windows), so clipboard contents cannot inject anything.
- Backup restore only accepts names like `config-YYYYMMDD-HHMMSS.json`.
- `/api/dev/raw` exists only with `TYPEDECK_DEV=1` (tests).
- Configuration files are written with mode 0600. The optional typing history (`keystrokes.jsonl`) is off by default, has the same mode,
  is capped at 4 MB (the oldest half is dropped), never leaves the computer, and its API needs the token like any other.

## Software capture

Software capture installs a system-wide keyboard hook on Windows, a keyboard event tap in a separate helper on macOS and, on Linux, an exclusive grab of the keyboards in `/dev/input`, the mechanisms keyloggers use, so it is built to be as small as it can be:

- It is never on by default and never turns on by itself. Only `POST /api/capture` with `consent: true` enables it, which the editor
  sends after showing what it does and does not do, and that same call records the consent on this installation
  (`capture-consent.json` next to the configuration, with the date and the version of the text; it is not part of `config.json` or of
  its backups). `POST /api/config` ignores changes to `settings.capture`. A restored backup, an imported configuration or a
  `config.json` that says `software` without that record is normalised to `board` at load, restore and start-up, the file is
  rewritten and the editor and the log say so. Configurations written by earlier builds of this branch that already said `software`
  therefore fall back to the board once, until the switch is turned on again from the editor.
- The hook callback (on macOS the helper's tap callback, which holds the same bitmask as key codes) decides with a bitmask of the keys that have a macro on the active layer. Keys outside it are not read, stored or
  forwarded to any other part of the program, unless "report every key" is on. With software capture that happens only while the key
  assistant is detecting your layout or when you turned the optional typing history on yourself; an open editor does not turn it on
  (its on-screen keyboard only lights up the keys that have a macro). With the board nothing changes: an open editor also asks for
  every key. The typing history, once on, records what you type in every application; it stays on the computer and can be deleted.
- The program's own `SendInput` events are marked and ignored by the hook, and so is anything the system flags as injected. On macOS the
  helper marks its own `CGEventPost` events with a private value and ignores them.
- Nothing about the hook leaves the computer. There is no network code in it, and the macOS helper has none either: it talks to the
  program only through its standard input and output, as plain text lines (`D`, `U`, `P`, `R` with a key code and the modifier flags).
- Fail-open: the hook is owned by the process. If it is killed, every key types again immediately. While the session is locked or the
  computer sleeps the filter lets everything through. On macOS the tap belongs to the helper and the helper exits when its
  standard input closes, so if the program dies the tap goes with it; if macOS disables a tap that was too slow, the helper releases
  what it held, turns it on again and tells the program.
- macOS permissions: the helper needs Accessibility and Input Monitoring. They are requested only when software capture is turned on,
  for the helper alone (it detaches itself from the program that launched it so the permission is not given to a terminal or to
  `typedeck`), and you can withdraw them in System Settings at any time. The helper is signed ad hoc, not notarized: Apple's paid
  program is not used. macOS hides password fields from all taps (secure input).
- Shared computers: the editor and its API listen on `127.0.0.1`, a port every local user shares. They need the token and the host
  check, but the token is part of the page the editor serves, so on a computer with several users any local user who can reach the
  port can use the API: run macros and actions (already true with the board) and, with software capture and the typing history on,
  read the history. Do not use Typedeck on shared computers.
- Limits that are Windows rules: a hook of a normal user does not see elevated windows or the UAC and lock screens, and some game
  anti-cheats refuse global hooks.
- Linux: the process receives every key of the grabbed keyboards, because it has to pass them on. It looks the key up in the bitmask
  and writes it to the virtual keyboard; only keys in the mask (or every key under the "report every key" conditions above) reach
  the rest of the program, and nothing is stored or sent. The keyboards are matched by capability (EV_KEY with all the letters, no
  pointer axes) and the program's own virtual keyboard is recognised by its `phys` marker and never grabbed, so there is no loop.
- Linux permissions: reading `/dev/input/event*` and writing `/dev/uinput` is what a keylogger needs and also what lets a program type
  into any session, the lock screen included. The packaged udev rule gives `/dev/uinput` to the `input` group, which on common
  distributions already owns the `event*` nodes; adding a user to that group is a decision about every program that user runs, so
  Typedeck never does it for you and `typedeck doctor` only explains it. The package also loads the `uinput` module at boot.
- Linux fail-open: the grab belongs to the file descriptor, so a killed process (even `SIGKILL`) gives the keyboards back at once.
  A process that is frozen and not dead keeps the grab: `SIGTSTP` is ignored while the capture runs, `SIGSTOP` cannot be, and the
  keyboards stay unresponsive until it resumes or is killed (with the mouse, or over ssh).
- Linux does not detect screen lock or a virtual terminal switch, so macros can run on the lock screen.

## OBS password

The OBS connection stores its password in `config.json`, readable only by the user and never sent anywhere. The editor's export
leaves it empty. OBS listens on `127.0.0.1` unless changed in its settings.

## Known limits

Anyone who already has a session on your computer can read the binary and the configuration, which only holds what you put in the
macros: do not put passwords in text actions. The security tests are in `internal/server/server_test.go`.

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's private vulnerability reporting on the repository, or write to the maintainer
address in the commit history.
