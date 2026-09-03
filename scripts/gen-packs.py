#!/usr/bin/env python3
"""Generates internal/packs/packs.json: the gallery macro packs.
Shortcuts are each application's macOS defaults (except OBS, which is driven over WebSocket). Only letters, digits, arrows and
function keys are used in shortcuts so they work on any keyboard layout."""
import json, os

BLUE, PURPLE, GREEN, RED, ORANGE, AMBER, PINK, TEAL, SKY, SLATE = "#2563eb", "#8b5cf6", "#16a34a", "#cc4848", "#f97316", "#f59e0b", "#ec4899", "#0ea5a4", "#0ea5e9", "#64748b"

def hk(keys): return {"type": "hotkey", "keys": keys}
def app(name, mode="toggle"): return {"type": "app", "app": name, "mode": mode}
def tx(text): return {"type": "text", "text": text}
def media(c): return {"type": "media", "cmd": c}
def system(c): return {"type": "system", "cmd": c}
def timer(m, label=""): return {"type": "timer", "minutes": m, "label": label}
def ssh(host, cmd): return {"type": "ssh", "host": host, "cmd": cmd, "show_output": True}
def http(method, url, body=""): return {"type": "http", "method": method, "url": url, "body": body}
def url(u): return {"type": "url", "url": u}
def seq(*steps): return {"type": "sequence", "steps": list(steps)}
def hud(t): return {"type": "hud", "text": t}
def obs(cmd, target=""):
    a = {"type": "obs", "cmd": cmd}
    if target: a["target"] = target
    return a
def confirm(a): a = dict(a); a["confirm"] = True; return a

def M(es, en, icon, tap=None, color=None, hold=None, double=None, tap_pc=None):
    m = {"label": {"es": es, "en": en}, "icon": icon}
    if tap_pc: m["tap_pc"] = tap_pc
    if color: m["color"] = color
    if tap: m["tap"] = tap
    if hold: m["hold"] = hold
    if double: m["double"] = double
    return m

categories = [
    ("streaming", "camera", "Streaming y vídeo en directo", "Streaming and live video"),
    ("comms", "chat", "Reuniones y mensajes", "Meetings and messaging"),
    ("creative", "edit", "Diseño, foto y vídeo", "Design, photo and video"),
    ("dev", "code", "Desarrollo", "Development"),
    ("work", "clock", "Productividad y ventanas", "Productivity and windows"),
    ("media", "music", "Música y vídeo", "Music and video"),
    ("system", "monitor", "Sistema", "System"),
    ("home", "server", "Casa y servidores", "Home and servers"),
    ("text", "type", "Texto y escritura", "Text and writing"),
    ("study", "bookmark", "Estudio", "Study"),
    ("gaming", "bolt", "Juegos", "Gaming"),
]

packs = []
MAC_ONLY = {"finalcut", "xcode", "mail", "mac-productivity", "windows", "system", "launcher", "study", "terminal", "photoshop-mac"}

def P(id, cat, icon, color, name, desc, macros, region="numpad", apps=None, auto=None, notes=None, tags=None, glob=None):
    p = {"id": id, "category": cat, "icon": icon, "color": color, "name": {"es": name[0], "en": name[1]}, "description": {"es": desc[0], "en": desc[1]},
         "region": region, "macros": macros}
    if notes: p["notes"] = {"es": notes[0], "en": notes[1]}
    if apps: p["apps"] = apps
    if auto: p["auto_apps"] = auto
    if tags: p["tags"] = tags
    if id in MAC_ONLY: p["os"] = ["darwin"]
    if glob: p["global"] = glob
    packs.append(p)

OBS_NOTE = ("Controla OBS por su servidor WebSocket, sin atajos de teclado. En OBS abre Herramientas, Ajustes del servidor WebSocket, actívalo, y copia el puerto y la contraseña en Typedeck (Ajustes, OBS Studio). Las escenas 1 a 4 son las de tu lista, en el orden que se ve en OBS.",
            "Controls OBS through its WebSocket server, with no keyboard shortcuts. In OBS open Tools, WebSocket Server Settings, turn it on, and copy the port and password into Typedeck (Settings, OBS Studio). Scenes 1 to 4 are the ones in your list, in the order OBS shows them.")

P("obs", "streaming", "camera", RED, ("OBS Studio", "OBS Studio"),
  ("Escenas, directo, grabación, silenciar micro y guardar el replay.", "Scenes, going live, recording, mic mute and saving the replay."),
  [M("Escena 1", "Scene 1", "monitor", obs("scene", "#1"), BLUE), M("Escena 2", "Scene 2", "monitor", obs("scene", "#2"), BLUE), M("Escena 3", "Scene 3", "monitor", obs("scene", "#3"), BLUE),
   M("Escena 4", "Scene 4", "monitor", obs("scene", "#4"), BLUE), M("Directo", "Go live", "wifi", confirm(obs("stream")), RED), M("Grabar", "Record", "play", obs("record"), RED),
   M("Pausar grabación", "Pause recording", "pause", obs("record_pause"), AMBER), M("Silenciar micro", "Mute mic", "mic", obs("mute", "@mic"), AMBER),
   M("Silenciar escritorio", "Mute desktop", "volume-mute", obs("mute", "@desktop"), AMBER), M("Guardar replay", "Save replay", "download", obs("replay_save"), GREEN),
   M("Abrir OBS", "Open OBS", "grid", app("OBS"), SLATE)],
  apps=["OBS Studio"], auto=["OBS"], notes=OBS_NOTE, tags=["stream", "twitch", "youtube", "grabar", "record"])

