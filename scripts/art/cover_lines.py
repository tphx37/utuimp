"""The utuimp social preview in the style of the pixel interface: the real
logo (assets/utuimp_logo.svg, pixel art, at a whole scale so it stays sharp),
a frame of fine lines held at round corners, the progress line's strands.

usage: cover_lines.py <logo.svg> <out.svg>
"""
import math
import re
import sys

W, H = 1280, 640
PINK, ORANGE, BLUE = "#ff7aa8", "#ffb37a", "#7ab8ff"
MONO = "font-family=\"'JetBrains Mono','Noto Sans Mono','DejaVu Sans Mono',monospace\""

logo_src = open(sys.argv[1]).read()
vb = re.search(r"viewBox=\"([^\"]+)\"", logo_src).group(1)
out = sys.argv[2]

# The logo's own drawing, without its outer <svg>, its clip renamed.
inner = re.sub(r"^.*?<svg[^>]*>", "", logo_src, count=1, flags=re.S)
inner = inner[: inner.rfind("</svg>")]
inner = re.sub(r'id="([^"]+)"', r'id="logo-\1"', inner)
inner = re.sub(r"url\(#([^)]+)\)", r"url(#logo-\1)", inner)


def smooth(a, b, x):
    t = max(0.0, min(1.0, (x - a) / (b - a)))
    return t * t * (3 - 2 * t)


def frame_path(x0, y0, x1, y1, r, amp):
    """A rounded frame whose straight sides sway a little between corners
    that hold them, as the interface's frames do."""
    pts = []

    def side(ax, ay, bx, by, nx, ny, k):
        n = 80
        length = math.hypot(bx - ax, by - ay)
        for i in range(n + 1):
            u = i / n
            env = math.sin(math.pi * u) ** 1.5  # held at both ends
            wob = amp * env * (0.65 * math.sin(u * length / 260 + k * 1.7) + 0.35 * math.sin(u * length / 120 - k))
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


def strand(x0, x1, y, amp, ph, f1, f2, n=200):
    """One strand of the played part: it leaves the line after the start and
    comes back to it at the point where the song is."""
    pts = []
    for i in range(n + 1):
        u = i / n
        x = x0 + (x1 - x0) * u
        env = smooth(0, 0.18, u) * smooth(0, 0.12, 1 - u)
        pts.append((x, y + amp * env * (0.62 * math.sin(u * f1 + ph) + 0.38 * math.sin(u * f2 - ph * 1.3))))
    return "M" + " L".join("%.1f %.1f" % p for p in pts)


s = []
a = s.append
a(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}">')
a(f"""<defs>
<linearGradient id="run" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="{PINK}"/><stop offset="0.5" stop-color="{ORANGE}"/><stop offset="1" stop-color="{BLUE}"/>
</linearGradient>
<linearGradient id="halo" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="{PINK}"/><stop offset="0.5" stop-color="#e8e2e6"/><stop offset="1" stop-color="{BLUE}"/>
</linearGradient>
<radialGradient id="tintA" cx="0.12" cy="0.1" r="0.7"><stop offset="0" stop-color="{PINK}" stop-opacity="0.11"/><stop offset="1" stop-color="{PINK}" stop-opacity="0"/></radialGradient>
<radialGradient id="tintB" cx="0.9" cy="0.95" r="0.7"><stop offset="0" stop-color="{BLUE}" stop-opacity="0.10"/><stop offset="1" stop-color="{BLUE}" stop-opacity="0"/></radialGradient>
<filter id="glow" x="-5%" y="-10%" width="110%" height="120%"><feGaussianBlur stdDeviation="6"/></filter>
<filter id="soft" x="-20%" y="-300%" width="140%" height="700%"><feGaussianBlur stdDeviation="3"/></filter>
<filter id="halo-blur" x="-80%" y="-80%" width="260%" height="260%"><feGaussianBlur stdDeviation="24"/></filter>
</defs>""")
a(f'<rect width="{W}" height="{H}" fill="#0f0d10"/>')
a(f'<rect width="{W}" height="{H}" fill="url(#tintA)"/><rect width="{W}" height="{H}" fill="url(#tintB)"/>')

# The frame: its light, then the line. One line, no ghost of it.
f = frame_path(56, 50, W - 56, H - 50, 30, 2.6)
a(f'<path d="{f}" fill="none" stroke="url(#run)" stroke-width="7" opacity="0.38" filter="url(#glow)"/>')
a(f'<path d="{f}" fill="none" stroke="url(#run)" stroke-width="2.2" stroke-linejoin="round"/>')

# The logo, at three times its pixels, with a soft light behind it, as the
# cover has in the interface.
ls = 144
lx, ly = (W - ls) / 2, 100
a(f'<rect x="{lx - 6}" y="{ly - 6}" width="{ls + 12}" height="{ls + 12}" rx="34" fill="url(#halo)" opacity="0.42" filter="url(#halo-blur)"/>')
a(f'<svg x="{lx}" y="{ly}" width="{ls}" height="{ls}" viewBox="{vb}">{inner}</svg>')

# The words, in the terminal's letters, as the old preview has them.
a(f'<g {MONO} text-anchor="middle">')
a(f'<text x="{W / 2}" y="340" font-size="78" font-weight="700" fill="#f2eef0" letter-spacing="2">utuimp</text>')
a(f'<text x="{W / 2}" y="388" font-size="26" fill="#b6adb3">universal terminal user interface media player</text>')
a(f'<text x="{W / 2}" y="534" font-size="20" fill="#8a8187">yandex music · apple music · spotify · youtube music · soundcloud · local files</text>')
a("</g>")

# The progress line: strands over the played part, a thin line for the
# rest, the point where the song is.
px0, px1, py, head = 316, 964, 458, 0.43
hx = px0 + (px1 - px0) * head
a(f'<path d="M{hx:.1f} {py} L{px1} {py}" stroke="#7d747a" stroke-width="1.6" stroke-linecap="round"/>')
for col, ph, f1, f2, amp in [(PINK, 0.3, 7.5, 15, 6.5), (ORANGE, 2.1, 6.2, 13, 5.5), (BLUE, 4.0, 8.4, 17, 6.0)]:
    d = strand(px0, hx, py, amp, ph, f1, f2)
    a(f'<path d="{d}" fill="none" stroke="{col}" stroke-width="5" opacity="0.28" filter="url(#soft)"/>')
    a(f'<path d="{d}" fill="none" stroke="{col}" stroke-width="2" opacity="0.92" stroke-linecap="round"/>')
a(f'<circle cx="{hx:.1f}" cy="{py}" r="9" fill="{PINK}" opacity="0.35" filter="url(#soft)"/>')
a(f'<circle cx="{hx:.1f}" cy="{py}" r="4.6" fill="#ffffff"/>')
a(f'<g {MONO} font-size="20" fill="#a39aa0">')
a(f'<text x="{px0 - 22}" y="{py + 7}" text-anchor="end">1:47</text>')
a(f'<text x="{px1 + 22}" y="{py + 7}" text-anchor="start">4:12</text>')
a("</g>")

a("</svg>")
open(out, "w").write("\n".join(s) + "\n")
