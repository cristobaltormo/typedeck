# Macros

Every key can have three actions: **tap**, **hold** and **double tap**. A key with only a hold or double action still types its own
character on a short press, so it never goes dead. The hold and double thresholds are in Settings (450 ms and 280 ms by default).

## Moving macros

Drag a key that has a macro onto another key. On an empty key it moves, on a key with a macro the two swap, and holding Alt (Option on a Mac)
copies it instead. The copy and paste buttons of the key panel do the same between layers.

## Layers

Up to nine layers, each with a name, a color and an optional list of apps that activate it by themselves. The **All layers** tab
holds macros that work on every layer; a key on the active layer wins over the same key there. A `layer` action can go to the next,
the previous or a given layer, and with **Hold** it can be momentary: the layer is active only while the key is held.

## Action types

| Type | What it does | Fields |
|---|---|---|
| App | Open, toggle (hide when already in front) or quit an application | `app`, `mode` |
| Link | Open a URL in the default browser. `{editor}` is this editor: if it is already open, its tab is focused instead of opening another | `url` |
| Command | Run a shell command, optionally showing its output | `cmd`, `show_output` |
| SSH | Run a command on a server (key authentication, `BatchMode`) | `host`, `cmd` |
| Web request | GET, POST, PUT or DELETE a URL, for home automation and webhooks | `method`, `url`, `body` |
| Shortcut | A key combination such as `cmd+shift+4` or `ctrl+alt+f5` | `keys` |
| Text | Type text. `{date}`, `{time}`, `{datetime}` and `{clipboard}` are replaced first | `text` |
| Sequence | Steps in order, up to 100 | `steps` |
| Condition | One list of steps or another, see below | `cond`, `target`, `not`, `steps`, `else` |
| Media | Play/pause, next, previous, volume, mute | `cmd` |
| System | Screenshot, lock, screensaver, sleep the display, dark mode, keep awake | `cmd` |
| Typing history | Turn the optional history on, off or toggle it | `cmd` |
| OBS Studio | Scenes, streaming, recording, mic mute, replay, virtual camera, studio mode, through OBS's own WebSocket | `cmd`, `target` |
| Timer | A countdown with a popup at the end | `minutes`, `label` |
| Layer | Go to a layer | `to`, `momentary` |
| Popup | Show a message on screen | `text` |

Shortcuts, text and media keys leave the board as real USB keystrokes, which needs no permission on any system.

Add **Ask for a double press** to an action that should not run by accident: the first press shows a popup and the second, within
three seconds, runs it.

## Recording a sequence

In a sequence, **Record** captures what you do next: typed text becomes a text step, shortcuts become shortcut steps and the pauses
you leave become wait steps. While recording, key capture is released so the editor receives your keys. Stop with **Save steps**
and then reorder, duplicate, retime or delete the steps. The browser keeps a few shortcuts for itself (Cmd+T, Cmd+W) and those
cannot be recorded; add them by hand.

## Conditions

A condition checks something at the moment the key is pressed and runs its **Then** steps, or its **Otherwise** steps when it is not
met (or the other way round with **Invert**). The steps inside are simple actions: no sequences or other conditions.

| Check | Value | Example |
|---|---|---|
| The front app is | part of the app name | `Chrome` |
| The active layer is | its number or its name | `2` or `Streaming` |
| The time is between | `HH:MM-HH:MM`, overnight works | `22:00-07:00` |
| The system is | `darwin`, `windows` or `linux` | `darwin` |
| OBS is streaming or recording | none | |
| The clipboard contains | text, case does not matter | `http` |

A condition can sit inside a sequence, so "if this window is in front, send this shortcut, otherwise that one" is one key.

## Sharing macros

**Export** a layer from its panel (or everything from the Gallery) and **Import** a file from the same places. The file is JSON:

```json
{
  "typedeck_macros": "1",
  "name": "Developer",
  "layers": [{ "name": "Developer", "color": "#2563eb", "keys": { "3A": { "tap": { "type": "app", "app": "Terminal", "mode": "toggle" } } } }],
  "global": {}
}
```

Keys are the USB usage in hex (`3A` is F1). Importing validates the file like the configuration, then shows how many keys it brings
and lists every shell command, SSH command and web request in it, because those run on your computer when you press the key. You
choose between adding the layers as new ones or merging them into the current layer. [../examples](../examples) has packs to start from.

## The gallery

Packs for OBS Studio, Discord, Zoom, Meet, Teams, Slack, Photoshop, Figma, Premiere, VS Code, Git, smart home, study and more.
They are laid out on the keys your keyboard really has (a 60 % keyboard has no function row, a TKL has no numpad) and use only each
application's own default shortcuts. Packs for one system are not offered on the others; on Windows and Linux Cmd becomes Ctrl.

## Typing history

An optional page, off by default, that keeps the key, the time and the duration of each press in `keystrokes.jsonl` in the
configuration folder (mode 0600, capped at 4 MB, with the oldest entries deleted after 1, 7, 30, 90 or 365 days). It shows what you
type as text, totals, your most used keys and the latest presses, and can be switched on and off from a key. Nothing leaves your
computer.
