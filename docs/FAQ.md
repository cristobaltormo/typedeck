# FAQ

**Does it work without the board?**
The editor and the actions run, but the keys cannot be intercepted: there is nothing between the keyboard and the computer. The board
is what makes any key of any keyboard programmable.

**What happens when Typedeck is not running?**
The board forwards the keyboard on its own and, when the program stops sending its heartbeat, stops capturing after 5 seconds. Every
key types its own character again. A crash cannot leave you without a keyboard.

**Does it add latency?**
About 0.13 ms per report inside the board, measured on a Leonardo with a USB Host Shield. See [PERFORMANCE.md](PERFORMANCE.md).

**Which keyboards work?**
Wired USB keyboards that use the standard boot protocol, which is nearly all of them. Wireless keyboards with a USB receiver usually
work too. A keyboard that asks for more than the shield can give (backlight, 500 mA) may not light up through it. The list that was
tested is in [COMPATIBILITY.md](COMPATIBILITY.md).

**Does it work with a KVM switch?**
Yes, with one caveat: when the KVM cuts power to the board the keyboard may not come back by itself. The firmware restarts the
board to recover it (20 to 40 seconds); powering the Leonardo from its DC jack avoids it. See [HARDWARE.md](HARDWARE.md).

**Is it a keylogger?**
No. Without the optional typing history the program only sees the keys that have a macro, and nothing about what you type. The
history is off by default, local, capped and deletable, and it is the only thing that ever records keys. See
[SECURITY.md](SECURITY.md).

**Can it run on a Raspberry Pi, or without Arduino?**
The board has to be an ATmega32U4 (Leonardo, Micro) or something compatible with the sketch. The Go program runs wherever Go does,
including a Raspberry Pi.

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
