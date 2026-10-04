# Hardware

- **Arduino Leonardo** (ATmega32U4, native USB). Tested with a Keystudio Leonardo R3 (KS0248).
- **USB Host Shield 2.0** (MAX3421E) with an ICSP header. Tested with a Yanmis. The Leonardo only exposes SPI on the ICSP
  header, which is why the shield needs one.
- Any wired USB keyboard. The tested one (BY Tech, SINO WEALTH controller) declares 500 mA.

Stack the shield on the Leonardo, plug the keyboard into the shield and the Leonardo into the computer.

## Flashing

The board is flashed once, and again when a release says the firmware changed (the number is shown by `typedeck doctor`; the
current one is 14). Every release attaches the ready-made image, `typedeck-firmware-<number>.hex`, so nothing has to be compiled.
It works the same on Windows, Linux and macOS, with [`arduino-cli`](https://arduino.github.io/arduino-cli/latest/installation/),
which brings `avrdude` along:

1. Stop Typedeck, because it holds the board's serial port and would break the upload:
   macOS `launchctl bootout gui/$(id -u)/cc.cristobal.typedeck`, Linux `systemctl --user stop typedeck`,
   Windows `Stop-ScheduledTask Typedeck; Stop-Process -Name typedeck` in PowerShell.
2. Install the AVR tools once: `arduino-cli core install arduino:avr`.
3. Find the port (`arduino-cli board list`: `COM4`, `/dev/ttyACM0`, `/dev/cu.usbmodem1101`) and upload:

```sh
arduino-cli upload --fqbn arduino:avr:leonardo --port <port> --input-file typedeck-firmware-14.hex
```

4. Start Typedeck again (log in again, or `Start-ScheduledTask Typedeck` / `systemctl --user start typedeck`) and run `typedeck doctor`.

`arduino-cli` resets the board into its bootloader by itself. If the upload says the port is not found, press the board's reset
button and run the command again within 8 seconds: the bootloader shows up as a different port for that long. With plain
`avrdude` the command is `avrdude -p atmega32u4 -c avr109 -P <bootloader port> -b 57600 -U flash:w:typedeck-firmware-14.hex:i`.
On Linux without the udev rule from the README the port may need `sudo` or the `dialout` group.

To compile it yourself: `arduino-cli compile --fqbn arduino:avr:leonardo firmware`. `scripts/flash.sh` (`make flash`) does the
whole cycle for the author's setup: it compiles here and uploads through a Mac (`MAC_HOST`, default `mac`) over ssh. Prebuilt images
of older firmware are in `firmware/known-good/`.

## Power

The keyboard is powered by the shield, which takes 5 V from the Leonardo, which takes it from the computer's USB port. Everything
therefore has to fit in what that port can give, and every time that port loses power the board, the shield and the keyboard
all restart at once.

### What happens behind a KVM switch

Observed on the reference setup (Leonardo behind a USB KVM switch shared by a Mac and a Windows PC). Switching the KVM cuts
power to the Leonardo port. The board comes back up, the supply voltage is normal (5.2 V), the shield chip initialises, but the
USB line reads as empty (`hrsl=0x03`, nothing attached) and the keyboard is dead. It happened on most switches.

What was tried, with the same keyboard connected throughout:

| Attempt | Result |
|---|---|
| Power cycling the keyboard port from the firmware, 0.6 s, 2.5 s, 6 s, 20 s and 60 s | No keyboard. Later found to do nothing physical: with VBUS "off" for 5 s the shield stayed in the running state and the keyboard stayed connected |
| Resetting the shield chip each time | No keyboard |
| Waiting 6 s with the keyboard powered before touching the shield chip | No keyboard |
| Immediate microcontroller reset (watchdog) | No keyboard |
| Passing through the bootloader (8 s stopped, as when uploading a sketch) | Keyboard back, sometimes on the second attempt |
| Unplugging everything for a minute | Keyboard back |

The bootloader restart is what firmware 10 does automatically, which recovers the keyboard after 20 to 40 seconds. It is a
safety net, not instant.

### Powering the board from the DC jack did not fix it

The obvious idea is to feed the Leonardo from its DC jack (7 to 9 V, 5.5 × 2.1 mm, centre positive) so the board never loses power when
the KVM switches. It was tried on the reference setup with a regulated 9 V 1.5 A adapter and it made things worse: the board went into a
restart loop every 8 to 10 seconds (the firmware restarts it when it sees no keyboard) and the keyboard never enumerated, with the
keyboard backlight on or off. The cause was not found (the 5 V rail was not measured). Do not rely on it, and if you try it, measure the
5 V pin and touch the on-board regulator after a minute. Do not use 12 V adapters.

Two other ideas are untested here: a USB hub with its own power supply between the shield and the keyboard (the firmware does not support
hubs behind the shield today), and a power switch on the keyboard's 5 V line.

Since the shield's VBUS switch does not cut the keyboard (check whether your clone bridges the switch with a solder jumper), the firmware
cannot power-cycle the keyboard itself. What does reproduce a good start by hand is plugging the keyboard into the shield *after* the board
is powered, so a self-powered hub between shield and keyboard, or a power switch on the keyboard line, would automate it.

## Lights

Firmware 11 keeps the TX and RX LEDs off (`DARK`) and the L LED only blinks when asked. The green ON LED is wired to the 5 V rail and
nothing in software can switch it; cover it with opaque tape or a drop of nail polish. Unplugging the board is the only way to cut it
besides that.

## Known quirks

- The `USB Host Shield 2.0` library does not tag reports with an id; the firmware therefore derives from `HIDComposite` and tells
  the interfaces apart by endpoint.
- Keyboards with backlights may not light them through the shield: the shield cannot supply what they ask for.
- Slightly bent shield pins did not cause symptoms, but re-seat the stack if the shield stops answering over SPI.
