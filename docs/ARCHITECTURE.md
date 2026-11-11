# Architecture

```
 USB keyboard --> USB Host Shield (MAX3421E) --> Arduino Leonardo --> computer (USB)
                                                     |  ^
                                         CDC serial  |  |  events / commands
                                                     v  |
                                              typedeck (Go)  <-- editor (127.0.0.1:7788)
                                              actions, layers, gestures, stats
```

## Who does what

| Part | Language | Job | Why there |
|---|---|---|---|
| `firmware/` | C++ (Arduino) | Reads the keyboard, forwards it to the computer, captures keys that have a macro, injects keystrokes | This is the part that has to be instant: about 130 µs per report, never through the computer |
| `cmd/`, `internal/` | Go | Gestures, layers, actions, API, statistics | One static binary, no dependencies, about 11 MB resident, 0 % CPU when idle |
| `web/ui/` | HTML, CSS and JS, no dependencies | The editor | About 100 KB on first load, no framework, nothing fetched from the network |
| `macos/hud/` | Swift | Floating popup on macOS | A native window that only exists while it is shown |
| `macos/capture/` | Swift | Software capture on macOS (`typedeck-capture`) | Started and watched by the program, it ends when the program's standard input closes |

Typing normally never depends on Typedeck: the board forwards the keyboard on its own, even when the program is not running.

The board is one of two ways to capture keys. On Windows, macOS and Linux the other is software capture, where a keyboard hook inside the
program (Windows), a helper process with an event tap (macOS) or a grab of the kernel's input devices (Linux) does what the board does. On Windows:

```
 any keyboard --> Windows --> WH_KEYBOARD_LL hook (typedeck) --> applications
                                      |  ^
                      swallowed keys  |  |  injected keys (SendInput)
                                      v  |
                          the same engine, layers, gestures, actions and editor
```

## Packages

| Package | Responsibility |
|---|---|
| `internal/capture` | Where key events come from. `Source` is the interface the engine, the server and `main` depend on; `board.Board` implements it for the hardware and `SoftwareSource` for the keyboard hook. The `Selector` holds both, switches between them at run time and keeps the idle one silent. Also the hook state machine (`Filter`), the scancode to HID usage tables, the Windows backend, the macOS backend (`darwin_*.go`, which drives the Swift helper) and the Linux backend (`backend_linux.go` and `linux_*.go`, plain `syscall` and `ioctl`, no cgo), with the evdev to HID usage table in `evdev_codes.go` |
| `internal/board` | Serial link to the board: detection by `WHO`, heartbeat, capture mask, events, `INFO`, `SYS`, `BUS`. Native serial ports for macOS, Linux and Windows |
| `internal/engine` | Gestures, layers, per-application layers, debounce, statistics |
| `internal/actions` | Runs the actions. Sends keys through the board and delegates everything OS-specific to `platform` |
| `internal/platform` | One implementation per OS (macOS, Linux, Windows): launch and quit apps, front window, software keys, clipboard, sleep, lock, notifications, floating popup, auto-start service. macOS key compensation lives here |
| `internal/obs` | A minimal RFC 6455 WebSocket client and the obs-websocket 5 requests, no dependencies |
| `internal/layouts` | Physical layouts (100, 80, 75, 65, 60 %, ISO and ANSI, numpad) and the algorithm that guesses which one a keyboard has from the keys observed |
| `internal/kbdb` | Brand and model: the name the keyboard reports, vendor by VID, verified models, form factor from name keywords |
| `internal/setup` | Setup summary, layout choice and compatibility checks (`/api/setup`) |
| `internal/packs` | Gallery packs, laid out on the keys the keyboard really has, adapted per OS |
| `internal/config` | Schema v3, migration from v1 and v2, validation, backups and recovery |
| `internal/hid` | USB key tables, text layouts (US, Spanish for macOS, Linux and Windows) and a report descriptor parser |
| `internal/server` | Loopback HTTP server: static editor and JSON API, a WebSocket for live events, the editing flag and macro file review |
| `internal/keylog` | The optional typing history: key, time and duration in a local file, retention, summary and the text rebuilt from the presses |

