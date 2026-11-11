# Limits

What Typedeck cannot do, by way of capture. The short version is in the [README](../README.md#limits).

## With the board

With the board there are six simultaneous keys (boot protocol, confirmed on the real keyboard) and no forwarding of the keyboard's built-in
mouse or system keys. Behind a KVM switch the keyboard can go missing after a power cut; the firmware recovers it by itself in under a minute and
[Hardware](HARDWARE.md) explains how to avoid it.

## Without the board: Windows software capture

- Windows does not show the hook keys typed into windows running as administrator or the UAC screen while Typedeck runs as a normal
  user, so macros do not work there. Running Typedeck as administrator lifts this for administrator windows, not for UAC.
- The anti-cheat of some games treats any global keyboard hook as suspicious and may refuse to start or flag it. Use the board for
  those games.
- The hook runs inside the computer, so the delay is higher than the 0.13 ms of the board (microseconds in the hook itself, plus the
  scheduling of the program). If the program freezes or is stopped, keys that have a macro type their own character until it is back.
- The limit of six simultaneous keys does not apply. Media keys, the keyboard's built-in system keys and keys of other keyboards are
  not captured, only the keys of the standard keyboard report.
- It sees the keys that have a macro, plus every key while detecting keys or with the typing history on; nothing
  else, and nothing leaves the computer. See [SECURITY.md](SECURITY.md).

## Without the board: macOS software capture

- macOS hides what you type into password fields (secure input) from every keyboard tool, so macros do not run there, and it
  stops showing keys to a tap while the screen is locked.
- It needs two permissions for the helper, Accessibility and Input Monitoring, and macOS may withdraw a tap that is too slow or after
  sleep; the helper turns it back on by itself and releases the keys it held.
- Caps Lock reaches a tap as a toggle, not as a key you can hold, so as a layer key it behaves as a tap; on the board it is a true
  held key. This comes from how macOS delivers it and has not been tried on a real keyboard yet.
- Typedeck sends its own keys with `CGEventPost` and media keys as system events; Caps Lock is switched through the HID system, so
  the short-press problem of the board does not apply.
- It sees only the keys that have a macro, plus every key while detecting keys or with the typing history on;
  the helper never sends anything else to the program and nothing leaves the computer. See [SECURITY.md](SECURITY.md).

## Without the board: Linux software capture

- It needs read access to every keyboard and permission to create a virtual one, which in practice means being in the `input`
  group. Anything running as a member of that group can read the keyboards, so see [SECURITY.md](SECURITY.md) before
  turning it on on a shared computer.
- It works below the desktop, so it also sees the lock screen and other virtual terminals: a macro can run while the screen is
  locked. Typedeck does not detect the lock.
- While it is on, the program is in the path of every key of the grabbed keyboards. If it dies, the kernel gives the keyboards back
  at once (tested with `kill -9`), but if it is frozen and not dead (`SIGSTOP`, a debugger) those keyboards do not respond until it
  resumes or you close it with the mouse or over ssh. Typedeck ignores `SIGTSTP`, so Ctrl+Z in the terminal that started it does not
  freeze it.
- The virtual keyboard replaces your keyboards for the desktop, so settings that match a keyboard by name (per-device layouts in
  Sway, for instance) must also match `Typedeck virtual keyboard`. Devices with a pointer in the same node, and nodes with no letter
  keys (media keys, power button, lid), are left alone.
- The virtual keyboard declares the keyboard keys 1 to 255 and 0x160 to 0x2BF (media and function keys such as `KEY_FN`), but not the
  button codes 0x100 to 0x15F or 0x2C0 and above, because declaring those makes the kernel and udev treat it as a mouse or a joystick;
  a key sent with one of those codes is not re-emitted. The ioctl numbers assume the generic Linux layout (x86, ARM, RISC-V), not MIPS or
  PowerPC. Text that the keyboard layout cannot type goes through the clipboard (`wl-copy` or `xclip` or `xsel`) and Ctrl+V.
- Delay: about 60 microseconds more at the median than a direct read (measured on a virtual machine, see
  [PERFORMANCE.md](PERFORMANCE.md)); idle it uses no CPU.