P("discord", "streaming", "chat", PURPLE, ("Discord", "Discord"),
  ("Silenciar, ensordecerte, cambiar de servidor y de canal sin salir del juego.", "Mute, deafen, and switch servers and channels without leaving your game."),
  [M("Silenciar", "Mute", "mic", hk("cmd+shift+m"), RED), M("Ensordecer", "Deafen", "volume-mute", hk("cmd+shift+d"), RED), M("Buscar", "Quick switcher", "search", hk("cmd+k"), BLUE),
   M("Servidor arriba", "Server up", "arrow-up", hk("cmd+alt+up"), PURPLE), M("Servidor abajo", "Server down", "arrow-down", hk("cmd+alt+down"), PURPLE),
   M("Canal arriba", "Channel up", "arrow-up", hk("alt+up"), PURPLE), M("Canal abajo", "Channel down", "arrow-down", hk("alt+down"), PURPLE),
   M("Subir archivo", "Upload file", "upload", hk("cmd+shift+u"), SKY), M("Abrir Discord", "Open Discord", "chat", app("Discord"), SLATE)],
  apps=["Discord"], auto=["Discord"], notes=("Los atajos son los de Discord para Mac por defecto. Silenciar y ensordecer son globales en Discord solo si activas la opción en sus ajustes.", "Shortcuts are Discord's Mac defaults. Mute and deafen only work from other apps if you enable that in Discord's settings."),
  tags=["voz", "voice", "gaming"])

P("obs-discord", "streaming", "camera", PURPLE, ("OBS Studio + Discord", "OBS Studio + Discord"),
  ("Lo esencial de los dos: escenas y directo en OBS, silencio y buscar en Discord.", "The essentials of both: scenes and going live in OBS, mute and quick switch in Discord."),
  [M("Escena 1", "Scene 1", "monitor", obs("scene", "#1"), BLUE), M("Escena 2", "Scene 2", "monitor", obs("scene", "#2"), BLUE), M("Escena 3", "Scene 3", "monitor", obs("scene", "#3"), BLUE),
   M("Directo", "Go live", "wifi", confirm(obs("stream")), RED), M("Grabar", "Record", "play", obs("record"), RED), M("Micro OBS", "OBS mic", "mic", obs("mute", "@mic"), AMBER),
   M("Silenciar Discord", "Discord mute", "mic", hk("cmd+shift+m"), PURPLE), M("Ensordecer Discord", "Discord deafen", "volume-mute", hk("cmd+shift+d"), PURPLE),
   M("Buscar Discord", "Discord search", "search", hk("cmd+k"), PURPLE), M("Guardar replay", "Save replay", "download", obs("replay_save"), GREEN),
   M("OBS", "OBS", "camera", app("OBS"), SLATE), M("Discord", "Discord", "chat", app("Discord"), SLATE)],
  apps=["OBS Studio", "Discord"], auto=["OBS"], notes=OBS_NOTE, tags=["stream", "directo", "twitch"])

P("streamer-pro", "streaming", "bolt", PINK, ("Streamer completo", "Full streamer"),
  ("OBS, Discord y música en una capa: escenas, directo, silencio, pista siguiente y volumen.", "OBS, Discord and music in one layer: scenes, going live, mute, next track and volume."),
  [M("Escena 1", "Scene 1", "monitor", obs("scene", "#1"), BLUE), M("Escena 2", "Scene 2", "monitor", obs("scene", "#2"), BLUE), M("Directo", "Go live", "wifi", confirm(obs("stream")), RED),
   M("Micro OBS", "OBS mic", "mic", obs("mute", "@mic"), AMBER), M("Silenciar Discord", "Discord mute", "mic", hk("cmd+shift+m"), PURPLE), M("Ensordecer", "Deafen", "volume-mute", hk("cmd+shift+d"), PURPLE),
   M("Reproducir", "Play", "play", media("playpause"), GREEN), M("Siguiente", "Next", "skip-next", media("next"), GREEN), M("Volumen -", "Volume -", "volume", media("voldown"), SLATE),
   M("Volumen +", "Volume +", "volume", media("volup"), SLATE), M("Guardar replay", "Save replay", "download", obs("replay_save"), GREEN), M("Captura", "Screenshot", "camera", system("screenshot"), SKY)],
  apps=["OBS Studio", "Discord"], notes=OBS_NOTE, tags=["stream", "twitch"])

P("zoom", "comms", "camera", BLUE, ("Zoom", "Zoom"),
  ("Micro, cámara, compartir pantalla, grabar y levantar la mano.", "Mic, camera, screen share, record and raise hand."),
  [M("Micro", "Mic", "mic", hk("cmd+shift+a"), RED, tap_pc=hk("alt+a")), M("Cámara", "Camera", "camera", hk("cmd+shift+v"), BLUE, tap_pc=hk("alt+v")), M("Compartir", "Share screen", "monitor", hk("cmd+shift+s"), GREEN, tap_pc=hk("alt+shift+s")),
   M("Grabar", "Record", "play", hk("cmd+shift+r"), RED, tap_pc=hk("alt+r")), M("Mano", "Raise hand", "star", hk("alt+y"), AMBER), M("Participantes", "Participants", "grid", hk("cmd+u"), SLATE, tap_pc=hk("alt+u")),
   M("Chat", "Chat", "chat", hk("cmd+shift+h"), SKY, tap_pc=hk("alt+h")), M("Salir", "Leave", "power", confirm(hk("cmd+w")), RED, tap_pc=confirm(hk("alt+q"))), M("Abrir Zoom", "Open Zoom", "grid", app("zoom.us"), SLATE)],
  apps=["Zoom"], auto=["zoom.us"], tags=["reunion", "meeting", "videollamada"])

P("meet", "comms", "camera", GREEN, ("Google Meet", "Google Meet"),
  ("Micro y cámara con una tecla, también con la pestaña en segundo plano.", "Mic and camera with one key."),
  [M("Micro", "Mic", "mic", hk("cmd+d"), RED), M("Cámara", "Camera", "camera", hk("cmd+e"), BLUE), M("Mano", "Raise hand", "star", hk("ctrl+cmd+h"), AMBER), M("Chat", "Chat", "chat", hk("ctrl+cmd+c"), SKY),
   M("Abrir Chrome", "Open Chrome", "globe", app("Google Chrome"), SLATE)],
  apps=["Google Meet", "Google Chrome"], notes=("Funciona en el navegador con la pestaña de Meet activa.", "Works in the browser with the Meet tab active."), tags=["reunion", "meeting"])

