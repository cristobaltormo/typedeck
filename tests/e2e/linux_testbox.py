#!/usr/bin/env python3
"""A GTK window of our own for the real Linux tests: it writes its text and every key it receives to files, so no real application
is ever typed into. Usage: linux_testbox.py [directory]"""
import os, sys, gi
gi.require_version("Gtk", "3.0")
from gi.repository import Gtk, GLib

DIR = sys.argv[1] if len(sys.argv) > 1 else os.path.expanduser("~/.cache/typedeck-testbox")
os.makedirs(DIR, exist_ok=True)
keys = open(os.path.join(DIR, "keys.txt"), "w", buffering=1)
win = Gtk.Window(title="Typedeck test box")
win.set_default_size(520, 260)
view = Gtk.TextView()
win.add(view)
buf = view.get_buffer()

def save(*_):
    start, end = buf.get_bounds()
    with open(os.path.join(DIR, "text.txt"), "w", encoding="utf-8") as f:
        f.write(buf.get_text(start, end, True))

def on_key(_w, ev):
    mods = ev.state & Gtk.accelerator_get_default_mod_mask()
    keys.write("%s|ctrl=%s|alt=%s|shift=%s\n" % (Gtk.accelerator_name(ev.keyval, 0), bool(mods & 4), bool(mods & 8), bool(mods & 1)))
    return False

buf.connect("changed", save)
view.connect("key-press-event", on_key)
win.connect("destroy", Gtk.main_quit)
win.show_all()
GLib.idle_add(lambda: (win.present(), view.grab_focus(), False)[-1])
Gtk.main()
