#!/usr/bin/env python3
"""Generates docs/COMPATIBILITY.md from internal/setup/compat.json (the same source the app shows)."""
import json, os

root = os.path.join(os.path.dirname(__file__), "..")
data = json.load(open(os.path.join(root, "internal", "setup", "compat.json"), encoding="utf-8"))
legend = data["status_legend"]
out = [
    "# Compatibility", "",
    "Generated from `internal/setup/compat.json`; do not edit by hand. The Compatibility page of the editor shows the same table "
    "next to your real setup.", "",
    "Status: " + ", ".join(f"**{v['en']}**" for v in legend.values()) + ". *Tested* means it was run on real hardware or a real "
    "machine; *Should work* means the code path is shared or simulated but not seen on that exact setup.", "",
]
for g in data["groups"]:
    out += [f"## {g['name']['en']}", "", "| Item | Status | Notes |", "|---|---|---|"]
    for it in g["items"]:
        out.append(f"| {it.get('name_en', it['name'])} | {legend[it['status']]['en']} | {it['note']['en']} |")
    out.append("")
open(os.path.join(root, "docs", "COMPATIBILITY.md"), "w", encoding="utf-8").write("\n".join(out))
print("docs/COMPATIBILITY.md generated")
