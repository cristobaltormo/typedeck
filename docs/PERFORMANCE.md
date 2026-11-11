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

## Software capture (no board)

Measured in a Windows Server 2025 virtual machine with keys sent as hardware events by the hypervisor (the hook ignores injected
keys, so the numbers are real hook calls). The callback time is measured inside the program around the decision, and the 300 ms that
Windows allows is the limit that matters.

| What | Result |
|---|---|
| Hook callback per key (decision and queueing) | 16 to 52 µs on average, 0.8 to 1.5 ms at worst (limit: 300 ms) |
| Program memory when idle | about 20 MB resident, 0 % CPU |

On macOS the helper times its tap callback the same way and the program shows it in `stats`; it has not been measured yet because the
tap could not be installed on the machine used for development (the permissions need a click in System Settings). macOS disables a
tap whose callback is too slow, so the number to watch there is the worst case.

`POST /api/board` with `{"cmd":"stats"}` (the Diagnostics page) returns the number of keys seen, `avg_us` and `max_us` of the callback
and `dropped`, the events lost because the program fell behind (should stay 0).

On Linux the cost is a read, the decision and a write to the virtual keyboard for every key of the grabbed keyboards. Measured in a Kali
Linux 2026 virtual machine (4 vCPU, kernel 7.2) with `tests/e2e/linux_capture.py`: a virtual keyboard created through `uinput`
stands for the real one and a second reader on the output of Typedeck's virtual keyboard timestamps the key-down, 300 presses
20 ms apart, against the same reader on the test keyboard alone.

| What | Result |
|---|---|
| Key down to an ordinary reader, nothing in between (baseline) | median 76 to 85 µs, 99th percentile 104 to 650 µs |
| The same key through Typedeck | median 135 to 141 µs, 99th percentile 226 µs to 1.4 ms |
| Added by Typedeck | about 55 to 65 µs at the median |
| Inside the program, read to write | 40 to 56 µs on average, 0.15 to 0.8 ms at worst |
| Idle with the capture on | 0.00 to 0.05 % CPU, 15 to 18 MB resident, 9 to 10 threads |

The spread of the worst cases is the virtual machine's scheduler, not the program. It was measured on a virtual machine and with
virtual keyboards; the end-to-end check on X11 used the machine's emulated PS/2 keyboard, and nothing was measured on physical
hardware.
