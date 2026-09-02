#!/usr/bin/env python3
"""Rasterise the Workman mark into every app-icon size the web client ships.

Run from frontend/:  python3 scripts/generate-icons.py

Source of truth is src/assets/workman-mark.svg. The maskable icon gets extra
padding because Android crops up to 20% off each edge of a maskable icon.
The tab favicon swapped in while a timer runs comes from the -tracking mark.
"""

import pathlib
import subprocess
import sys

try:
    import cairosvg
except ImportError:
    sys.exit('cairosvg is required: pip install cairosvg')

ROOT = pathlib.Path(__file__).resolve().parent.parent
MARK = ROOT / 'src/assets/workman-mark.svg'
TRACKING_MARK = ROOT / 'src/assets/workman-mark-tracking.svg'
ICONS = ROOT / 'public/images/icons'

# Icons drawn edge to edge.
PLAIN = {
    'android-chrome-192x192.png': 192,
    'android-chrome-512x512.png': 512,
    'apple-touch-icon.png': 180,
    'apple-touch-icon-180x180.png': 180,
    'apple-touch-icon-152x152.png': 152,
    'apple-touch-icon-120x120.png': 120,
    'apple-touch-icon-76x76.png': 76,
    'apple-touch-icon-60x60.png': 60,
    'favicon-16x16.png': 16,
    'favicon-32x32.png': 32,
    'msapplication-icon-144x144.png': 144,
    'mstile-150x150.png': 150,
}


def render(svg: pathlib.Path, out: pathlib.Path, size: int, pad_ratio: float = 0.0) -> None:
    source = svg.read_text()
    if pad_ratio:
        # Widen the viewBox so the mark sits inside the maskable safe zone.
        import re

        match = re.search(r'viewBox="([\d.\-\s]+)"', source)
        x, y, w, h = (float(v) for v in match.group(1).split())
        pad = w * pad_ratio
        source = source.replace(
            match.group(0),
            f'viewBox="{x - pad} {y - pad} {w + pad * 2} {h + pad * 2}"',
        )
    out.parent.mkdir(parents=True, exist_ok=True)
    cairosvg.svg2png(
        bytestring=source.encode(),
        write_to=str(out),
        output_width=size,
        output_height=size,
    )
    print(f'  {out.relative_to(ROOT)}  {size}x{size}')


def main() -> None:
    for required in (MARK, TRACKING_MARK):
        if not required.exists():
            sys.exit(f'missing {required}')

    print('Rendering app icons')
    for name, size in PLAIN.items():
        render(MARK, ICONS / name, size)

    # useTimeTrackingFavicon swaps this in while the clock is running.
    render(TRACKING_MARK, ICONS / 'favicon-tracking-32x32.png', 32)

    render(MARK, ICONS / 'icon-maskable.png', 1024, pad_ratio=0.18)
    render(MARK, ICONS / 'badge-monochrome.png', 96)

    # The SVG favicon modern browsers prefer. Drop the fixed width/height so it
    # scales purely from the viewBox at whatever size the tab asks for.
    import re

    favicon_svg = re.sub(r'\s+width="64"\s+height="64"', '', MARK.read_text(), count=1)
    (ICONS / 'workman-mark.svg').write_text(favicon_svg)
    print(f'  {(ICONS / "workman-mark.svg").relative_to(ROOT)}')

    # Multi-resolution .ico for legacy browsers.
    try:
        from PIL import Image

        frames = [
            Image.open(ICONS / 'favicon-32x32.png').convert('RGBA'),
        ]
        frames[0].save(
            ROOT / 'public/favicon.ico',
            format='ICO',
            sizes=[(16, 16), (32, 32), (48, 48)],
        )
        print('  public/favicon.ico')
    except ImportError:
        print('  (skipped favicon.ico — Pillow not installed)')

    subprocess.run(['ls', '-la', str(ICONS)], check=False)


if __name__ == '__main__':
    main()
