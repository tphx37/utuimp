"""A made-up music library for README screenshots: synthesized songs, drawn
covers, lyrics written here. Nothing in it belongs to anyone else.

usage: readme_library.py <music dir>
"""
import math
import os
import subprocess
import sys
import wave

import numpy as np

OUT = sys.argv[1]
os.makedirs(OUT, exist_ok=True)
SR = 44100
rng = np.random.default_rng(7)

ALBUMS = [
    ("Paper Suns", "Lumen Drift", [(1.0, 0.42, 0.55), (1.0, 0.70, 0.35), (0.45, 0.30, 0.85)], 0.0),
    ("Low Tide Radio", "Marina Vale", [(0.20, 0.80, 0.85), (0.15, 0.35, 0.90), (0.95, 0.85, 0.55)], 1.7),
    ("Neon Orchard", "Kite Weather", [(0.65, 0.95, 0.40), (0.95, 0.35, 0.75), (0.30, 0.20, 0.55)], 3.1),
]

SONGS = [
    # title, album index, bpm, root midi, progression (semitones from root, minor/major)
    ("Glass Harbor", 0, 96, 57, [(0, "m"), (8, "M"), (3, "M"), (10, "M")]),
    ("Slow Satellites", 0, 84, 55, [(0, "M"), (9, "m"), (5, "M"), (7, "M")]),
    ("Orange Hours", 1, 104, 60, [(0, "M"), (7, "M"), (9, "m"), (5, "M")]),
    ("Undertow Postcards", 1, 90, 52, [(0, "m"), (5, "m"), (10, "M"), (3, "M")]),
    ("Fruit for the Moon", 2, 112, 62, [(0, "M"), (4, "m"), (5, "M"), (7, "M")]),
]

LYRICS = {
    "Glass Harbor": [
        "the boats come in made of light",
        "we count them from the window",
        "every hull a little song",
        "and the water hums along",
        "glass harbor, hold the evening",
        "till the last one finds the shore",
        "paper suns above the bay",
        "fold the day and put it away",
    ],
    "Orange Hours": [
        "orange hours on the kitchen wall",
        "radio low, the kettle calls",
        "you were humming half a tune",
        "something about june",
        "we can stay a little longer",
        "let the afternoon get warmer",
    ],
    "Fruit for the Moon": [
        "pick a pear for the moon tonight",
        "it is hungry and so bright",
        "kites are sleeping in the trees",
        "dreaming of a bigger breeze",
    ],
}


def note(m):
    return 440.0 * 2 ** ((m - 69) / 12)


def env(n, a, r):
    e = np.ones(n)
    ai, ri = int(a * SR), int(r * SR)
    if ai:
        e[:ai] = np.linspace(0, 1, ai)
    if ri:
        e[-ri:] *= np.linspace(1, 0, ri)
    return e


def song(bpm, root, prog, seconds):
    n = int(seconds * SR)
    out = np.zeros(n)
    beat = 60 / bpm
    bar = beat * 4
    t_all = np.arange(n) / SR
    # pads: the chord, soft
    i = 0
    t0 = 0.0
    while t0 < seconds:
        off, q = prog[i % len(prog)]
        third = 3 if q == "m" else 4
        s, e = int(t0 * SR), min(n, int((t0 + bar) * SR))
        tt = t_all[s:e] - t0
        chord = sum(np.sin(2 * math.pi * note(root + off + k) * tt + 0.3 * np.sin(2 * math.pi * 0.5 * tt))
                    for k in (0, third, 7, 12))
        out[s:e] += 0.05 * chord * env(e - s, 0.25, 0.3)
        # bass on each beat
        for b in range(4):
            bs = int((t0 + b * beat) * SR)
            be = min(n, bs + int(beat * 0.9 * SR))
            if bs >= n:
                break
            tb = np.arange(be - bs) / SR
            f = note(root + off - 12)
            out[bs:be] += 0.22 * np.sin(2 * math.pi * f * tb) * np.exp(-tb * 3)
        # arpeggio in eighths
        arp = [0, third, 7, 12, 7, third]
        for k in range(8):
            as_ = int((t0 + k * beat / 2) * SR)
            ae = min(n, as_ + int(beat / 2 * SR))
            if as_ >= n:
                break
            ta = np.arange(ae - as_) / SR
            f = note(root + off + 12 + arp[k % len(arp)])
            out[as_:ae] += 0.07 * (np.sin(2 * math.pi * f * ta) + 0.3 * np.sin(4 * math.pi * f * ta)) * np.exp(-ta * 6)
        t0 += bar
        i += 1
    # drums: kick on 1 and 3, snare on 2 and 4, hats in eighths
    kb = 0.0
    k = 0
    while kb < seconds:
        s = int(kb * SR)
        L = min(n - s, int(0.35 * SR))
        tk = np.arange(L) / SR
        if k % 4 in (0, 2):
            f = 50 + 90 * np.exp(-tk * 30)
            out[s:s + L] += 0.5 * np.sin(2 * math.pi * np.cumsum(f) / SR) * np.exp(-tk * 9)
        else:
            out[s:s + L] += 0.18 * rng.standard_normal(L) * np.exp(-tk * 18)
        for h in (0, 0.5):
            hs = int((kb + h * beat) * SR)
            hl = min(n - hs, int(0.05 * SR))
            if hl > 0:
                out[hs:hs + hl] += 0.05 * rng.standard_normal(hl) * np.exp(-np.arange(hl) / SR * 80)
        kb += beat
        k += 1
    # fade in and out, normalise
    out *= env(n, 1.5, 4.0)
    out /= np.max(np.abs(out)) * 1.15
    return out