P("teams", "comms", "chat", BLUE, ("Microsoft Teams", "Microsoft Teams"),
  ("Micro, cámara, compartir, mano y colgar.", "Mic, camera, share, hand and hang up."),
  [M("Micro", "Mic", "mic", hk("cmd+shift+m"), RED), M("Cámara", "Camera", "camera", hk("cmd+shift+o"), BLUE), M("Compartir", "Share", "monitor", hk("cmd+shift+e"), GREEN),
   M("Mano", "Raise hand", "star", hk("cmd+shift+k"), AMBER), M("Colgar", "Hang up", "power", confirm(hk("cmd+shift+h")), RED), M("Abrir Teams", "Open Teams", "grid", app("Microsoft Teams"), SLATE)],
  apps=["Microsoft Teams"], auto=["Microsoft Teams"], tags=["reunion", "meeting"])

P("slack", "comms", "chat", PURPLE, ("Slack", "Slack"),
  ("Saltar entre conversaciones, hilos, menciones y no leídos.", "Jump between conversations, threads, mentions and unreads."),
  [M("Buscar", "Quick switcher", "search", hk("cmd+k"), BLUE), M("Mensajes", "DMs", "chat", hk("cmd+shift+k"), PURPLE), M("Hilos", "Threads", "list", hk("cmd+shift+t"), PURPLE),
   M("Menciones", "Mentions", "bell", hk("cmd+shift+m"), AMBER), M("No leídos", "Unreads", "mail", hk("cmd+shift+a"), AMBER), M("Huddle", "Huddle", "mic", hk("cmd+shift+h"), GREEN),
   M("Canal arriba", "Channel up", "arrow-up", hk("alt+up"), SLATE), M("Canal abajo", "Channel down", "arrow-down", hk("alt+down"), SLATE), M("Abrir Slack", "Open Slack", "grid", app("Slack"), SLATE)],
  apps=["Slack"], auto=["Slack"], tags=["trabajo", "work"])

P("mail", "comms", "mail", SKY, ("Correo (Mail)", "Mail"),
  ("Redactar, responder, reenviar, archivar y marcar.", "Compose, reply, forward, archive and flag."),
  [M("Nuevo", "New", "edit", hk("cmd+n"), BLUE), M("Responder", "Reply", "arrow-left", hk("cmd+r"), BLUE), M("Responder a todos", "Reply all", "arrow-left", hk("cmd+shift+r"), BLUE),
   M("Reenviar", "Forward", "arrow-right", hk("cmd+shift+f"), BLUE), M("Enviar", "Send", "send", hk("cmd+shift+d"), GREEN), M("Archivar", "Archive", "download", hk("ctrl+cmd+a"), AMBER),
   M("Marcar", "Flag", "bookmark", hk("cmd+shift+l"), ORANGE), M("Abrir Mail", "Open Mail", "mail", app("Mail"), SLATE)],
  apps=["Mail"], auto=["Mail"], tags=["email", "correo"])

P("photoshop", "creative", "edit", BLUE, ("Photoshop", "Photoshop"),
  ("Herramientas de una letra, deshacer y capas nuevas.", "One-letter tools, undo and new layers."),
  [M("Mover", "Move", "grid", hk("v"), BLUE), M("Pincel", "Brush", "edit", hk("b"), BLUE), M("Borrador", "Eraser", "trash", hk("e"), BLUE), M("Selección", "Marquee", "grid", hk("m"), BLUE),
   M("Texto", "Type", "type", hk("t"), BLUE), M("Mano", "Hand", "grid", hk("h"), SLATE), M("Deshacer", "Undo", "undo", hk("cmd+z"), AMBER), M("Capa nueva", "New layer", "plus", hk("cmd+shift+n"), GREEN),
   M("Exportar web", "Save for web", "download", hk("alt+shift+cmd+s"), GREEN), M("Ajustar a ventana", "Fit on screen", "monitor", hk("cmd+0"), SLATE)],
  apps=["Adobe Photoshop"], tags=["foto", "diseño", "adobe"], notes=("Funciona en la app que tengas delante: no hace falta indicar el nombre exacto de Photoshop.", "Works in whichever app is in front: no need to name Photoshop exactly."))

P("illustrator", "creative", "edit", ORANGE, ("Illustrator", "Illustrator"),
  ("Selección, pluma, forma, texto y zoom con una tecla.", "Selection, pen, shape, type and zoom with one key."),
  [M("Selección", "Selection", "grid", hk("v"), BLUE), M("Selección directa", "Direct selection", "grid", hk("a"), BLUE), M("Pluma", "Pen", "edit", hk("p"), ORANGE), M("Texto", "Type", "type", hk("t"), ORANGE),
   M("Rectángulo", "Rectangle", "grid", hk("m"), ORANGE), M("Elipse", "Ellipse", "grid", hk("l"), ORANGE), M("Pincel", "Brush", "edit", hk("b"), ORANGE), M("Zoom", "Zoom", "search", hk("z"), SLATE),
   M("Mano", "Hand", "grid", hk("h"), SLATE), M("Deshacer", "Undo", "undo", hk("cmd+z"), AMBER)],
  apps=["Adobe Illustrator"], tags=["vector", "adobe"])

