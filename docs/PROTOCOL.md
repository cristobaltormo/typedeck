# Serial protocol (firmware 10)

115200 baud, one command per line (`\n`). The firmware answers with one line, except `INFO` and `RDESC`, which end in `END`.
Events can arrive at any time.

| Command | Reply | Description |
|---|---|---|
| `WHO` | `TYPEDECK-FW 10` | Identification. Typedeck finds the board with it |
| `HB` | none | Heartbeat. With none for 5 s the board stops capturing |
| `MASK <64 hex>` | `OK` | 256 bits: bit n captures HID usage n |
| `KEY <mods> <usage>` | `OK` | Tap a key. `mods` in hex: 01 Ctrl, 02 Shift, 04 Alt, 08 GUI, 40 right Alt (Alt Gr) |
| `CONS <usage>` | `OK` | Tap a media key (consumer page) |
| `INFO` | `vid=... pid=... mfr="..." prod="..." ...` then `END` | Details of the connected keyboard |
| `RDESC <iface> <len>` | hex dump then `END` | HID report descriptor of an interface |
| `SYS` | `mcu= f_cpu= board= fw= vcc_mv= free_ram= max3421e_rev= uptime_s=` | The board itself: real supply voltage, free memory, shield chip revision |
| `BUS` | `hrsl=0x.. estado=0x.. reinicios=N sin_teclado_s=N` | USB line state (HRSL: 0x80 J, 0x40 K, 0x00 nothing attached) and board restarts since the keyboard was last seen |
| `STATS` | `n= media_us= max_us= hid_listo= estado= captura=` | Internal latency and state |
| `WATCH 0\|1` | `OK` | Emit `W <usage>` for every key pressed |
| `VBUS 0\|1` | `OK` | Diagnostic only: drives the shield's VBUS switch. It does not cut the keyboard's power on the reference shield |
| `BOOTLOG` | `cold=N recoveries=N slow=N first_seen_ds=N this_boot=cold\|recovery` | Persistent boot statistics (EEPROM): cold starts, restarts caused by a missing keyboard, boots where the keyboard took over 5 s, deciseconds until the keyboard was first seen |
| `REBOOT` | `OK` | Restarts the board through the bootloader; the serial port disappears for about 20 s |
| `DARK 0\|1` | `OK` | 1 (default) keeps the TX/RX LEDs off; 0 hands them back to the USB core |
| `SIMQ <16 hex>` | `OK` | Simulated keyboard report, logic only (for tests) |
| `DBG 0\|1`, `LEDS <n>`, `LAYER <n>`, `L 0\|1` | `OK` | Debugging and the on-board LED |

Events: `D <usage> <mods>` (captured key down), `U <usage>` (up), `W <usage>` (observed), `K 1` and `K 0` (keyboard attached to or
removed from the shield), `RESET sin teclado` (the board is about to restart to recover the keyboard).

The firmware forwards two keyboard reports: the boot report (interface 1) and the consumer report (report 2 of interface 2).
It does not forward the NKRO mode, system control or the keyboard's built-in mouse yet; the current limit is 6 simultaneous keys.

## Keyboard recovery

After a power cut the shield sometimes stops seeing the keyboard (the bus reads as nothing attached while the supply voltage is
fine). Cycling the keyboard port power and resetting the shield chip do not help; passing through the bootloader, as when a
sketch is uploaded, does. Firmware 10 therefore restarts itself through the bootloader when no keyboard has shown up for 9 s, up
to three times in a row, and without limit once a keyboard has been seen on that board (a flag in EEPROM). It also retries a
failed shield initialisation instead of stopping. See [HARDWARE.md](HARDWARE.md) for the root cause and the fix that avoids it.
