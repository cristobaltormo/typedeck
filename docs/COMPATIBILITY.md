# Compatibility

Generated from `internal/setup/compat.json`; do not edit by hand. The Compatibility page of the editor shows the same table next to your real setup.

Status: **Tested**, **Should work**, **Planned**, **Not supported**. *Tested* means it was run on real hardware or a real machine; *Should work* means the code path is shared or simulated but not seen on that exact setup.

## Board

| Item | Status | Notes |
|---|---|---|
| Arduino Leonardo / Keystudio Leonardo R3 (ATmega32U4, 5 V, 16 MHz) | Tested | The one the author uses. The recommended choice. |
| Arduino Micro | Should work | Same microcontroller and firmware; it does not stack on the shield, so the SPI must be wired. |
| SparkFun Pro Micro and 5 V, 16 MHz clones | Should work | Same microcontroller. The shield pins may need changing in the firmware. Untested. |
| Teensy 4.x (native host port) | Planned | Would be the best shield-free option: the serial protocol is the same, only another firmware is needed. |
| Raspberry Pi Pico (PIO-USB) | Planned | Same serial protocol with another firmware. |
| Arduino Uno, Nano, Mega | Not supported | No native USB: they cannot act as a keyboard. |
| ESP32-S3 | Not supported | A single USB OTG port: it cannot be host and keyboard at once. |

## USB Host Shield

| Item | Status | Notes |
|---|---|---|
| USB Host Shield 2.0 (MAX3421E) with ICSP header (Yanmis) | Tested | SPI arrives through the ICSP header, no wires. |
| Circuits@Home, SparkFun and other MAX3421E shields | Should work | Same library. Check they have an ICSP header. |
| Shield without an ICSP header | Should work | Works by wiring MOSI, MISO and SCK from the board to the shield's pins 11, 12 and 13. |
| 3.3 V shield with a 5 V board | Not supported | May damage the shield: use a 5 V one or a level shifter. |
| USB hub between the shield and the keyboard | Should work | The library supports it. Untested. |

## Keyboard

| Item | Status | Notes |
|---|---|---|
| Wired USB keyboard with a boot report | Tested | Tested with a full-size BY Tech keyboard (SINO WEALTH controller). |
| Wired mechanical keyboards running QMK, VIA or ZMK | Should work | They usually have a boot report and NKRO on another interface. |
| Wireless receiver that presents itself as a keyboard | Should work | Untested. Combined keyboard and mouse receivers may need more interfaces. |
| Keyboard without a boot report (NKRO only) | Not supported | Typedeck detects it and says so. Reading its reports from the descriptor is planned. |
| Bluetooth keyboards | Not supported | They would need a Bluetooth host module on the board. |
| Simultaneous keys | Should work | Up to 6 at once (boot protocol). The unlimited mode is planned. |
| Keyboards that draw more than 500 mA | Not supported | Strong backlighting can make them reset. Power the board from a port with enough current. |

## Computer

| Item | Status | Notes |
|---|---|---|
| macOS 27 on Apple Silicon | Tested | The author's machine, with the Spanish ISO layout. Every feature tested with the real board. |
| macOS 12 or later, Apple Silicon or Intel | Should work | Build for your architecture (make build). |
| Linux with X11 (Debian, Ubuntu, Fedora, Arch...) | Should work | Tested on Debian 13 with a simulated board (16 checks) and a virtual X11: applications through their .desktop files, active window, per-application layers, software keys, clipboard and the user service (12 checks). Not yet tried on a real desktop with a screen. |
| Linux with Wayland | Should work | Same as X11, with limits: per-application layers only work on Sway and Hyprland (GNOME and KDE do not reveal the active window) and software keys need wtype. With the board connected, shortcuts, text and media do not depend on any of this. |
| Windows 11 | Tested | Tested on a real Windows 11 with the board: typing through the board with the Spanish layout (11 checks), per-application layers, software keys and text, opening, minimising and closing applications, own popup and start with the session (scheduled task). With Smart App Control turned on Windows blocks unsigned .exe files: turn it off or sign the program. Windows 10 should work the same (untested). |
| KVM switch between the computer and the board | Tested | The board sits behind a KVM in the author's setup. |
| OBS Studio 28 or later (WebSocket control) | Tested | Tested with OBS 32.2.2 on macOS: scenes, mute, studio mode and password. The protocol is the same on Windows and Linux. |

## Mac keyboard layout

| Item | Status | Notes |
|---|---|---|
| Spanish ISO and US English | Tested | Text, symbols and accents tested against the real system. |
| Other QWERTY layouts (British, Latin American, Italian...) | Should work | Letters and digits through the board; punctuation is pasted from the clipboard (needs the Accessibility permission). |
| AZERTY, QWERTZ, Dvorak, Colemak | Should work | Shortcuts and text are sent by software (needs the Accessibility permission). |
