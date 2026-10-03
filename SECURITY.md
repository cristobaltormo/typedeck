# Security

Typedeck runs commands you configure and sits between your keyboard and your computer, so a problem in it matters. Please report
vulnerabilities privately through GitHub's security advisories for this repository instead of opening a public issue. You can expect
an answer within a few days.

Things worth knowing when you run it:

- The editor listens only on `127.0.0.1` and every request needs a token that is generated on each start. The full model, including
  the WebSocket, is in [docs/SECURITY.md](docs/SECURITY.md).
- A macro file you import can contain shell commands, SSH commands and web requests. The import dialog lists them before anything is
  added; read them. Nothing in a file runs until you press its key.
- The optional typing history is off by default, stays in a file on your computer (mode 0600, capped at 4 MB, deleted after the
  retention you choose) and never leaves it. Turn it off, or delete it, from the History page whenever you like.
- Keys are only taken from your keyboard while the program is running. If it stops, every key types again after 5 seconds.

Supported versions: the latest release and the `main` branch.