P("figma", "creative", "edit", PURPLE, ("Figma", "Figma"),
  ("Herramientas, duplicar, agrupar y ajustar el zoom.", "Tools, duplicate, group and zoom."),
  [M("Mover", "Move", "grid", hk("v"), BLUE), M("Marco", "Frame", "grid", hk("f"), PURPLE), M("Rectángulo", "Rectangle", "grid", hk("r"), PURPLE), M("Elipse", "Ellipse", "grid", hk("o"), PURPLE),
   M("Texto", "Text", "type", hk("t"), PURPLE), M("Pluma", "Pen", "edit", hk("p"), PURPLE), M("Duplicar", "Duplicate", "copy", hk("cmd+d"), GREEN), M("Agrupar", "Group", "layers", hk("cmd+g"), GREEN),
   M("Zoom 100%", "Zoom 100%", "search", hk("shift+0"), SLATE), M("Ajustar", "Zoom to fit", "monitor", hk("shift+1"), SLATE)],
  apps=["Figma"], auto=["Figma"], tags=["ui", "diseño", "design"])

P("premiere", "creative", "play", PURPLE, ("Premiere Pro", "Premiere Pro"),
  ("Corte, selección, reproducir con J K L y marcadores.", "Razor, select, J K L playback and markers."),
  [M("Selección", "Selection", "grid", hk("v"), BLUE), M("Cuchilla", "Razor", "edit", hk("c"), RED), M("Atrás", "Reverse", "skip-prev", hk("j"), PURPLE), M("Parar", "Stop", "pause", hk("k"), PURPLE),
   M("Adelante", "Forward", "skip-next", hk("l"), PURPLE), M("Reproducir", "Play", "play", hk("space"), GREEN), M("Cortar aquí", "Add edit", "edit", hk("cmd+k"), RED), M("Marcador", "Marker", "bookmark", hk("m"), AMBER),
   M("Entrada", "Mark in", "arrow-right", hk("i"), SLATE), M("Salida", "Mark out", "arrow-left", hk("o"), SLATE), M("Borrar y cerrar", "Ripple delete", "trash", hk("shift+delete"), RED)],
  apps=["Adobe Premiere Pro"], tags=["video", "edicion", "editing", "adobe"])

P("finalcut", "creative", "play", BLUE, ("Final Cut Pro", "Final Cut Pro"),
  ("Selección, cuchilla, J K L, marcadores y cortar en el cursor.", "Select, blade, J K L, markers and split at playhead."),
  [M("Selección", "Select", "grid", hk("a"), BLUE), M("Cuchilla", "Blade", "edit", hk("b"), RED), M("Atrás", "Reverse", "skip-prev", hk("j"), PURPLE), M("Parar", "Stop", "pause", hk("k"), PURPLE),
   M("Adelante", "Forward", "skip-next", hk("l"), PURPLE), M("Reproducir", "Play", "play", hk("space"), GREEN), M("Cortar aquí", "Split", "edit", hk("cmd+b"), RED), M("Marcador", "Marker", "bookmark", hk("m"), AMBER),
   M("Entrada", "Mark in", "arrow-right", hk("i"), SLATE), M("Salida", "Mark out", "arrow-left", hk("o"), SLATE)],
  apps=["Final Cut Pro"], auto=["Final Cut Pro"], tags=["video", "edicion", "editing"])

P("blender", "creative", "cpu", ORANGE, ("Blender", "Blender"),
  ("Mover, rotar, escalar, modo edición y menú de añadir.", "Grab, rotate, scale, edit mode and the add menu."),
  [M("Mover", "Grab", "grid", hk("g"), BLUE), M("Rotar", "Rotate", "grid", hk("r"), BLUE), M("Escalar", "Scale", "grid", hk("s"), BLUE), M("Modo edición", "Edit mode", "edit", hk("tab"), ORANGE),
   M("Añadir", "Add", "plus", hk("shift+a"), GREEN), M("Duplicar", "Duplicate", "copy", hk("shift+d"), GREEN), M("Seleccionar todo", "Select all", "grid", hk("a"), SLATE), M("Panel lateral", "Sidebar", "monitor", hk("n"), SLATE),
   M("Buscar", "Search", "search", hk("f3"), AMBER), M("Deshacer", "Undo", "undo", hk("cmd+z"), AMBER)],
  apps=["Blender"], auto=["Blender"], tags=["3d"])

P("lightroom", "creative", "camera", SKY, ("Lightroom Classic", "Lightroom Classic"),
  ("Marcar, rechazar y puntuar fotos con una tecla.", "Flag, reject and rate photos with one key."),
  [M("Elegir", "Pick", "check", hk("p"), GREEN), M("Rechazar", "Reject", "x", hk("x"), RED), M("Quitar marca", "Unflag", "minus", hk("u"), SLATE), M("1 estrella", "1 star", "star", hk("1"), AMBER),
   M("2 estrellas", "2 stars", "star", hk("2"), AMBER), M("3 estrellas", "3 stars", "star", hk("3"), AMBER), M("4 estrellas", "4 stars", "star", hk("4"), AMBER), M("5 estrellas", "5 stars", "star", hk("5"), AMBER),
   M("Cuadrícula", "Grid", "grid", hk("g"), BLUE), M("Lupa", "Loupe", "search", hk("e"), BLUE), M("Revelar", "Develop", "sliders", hk("d"), PURPLE)],
  apps=["Adobe Lightroom Classic"], tags=["foto", "photo", "adobe"])

P("vscode", "dev", "code", BLUE, ("Visual Studio Code", "Visual Studio Code"),
  ("Abrir archivo, paleta, paneles, buscar, renombrar y formatear.", "Open file, palette, panels, search, rename and format."),
  [M("Abrir archivo", "Quick open", "folder", hk("cmd+p"), BLUE), M("Paleta", "Command palette", "search", hk("cmd+shift+p"), BLUE), M("Barra lateral", "Sidebar", "monitor", hk("cmd+b"), SLATE),
   M("Panel", "Panel", "terminal", hk("cmd+j"), SLATE), M("Buscar en todo", "Search all", "search", hk("cmd+shift+f"), AMBER), M("Siguiente ocurrencia", "Next match", "edit", hk("cmd+d"), GREEN),
   M("Renombrar", "Rename", "edit", hk("f2"), GREEN), M("Ir a definición", "Go to definition", "code", hk("f12"), PURPLE), M("Formatear", "Format", "sliders", hk("alt+shift+f"), GREEN),
   M("Mover línea arriba", "Move line up", "arrow-up", hk("alt+up"), SLATE), M("Mover línea abajo", "Move line down", "arrow-down", hk("alt+down"), SLATE), M("Abrir VS Code", "Open VS Code", "code", app("Visual Studio Code"), SLATE)],
  apps=["Visual Studio Code"], auto=["Visual Studio Code", "Code"], tags=["editor", "programar"])

