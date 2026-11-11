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
- Keys are only taken from your keyboard while the program is running. If it stops, every key types again after 5 seconds with the
  board and at once with software capture.
- Software capture (Windows, macOS and Linux, no board) installs a global keyboard hook, on macOS an event tap in the `typedeck-capture` helper and on Linux an exclusive grab of the keyboards in `/dev/input` with a virtual keyboard to pass the keys on, which is what keyloggers use. It is off until you turn it on
  in the editor after an explicit notice, it reads only the keys that have a macro (every other key is reported to the program only
  while you detect keys or if you turn on the optional typing history, which then records what you type in every application), and
  it cannot be enabled through a configuration file you import or a backup you restore: the consent is recorded on this installation
  when you accept it in the editor. On macOS it also needs
  the Accessibility and Input Monitoring permissions, which you grant to the helper; on Linux it needs your user in the `input` group,
  which by itself lets any program you run read every keyboard. Details in
  [docs/SECURITY.md](docs/SECURITY.md).

- On a computer with several users the editor's port is shared: it listens on 127.0.0.1 and is protected by a token and a host check, but the token is served to anyone who can open that page, so any local user with access to the port could use the API (this was already so for the actions with the board). With software capture on, that includes reading the typing history if you turned it on. Do not use Typedeck on shared computers.

Supported versions: the latest release and the `main` branch.
