# Serial protocol (firmware 4)

115200 baud, one command per line (`\n`). The firmware answers with one line, except `INFO` and `RDESC`, which end in `END`.
Events can arrive at any time.

| Command | Reply | Description |
|---|---|---|
| `WHO` | `TYPEDECK-FW 4` | Identification. Typedeck finds the board with it |
| `HB` | none | Heartbeat. With none for 5 s the board stops capturing |
| `MASK <64 hex>` | `OK` | 256 bits: bit n captures HID usage n |
| `KEY <mods> <usage>` | `OK` | Tap a key. `mods` in hex: 01 Ctrl, 02 Shift, 04 Alt, 08 GUI |
| `CONS <usage>` | `OK` | Tap a media key (consumer page) |
| `INFO` | `vid=... pid=... mfr="..." prod="..." ...` then `END` | Details of the connected keyboard |
| `RDESC <iface> <len>` | hex dump then `END` | HID report descriptor of an interface |
| `SYS` | `mcu= f_cpu= board= fw= vcc_mv= free_ram= max3421e_rev= uptime_s=` | The board itself: real supply voltage, free memory, shield chip revision |
| `STATS` | `n= media_us= max_us= hid_listo= estado= captura=` | Internal latency and state |
| `WATCH 0\|1` | `OK` | Emit `W <usage>` for every key pressed |
| `SIMQ <16 hex>` | `OK` | Simulated keyboard report, logic only (for tests) |
| `DBG 0\|1`, `LEDS <n>`, `LAYER <n>`, `L 0\|1` | `OK` | Debugging and the on-board LED |

Events: `D <usage> <mods>` (captured key down), `U <usage>` (up), `W <usage>` (observed), `K 1` and `K 0` (keyboard attached to or
removed from the shield).

The firmware forwards two keyboard reports: the boot report (interface 1) and the consumer report (report 2 of interface 2).
It does not forward the NKRO mode, system control or the keyboard's built-in mouse yet; the current limit is 6 simultaneous keys.
