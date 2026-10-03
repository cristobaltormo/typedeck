# Firmware

The sketch for the Arduino Leonardo (ATmega32U4) that forwards the keyboard on the USB Host Shield to the computer and reports the
keys that have a macro. The serial protocol is in [../docs/PROTOCOL.md](../docs/PROTOCOL.md).

## Build and flash

```sh
arduino-cli core install arduino:avr
arduino-cli lib install "USB Host Shield Library 2.0" "HID-Project"
arduino-cli compile --fqbn arduino:avr:leonardo --output-dir /tmp/typedeck-fw firmware
```

`scripts/flash.sh` compiles and uploads it through a Mac (see [../docs/HARDWARE.md](../docs/HARDWARE.md)); by hand,
`avrdude -p atmega32u4 -c avr109 -P <port> -U flash:w:firmware.ino.hex` after pressing the board's reset button.

`known-good/` holds images of older versions that were verified on real hardware.

## License

GPL-2.0, because it links the USB Host Shield Library 2.0 (GPL). The rest of the repository is MIT; see [../NOTICE](../NOTICE).
