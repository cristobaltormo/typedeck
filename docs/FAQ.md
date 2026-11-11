# FAQ

**Does it work without the board?**
On Windows, macOS and Linux, yes: turn on software capture (the editor offers **Use without a board** when no board is found, and Settings
has the switch). On Windows a keyboard hook inside the program takes the keys that have a macro; on macOS the small helper
`typedeck-capture` does it with an event tap and macOS asks you for the Accessibility and Input Monitoring permissions. Gestures,
layers, actions and the editor are the same. On Linux the program grabs your keyboards under the desktop and passes the keys it does
not take on through a virtual keyboard, which needs your user in the `input` group and the `uinput` module (see
[TROUBLESHOOTING.md](TROUBLESHOOTING.md#software-capture-on-linux-says-it-is-not-running)). It never turns on by itself.

**What does software capture not do?**
It cannot see keys typed into windows running as administrator or the UAC prompt unless Typedeck runs as administrator too (and
never UAC), some game anti-cheats reject global keyboard hooks (use the board for those), its delay is higher than the board's, and
if the program dies keys with a macro type their own character again. The six-key limit of the board does not apply. Media keys and
the keyboard's built-in system keys are not captured. On macOS it cannot see password fields (secure input), it needs the two
permissions above, and Caps Lock arrives as a toggle, so as a layer key it behaves as a tap (not tried on a real keyboard yet). On Linux
it also sees the lock screen (macros can run there), it needs the `input` group, and a frozen but living process leaves the grabbed
keyboards unresponsive until it resumes or is killed (a killed one gives them back at once).

**Is software capture a keylogger?**
It uses the same system mechanism as one (a hook on Windows, an event tap on macOS, a grab of the input devices on Linux), which is why it is off until you choose it and asks for consent. The program reads only
the keys that have a macro on the active layer. Every other key passes through without being read, except while the key assistant
detects your layout and if you turn the optional typing history on: that history records what you type in every application (it stays
on your computer and you can delete it). An open editor does not make it read more. Nothing is sent anywhere, and on a computer
shared with other users you should not use it, because the editor's port is shared. See [SECURITY.md](SECURITY.md).

**What happens when Typedeck is not running?**
The board forwards the keyboard on its own and, when the program stops sending its heartbeat, stops capturing after 5 seconds. Every
key types its own character again. With software capture the hook belongs to the process, so it disappears with it and the keys
type again at once; on macOS the helper sees that the program is gone and exits, which removes the tap; on Linux the kernel drops the
grab when the process's descriptors close. A crash cannot leave you without a keyboard (a program that is frozen but alive can, on Linux:
see [TROUBLESHOOTING.md](TROUBLESHOOTING.md#software-capture-on-linux-says-it-is-not-running)).

**Does it add latency?**
About 0.13 ms per report inside the board, measured on a Leonardo with a USB Host Shield. With software capture the hook itself takes
microseconds per key, plus the time the system needs to schedule the program; on Linux about 60 µs more than a direct read, measured on a
virtual machine. See [PERFORMANCE.md](PERFORMANCE.md).

**Which keyboards work?**
Wired USB keyboards that use the standard boot protocol, which is nearly all of them. Wireless keyboards with a USB receiver usually
work too. A keyboard that asks for more than the shield can give (backlight, 500 mA) may not light up through it. The list that was
tested is in [COMPATIBILITY.md](COMPATIBILITY.md).

**Does it work with a KVM switch?**
Yes, with one caveat: when the KVM cuts power to the board the keyboard may not come back by itself. The firmware restarts the
board to recover it (20 to 40 seconds); if it does not, unplug the board and plug it in before the keyboard. See [HARDWARE.md](HARDWARE.md).

**Is it a keylogger?**
No. Without the optional typing history the program only sees the keys that have a macro, and nothing about what you type (with the board, an open editor also shows the keys you press on its on-screen keyboard, without storing them). The
history is off by default, local, capped and deletable, and it is the only thing that ever records keys. See
[SECURITY.md](SECURITY.md).

**Can it run on a Raspberry Pi, or without Arduino?**
The board has to be an ATmega32U4 (Leonardo, Micro) or something compatible with the sketch, except with software capture
(Windows, macOS, Linux), which needs no hardware. The Go program runs wherever Go does, including a Raspberry Pi.

**Why not QMK or a macro keyboard?**
Those need a keyboard you can flash. Typedeck leaves the keyboard you already have as it is and adds the macros between it and the
computer.

**How do I move my macros to another computer?**
Settings, Export configuration, or export a layer from its panel and import it on the other machine.

**Why is there no Docker image or npm package?**
Typedeck has to open the board's serial port, launch applications, read the front window, type into the desktop and use the clipboard,
all on the host. A container cannot reach any of that without giving up what makes it a container, and an npm package would only wrap
the same Go binary. Releases ship the binary itself, archives, and `.deb` and `.rpm` packages instead; `go install` works too.

**Where are my files?**
macOS and Linux `~/.config/typedeck`, Windows `%APPDATA%\typedeck`. The configuration is one
JSON file, with the last 40 versions kept in `backups/`.

**Does anything go over the network?**
Only what a macro does (a web request, SSH, OBS on your own computer). Typedeck makes no connection of its own, has no telemetry and
loads nothing from the internet.