def cover(cols, seed, path):
    S = 600
    y, x = np.mgrid[0:S, 0:S] / S
    img = np.zeros((S, S, 3))
    r = np.random.default_rng(int(seed * 100))
    base = np.array(cols[2]) * 0.35
    img[:] = base
    for j in range(7):
        c = np.array(cols[j % 3])
        cx, cy = r.uniform(0.1, 0.9, 2)
        rad = r.uniform(0.18, 0.42)
        w = np.exp(-((x - cx) ** 2 + (y - cy) ** 2) / (2 * rad ** 2))
        img = img * (1 - 0.75 * w[..., None]) + c * 0.75 * w[..., None]
    style = int(round(seed / 1.6))
    if style == 1:  # a low sun over the sea
        d = np.sqrt((x - 0.5) ** 2 + (y - 0.46) ** 2)
        img = img + (np.clip((0.2 - d) * 40, 0, 1) * 0.55)[..., None] * np.array([1.0, 0.9, 0.7])
        img = img * np.where(y > 0.6, 0.75, 1.0)[..., None]
    if style == 2:  # round fruit, hanging
        for j in range(6):
            cx, cy, rr = r.uniform(0.15, 0.85), r.uniform(0.2, 0.8), r.uniform(0.05, 0.11)
            d = np.sqrt((x - cx) ** 2 + (y - cy) ** 2)
            ring = np.exp(-((d - rr) ** 2) / (2 * 0.004 ** 2))
            img = img + 0.35 * ring[..., None] + (0.12 * (d < rr))[..., None]
    # a few soft bands, the way light lies on water
    for j in range(5 if style == 0 else 0):
        ph = r.uniform(0, 6.28)
        yy = 0.2 + 0.15 * j + 0.04 * np.sin(x * 6 + ph + seed)
        band = np.exp(-((y - yy) ** 2) / (2 * 0.006 ** 2))
        img = img + 0.22 * band[..., None]
    img += r.normal(0, 0.012, img.shape)  # grain
    img = np.clip(img, 0, 1)
    data = (img * 255).astype(np.uint8)
    ppm = path + ".ppm"
    with open(ppm, "wb") as f:
        f.write(b"P6 %d %d 255\n" % (S, S))
        f.write(data.tobytes())
    subprocess.run(["magick", ppm, "-quality", "92", path], check=True)
    os.remove(ppm)


def lrc(lines, start, step, word_sync=False):
    def stamp(t):
        return "%02d:%05.2f" % (int(t // 60), t % 60)

    out = []
    t = start
    for line in lines:
        text = line
        if word_sync:
            words = line.split(" ")
            duration = step * 0.86
            text = "".join("<%s>%s%s" % (stamp(t + i * duration / len(words)), word,
                                       " " if i + 1 < len(words) else "")
                           for i, word in enumerate(words))
            text += "<%s>" % stamp(t + duration)
        out.append("[%s]%s" % (stamp(t), text))
        t += step
    return "\n".join(out) + "\n"


covers = {}
for title, artist, cols, seed in ALBUMS:
    p = os.path.join(OUT, title.replace(" ", "_") + ".jpg")
    cover(cols, seed, p)
    covers[title] = p

for no, (title, ai, bpm, root, prog) in enumerate(SONGS, 1):
    album, artist, _, _ = ALBUMS[ai]
    secs = 150 + 13 * no
    a = song(bpm, root, prog, secs)
    st = np.stack([a, np.roll(a, 90)], axis=1)
    pcm = (st * 32767).astype("<i2")
    wav = os.path.join(OUT, "tmp.wav")
    with wave.open(wav, "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(SR)
        w.writeframes(pcm.tobytes())
    base = os.path.join(OUT, "%02d %s" % (no, title))
    subprocess.run(["ffmpeg", "-loglevel", "error", "-y", "-i", wav, "-i", covers[album],
                    "-map", "0:a", "-map", "1:v", "-c:a", "libmp3lame", "-q:a", "4", "-c:v", "mjpeg",
                    "-id3v2_version", "3", "-disposition:v", "attached_pic",
                    "-metadata", "title=" + title, "-metadata", "artist=" + artist,
                    "-metadata", "album=" + album, "-metadata", "track=%d" % no,
                    "-metadata:s:v", "comment=Cover (front)", base + ".mp3"], check=True)
    os.remove(wav)
    if title in LYRICS:
        beat = 60 / bpm
        with open(base + ".lrc", "w") as f:
            f.write(lrc(LYRICS[title] * 6, 4 * beat * 2, 4 * beat * 2, word_sync=title == "Glass Harbor"))
for p in covers.values():
    os.remove(p)
print("ok")
