"""A picture for the README: a screenshot of utuimp in a frame of strands
held at round corners, as the interface draws its frames, in the colours of
the song on the screenshot, with a title and a line under it.

usage: readme_card.py <shot.png> <out.svg> <title> <line> <c1> <c2> <c3>

Render with: rsvg-convert out.svg -o out.png
"""
import base64
import math
import struct
import sys

shot, out, title, line = sys.argv[1:5]
C1, C2, C3 = sys.argv[5:8]
MONO = "font-family=\"'JetBrains Mono','Noto Sans Mono','DejaVu Sans Mono',monospace\""


def png_size(path):
    with open(path, "rb") as f:
        head = f.read(24)
    return struct.unpack(">II", head[16:24])


def frame(x0, y0, x1, y1, r, amp, ph, f1=1.0):
    """A rounded frame whose sides sway between the corners holding them."""
    pts = []

    def side(ax, ay, bx, by, nx, ny, k):
        length = math.hypot(bx - ax, by - ay)
        n = max(20, int(length / 6))
        for i in range(n + 1):
            u = i / n
            env = math.sin(math.pi * u) ** 1.3
            wob = amp * env * (0.6 * math.sin(u * length / 150 * f1 + ph + k * 1.9)
                               + 0.4 * math.sin(u * length / 70 * f1 - ph * 1.4 + k))
            pts.append((ax + (bx - ax) * u + nx * wob, ay + (by - ay) * u + ny * wob))

    def corner(cx, cy, a0):
        for i in range(1, 16):
            a = a0 + (math.pi / 2) * i / 16
            pts.append((cx + r * math.cos(a), cy + r * math.sin(a)))

    side(x0 + r, y0, x1 - r, y0, 0, -1, 0)
    corner(x1 - r, y0 + r, -math.pi / 2)
    side(x1, y0 + r, x1, y1 - r, 1, 0, 1)
    corner(x1 - r, y1 - r, 0)
    side(x1 - r, y1, x0 + r, y1, 0, 1, 2)
    corner(x0 + r, y1 - r, math.pi / 2)
    side(x0, y1 - r, x0, y0 + r, -1, 0, 3)
    corner(x0 + r, y0 + r, math.pi)
    return "M" + " L".join("%.1f %.1f" % p for p in pts) + " Z"


sw, sh = png_size(shot)
SW = 1200  # the screenshot's width on the card
SH = round(sh * SW / sw)
M = 64  # margin round it
W = SW + 2 * M
CAP = 132  # the words under it
H = SH + 2 * M + CAP
sx, sy = M, M
data = base64.b64encode(open(shot, "rb").read()).decode()

s = []
a = s.append
a(f'<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 {W} {H}" width="{W}" height="{H}">')
a(f"""<defs>
<linearGradient id="run" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="{C1}"/><stop offset="0.5" stop-color="{C2}"/><stop offset="1" stop-color="{C3}"/>
</linearGradient>
<radialGradient id="tintA" cx="0.1" cy="0.08" r="0.75"><stop offset="0" stop-color="{C1}" stop-opacity="0.13"/><stop offset="1" stop-color="{C1}" stop-opacity="0"/></radialGradient>
<radialGradient id="tintB" cx="0.92" cy="0.95" r="0.75"><stop offset="0" stop-color="{C3}" stop-opacity="0.11"/><stop offset="1" stop-color="{C3}" stop-opacity="0"/></radialGradient>
<clipPath id="shot"><rect x="{sx}" y="{sy}" width="{SW}" height="{SH}" rx="14"/></clipPath>
<filter id="glow" x="-5%" y="-10%" width="110%" height="120%"><feGaussianBlur stdDeviation="7"/></filter>
</defs>""")
a(f'<rect width="{W}" height="{H}" rx="28" fill="#0f0d10"/>')
a(f'<rect width="{W}" height="{H}" rx="28" fill="url(#tintA)"/><rect width="{W}" height="{H}" rx="28" fill="url(#tintB)"/>')

# The frame round the screenshot: three strands held at the same corners,
# each swaying its own way, their light under them.
g = 22
fr = [frame(sx - g, sy - g, sx + SW + g, sy + SH + g, 30, amp, ph, f1)
      for amp, ph, f1 in [(4.6, 0.0, 1.0), (3.8, 2.2, 1.15), (3.0, 4.1, 0.85)]]
a(f'<path d="{fr[0]}" fill="none" stroke="url(#run)" stroke-width="9" opacity="0.32" filter="url(#glow)"/>')
for d, w, op in zip(fr, [2.2, 1.4, 1.1], [1.0, 0.6, 0.42]):
    a(f'<path d="{d}" fill="none" stroke="url(#run)" stroke-width="{w}" opacity="{op}" stroke-linejoin="round"/>')

a(f'<rect x="{sx}" y="{sy}" width="{SW}" height="{SH}" rx="14" fill="#000"/>')
a(f'<image x="{sx}" y="{sy}" width="{SW}" height="{SH}" clip-path="url(#shot)" preserveAspectRatio="xMidYMid slice" xlink:href="data:image/png;base64,{data}"/>')

ty = sy + SH + g + 62
a(f'<g {MONO}>')
a(f'<text x="{sx}" y="{ty}" font-size="34" font-weight="700" fill="#f2eef0">{title}</text>')
a(f'<text x="{sx}" y="{ty + 40}" font-size="21" fill="#a8a0a6">{line}</text>')
a("</g>")
# At the end of the title line, a short run of strands in the frame's
# colours, as the progress line's played part: apart in the middle, one line
# at the ends.
x0, x1, yc = sx + SW - 200, sx + SW, ty - 12
for col, ph, amp in [(C1, 0.3, 6.0), (C2, 2.1, 5.0), (C3, 4.0, 5.5)]:
    pts = []
    for i in range(61):
        u = i / 60
        env = math.sin(math.pi * u) ** 1.2
        pts.append((x0 + (x1 - x0) * u, yc + amp * env * math.sin(u * 7.5 + ph)))
    d = "M" + " L".join("%.1f %.1f" % p for p in pts)
    a(f'<path d="{d}" fill="none" stroke="{col}" stroke-width="2" opacity="0.9" stroke-linecap="round"/>')
a(f'<circle cx="{x1}" cy="{yc}" r="3.6" fill="#ffffff"/>')
a("</svg>")
open(out, "w").write("\n".join(s) + "\n")
