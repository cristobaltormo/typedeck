# Troubleshooting

Run `typedeck doctor` first. It checks the configuration, the OS tools the program needs, the serial port and, when the program is
running, whether the board and the keyboard are seen.

## The editor says the board is not connected

- Check the cable and that no other program holds the port (a serial monitor, a second copy of Typedeck).
- Linux: your user must be able to open `/dev/ttyACM*`. Copy `packaging/linux/70-typedeck.rules` to `/etc/udev/rules.d/` and run
  `sudo udevadm control --reload && sudo udevadm trigger --action=add --subsystem-match=tty`, or add the user to the `dialout` group
  and sign in again. The rule file name starts with 70 on purpose: the `uaccess` tag has to be set before `73-seat-late.rules`. The rule
  covers the Arduino Leonardo and Micro and the 5 V SparkFun Pro Micro, and tells ModemManager to leave the board alone, because on
  some distributions it probes new serial ports and interrupts the connection.
- Windows: the board has to appear as a COM port in Device Manager (usbser.sys driver). Typedeck only opens COM ports whose USB
  vendor and product ids are those of the Arduino Leonardo and Micro (2341:8036, 2341:0036, 2341:8037) or the SparkFun Pro Micro
  (1B4F:9206, 1B4F:9204), so it never touches your other serial devices. `TYPEDECK_PORT=COM5` forces a port and skips that filter.
- `TYPEDECK_PORT=/dev/ttyACM1` (a comma separated list of paths or patterns) does the same on any system.
- An outdated firmware is reported explicitly: flash the board again.

## The board is connected but the keyboard is not seen

`/api/board` with `{"cmd":"bus"}` shows the state of the USB line. `hrsl=0x84` or `0x40`/`0x80` means a device is attached;
`hrsl=0x03` with `state=0x12` means the shield sees nothing.

- Unplug the keyboard from the shield and plug it in again.
- If it appeared right after switching a KVM, see [HARDWARE.md](HARDWARE.md): the firmware restarts itself through the
  bootloader and recovers the keyboard in 20 to 40 seconds. Pressing the board's reset button does the same.
- If it does not recover, unplug the board and plug it in first, then the keyboard (a keyboard plugged into a board that is already running is detected; both starting at once is not).

## Shortcuts or text come out wrong

- macOS: modifiers remapped per keyboard and ANSI/ISO type are compensated. `typedeck doctor` and the Diagnostics page show what
  macOS reports.
- Text with unusual layouts (AZERTY, Dvorak) is pasted from the clipboard instead of typed; in macOS that needs the Accessibility
  permission, on Linux `xdotool` (X11) or `wtype` (Wayland) and a clipboard tool.
- Windows: `Alt Gr+4` is a dead key, which is accounted for. Pick the layout explicitly in Settings if detection is wrong.

## Windows blocks the executable

Windows 11 with Smart App Control on blocks unsigned programs ("blocked by your organisation's Device Guard policy"). Turn it off
(Windows Security, App and browser control) or build the program yourself; turning it off cannot be undone without reinstalling.

SmartScreen ("Windows protected your PC") and Microsoft Defender can also warn about, or quarantine, a new unsigned program: that is
a reputation check, not a detection of anything in the code. Choose More info, Run anyway, or add the folder of `typedeck.exe` to the
Defender exclusions. Checking the download first removes the doubt: compare it with `SHA256SUMS` (`Get-FileHash typedeck.exe`) or run
`gh attestation verify typedeck.exe --repo cristobaltormo/typedeck`.

The executables are not code signed because a code signing certificate is a yearly payment and it is not worth it for a free tool;
signing is a decision for the owner of the project. Until then the executable carries a manifest, version information and an icon
(which lowers the heuristic warnings) and it builds reproducibly from source with `make dist`.

## Typedeck on Windows does not start or does not stay

- Look at `%LOCALAPPDATA%\typedeck\typedeck.log` (it rotates to `typedeck.log.1` at 1 MB). A fatal error at start-up also shows a
  message box.
- "Already running": there is one copy per user and configuration folder. Opening `typedeck.exe` again opens the editor of the running
  copy; use the notification area icon, Quit, to stop it, or `typedeck-cli.exe uninstall`.
- `typedeck.exe` is built for the GUI, so a terminal does not wait for it and may not show its output; use `typedeck-cli.exe` there.
- The scheduled task is created for your own account, needs no administrator rights, has no time limit and runs on battery. Check
  it with `typedeck-cli.exe doctor`; if Windows refuses a task, `install` uses the `Run` key and says so.
- The typing history `keystrokes.jsonl` lives in `%LOCALAPPDATA%\typedeck` (it does not roam with the profile); an old copy in
  `%APPDATA%\typedeck` is moved there at start.

## The key that opens the editor opens another tab

It focuses the tab that is already open when it can find it: Chromium browsers and Safari on macOS (the first time, macOS asks
whether Typedeck may control the browser: allow it), Hyprland, Sway and X11 on Linux, and any window titled "Typedeck" on Windows.
Otherwise it opens the page as before.

