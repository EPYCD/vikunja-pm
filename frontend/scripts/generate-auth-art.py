#!/usr/bin/env python3
"""Generate the Workman sign-in backdrop.

A minimal landscape — a crimson sun behind four receding ridges — rendered
entirely as a JetBrains Mono character matrix. The terminal-halftone texture the
console language already implies, at page scale.

Deterministic: a fixed seed, so re-running produces the identical image and the
asset does not churn in git.

Run from frontend/:  python3 scripts/generate-auth-art.py
"""

import glob
import math
import pathlib
import random
import sys

try:
    from PIL import Image, ImageDraw, ImageFont
except ImportError:
    sys.exit('Pillow is required: pip install pillow')

try:
    from fontTools.ttLib import TTFont
except ImportError:
    sys.exit('fonttools is required: pip install fonttools brotli')

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / 'src/assets'

W, H = 1920, 1200
CELL_W, CELL_H = 10, 15
FONT_SIZE = 14
SEED = 20260901

SKY_RAMP = list('`.\'^:')
SUN_RAMP = list(':=*o08%#@')
# Nearest ridge last: denser and heavier as it comes forward.
RIDGE_RAMP = [list(':=+o'), list('+o08'), list('08%#'), list('8%#@')]

SUN_CX, SUN_CY_R, SUN_R_R = 0.72, 0.46, 0.185

# base height (fraction of frame), amplitude, noise frequency
RIDGES = [
    (0.56, 0.075, 5),
    (0.66, 0.095, 8),
    (0.78, 0.110, 13),
    (0.92, 0.130, 21),
]

THEMES = {
    'dark': {
        'bg': (10, 10, 11),
        # Dusk: the sky has to carry enough value for the ridge silhouettes to
        # have something to sit against, or the whole image reads as mud.
        'sky': ((18, 18, 23), (104, 90, 102)),
        'sun': ((255, 138, 140), (196, 28, 34)),
        # Atmospheric perspective: distant ridges hazy and light, near ridges
        # dark, so the layers separate by value rather than by outline.
        'ridges': [(132, 114, 132), (86, 74, 92), (48, 42, 56), (16, 15, 19)],
        'rim': (232, 52, 58),
        'name': 'auth-backdrop.webp',
    },
    'light': {
        'bg': (236, 238, 242),
        'sky': ((231, 234, 240), (176, 182, 196)),
        'sun': ((216, 58, 64), (170, 20, 26)),
        'ridges': [(196, 201, 211), (154, 160, 173), (108, 114, 129), (56, 61, 73)],
        'rim': (186, 28, 34),
        'name': 'auth-backdrop-light.webp',
    },
}


def noise_1d(n, freq, rng):
    """Smooth 1D noise by upsampling a small random lattice."""
    small = Image.new('L', (max(2, int(freq)), 1))
    small.putdata([rng.randrange(256) for _ in range(small.width)])
    big = small.resize((n, 1), Image.BICUBIC).load()
    return [big[i, 0] / 255.0 for i in range(n)]


def mix(a, b, t):
    t = max(0.0, min(1.0, t))
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))


def main():
    src = glob.glob(str(ROOT / 'src/assets/fonts/JetBrainsMono-latin_*.woff2'))
    if not src:
        sys.exit('JetBrains Mono not found — run scripts/fonts-download.sh first')

    ttf = pathlib.Path('/tmp/wm-jetbrains-mono.ttf')
    f = TTFont(src[0])
    f.flavor = None
    f.save(str(ttf))
    font = ImageFont.truetype(str(ttf), FONT_SIZE)

    cols, rows = W // CELL_W, H // CELL_H
    rng = random.Random(SEED)

    # Ridge silhouettes, far to near.
    profiles = []
    for base, amp, freq in RIDGES:
        n1 = noise_1d(cols, freq, rng)
        n2 = noise_1d(cols, freq * 2, rng)
        profiles.append([
            (base + (n1[x] - 0.5) * amp * 2 + (n2[x] - 0.5) * amp * 0.7) * rows
            for x in range(cols)
        ])

    sun_cx, sun_cy, sun_r = cols * SUN_CX, rows * SUN_CY_R, rows * SUN_R_R
    aspect = CELL_W / CELL_H
    jitter = random.Random(SEED + 1)

    for theme, cfg in THEMES.items():
        img = Image.new('RGB', (W, H), cfg['bg'])
        draw = ImageDraw.Draw(img)

        for y in range(rows):
            for x in range(cols):
                # Which ridge owns this cell? Nearest one whose profile is above it.
                layer = None
                for i in range(len(profiles) - 1, -1, -1):
                    if y >= profiles[i][x]:
                        layer = i
                        break

                if layer is not None:
                    ramp = RIDGE_RAMP[layer]
                    depth = (y - profiles[layer][x]) / max(1.0, rows * 0.16)
                    t = min(1.0, 0.25 + depth)
                    ch = ramp[min(len(ramp) - 1, int(t * len(ramp)))]
                    color = cfg['ridges'][layer]
                    # A lit rim on the crest, brightest where the sun sits behind.
                    if y - profiles[layer][x] < 1.2:
                        glow = max(0.0, 1.0 - abs(x - sun_cx) / (cols * 0.60))
                        color = mix(color, cfg['rim'], 0.34 + glow * 0.60)
                    draw.text((x * CELL_W, y * CELL_H), ch, font=font, fill=color)
                    continue

                # Sky, and the sun sitting in it.
                d = math.hypot((x - sun_cx) * aspect, y - sun_cy) / sun_r
                if d <= 1.0:
                    t = 1.0 - d
                    ch = SUN_RAMP[min(len(SUN_RAMP) - 1, int((0.30 + t * 0.85) * len(SUN_RAMP)))]
                    color = mix(cfg['sun'][1], cfg['sun'][0], t * 1.15)
                    draw.text((x * CELL_W, y * CELL_H), ch, font=font, fill=color)
                    continue

                # Corona fading out of the disc.
                halo = max(0.0, 1.0 - (d - 1.0) / 0.85)
                density = 0.18 + (y / rows) * 0.62 + halo * 0.95
                if jitter.random() > density * 0.8:
                    continue
                ch = SKY_RAMP[min(len(SKY_RAMP) - 1, int(density * len(SKY_RAMP)))]
                color = mix(cfg['sky'][0], cfg['sky'][1], y / rows)
                if halo > 0:
                    color = mix(color, cfg['rim'], halo * 0.55)
                draw.text((x * CELL_W, y * CELL_H), ch, font=font, fill=color)

        # Lossless WebP: the image is flat glyph shapes over flat ground, so a
        # limited-palette lossless encode beats both PNG and any lossy setting.
        dest = OUT / cfg['name']
        img.save(dest, 'WEBP', lossless=True, quality=100, method=6)
        print(f'  {dest.relative_to(ROOT)}  {dest.stat().st_size // 1024} KB  ({theme})')


if __name__ == '__main__':
    main()