P("xcode", "dev", "cpu", BLUE, ("Xcode", "Xcode"),
  ("Compilar, ejecutar, parar, probar y navegar.", "Build, run, stop, test and navigate."),
  [M("Compilar", "Build", "cpu", hk("cmd+b"), BLUE), M("Ejecutar", "Run", "play", hk("cmd+r"), GREEN), M("Parar", "Stop", "pause", hk("cmd+."), RED), M("Probar", "Test", "check", hk("cmd+u"), GREEN),
   M("Limpiar", "Clean", "trash", confirm(hk("cmd+shift+k")), AMBER), M("Abrir rápido", "Open quickly", "search", hk("cmd+shift+o"), BLUE), M("Ir a definición", "Jump to definition", "code", hk("ctrl+cmd+j"), PURPLE),
   M("Navegador", "Navigator", "folder", hk("cmd+0"), SLATE), M("Inspector", "Utilities", "sliders", hk("cmd+alt+0"), SLATE), M("Consola", "Debug area", "terminal", hk("cmd+shift+y"), SLATE)],
  apps=["Xcode"], auto=["Xcode"], tags=["ios", "swift", "apple"])

P("jetbrains", "dev", "code", PINK, ("JetBrains (IntelliJ, PyCharm, WebStorm)", "JetBrains (IntelliJ, PyCharm, WebStorm)"),
  ("Buscar acción, archivos recientes, ejecutar, depurar y refactorizar.", "Find action, recent files, run, debug and refactor."),
  [M("Buscar acción", "Find action", "search", hk("cmd+shift+a"), BLUE), M("Recientes", "Recent files", "clock", hk("cmd+e"), BLUE), M("Ejecutar", "Run", "play", hk("ctrl+r"), GREEN), M("Depurar", "Debug", "cpu", hk("ctrl+d"), GREEN),
   M("Formatear", "Reformat", "sliders", hk("cmd+alt+l"), PURPLE), M("Renombrar", "Rename", "edit", hk("shift+f6"), PURPLE), M("Ir a declaración", "Go to declaration", "code", hk("cmd+b"), PURPLE),
   M("Buscar en ruta", "Find in path", "search", hk("cmd+shift+f"), AMBER)],
  apps=["IntelliJ IDEA", "PyCharm", "WebStorm"], tags=["java", "python", "kotlin"])

P("terminal", "dev", "terminal", GREEN, ("Terminal y pestañas", "Terminal and tabs"),
  ("Pestañas, limpiar, interrumpir y buscar en la terminal.", "Tabs, clear, interrupt and search in the terminal."),
  [M("Pestaña nueva", "New tab", "plus", hk("cmd+t"), BLUE), M("Cerrar pestaña", "Close tab", "x", hk("cmd+w"), RED), M("Ventana nueva", "New window", "terminal", hk("cmd+n"), BLUE), M("Limpiar", "Clear", "trash", hk("cmd+k"), AMBER),
   M("Interrumpir", "Interrupt", "power", confirm(hk("ctrl+c")), RED), M("Buscar", "Find", "search", hk("cmd+f"), SLATE), M("Siguiente pestaña", "Next tab", "arrow-right", hk("ctrl+tab"), SLATE),
   M("Abrir Terminal", "Open Terminal", "terminal", app("Terminal"), SLATE)],
  apps=["Terminal", "iTerm2"], tags=["consola", "shell"])

P("git", "dev", "code", ORANGE, ("Git en la terminal", "Git in the terminal"),
  ("Escribe los comandos de Git más usados en la terminal que tengas delante.", "Types the most common Git commands into whichever terminal is in front."),
  [M("status", "status", "list", tx("git status\n"), BLUE), M("diff", "diff", "code", tx("git diff\n"), BLUE), M("pull", "pull", "download", tx("git pull\n"), GREEN), M("push", "push", "upload", confirm(tx("git push\n")), RED),
   M("add todo", "add all", "plus", tx("git add -A\n"), GREEN), M("commit", "commit", "check", tx("git commit -m \""), AMBER), M("log", "log", "clock", tx("git log --oneline -15\n"), SLATE),
   M("rama nueva", "new branch", "layers", tx("git checkout -b "), PURPLE), M("stash", "stash", "folder", tx("git stash\n"), SLATE)],
  apps=["Terminal", "iTerm2"], notes=("Cada macro escribe el texto como si lo teclearas, así que la terminal debe estar delante. 'commit' deja abiertas las comillas para que escribas el mensaje.", "Each macro types the text as if you typed it, so the terminal must be in front. 'commit' leaves the quote open so you can type the message."),
  tags=["git", "versiones"])

P("browser", "dev", "globe", AMBER, ("Navegador y herramientas web", "Browser and web tools"),
  ("Pestañas, barra de direcciones, consola de desarrollo y recarga forzada.", "Tabs, address bar, dev tools and hard reload."),
  [M("Pestaña nueva", "New tab", "plus", hk("cmd+t"), BLUE), M("Cerrar", "Close", "x", hk("cmd+w"), RED), M("Reabrir", "Reopen", "undo", hk("cmd+shift+t"), AMBER), M("Barra de direcciones", "Address bar", "search", hk("cmd+l"), BLUE),
   M("Herramientas", "Dev tools", "code", hk("cmd+alt+i"), PURPLE), M("Recargar", "Reload", "redo", hk("cmd+r"), GREEN), M("Recarga forzada", "Hard reload", "redo", hk("cmd+shift+r"), GREEN),
   M("Incógnito", "Incognito", "eye", hk("cmd+shift+n"), SLATE), M("Pestaña siguiente", "Next tab", "arrow-right", hk("cmd+alt+right"), SLATE), M("Pestaña anterior", "Previous tab", "arrow-left", hk("cmd+alt+left"), SLATE)],
  apps=["Google Chrome", "Safari"], tags=["web", "chrome"])

