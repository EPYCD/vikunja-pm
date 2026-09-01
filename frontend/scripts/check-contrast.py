#!/usr/bin/env python3
"""Check the Workman palette against WCAG AA, in both themes.

Reads the real token values out of src/styles/custom-properties/colors.scss, so
it fails when someone changes a colour rather than passing on a stale copy.

Run from frontend/:  python3 scripts/check-contrast.py
Exits non-zero if any pair falls short.
"""

import colorsys
import pathlib
import re
import sys

COLORS = pathlib.Path(__file__).resolve().parent.parent / 'src/styles/custom-properties/colors.scss'

# fg token, bg token, required ratio, description.
# 4.5 for text, 3.0 for the edge of a control (WCAG 1.4.11).
CHECKS = [
    ('--wm-text', '--wm-canvas', 4.5, 'body text on canvas'),
    ('--wm-text', '--wm-surface', 4.5, 'body text on surface'),
    ('--wm-text-secondary', '--wm-canvas', 4.5, 'secondary text on canvas'),
    ('--wm-text-secondary', '--wm-surface', 4.5, 'secondary text on surface'),
    ('--wm-text-tertiary', '--wm-canvas', 4.5, 'mono micro-labels on canvas'),
    ('--wm-text-tertiary', '--wm-surface', 4.5, 'mono micro-labels on surface'),
    ('--wm-accent-text', '--wm-surface', 4.5, 'accent text on surface'),
    ('--wm-accent-text', '--wm-canvas', 4.5, 'accent text on canvas'),
    ('--wm-on-accent', '--wm-accent', 4.5, 'button label on accent fill'),
    ('--wm-line-control', '--wm-surface', 3.0, 'control border on surface'),
    ('--wm-accent', '--wm-canvas', 3.0, 'focus ring on canvas'),
    ('--wm-accent', '--wm-surface', 3.0, 'focus ring on surface'),
]

# Status colours are declared as separate -h / -s / -l values, not one hsl().
STATUS = ['success', 'warning', 'danger', 'info']

DECL = re.compile(r'(--[a-z0-9-]+):\s*([^;]+);')
AXIS = re.compile(r'(--wm-[a-z]+)-([hsl]):\s*([\d.]+)')
# Each hsl() argument is independently either a literal or a var() reference —
# e.g. hsl(var(--wm-accent-h), 84%, 68%) — so parse the three slots generically
# rather than pattern-matching whole shapes.
HSL_CALL = re.compile(r'hsla?\(((?:[^()]|\([^()]*\))*)\)')
ARG_VAR = re.compile(r'var\(\s*(--[a-z0-9-]+)\s*[,)]')
ARG_NUM = re.compile(r'^\s*([\d.]+)(?:deg|%)?\s*$')


def split_args(inner):
    """Split hsl(...) arguments on top-level commas."""
    args, depth, cur = [], 0, ''
    for ch in inner:
        if ch == '(':
            depth += 1
        elif ch == ')':
            depth -= 1
        if ch == ',' and depth == 0:
            args.append(cur)
            cur = ''
        else:
            cur += ch
    if cur.strip():
        args.append(cur)
    return args


def luminance(triple):
    h, s, ll = triple
    r, g, b = colorsys.hls_to_rgb(h / 360, ll / 100, s / 100)

    def lin(c):
        return c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4

    return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)


def ratio(a, b):
    la, lb = luminance(a), luminance(b)
    return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)


def axes(src):
    """Tokens assembled from -h / -s / -l parts."""
    out = {}
    for name, axis, num in AXIS.findall(src):
        out.setdefault(name, {})[axis] = float(num)
    return out


def resolve(value, ax, axis_index):
    """Resolve one hsl() argument to a number, following var() into the axes."""
    m = ARG_NUM.match(value)
    if m:
        return float(m.group(1))
    m = ARG_VAR.search(value)
    if not m:
        return None
    ref = m.group(1)
    for suffix, key in (('-h', 'h'), ('-s', 's'), ('-l', 'l')):
        if ref.endswith(suffix):
            parts = ax.get(ref[: -len(suffix)], {})
            return parts.get(key)
    # A whole-triple reference such as var(--wm-accent-hsl): the slot position
    # tells us which axis this argument wanted.
    if ref.endswith('-hsl'):
        return ax.get(ref[:-4], {}).get('hsl'[axis_index])
    return None


def build(src_light, src_dark):
    ax_light = axes(src_light)
    ax_dark = {k: dict(v) for k, v in ax_light.items()}
    for name, parts in axes(src_dark).items():
        ax_dark.setdefault(name, {}).update(parts)

    def collect(src, ax, base=None):
        tokens = dict(base or {})
        for name, parts in ax.items():
            if {'h', 's', 'l'} <= parts.keys():
                tokens[name] = (parts['h'], parts['s'], parts['l'])
        for name, value in DECL.findall(src):
            m = HSL_CALL.search(value)
            if not m:
                continue
            args = split_args(m.group(1))
            if len(args) < 3:
                continue
            triple = [resolve(a, ax, i) for i, a in enumerate(args[:3])]
            if all(v is not None for v in triple):
                tokens[name] = tuple(triple)
        return tokens

    light = collect(src_light, ax_light)
    dark = collect(src_dark, ax_dark, base=light)
    return light, dark


def main():
    text = COLORS.read_text()
    split = text.index('&.dark')
    light, dark = build(text[:split], text[split:])

    checks = list(CHECKS) + [
        (f'--wm-{s}', '--wm-surface', 4.5, f'{s} on surface') for s in STATUS
    ]

    failures = []
    for theme_name, tokens in (('light', light), ('dark', dark)):
        print(f'\n=== {theme_name} ===')
        for fg, bg, need, label in checks:
            if fg not in tokens or bg not in tokens:
                print(f'  [SKIP] {label} — {fg if fg not in tokens else bg} not found')
                continue
            r = ratio(tokens[fg], tokens[bg])
            ok = r >= need
            if not ok:
                failures.append(f'{theme_name}: {label} is {r:.2f}:1, needs {need}:1')
            print(f'  [{"PASS" if ok else "FAIL"}] {r:5.2f}:1  (need {need})  {label}')

    if failures:
        print('\n' + '\n'.join(f'  {f}' for f in failures))
        print(f'\n{len(failures)} contrast failure(s)')
        return 1

    print('\nAll contrast checks pass in both themes.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
