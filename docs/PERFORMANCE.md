# Performance

Measured on the reference setup (Arduino Leonardo, Yanmis USB Host Shield, BY Tech keyboard, behind a KVM, macOS on an Apple
silicon Mac). The numbers come from the tests in `tests/e2e/hardware.py`, which any setup can run.

| What | Result |
|---|---|
| Forwarding one keyboard report inside the board | about 0.13 ms on average, 0.2 ms at worst |
| A captured key reaching the program and starting its action | a few milliseconds over the serial link |
| Opening the editor from its key when it is already open | about 150 ms (the time the OS takes to run the tab lookup) |
| Program memory when idle | about 11 MB resident, 0 % CPU |
| First load of the editor | about 100 KB |

The board does not wait for the computer to type: reports are forwarded as they arrive, so typing feels the same with or without
Typedeck. The macOS service runs at interactive priority; at background priority, starting any program from an action took about
three times longer.

To measure your own board, run `STATS` from the Diagnostics page or `POST /api/board` with `{"cmd":"stats"}`: `avg_us` and `max_us`
are the internal latency since the last reading.