P("homelab", "home", "server", GREEN, ("Servidor por SSH", "Server over SSH"),
  ("Plantilla: cambia mi-servidor por tu alias de SSH. El resultado sale en el cartel.", "Template: replace mi-servidor with your SSH alias. The result shows on screen."),
  [M("Uptime", "Uptime", "clock", ssh("mi-servidor", "uptime -p"), GREEN), M("Disco", "Disk", "database", ssh("mi-servidor", "df -h / | tail -1 | awk '{print $5 \" usado\"}'"), BLUE),
   M("Memoria", "Memory", "cpu", ssh("mi-servidor", "free -h | awk '/Mem/ {print $3 \" de \" $2}'"), AMBER), M("Carga", "Load", "activity", ssh("mi-servidor", "cut -d' ' -f1-3 /proc/loadavg"), PURPLE),
   M("Servicios caídos", "Failed services", "shield", ssh("mi-servidor", "systemctl --failed --no-legend | wc -l"), RED), M("Reiniciar", "Reboot", "power", confirm(ssh("mi-servidor", "sudo reboot")), RED)],
  apps=["SSH"], notes=("Necesita acceso por SSH con clave (sin contraseña) y un alias en ~/.ssh/config. 'Reiniciar' pide doble pulsación.", "Needs SSH key access (no password) and an alias in ~/.ssh/config. 'Reboot' asks for a double press."), tags=["linux", "proxmox", "homelab"])

P("docker", "home", "server", SKY, ("Docker por SSH", "Docker over SSH"),
  ("Contenedores en marcha, estadísticas y reinicios en un servidor con Docker.", "Running containers, stats and restarts on a Docker host."),
  [M("Contenedores", "Containers", "list", ssh("mi-servidor", "docker ps --format '{{.Names}}: {{.Status}}' | head -3"), BLUE), M("Cuántos", "How many", "grid", ssh("mi-servidor", "docker ps -q | wc -l"), GREEN),
   M("Parados", "Stopped", "pause", ssh("mi-servidor", "docker ps -aq --filter status=exited | wc -l"), AMBER), M("Espacio", "Disk use", "database", ssh("mi-servidor", "docker system df --format '{{.Type}}: {{.Size}}' | head -2"), SLATE),
   M("Limpiar", "Prune", "trash", confirm(ssh("mi-servidor", "docker system prune -f | tail -1")), RED)],
  apps=["Docker"], notes=("Cambia mi-servidor por tu alias de SSH.", "Replace mi-servidor with your SSH alias."), tags=["contenedores", "containers"])

P("smart-home", "home", "bolt", AMBER, ("Casa conectada (HTTP)", "Smart home (HTTP)"),
  ("Plantilla para enchufes y luces con API web: Shelly, Tasmota y Home Assistant.", "Template for plugs and lights with a web API: Shelly, Tasmota and Home Assistant."),
  [M("Shelly on/off", "Shelly toggle", "bolt", http("GET", "http://192.168.1.50/relay/0?turn=toggle"), AMBER), M("Tasmota on/off", "Tasmota toggle", "bolt", http("GET", "http://192.168.1.51/cm?cmnd=Power%20Toggle"), AMBER),
   M("Home Assistant", "Home Assistant", "home", http("POST", "http://homeassistant.local:8123/api/webhook/TU_ID"), BLUE), M("Luz salón", "Living room light", "sun", http("GET", "http://192.168.1.52/relay/0?turn=toggle"), AMBER)],
  apps=["Home Assistant"], notes=("Las direcciones son ejemplos: cambia las IP y el identificador del webhook por los tuyos.", "The addresses are examples: replace the IPs and the webhook id with yours."), tags=["domotica", "domotics", "iot"])

P("mac-productivity", "work", "monitor", BLUE, ("Mac: escritorios y apps", "Mac: spaces and apps"),
  ("Mission Control, escritorios, Spotlight, forzar salida y emojis.", "Mission Control, spaces, Spotlight, force quit and emoji."),
  [M("Mission Control", "Mission Control", "layers", hk("ctrl+up"), BLUE), M("Ventanas de la app", "App windows", "grid", hk("ctrl+down"), BLUE), M("Escritorio izq.", "Space left", "arrow-left", hk("ctrl+left"), SLATE),
   M("Escritorio der.", "Space right", "arrow-right", hk("ctrl+right"), SLATE), M("Spotlight", "Spotlight", "search", hk("cmd+space"), PURPLE), M("Forzar salida", "Force quit", "power", hk("alt+cmd+escape"), RED),
   M("Emojis", "Emoji picker", "star", hk("ctrl+cmd+space"), AMBER), M("Captura", "Screenshot", "camera", hk("cmd+shift+4"), SKY), M("Bloquear", "Lock", "lock", confirm(system("lock")), RED)],
  tags=["mac", "macos"])

P("windows", "work", "monitor", TEAL, ("Ventanas (Rectangle)", "Window tiling (Rectangle)"),
  ("Mitades, maximizar, centrar y cambiar de pantalla con Rectangle.", "Halves, maximize, center and move between displays with Rectangle."),
  [M("Mitad izquierda", "Left half", "arrow-left", hk("ctrl+alt+left"), TEAL), M("Mitad derecha", "Right half", "arrow-right", hk("ctrl+alt+right"), TEAL), M("Mitad superior", "Top half", "arrow-up", hk("ctrl+alt+up"), TEAL),
   M("Mitad inferior", "Bottom half", "arrow-down", hk("ctrl+alt+down"), TEAL), M("Maximizar", "Maximize", "monitor", hk("ctrl+alt+return"), BLUE), M("Centrar", "Center", "grid", hk("ctrl+alt+c"), BLUE),
   M("Otra pantalla", "Next display", "arrow-right", hk("ctrl+alt+cmd+right"), PURPLE), M("Restaurar", "Restore", "undo", hk("ctrl+alt+delete"), SLATE)],
  apps=["Rectangle"], notes=("Necesita Rectangle (gratuito, rectangleapp.com) con sus atajos por defecto.", "Needs Rectangle (free) with its default shortcuts."), tags=["ventanas", "windows"])

