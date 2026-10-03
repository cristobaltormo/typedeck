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

## OBS password

The OBS connection stores its password in `config.json`, readable only by the user and never sent anywhere. The editor's export
leaves it empty. OBS listens on `127.0.0.1` unless changed in its settings.

## Known limits

Anyone who already has a session on your computer can read the binary and the configuration, which only holds what you put in the
macros: do not put passwords in text actions. The security tests are in `internal/server/server_test.go`.

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's private vulnerability reporting on the repository, or write to the maintainer
address in the commit history.
