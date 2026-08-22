# Hardware

- **Arduino Leonardo** (ATmega32U4, native USB). Tested with a Keystudio Leonardo R3 (KS0248).
- **USB Host Shield 2.0** (MAX3421E) with an ICSP header. Tested with a Yanmis. The Leonardo only exposes SPI on the ICSP
  header, which is why the shield needs one.
- Any wired USB keyboard. If it draws a lot of current (backlit keyboards ask for up to 500 mA), plug the board into a USB port
  that can supply it.

## Flashing

`scripts/flash.sh` compiles with `arduino-cli` and uploads with `avrdude` through a Mac, stopping Typedeck while it works: the
program would otherwise open the bootloader port and break the session.

## Known quirks

- After moving the board to another USB port the keyboard may not start: the firmware power cycles the shield at boot and every
  15 s while no device is attached.
- The `USB Host Shield 2.0` library does not tag reports with an id; the firmware therefore derives from `HIDComposite` and tells
  the interfaces apart by endpoint.