P("obsidian", "work", "edit", PURPLE, ("Obsidian", "Obsidian"),
  ("Buscar nota, paleta, nota nueva, editar o ver y búsqueda.", "Quick switcher, palette, new note, edit or preview and search."),
  [M("Buscar nota", "Quick switcher", "search", hk("cmd+o"), BLUE), M("Paleta", "Command palette", "command", hk("cmd+p"), BLUE), M("Nota nueva", "New note", "plus", hk("cmd+n"), GREEN),
   M("Editar o ver", "Edit / preview", "edit", hk("cmd+e"), PURPLE), M("Buscar", "Search", "search", hk("cmd+shift+f"), AMBER), M("Abrir Obsidian", "Open Obsidian", "folder", app("Obsidian"), SLATE)],
  apps=["Obsidian"], auto=["Obsidian"], tags=["notas", "notes"])

P("focus", "work", "clock", RED, ("Foco y pomodoro", "Focus and pomodoro"),
  ("Temporizadores, silenciar y cerrar distracciones con una tecla.", "Timers, mute and closing distractions with one key."),
  [M("Pomodoro 25", "Pomodoro 25", "clock", timer(25, "Pomodoro"), RED), M("Descanso 5", "Break 5", "clock", timer(5, "Descanso"), GREEN), M("Descanso 15", "Break 15", "clock", timer(15, "Descanso largo"), TEAL),
   M("Empezar foco", "Start focus", "bolt", seq(media("mute"), hud("Foco"), timer(25, "Foco")), PURPLE),
   M("Cerrar distracciones", "Close distractions", "power", confirm(seq(app("Discord", "quit"), app("Slack", "quit"), app("WhatsApp", "quit"))), AMBER), M("Mantener despierto", "Stay awake", "bolt", system("caffeinate"), SKY)],
  tags=["pomodoro", "concentracion", "focus"], notes=("'Cerrar distracciones' cierra Discord, Slack y WhatsApp y pide doble pulsación. Edita la lista a tu gusto.", "'Close distractions' quits Discord, Slack and WhatsApp and asks for a double press. Edit the list to taste."))

P("media", "media", "music", PURPLE, ("Multimedia", "Media"),
  ("Reproducción y volumen para Spotify o Música.", "Playback and volume for Spotify or Music."),
  [M("Anterior", "Previous", "skip-prev", media("prev"), BLUE), M("Reproducir", "Play", "play", media("playpause"), PURPLE, hold=media("next")), M("Siguiente", "Next", "skip-next", media("next"), BLUE),
   M("Volumen -", "Volume -", "volume", media("voldown"), SLATE), M("Silenciar", "Mute", "volume-mute", media("mute"), RED), M("Volumen +", "Volume +", "volume", media("volup"), SLATE),
   M("Abrir Spotify", "Open Spotify", "music", app("Spotify"), GREEN)], region="numpad", apps=["Spotify", "Music"], auto=["Spotify"], tags=["musica", "music", "volumen"])

P("youtube", "media", "play", RED, ("YouTube en el navegador", "YouTube in the browser"),
  ("Pausa, saltar 10 s, pantalla completa y silencio, con la pestaña de YouTube delante.", "Pause, skip 10 s, fullscreen and mute, with the YouTube tab in front."),
  [M("Pausa", "Pause", "pause", hk("k"), RED), M("Atrás 10 s", "Back 10 s", "skip-prev", hk("j"), BLUE), M("Adelante 10 s", "Forward 10 s", "skip-next", hk("l"), BLUE), M("Pantalla completa", "Fullscreen", "monitor", hk("f"), GREEN),
   M("Silencio", "Mute", "volume-mute", hk("m"), AMBER), M("Siguiente vídeo", "Next video", "arrow-right", hk("shift+n"), PURPLE)],
  apps=["YouTube"], tags=["video"])

P("system", "system", "monitor", TEAL, ("Sistema del Mac", "Mac system"),
  ("Captura, bloqueo, modo oscuro y mantener el Mac despierto.", "Screenshot, lock, dark mode and keeping the Mac awake."),
  [M("Captura", "Screenshot", "camera", system("screenshot"), SKY), M("Salvapantallas", "Screen saver", "moon", system("screensaver"), SLATE), M("Bloquear", "Lock", "lock", confirm(system("lock")), RED),
   M("Modo oscuro", "Dark mode", "sun", system("darkmode"), AMBER), M("Despierto", "Stay awake", "bolt", system("caffeinate"), AMBER), M("Apagar pantalla", "Display off", "monitor", system("sleepdisplay"), SLATE),
   M("Monitor de actividad", "Activity Monitor", "cpu", app("Activity Monitor"), BLUE), M("Ajustes", "Settings", "sliders", app("System Settings"), BLUE), M("Terminal", "Terminal", "terminal", app("Terminal"), GREEN)],
  tags=["mac"])

P("snippets", "text", "edit", PINK, ("Texto rápido: fecha y hora", "Quick text: date and time"),
  ("Fecha, hora y portapapeles como texto limpio.", "Date, time and clipboard as plain text."),
  [M("Fecha", "Date", "calendar", tx("{date}"), BLUE), M("Hora", "Time", "clock", tx("{time}"), BLUE), M("Fecha y hora", "Date and time", "calendar", tx("{datetime}"), BLUE),
   M("Pegar sin formato", "Paste plain", "clipboard", tx("{clipboard}"), GREEN)], tags=["fecha", "date"])