## Per-application layers do nothing

They need to know which window is in front: macOS, Windows, X11 (`xprop` or `xdotool`), Sway and Hyprland. GNOME and KDE on Wayland
do not expose it.

## OBS actions fail

In OBS enable Tools, WebSocket Server Settings, then copy the port and password into Settings, OBS Studio and press Test
connection. `@desktop` needs a desktop audio source and the replay action needs the replay buffer enabled in OBS.

## Software capture on Linux says it is not running

The editor shows the reason and `typedeck doctor` prints it with the command that fixes it:

- **No permission to open `/dev/uinput`, or to read the keyboards in `/dev/input`:** add your user to the `input` group, then sign out
  and in again so the session gets the group (`sudo usermod -aG input $USER`; check with `id`). The `.deb` and `.rpm` packages install
  `/usr/lib/udev/rules.d/70-typedeck.rules`, which gives `/dev/uinput` to that group; if you installed the binary by hand, copy
  `packaging/linux/70-typedeck.rules` there, run `sudo udevadm control --reload && sudo udevadm trigger`, or give the node to the group
  yourself. Check the nodes with `ls -l /dev/uinput /dev/input/event*`: both need group `input` and mode `rw` for the group. Being in
  the group lets every program you run read your keyboards; see [SECURITY.md](SECURITY.md).
- **The `uinput` module is not loaded** (`/dev/uinput` does not exist): `sudo modprobe uinput`, and `echo uinput | sudo tee
  /etc/modules-load.d/typedeck.conf` to load it at every boot (the packages do this).
- **Typedeck runs as a service and still has no access:** a systemd user service gets the groups of your login, so the group must exist
  when you log in; if you added it a minute ago, log out and in, or reboot.
- **The keyboard does not respond at all and Typedeck is running:** the program is frozen, not dead, and holds the keyboards. Close it
  with the mouse or from another machine over ssh (`kill -9 $(pidof typedeck)`); the keyboards come back at once. Ctrl+Z in the terminal that
  started it does not do this, because `SIGTSTP` is ignored.
- **A keyboard is not captured:** only nodes with all the letter keys and no pointer axes are taken, and one that another program
  grabbed first (a remapper such as keyd or kmonad) cannot be grabbed twice; the remapper's own virtual keyboard is taken instead. A
  keyboard that has a key held down when Typedeck starts is grabbed as soon as that key is released.
- **Per-keyboard settings stopped applying:** the desktop now sees `Typedeck virtual keyboard`; make settings that match a keyboard by name
  (Sway `input` blocks, for example) cover it too.
- **Text with characters your layout cannot type does nothing:** it goes through the clipboard, which needs `wl-clipboard` on Wayland or
  `xclip` (or `xsel`) on X11.

## Software capture (no board) does not take a key

- Run `typedeck doctor` from your own session: it checks that the hook installs, that the session is interactive and whether Typedeck
  runs elevated. On macOS it checks that `typedeck-capture` is next to `typedeck`, that it runs and which permission is missing. On
  Linux it checks `/dev/uinput`, the keyboards in `/dev/input` and the clipboard tool (see the next section).
- macOS, the editor says the capture helper is not running or `doctor` says a permission is missing: open System Settings, Privacy &
  Security, and turn on `typedeck-capture` under **Accessibility** and under **Input Monitoring**. If it is not listed, press `+` and
  choose the file that sits next to `typedeck` (Cmd+Shift+G types a path). The list shows the helper by its file name; Typedeck retries
  from time to time (after a failure the wait grows from 4 seconds up to 2 minutes), so there is nothing to restart. If you move or replace the helper (every update does), macOS treats it as a new program: select the old `typedeck-capture` entries in both lists, remove them with `-` and add the new file once, so the lists do not fill up with copies.
- macOS, `typedeck-capture` is blocked or "cannot be opened": the archive was downloaded with a browser and carries a quarantine mark;
  run `xattr -dr com.apple.quarantine <folder with typedeck>` once. The helper is signed ad hoc, not notarized, so macOS never
  vouches for it.
- macOS, a macro does nothing while you type in a password field or while a password prompt is open: that is secure input, and macOS
  hides those keys from every tool.
- macOS, Caps Lock as a layer key does not stay held: macOS delivers Caps Lock as a toggle, so Typedeck sees a tap. Use another
  key for layers or the board. Not tried on a real keyboard yet.
- The editor says the hook is not installed: Typedeck retries from time to time (4 seconds at first, up to 2 minutes). If it keeps failing, another security tool is
  probably blocking global hooks; go back to the board from Settings, Key capture.
- A macro does nothing inside one window only: that window probably runs as administrator (or is the UAC prompt). Windows hides its
  keys from a normal program. Run Typedeck as administrator for the former, or use the board.
- A game stops or complains when Typedeck is running: its anti-cheat dislikes global keyboard hooks. Close Typedeck, or use the board.
- After resuming from sleep or unlocking the screen the hook is installed again by itself; a watchdog also reinstalls it if Windows
  dropped it because the program was frozen for too long.
