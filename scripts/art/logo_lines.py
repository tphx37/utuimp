"""The utuimp logo, black and white, its ribbons made of the interface's
fine swaying strands instead of pixels. The note keeps its place and shape.

usage: logo_lines.py <out.svg> [square]

"square" leaves the corners square, for places that round or mask the icon
themselves.
"""
import math
import sys

S = 1024
out = sys.argv[1]
R = 0 if sys.argv[2:] == ["square"] else 9.5 / 48 * S


def smooth(a, b, x):
    t = max(0.0, min(1.0, (x - a) / (b - a)))
    return t * t * (3 - 2 * t)


def path(pts):
    return "M" + " L".join("%.1f %.1f" % p for p in pts)


def bundle(base, n, spread, pinches, seed, grey, width, n_pts=220):
    """n strands round a base curve. They part and come together again at
    the pinches, as the progress line's strands are held where the song is."""
    res = []
    for k in range(n):
        f = (k / max(1, n - 1)) * 2 - 1  # -1..1 across the bundle
        ph = seed + k * 2.399  # golden angle: no two strands alike
        pts = []
        for i in range(n_pts + 1):
            u = i / n_pts
            x = -40 + (S + 80) * u
            pinch = 1.0
            for p, w in pinches:
                pinch *= 0.18 + 0.82 * smooth(0, w, abs(u - p))
            wav = 0.55 * math.sin(u * 9.0 + ph) + 0.45 * math.sin(u * 17.0 - ph * 1.3)
            y = base(u) + pinch * spread * (f * 0.7 + 0.6 * wav)
            pts.append((x, y))
        # the middle strands are the light ones, the outer ones fade
        lum = grey * (1 - 0.55 * abs(f)) * (0.85 + 0.15 * math.sin(ph * 3))
        res.append((path(pts), lum, width * (1.25 - 0.5 * abs(f))))
    return res


def hexgrey(v):
    c = max(0, min(255, int(round(v * 255))))
    return "#%02x%02x%02x" % (c, c, c)


# Where the pixel logo has its ribbons: one up high, falling and rising,
# one through the middle, a wide bright one low down.
high = lambda u: S * (0.13 + 0.17 * smooth(0.05, 0.62, u) - 0.05 * smooth(0.6, 1.0, u) + 0.02 * math.sin(u * 5))
mid = lambda u: S * (0.475 + 0.025 * math.sin(u * 4.2 + 0.6) + 0.03 * smooth(0.55, 1, u))
low = lambda u: S * (0.70 + 0.03 * math.sin(u * 3.3 + 2.0) - 0.02 * smooth(0, 0.4, u))

strands = (
    bundle(high, 10, 40, [(0.42, 0.22)], 0.4, 0.75, 2.6)
    + bundle(mid, 12, 46, [(0.24, 0.2), (0.86, 0.18)], 1.7, 0.95, 3.0)
    + bundle(low, 16, 70, [(0.62, 0.26)], 3.1, 1.15, 3.6)
)

# The note, on the logo's grid (48 to the side), drawn smooth.
g = S / 48
stem_x0, stem_x1, top, foot = 23.0 * g, 25.4 * g, 13.6 * g, 29.6 * g
note = (
    f"M{stem_x0:.1f} {top:.1f} L{stem_x1:.1f} {top:.1f} "
    # the flag, out and down to the right, curling back in
    f"C{27.6 * g:.1f} {15.6 * g:.1f} {31.4 * g:.1f} {17.4 * g:.1f} {31.0 * g:.1f} {21.0 * g:.1f} "
    f"C{30.8 * g:.1f} {22.4 * g:.1f} {30.2 * g:.1f} {23.4 * g:.1f} {29.3 * g:.1f} {24.0 * g:.1f} "
    f"C{29.7 * g:.1f} {21.2 * g:.1f} {28.0 * g:.1f} {19.4 * g:.1f} {stem_x1:.1f} {18.6 * g:.1f} "
    f"L{stem_x1:.1f} {foot:.1f} "
    # the head
    f"C{stem_x1 + 1.4 * g:.1f} {32.6 * g:.1f} {23.0 * g:.1f} {34.6 * g:.1f} {20.6 * g:.1f} {34.4 * g:.1f} "
    f"C{18.2 * g:.1f} {34.2 * g:.1f} {17.4 * g:.1f} {32.6 * g:.1f} {18.0 * g:.1f} {31.0 * g:.1f} "
    f"C{18.8 * g:.1f} {29.0 * g:.1f} {21.4 * g:.1f} {28.2 * g:.1f} {stem_x0:.1f} {28.6 * g:.1f} Z"
)

s = []
a = s.append
a(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {S} {S}" width="{S}" height="{S}">')
a(f"""<defs>
<clipPath id="corners"><rect width="{S}" height="{S}" rx="{R:.1f}"/></clipPath>
<linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="#0b0b0b"/><stop offset="0.6" stop-color="#121212"/><stop offset="1" stop-color="#080808"/>
</linearGradient>
<radialGradient id="lift" cx="0.5" cy="0.55" r="0.6"><stop offset="0" stop-color="#ffffff" stop-opacity="0.06"/><stop offset="1" stop-color="#ffffff" stop-opacity="0"/></radialGradient>
<linearGradient id="fade" x1="0" y1="0" x2="1" y2="0">
<stop offset="0" stop-color="#fff" stop-opacity="0.55"/><stop offset="0.25" stop-color="#fff"/><stop offset="0.75" stop-color="#fff"/><stop offset="1" stop-color="#fff" stop-opacity="0.55"/>
</linearGradient>
<mask id="ends"><rect width="{S}" height="{S}" fill="url(#fade)"/></mask>
<filter id="glow" x="-10%" y="-30%" width="120%" height="160%"><feGaussianBlur stdDeviation="9"/></filter>
<filter id="note-light" x="-60%" y="-60%" width="220%" height="220%"><feGaussianBlur stdDeviation="26"/></filter>
</defs>""")
a('<g clip-path="url(#corners)">')
a(f'<rect width="{S}" height="{S}" fill="url(#bg)"/><rect width="{S}" height="{S}" fill="url(#lift)"/>')
a('<g mask="url(#ends)" fill="none" stroke-linecap="round" stroke-linejoin="round">')
# their light first, then the lines
a('<g filter="url(#glow)" opacity="0.8">')
for d, lum, w in strands:
    a(f'<path d="{d}" stroke="{hexgrey(lum)}" stroke-width="{w * 3:.1f}" opacity="0.45"/>')
a("</g>")
for d, lum, w in strands:
    a(f'<path d="{d}" stroke="{hexgrey(lum)}" stroke-width="{w:.2f}" opacity="0.9"/>')
a("</g>")
# the note: a soft light behind it, a dark rim that parts it from the strands
a(f'<path d="{note}" fill="#ffffff" opacity="0.22" filter="url(#note-light)"/>')
a(f'<path d="{note}" fill="#050505" stroke="#050505" stroke-width="{1.3 * g:.1f}" stroke-linejoin="round"/>')
a(f'<path d="{note}" fill="#f7f7f7"/>')
a("</g>")
a("</svg>")
open(out, "w").write("\n".join(s) + "\n")
