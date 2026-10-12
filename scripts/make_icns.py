#!/usr/bin/env python3
"""Build a macOS .icns from one 1024px PNG (PNG-compressed icon entries).
usage: make_icns.py <AppIcon.png> <AppIcon.icns>"""
import io, struct, sys
from PIL import Image

src = Image.open(sys.argv[1]).convert("RGBA")
entries = [("icp4", 16), ("icp5", 32), ("icp6", 64), ("ic07", 128), ("ic08", 256), ("ic09", 512),
           ("ic10", 1024), ("ic11", 32), ("ic12", 64), ("ic13", 256), ("ic14", 512)]
body = b""
for typ, size in entries:
    buf = io.BytesIO()
    src.resize((size, size), Image.LANCZOS).save(buf, "PNG")
    data = buf.getvalue()
    body += typ.encode() + struct.pack(">I", len(data) + 8) + data
open(sys.argv[2], "wb").write(b"icns" + struct.pack(">I", len(body) + 8) + body)
print("✔", sys.argv[2])
