"""The utuimp social preview in black and white: the lines logo, frames of
boxes made of strands that sway between the round corners holding them, as
the interface's frames do, and the progress line's strands, all in greys.

usage: cover_mono.py <logo.svg> <out.svg>
"""
import math
import re
import sys

W, H = 1280, 640
MONO = "font-family=\"'JetBrains Mono','Noto Sans Mono','DejaVu Sans Mono',monospace\""

logo_src = open(sys.argv[1]).read()
out = sys.argv[2]
vb = re.search(r'viewBox="([^"]+)"', logo_src).group(1)
inner = re.sub(r"^.*?<svg[^>]*>", "", logo_src, count=1, flags=re.S)
inner = inner[: inner.rfind("</svg>")]
inner = re.sub(r'id="([^"]+)"', r'id="logo-\1"', inner)
inner = re.sub(r"url\(#([^)]+)\)", r"url(#logo-\1)", inner)


def smooth(a, b, x):
    t = max(0.0, min(1.0, (x - a) / (b - a)))
    return t * t * (3 - 2 * t)


def path(pts, close=False):
    return "M" + " L".join("%.1f %.1f" % p for p in pts) + (" Z" if close else "")


def frame(x0, y0, x1, y1, r, amp, ph, f1=1.0):
    """A rounded frame whose sides sway between the corners that hold them:
    still at the corners, most in the middle of a side."""
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
    return path(pts, True)


def strand(x0, x1, y, amp, ph, f1, f2, n=200):
    """One strand of the played part: it leaves the line after the start and
    comes back to it where the song is."""
    pts = []
    for i in range(n + 1):
        u = i / n
        env = smooth(0, 0.18, u) * smooth(0, 0.12, 1 - u)
        pts.append((x0 + (x1 - x0) * u, y + amp * env * (0.62 * math.sin(u * f1 + ph) + 0.38 * math.sin(u * f2 - ph * 1.3))))
    return path(pts)


# The light running round a frame: white at one corner, grey round the far
# side, as the colours run round the panel in focus.
RUN = [("#ffffff", 0), ("#6e6e6e", 0.5), ("#e6e6e6", 1)]

s = []
a = s.append
a(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}">')
a("<defs>")
a('<linearGradient id="run" x1="0" y1="0" x2="1" y2="1">' + "".join(f'<stop offset="{o}" stop-color="{c}"/>' for c, o in RUN) + "</linearGradient>")
a('<linearGradient id="dim" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#9a9a9a"/><stop offset="1" stop-color="#5a5a5a"/></linearGradient>')
a('<radialGradient id="lift" cx="0.5" cy="0.32" r="0.75"><stop offset="0" stop-color="#ffffff" stop-opacity="0.07"/><stop offset="1" stop-color="#ffffff" stop-opacity="0"/></radialGradient>')
a('<filter id="glow" x="-5%" y="-10%" width="110%" height="120%"><feGaussianBlur stdDeviation="6"/></filter>')
a('<filter id="soft" x="-20%" y="-300%" width="140%" height="700%"><feGaussianBlur stdDeviation="3"/></filter>')
a('<filter id="halo-blur" x="-80%" y="-80%" width="260%" height="260%"><feGaussianBlur stdDeviation="24"/></filter>')
a("</defs>")
a(f'<rect width="{W}" height="{H}" fill="#0d0d0d"/><rect width="{W}" height="{H}" fill="url(#lift)"/>')

# The outer box, the one in focus: three strands held at the same corners,
# each swaying its own way, their light under them.
outer = [frame(56, 50, W - 56, H - 50, 30, amp, ph, f1) for amp, ph, f1 in [(5.0, 0.0, 1.0), (4.2, 2.2, 1.15), (3.4, 4.1, 0.85)]]
a(f'<path d="{outer[0]}" fill="none" stroke="url(#run)" stroke-width="8" opacity="0.30" filter="url(#glow)"/>')
for d, w, op in zip(outer, [2.2, 1.4, 1.1], [1.0, 0.65, 0.45]):
    a(f'<path d="{d}" fill="none" stroke="url(#run)" stroke-width="{w}" opacity="{op}" stroke-linejoin="round"/>')

# A box inside it round the player, out of focus: one quieter line.
bx0, by0, bx1, by1 = 230, 418, W - 230, 498
inner_box = frame(bx0, by0, bx1, by1, 18, 2.4, 1.3, 1.6)
a(f'<path d="{inner_box}" fill="none" stroke="url(#dim)" stroke-width="1.3" opacity="0.8" stroke-linejoin="round"/>')

# The logo, with a soft light behind it, as the cover has in the interface.
ls = 144
lx, ly = (W - ls) / 2, 100
a(f'<rect x="{lx - 6}" y="{ly - 6}" width="{ls + 12}" height="{ls + 12}" rx="34" fill="#d8d8d8" opacity="0.30" filter="url(#halo-blur)"/>')
a(f'<svg x="{lx}" y="{ly}" width="{ls}" height="{ls}" viewBox="{vb}">{inner}</svg>')

a(f'<g {MONO} text-anchor="middle">')
a(f'<text x="{W / 2}" y="340" font-size="78" font-weight="700" fill="#f2f2f2" letter-spacing="2">utuimp</text>')
a(f'<text x="{W / 2}" y="388" font-size="26" fill="#b0b0b0">universal terminal user interface media player</text>')
a(f'<text x="{W / 2}" y="552" font-size="20" fill="#858585">yandex music · apple music · spotify · youtube music · soundcloud · local files</text>')
a("</g>")

# The progress line inside its box.
px0, px1, py, head = 352, 928, 458, 0.43
hx = px0 + (px1 - px0) * head
a(f'<path d="M{hx:.1f} {py} L{px1} {py}" stroke="#6e6e6e" stroke-width="1.6" stroke-linecap="round"/>')
for col, ph, f1, f2, amp in [("#ffffff", 0.3, 7.5, 15, 6.5), ("#bdbdbd", 2.1, 6.2, 13, 5.5), ("#8a8a8a", 4.0, 8.4, 17, 6.0)]:
    d = strand(px0, hx, py, amp, ph, f1, f2)
    a(f'<path d="{d}" fill="none" stroke="{col}" stroke-width="5" opacity="0.25" filter="url(#soft)"/>')
    a(f'<path d="{d}" fill="none" stroke="{col}" stroke-width="2" opacity="0.92" stroke-linecap="round"/>')
a(f'<circle cx="{hx:.1f}" cy="{py}" r="9" fill="#ffffff" opacity="0.30" filter="url(#soft)"/>')
a(f'<circle cx="{hx:.1f}" cy="{py}" r="4.6" fill="#ffffff"/>')
a(f'<g {MONO} font-size="20" fill="#9a9a9a">')
a(f'<text x="{px0 - 22}" y="{py + 7}" text-anchor="end">1:47</text>')
a(f'<text x="{px1 + 22}" y="{py + 7}" text-anchor="start">4:12</text>')
a("</g>")

a("</svg>")
open(out, "w").write("\n".join(s) + "\n")