P("email-snippets", "text", "mail", SKY, ("Respuestas de correo", "Email replies"),
  ("Frases hechas para escribir más rápido. Edítalas con tus datos.", "Ready-made phrases to write faster. Edit them with your details."),
  [M("Saludo", "Greeting", "mail", tx("Hola,\n\n"), BLUE), M("Gracias", "Thanks", "heart", tx("Muchas gracias por tu mensaje."), GREEN), M("Lo reviso", "I'll check", "check", tx("Lo reviso y te digo algo en cuanto pueda."), AMBER),
   M("Adjunto", "Attached", "download", tx("Te adjunto el documento."), SLATE), M("Despedida", "Sign-off", "send", tx("\nUn saludo,\n"), PURPLE), M("Teléfono", "Phone", "bell", tx("600 000 000"), SLATE)],
  notes=("Los datos de ejemplo (teléfono) son falsos: cámbialos. No guardes contraseñas en macros de texto.", "Example details (phone) are fake: change them. Do not store passwords in text macros."), tags=["correo", "email"])

P("code-snippets", "text", "code", GREEN, ("Fragmentos de código", "Code snippets"),
  ("Esqueletos que se repiten mucho al programar.", "Skeletons you type over and over."),
  [M("console.log", "console.log", "terminal", tx("console.log()"), AMBER), M("if", "if", "code", tx("if () {\n}"), BLUE), M("función", "function", "code", tx("function () {\n}"), BLUE), M("TODO", "TODO", "edit", tx("// TODO: "), RED),
   M("try/catch", "try/catch", "shield", tx("try {\n} catch (e) {\n}"), PURPLE), M("bucle for", "for loop", "redo", tx("for (let i = 0; i < n; i++) {\n}"), GREEN)],
  tags=["javascript", "programar"])

P("study", "study", "bookmark", TEAL, ("Estudio", "Study"),
  ("Pomodoro, notas, captura al portapapeles y música para concentrarte.", "Pomodoro, notes, screenshot to clipboard and music to focus."),
  [M("Pomodoro 25", "Pomodoro 25", "clock", timer(25, "Estudio"), RED), M("Descanso 5", "Break 5", "clock", timer(5, "Descanso"), GREEN), M("Notas", "Notes", "edit", app("Notes"), AMBER), M("Nota nueva", "New note", "plus", seq(app("Notes", "open"), hk("cmd+n")), AMBER),
   M("Captura al portapapeles", "Screenshot to clipboard", "camera", hk("ctrl+cmd+shift+4"), SKY), M("Música", "Music", "music", media("playpause"), PURPLE), M("Silenciar", "Mute", "volume-mute", media("mute"), SLATE),
   M("Diccionario", "Dictionary", "search", app("Dictionary"), BLUE)],
  apps=["Notes"], tags=["estudiante", "student", "universidad"])

P("gaming", "gaming", "bolt", ORANGE, ("Juegos y charla", "Gaming and chat"),
  ("Silenciar Discord, guardar el replay de OBS, captura y control de música sin salir del juego.", "Mute Discord, save the OBS replay, screenshot and music control without leaving your game."),
  [M("Silenciar Discord", "Discord mute", "mic", hk("cmd+shift+m"), PURPLE), M("Ensordecer", "Deafen", "volume-mute", hk("cmd+shift+d"), PURPLE), M("Guardar replay", "Save replay", "download", obs("replay_save"), GREEN),
   M("Captura", "Screenshot", "camera", system("screenshot"), SKY), M("Música", "Music", "music", media("playpause"), GREEN), M("Siguiente", "Next", "skip-next", media("next"), GREEN),
   M("Volumen -", "Volume -", "volume", media("voldown"), SLATE), M("Volumen +", "Volume +", "volume", media("volup"), SLATE)],
  apps=["Discord", "OBS Studio"], notes=OBS_NOTE, tags=["juegos", "games", "gaming"])

launch = [("06", "Google Chrome", "Chrome", "globe", AMBER), ("19", "Visual Studio Code", "VS Code", "code", BLUE), ("17", "Terminal", "Terminal", "terminal", GREEN),
          ("16", "Spotify", "Spotify", "music", GREEN), ("07", "Discord", "Discord", "chat", PURPLE), ("09", "Finder", "Finder", "folder", SLATE), ("11", "Notes", "Notas", "edit", AMBER),
          ("10", "Mail", "Correo", "mail", SKY), ("1A", "WhatsApp", "WhatsApp", "chat", GREEN)]
P("launcher", "work", "keyboard", AMBER, ("Lanzador con Bloq Mayús", "Launcher on Caps Lock"),
  ("Mantén Bloq Mayús y pulsa una letra para abrir o alternar una app. Una pulsación corta sigue siendo Esc.", "Hold Caps Lock and press a letter to open or toggle an app. A short tap still acts as Esc."),
  [dict(label={"es": es, "en": es}, icon=ic, color=col, tap=app(name), key=k) for (k, name, es, ic, col) in launch], region="letters",
  glob={"39": {"tap": hk("esc"), "hold": {"type": "layer", "to": "next", "momentary": True}, "label": "Capa", "icon": "layers"}},
  tags=["lanzador", "launcher", "bloq mayus", "caps"], notes=("Las letras se colocan sobre el teclado alfabético: C, V, T, S, D, F, N, M, W. Se puede cambiar la tecla de cada app en el editor.", "Letters are placed on the alphabet keys. You can change each app's key in the editor."))

out = {"categories": [{"id": i, "icon": ic, "name": {"es": es, "en": en}} for (i, ic, es, en) in categories], "packs": packs}
dst = os.path.join(os.path.dirname(__file__), "..", "internal", "packs", "packs.json")
with open(dst, "w", encoding="utf-8") as f:
    json.dump(out, f, indent=1, ensure_ascii=False)
print(len(packs), "paquetes en", len(categories), "categorias,", sum(len(p["macros"]) for p in packs), "macros")
