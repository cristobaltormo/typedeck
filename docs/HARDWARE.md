# Hardware

- **Arduino Leonardo** (ATmega32U4, native USB). Tested with a Keystudio Leonardo R3 (KS0248).
- **USB Host Shield 2.0** (MAX3421E) with an ICSP header. Tested with a Yanmis. The Leonardo only exposes SPI on the ICSP
  header, which is why the shield needs one.
- Any wired USB keyboard. The tested one (BY Tech, SINO WEALTH controller) declares 500 mA.

Stack the shield on the Leonardo, plug the keyboard into the shield and the Leonardo into the computer.

## Flashing

`scripts/flash.sh` compiles with `arduino-cli` and uploads with `avrdude` through a Mac (`MAC_HOST`, default `mac`), stopping
Typedeck while it works: the program would otherwise open the bootloader port and break the session. Prebuilt images of older
firmware are in `firmware/known-good/`.

To build and upload by hand: `arduino-cli compile --fqbn arduino:avr:leonardo firmware` and
`avrdude -p atmega32u4 -c avr109 -P <port> -U flash:w:firmware.ino.hex`. Pressing the board's reset button before uploading
puts it in the bootloader for about 8 seconds.

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

### Recommended fix: power the board separately

The cause looks like the keyboard and the board starting up at the same instant. The fix that removes it is to stop the board
from losing power when the KVM switches: feed the Leonardo from its DC jack (7 to 9 V, 5.5 × 2.1 mm, centre positive). The Leonardo
selects the higher source on its own, so the USB cable then only carries data, and a KVM switch just moves the data connection.

- Use a regulated 9 V adapter, 1 A or more (1.5 A is comfortable). The on-board regulator is linear, so it dissipates
  `(Vin - 5 V) × current`: at 9 V and 0.3 A that is about 1.2 W. Prefer 7.5 V when you can; touch the regulator after a minute and
  unplug if it is too hot to hold a finger on.
- Do not use 12 V adapters.
- A USB hub with its own power supply between the shield and the keyboard should work for the same reason (the keyboard never loses
  power), at the cost of the hub.

This is recommended, not yet verified on the reference setup.

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