## How the layout is decided

1. What the user picked for that keyboard (VID:PID).
2. A verified model in `internal/kbdb/db.json` (needs exact vendor and product: generic controller PIDs are reused).
3. The form factor in the keyboard name ("TKL", "60 %", "75 %"...).
4. ISO or ANSI from the keyboard type the OS records, when it records one.

The key assistant watches what you press and scores the 11 layouts with the harmonic mean of precision and coverage. When ISO
and ANSI cannot be told apart it says which keys to press (`<` and `ç`). Many ISO controllers send `0x31` for the key next to
Enter instead of `0x32`; that is accounted for.

## Capture and fail-open

1. Typedeck computes the keys that have a macro on the active layer (plus global ones) and sends `MASK <256 bits>`.
2. The board removes those keys from the report it forwards and sends `D <usage> <mods>` and `U <usage>` instead.
3. Typedeck sends `HB` every second. With no heartbeat for 5 s the board stops capturing and every key types its own
   character again. A crashed program can never leave the keyboard unusable.
4. A layer change sends a new mask (one 66-byte line).
5. A key that only has "hold" or "double" forwards the original key on a short press, so it never goes dead.

## Software capture

`capture.Source` is what the engine needs from whatever feeds it, and it is made of two small interfaces: `Capturer` (a capture mask in
with `SyncMask`, key events out as `D`, `U`, `P`, `R` with the HID usage and the modifier byte exactly as the board sends them, "report
every key" with `Keys`, connection state, `Pause` and `Resume`) and `Injector` (`Tap`, `TapHeld`, `Consumer`). What only the board has is
a separate interface each, found with a type assertion where it is used: `LayerSetter` (the layer for the board's LEDs), `Commander`,
`RawDoer` and `SysReporter`. The software source implements `Commander` for `ping` and `stats` only. Nothing in the engine knows which
source is behind it.

The `Selector` holds both sources and routes to the active one. `Select` is serialised, and if the software source cannot start it
goes back to the board by itself (the engine also tells the user with a notice). `Pause` and `Resume` go to both sources, so a mode
change between them can never leave one paused. The software source tells the engine about state changes (connected, disconnected)
through the same goroutine that delivers key events, in order, and retries a failed start with a growing delay (4 s, doubling up to
2 minutes) instead of every 4 s, which on Linux would have created a virtual keyboard each time and on macOS launched the helper.

`config.Settings.Capture` chooses the source: `board` (the default, and what every existing configuration gets) or `software`. It is a
different setting from `Input`, which only decides how typed output leaves the program (`auto`, `hardware` or `software`); with
software capture the editor hides `Input` because the program always sends its own keys. Software capture never turns on by itself and
the configuration endpoint ignores changes to it: the editor asks for explicit consent and calls `POST /api/capture`, because a
keyboard hook looks exactly like a keylogger. That call also writes `capture-consent.json` (date and version of the text), outside
`config.json` so a backup or an imported file cannot carry it; the engine turns a `software` mode without that record into `board` at
start-up, restore and every configuration change. When no board shows up for a few seconds the editor offers it.

How the Windows backend works:

1. One goroutine locked to an OS thread installs a `WH_KEYBOARD_LL` hook and runs a message loop. The hook callback is created once
   (every `syscall.NewCallback` takes a slot that is never released). The hook handle and the thread id belong to that thread's
   own state, and `Start` always waits for it to be ready or finished: if it takes more than 3 s the thread is cancelled and
   `Start` returns an error, so no orphan hook is left behind. If the hook cannot be reinstalled the thread ends, the backend stops
   being alive and the supervisor starts it again.
2. The callback must return in well under 300 ms or Windows removes the hook, so it only reads the mask (four atomic words), takes a
   short lock to update a per-key state table and pushes the event into a buffered channel without ever blocking. A pump goroutine
   delivers the events to the engine. Measured, the callback takes a few microseconds.
3. Events marked `LLKHF_INJECTED` (including the program's own `SendInput`, which also carries a private `dwExtraInfo`) are ignored, so
   injecting a key can never loop. Only keys in the active mask are swallowed. A swallowed key-down also swallows its auto-repeats and
   its key-up even if the mask changed in between, and a key the system already saw going down keeps passing until it is released, so
   no key is ever left stuck. Keys outside the mask pass without the program reading them unless "report every key" is on, which has the
   same rules as `KEYS 1` on the board when the board is the source (editor open, detecting keys or history on). With software capture it
   is stricter: only while detecting keys or with the history on, because otherwise any open WebSocket would be enough to make the program read every key.
4. `scancode` plus the extended flag becomes a HID usage with an own table (`capture/scancodes.go`) and back for injection. Details that
   matter: the key next to Enter is reported as `0x31`, as many ISO controllers do; Pause, Num Lock and Print Screen are told apart
   by virtual key because they share scancodes; the fake shifts Windows invents around the navigation keys and the phantom left Ctrl of
   Alt Gr (scancode `0x21D`) are ignored, so Alt Gr is just right Alt.
5. Injection uses `SendInput` with scancodes (`KEYEVENTF_SCANCODE`, and `KEYEVENTF_EXTENDEDKEY` where needed) so the layout of the
   system turns them into characters exactly as with a physical key, modifiers go down in one call and come up in another after the
   hold time, and text the layout cannot type goes out with `KEYEVENTF_UNICODE`.
6. Fail-open: the hook belongs to the process, so killing it leaves the keyboard normal at once. Lock, disconnect and suspend arrive
   through one path only, the tray window of the program (`internal/platform`), which calls `Pause` on the selector: the filter lets
   everything pass and releases what it holds. `Resume` brings it back and asks the backend to install the hook again. A
   watchdog checks every 5 s, only while you are using the computer and the hook has been silent for 10 s, by injecting an unassigned
   virtual key with a private marker and reinstalling the hook if it does not see it.

### macOS backend

Go cannot call CoreGraphics without cgo and the binary stays `CGO_ENABLED=0`, so the tap lives in `macos/capture/capture.swift`, built
as `typedeck-capture` and found next to the `typedeck` binary (symlinks resolved). `capture.macBackend` starts it with `run`, in the
same spirit as the board protocol: plain text lines, one command or event per line.

| Program to helper | Meaning |
|---|---|
| `CFG <32 hex> <reportAll> <active>` | the mask as a bitmap of macOS key codes (128 bits), report every key, paused or not |
| `KEY d\|u <code> <flags hex>` | post a key event; modifiers are separate `flagsChanged` events with the accumulated flags |
| `CAPS`, `MEDIA <n>`, `TEXT <hex utf-8>`, `QUIT` | toggle Caps Lock, media key (`NX_KEYTYPE_*`), Unicode text, stop |

| Helper to program | Meaning |
|---|---|
| `READY <ansi\|iso\|jis>` or `ERR permission accessibility=0\|1 input_monitoring=0\|1` | first line; the helper exits with status 3 on `ERR` |
| `D <code> <flags hex>`, `U`, `P`, `R <code>` | the events of `capture.Filter`, with the device flags of the modifiers on `D` |
| `KBTYPE <layout>`, `TAPOFF <reason>`, `TAPON`, `LAT <n> <avg us> <max us>` | keyboard layout change, tap disabled and re-enabled, callback timing |

1. The table of key codes lives only in Go (`capture/darwin_keymap.go`): the program converts the usage mask to a code mask and the
   events back, so the helper knows nothing about HID and the table is tested on any system. The two keys that ISO keyboards add
   (`0x35` and `0x64`) swap codes 10 and 50 depending on the layout type macOS reports, the same crossing
   `platform/darwin_quirks.go` compensates when typing; the layout comes from the helper (`READY` and `KBTYPE`).
2. The decision runs in the helper, in the tap callback, with the same state machine as `Filter` (held, swallowed, reported): only
   codes in the mask are swallowed and reported, everything else passes without being sent anywhere. `Filter.Observe` feeds the helper's
   events into the Go filter, so `Release`, the event channel and `dropped` are shared with Windows, and `Filter.Changed` tells the
   backend to send a new `CFG` when the mask, report-all or pause change.
3. A swallowed modifier (`flagsChanged`) hides its flag from the events that follow, since macOS computes flags before the tap.
   Caps Lock arrives as one `flagsChanged` per toggle: a masked Caps Lock is reported as `D` then `U`, swallowed, and the HID Caps Lock
   state is switched off again through IOKit. Injecting Caps Lock toggles the same state, so the quirk that macOS ignores a short
   Caps Lock press (the firmware holds it 150 ms for that) does not apply here.
4. The helper marks its own `CGEventPost` events (`eventSourceUserData`) and ignores them. Text goes out as Unicode strings in
   chunks of at most 16 UTF-16 units; media keys are `systemDefined` events. Typedeck's own `AdaptStroke` compensation (modifier
   remap and key swap of the board's keyboard) is skipped in software mode because those quirks belong to the board.
5. Fail-open: the tap belongs to the helper. The helper exits when its standard input closes (the program died or stopped it) and on
   `QUIT`; the backend sends `QUIT`, closes the pipe and kills it after 2 s. When it dies, `Filter.Release` lets go of held keys. macOS
   disables a slow tap (`tapDisabledByTimeout`) or one interrupted by the user; the helper releases what it held, enables it again,
   and reports `TAPOFF` and `TAPON`. It also does so after wake and unlock, and a 3 s timer checks `tapIsEnabled`. If macOS keeps the
   tap disabled the helper reports `ERR` and exits, and the program retries every 4 s as with any failed start.
6. Permissions: Accessibility and Input Monitoring, requested only when software capture is turned on (`run --prompt`, once per
   process; later retries do not prompt). TCC attributes a permission to the "responsible process", normally the program that
   launched the helper (a terminal, or `typedeck` under launchd), so the helper re-launches itself once with
   `responsibility_spawnattrs_setdisclaim` and becomes its own responsible process: the grant belongs to `typedeck-capture` wherever
   it is started from. `typedeck-capture --check` reports the permissions and whether a tap can be created, without prompting, and
   is what `Checks()` and `typedeck doctor` use.
7. Packaging: `make macos-helpers` builds it (and the popup helper) with `swiftc` for arm64 and x86_64, joins them with `lipo` and
   signs them ad hoc with a fixed identifier (`cc.cristobal.typedeck.capture`); CI and the release do it on a macOS runner. Ad hoc
   means no Apple developer account and no notarization. `typedeck-capture --selftest` runs the state machine against synthetic
   events and needs no permission.

### Linux backend

Linux has no hook API, but every keyboard is an evdev node, so the backend works under the compositor and X11 and Wayland behave the
same. No library and no cgo: `syscall` and `ioctl` with the numbers of the generic Linux layout (checked by a test).

```
 keyboard --> /dev/input/eventN (grabbed, EVIOCGRAB) --> typedeck --> /dev/uinput "Typedeck virtual keyboard" --> libinput, X, console
                                                           |  ^
                                           swallowed keys  |  |  injected keys, paste, media keys
                                                           v  |
                                         the same engine, layers, gestures, actions and editor
```

1. Which devices: every `event*` node that has EV_KEY with all 26 letters and neither EV_REL nor EV_ABS, and whose `phys` is not the
   marker of Typedeck's own virtual keyboard. Media-only nodes, the power button, the lid switch and mice (also a keyboard with a
   touchpad in the same node) are never opened for grabbing, so they keep working untouched.
2. Grab all the time, not on demand. A key that has a macro has to be removed before any other program reads it, and an event that
   reached a compositor cannot be taken back, so taking the key only when it arrives is too late for the first press. Grabbing only
   while the mask is not empty would save nothing real and would add a grab and release in the middle of typing every time a layer
   changes. So every matching keyboard is grabbed while software capture is on, and every event is passed on or swallowed.
3. Before grabbing, the backend waits until the keyboard has no key down (`EVIOCGKEY`), then drains the events queued while waiting.
   Grabbing with a key held would leave that key down in the compositor, which would auto-repeat it forever. If the grab itself
   fails (`EBUSY`, another program holds the device) that keyboard is not counted as captured: the failure is kept for the session and
   shown by `typedeck doctor` and as a warning in the editor's setup. The access to `/dev/input` is checked before the virtual keyboard is
   created, so a missing permission never leaves a new virtual device behind on every retry.
4. One virtual keyboard for everything (`UI_DEV_SETUP`, bus virtual, keys 1 to 255 and 0x160 to 0x2BF, LEDs, `MSC_SCAN`, `phys=typedeck-virtual`). The
   physical events are read a packet at a time (up to `SYN_REPORT`); swallowed `EV_KEY` events (and the `MSC_SCAN` before them) are
   removed from the packet and the rest is written in one `write`, so modifiers, order, `MSC_SCAN` and the rest pass untouched.
   The virtual device does not declare `EV_REP`, because the kernel then repeats on its own on top of the repeats being passed
   on (value 2), which would double them; the console and the desktops repeat as with a real keyboard. Using one device for all
   keyboards also puts the program's own keystrokes in the same queue as the typed ones, in order. The price is that settings that
   match a keyboard by name see `Typedeck virtual keyboard` instead.
5. The same rules as the other backends: `Filter.Handle` decides on the press (value 1) and the release (value 0). A press that was
   swallowed keeps swallowing its auto-repeats (value 2) and its release even if the mask changed or the filter paused in between,
   and a press that was passed on keeps passing, so a key can never be left stuck on the virtual keyboard. Keys with no HID usage
   (most media keys) are passed on without being looked at.
6. LEDs: the compositor or the console writes `EV_LED` to the virtual keyboard; the backend copies it to every grabbed keyboard. A
   grabbed device does not receive LED changes from anyone else.
7. Hotplug: an inotify watch on `/dev/input` (create, delete, attribute change) triggers a rescan 150 ms later, and while some node
   was refused for lack of permission it retries every 3 s, because udev sets the permissions after the node appears. Unplugging a
   keyboard with a key held releases that key on the virtual keyboard. `SYN_DROPPED` discards the packet and compares `EVIOCGKEY`
   with what the backend believes is down, passing on the difference.
8. Injection: the program's own strokes, media keys and the Ctrl+V of the paste go out through the same virtual keyboard
   (modifiers down in one write, key, the hold time, then everything up in reverse), so they never loop (the virtual keyboard is never
   grabbed). Text that the layout can type is typed that way; text it cannot goes through the clipboard (`wl-copy`, `xclip` or `xsel`
   as for the board) and the Ctrl+V comes from the virtual keyboard instead of `xdotool` or `wtype`.
9. Fail-open: the grab belongs to the file descriptor, so when the process dies, however it dies, the kernel releases the grab as the
   descriptors close and the virtual keyboard disappears with them (measured: with `SIGKILL` the keys reached an ordinary reader 2 ms
   after the signal). A stop from the program ungrabs and releases what it held. `SIGTSTP` is ignored while the capture runs, because Ctrl+Z
   in the terminal that started Typedeck would otherwise freeze it. What cannot be solved from inside is a frozen but living process
   (`SIGSTOP`, a debugger): the grabbed keyboards do not respond until it resumes or is killed.
10. Permissions: the grab needs read and write access to the `event*` nodes and `/dev/uinput`. The package ships a udev rule that gives
    `/dev/uinput` to the `input` group (the `event*` nodes already belong to it on common distributions) and a `modules-load.d` file
    for `uinput`. `Checks()` opens `/dev/uinput` and every `event*` node without grabbing or creating anything, and reports what is
    missing with the command that fixes it. When the backend cannot start, the error carries a code (`no_uinput`, `uinput_access`,
    `input_access`) that the setup page shows in the editor's language.

Not covered: nothing tells the backend that the screen is locked or that another virtual terminal is in front, so macros keep
running there; devices that another program already grabbed (a remapper such as keyd) cannot be grabbed twice and are skipped (the
remapper's own virtual keyboard is grabbed instead, since it is the one the desktop sees); a keyboard held down for the whole of the
start-up never gets grabbed.

Porting it to another system means implementing `capture.Backend` (start and stop a global key tap feeding a `capture.Filter`, inject
keys, media keys and text, report checks for `doctor`) and nothing else: the selection, the engine, the editor and the tests of the
state machine are already shared.

Limits that come from the platform and cannot be fixed in the program: a hook of a normal user does not see keys typed into elevated
windows or the UAC and lock screens; the anti-cheat of some games rejects global hooks; and a hook adds the scheduling latency of the
program, which the board does not have.

## Gestures

Tap runs on key down when there is no hold or double; with hold, on release before the threshold; with double, after the double
tap window. A `layer` action with `momentary` activates the layer while the key is held, and also the moment another captured key goes down; for that
the capture mask includes the keys of the layers a held key can reach, and a captured key with no action in the layer in force is typed back.

## The live connection

The editor opens a WebSocket (`/api/ws`) that carries events, key presses for the on-screen keyboard and one message back: whether
a text field of the editor has focus. While it does, the program releases the capture mask, so typing the letter of a key that has a
macro types it instead of running the macro; the mask comes back when the field loses focus, when the window does, or when the
connection closes. The board reports every other key (`KEYS 1`) only while an editor is connected or the history is on.

## Typing through the board

`KEY <mods> <usage>` taps a key and `CONS <usage>` a media key. Typedeck turns text into keystrokes with the layout of the
computer (US, and Spanish for macOS, Linux or Windows) and the OS layer adapts them:

- **macOS:** per-device modifier remapping (`com.apple.keyboard.modifiermapping.*`) and the keyboard type (`com.apple.keyboardtype`)
  that crosses the `<` and `º` keys when a board registered as ANSI carries an ISO layout.
- **Windows and Linux:** Spanish text uses Alt Gr (right Alt); on Windows `Alt Gr+4` is a dead key, so `~` takes a space after it.
- Layouts where letters move (AZERTY, Dvorak...) never type through the board: text falls back to the clipboard.

## Platform layer

`platform.Current` is chosen at build time (`darwin.go`, `linux.go`, `windows.go`). Everything the rest of the program needs
from the OS goes through that interface, so adding a system means implementing it once. Linux discovers applications from
`.desktop` files and the focused window from Hyprland, Sway or X11 (xprop, then xdotool); Windows uses user32 and kernel32
directly (no PowerShell), a scheduled task for auto-start, and its own popup window instead of system notifications.

Details that are easy to break:

- Child processes that inherit the output pipe must not keep the program waiting: commands run with a timeout that also kills the
  children, and launching a graphical application does not collect its output (a quick failure is reported, still running after 3 s
  counts as launched).
- Linux finds the focused window on Hyprland, Sway and X11 (EWMH `_NET_ACTIVE_WINDOW`, then the input focus when no window manager
  sets it). GNOME and KDE on Wayland expose nothing, so per-application layers are unavailable there.
- On macOS the board's keyboard type is 40 (ANSI) or 41 (ISO). With ANSI and an ISO layout, macOS swaps the two keys that do not
  exist on ANSI (usages `0x35` and `0x64`), which the key compensation undoes.
- Windows builds use the GUI subsystem so the program starts with the session without opening a window; on start it reattaches
  to the parent console so output redirected over ssh or into files still works. Console windows of child processes are hidden, and
  `cmd /C` commands are passed raw because Go escapes quotes for the C runtime, which `cmd.exe` does not understand. Quoting for
  `cmd.exe` drops quotes, `%` and `^` because those are the characters that stay active inside double quotes.
- Quitting an application on Windows asks nicely: `taskkill` by executable first, then `WM_CLOSE` to every visible window whose
  title matches, because Store apps ignore `taskkill` without `/F`.
- The Windows serial port is opened raw at 115200 baud with DTR asserted, because the board only talks when DTR is on; reads time
  out after 100 ms and loop so closing the port never waits on a blocked read. Ports are listed from the registry
  (`HKLM\HARDWARE\DEVICEMAP\SERIALCOMM`, usbser.sys only), skipping Bluetooth and real ports that could stall the probe.
- When the per-application rules change, they are evaluated again against the window that is already in front.

## Firmware notes

- The sketch defines `protected` as `public` before including the USB libraries because `BootKeyboard` keeps its report in a protected
  member that has to be forwarded as is. Report ids 1 to 3 are rollover errors, never keys.
- It listens to both keyboard interfaces: endpoint 1 is the boot keyboard and endpoint 2 carries the reports that have an id (media
  keys, system control, the full keyboard, the mouse).
- `HOLD <usage>` keeps a key down until `RELEASE`. Each `HOLD` re-arms an 8 s deadline and the key is also released when the heartbeat
  stops, so a crashed host cannot leave a key stuck. Held keys are kept apart from the injected ones because `tapKey` clears those.
- When no keyboard shows up the board restarts through the bootloader, as the Arduino core does to upload a sketch: it waits there for
  about 8 s and then starts the sketch. After a power cut (a KVM switch) neither resetting the shield chip nor an immediate restart
  recovers the keyboard, but this does. It happens at most 3 times in a row unless a keyboard was ever seen, and the counters survive the
  restart. Cutting the keyboard port power from the firmware does nothing on the reference shield.
- `DARK` turns the RX and TX LEDs into inputs because the USB core keeps toggling them.

## Windows notes

- `typedeck.exe` uses the GUI subsystem so that starting it with the session shows no console; a GUI program is not waited for by cmd
  or PowerShell, so the exit code of `doctor` would be lost. The release therefore also ships `typedeck-cli.exe`, built from the same
  source for the console subsystem.
- A hidden top level window (class `TypedeckShell`, 0 by 0, not in the taskbar) owns the notification icon and receives `WM_CLOSE`,
  `WM_QUERYENDSESSION`/`WM_ENDSESSION`, `WM_POWERBROADCAST` and session change messages. `taskkill` without `/F` only closes
  processes that have a visible window, which is why that window is shown but has no size. On close or sign out the board mask is
  released and the statistics are saved; on lock or suspend the mask is released and taken again afterwards.
- One copy per configuration folder: the mutex is `Local\Typedeck-<hash of the configuration folder>`, so tests can run with their own
  `APPDATA` next to a real copy.
- The serial port is opened with `FILE_FLAG_OVERLAPPED`. A synchronous handle serialises I/O: a read waiting for its 100 ms timeout
  would hold back the heartbeat or a mask update written from another goroutine. Each direction owns its buffer and its
  `OVERLAPPED`, kept on the heap because the kernel writes into them. A write that completes short is an `io.ErrShortWrite`.
- Ports are chosen by USB vendor and product id from `HKLM\SYSTEM\CurrentControlSet\Enum\USB`, crossed with `SERIALCOMM` so
  that stale entries of unplugged boards are ignored. Probing other serial devices could reset them.
- Typed text is pasted with delayed clipboard rendering: the clipboard only receives the text when the target program asks for it
  (`WM_RENDERFORMAT`), and the previous text is restored right after, instead of after a fixed sleep. A clipboard that holds anything
  other than plain text (an image, files) is never touched: the text is typed as Unicode input instead. Pasted text is excluded from the
  Windows clipboard history and the cloud clipboard.
- `cmd` metacharacters in `{clipboard}` are escaped with carets and the quotes made literal, so nothing is dropped silently.
- Keys are sent with their scan code and the extended flag from a table (`isExtendedVK`); the keyboard layout is the one of the
  window in front: the layout id comes from the high word of the HKL and the variants (`0xF0xx`) are resolved in the registry, and
  only the Spanish (Spain) layout `0000040A` selects `es-win`.
- The executable carries a manifest (as invoker, Windows 10 and 11, per monitor DPI v2, long paths, UTF-8 code page), version
  information and the icon, generated by `packaging/windows/mkres` into `cmd/typedeck/rsrc_windows_<arch>.syso` by `make winres`; there
  are no new dependencies in the binary. The icon is the project logo (`packaging/windows/typedeck.svg`) in `typedeck.ico`.

## Security

See [SECURITY.md](SECURITY.md).
