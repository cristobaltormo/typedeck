# Troubleshooting

Run `typedeck doctor` first. It checks the configuration, the OS tools the program needs, the serial port and, when the program is
running, whether the board and the keyboard are seen.

## The editor says the board is not connected

- Check the cable and that no other program holds the port (a serial monitor, a second copy of Typedeck).
- Linux: your user must be able to open `/dev/ttyACM*`. Copy `packaging/linux/99-typedeck.rules` to `/etc/udev/rules.d/` and run
  `sudo udevadm control --reload && sudo udevadm trigger`, or add the user to the `dialout` group and sign in again.
- Windows: the board has to appear as a COM port in Device Manager (usbser.sys driver). `TYPEDECK_PORT=COM5` forces a port.
- `TYPEDECK_PORT=/dev/ttyACM1` (a comma separated list of paths or patterns) does the same on any system.
- An outdated firmware is reported explicitly: flash the board again.

## The board is connected but the keyboard is not seen

`/api/board` with `{"cmd":"bus"}` shows the state of the USB line. `hrsl=0x84` or `0x40`/`0x80` means a device is attached;
`hrsl=0x03` with `state=0x12` means the shield sees nothing.

- Unplug the keyboard from the shield and plug it in again.
- If it appeared right after switching a KVM, see [HARDWARE.md](HARDWARE.md): the firmware restarts itself through the
  bootloader and recovers the keyboard in 20 to 40 seconds. Pressing the board's reset button does the same.
- If it keeps happening, power the Leonardo from its DC jack as described in the hardware notes.

## Shortcuts or text come out wrong

- macOS: modifiers remapped per keyboard and ANSI/ISO type are compensated. `typedeck doctor` and the Diagnostics page show what
  macOS reports.
- Text with unusual layouts (AZERTY, Dvorak) is pasted from the clipboard instead of typed; in macOS that needs the Accessibility
  permission, on Linux `xdotool` (X11) or `wtype` (Wayland) and a clipboard tool.
- Windows: `Alt Gr+4` is a dead key, which is accounted for. Pick the layout explicitly in Settings if detection is wrong.

## Windows blocks the executable

Windows 11 with Smart App Control on blocks unsigned programs ("blocked by your organisation's Device Guard policy"). Turn it off
(Windows Security, App and browser control) or build the program yourself; turning it off cannot be undone without reinstalling.

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
