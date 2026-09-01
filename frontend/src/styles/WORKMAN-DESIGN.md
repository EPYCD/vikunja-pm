# Workman design system

The reference is an operations console, not a productivity app: near-black ground,
hairline structure, monospace data, one decisive accent. Dense, precise, quiet
until something needs attention. Think mission-control instrument panel.

Every rule below is already expressed as a token or mixin. **Use the tokens.**
Do not hand-write hex values, radii, durations or font stacks.

---

## 1. The five signatures

These are what make a screen read as Workman. A surface that uses none of them is
not finished.

1. **The 45° chamfer.** One or two corners are *cut away*, not rounded. Panels,
   inputs, primary actions, step badges. `@include chamfer($size, $corners)`.
2. **Monospace micro-labels.** Every table header, section eyebrow, metric
   caption, status chip and unit is uppercase JetBrains Mono with `0.08em`
   tracking at 11px. `@include mono-label`. This does more work than any other
   single rule.
3. **The graticule.** A faint blueprint grid behind empty states, auth screens
   and large voids. `@include graticule`.
4. **Hairlines over shadows.** Structure comes from 1px `var(--wm-line)` rules.
   Shadows only for things that genuinely float (dropdown, modal, dragged card).
5. **Accent restraint.** Crimson marks exactly one thing per view: the primary
   action, or the active item. If two things are red, one of them is wrong.

## 2. Color

Layer 1 (`--wm-*`) is the source of truth; the legacy Bulma/grey names map onto
it. Only layer 1 is redefined for dark mode.

| Token | Use |
| --- | --- |
| `--wm-canvas` | Page floor — the darkest surface |
| `--wm-surface` | Cards, panels, the thing you read off |
| `--wm-surface-raised` | Dropdowns, modals, popovers |
| `--wm-surface-hover` | Row and control hover |
| `--wm-line` / `--wm-line-strong` / `--wm-line-faint` | Hairlines |
| `--wm-text` / `--wm-text-secondary` / `--wm-text-tertiary` | Text ramp |
| `--wm-accent` | Brand + primary action + active state |
| `--wm-accent-text` | Accent as *text or icon* (contrast-safe) |
| `--wm-accent-wash` / `--wm-accent-wash-strong` | Selected rows, tinted chips |
| `--wm-accent-line` | Accent-tinted hairline |

Status colors keep their own hues: `--success`, `--warning`, `--danger`, `--info`.
`--danger` is the hotter, brighter red — it must never be mistaken for the brand
crimson. When you need accent-colored *text*, use `--wm-accent-text`, never
`--wm-accent` (which fails contrast on light backgrounds).

`--wm-line-control` exists because WCAG wants 3:1 for the edge of a form
control, where the border is the only thing marking the field. Applying that to
every hairline would make the decorative rules shout, so control borders use
this token and separators use `--wm-line` / `--wm-line-faint`.

**Every colour pair is verified.** `python3 scripts/check-contrast.py` reads the
real values out of `colors.scss` and fails on anything below AA in either theme.
Run it after touching a colour.

## 3. Type

| Face | Variable | Job |
| --- | --- | --- |
| Space Grotesk | `$workman-display-font` | Wordmark, headings, empty-state titles |
| Inter | `$family-sans-serif` | All interface and body text |
| JetBrains Mono | `$workman-mono-font` | Labels, IDs, counts, dates, durations |

Sizes are `--wm-text-2xs` … `--wm-text-2xl`. Body is `--wm-text-base` (14px).
Anything numeric gets `@include mono-data` so digits don't jitter as they update.
Task identifiers (`PROJ-12`), timers, dates, percentages and counts are all mono.

## 4. Geometry and motion

- Chamfer: `--wm-chamfer-sm|--wm-chamfer|--wm-chamfer-lg|--wm-chamfer-xl` (6/10/16/24px).
- Radius (only where a chamfer would fight — avatars, bubbles, pills, ≤16px things):
  `--wm-radius-xs|sm|DEFAULT|full`.
- Controls: `--wm-control-height` 34px (26 small, 42 large).
- Spacing: `--wm-space-1` … `--wm-space-7` on a 4px base.
- Motion: `--wm-duration-fast|--wm-duration|--wm-duration-slow` with `--wm-ease`.
  Transitions are short and mechanical. Nothing bounces.

## 5. Mixins

```scss
@include chamfer($size: $chamfer, $corners: bottom-right);  // shape
@include chamfer-outline(var(--wm-line));                   // 1px outline that follows the cut
@include mono-label($size: var(--wm-text-2xs));             // uppercase tracked mono
@include mono-data;                                          // tabular figures
@include graticule($size: var(--wm-grid-size));             // blueprint grid
@include focus-ring;                                         // accent focus ring
```

**Chamfer + border do not mix.** `clip-path` slices a `border` open along the
diagonal. A chamfered element that needs an outline uses `chamfer-outline`
(four offset drop-shadows tracing the clipped alpha mask) and no `border`.
Chamfered elements also can't cast a `box-shadow` — use the outline, or put the
shadow on an unclipped parent.

## 6. Component patterns

- **Buttons.** Default is *outlined*: hairline border, transparent fill,
  mono-uppercase label, 34px tall, sharp corners. Primary is filled crimson with
  a bottom-right chamfer. Tertiary is text-only. Trailing glyphs (`+`, `→`) are
  welcome on outlined actions.
- **Panels/cards.** `--wm-surface`, 1px `--wm-line`, chamfered bottom-right at
  `--wm-chamfer`. No radius, no drop shadow.
- **Tables.** Header row is a `mono-label` band on `--wm-surface-sunken` with a
  hairline beneath. Rows separated by `--wm-line-faint`. Hover is
  `--wm-surface-hover`. Numeric columns right-aligned and mono.
- **Inputs.** Flat `--wm-surface`, 1px `--wm-line`, no inner shadow, chamfered
  bottom-right, accent border + `focus-ring` on focus.
- **Empty states.** Dashed 1px `--wm-line-strong` container over a graticule,
  Space Grotesk title, mono caption.
- **Active nav item.** A 2px crimson rail on the inline-start edge plus
  `--wm-accent-wash` fill. Never a rounded filled pill.
- **Section headers.** `mono-label` eyebrow with a count (`PROJECTS / 12`),
  optionally followed by a hairline rule that fills the remaining width.

## 7. Non-negotiables

- Both themes must work. Dark is the identity; light is the same system
  inverted, and is not allowed to regress.
- Contrast: body text ≥ 4.5:1, large text and UI borders ≥ 3:1, in **both** themes.
- Never remove an `aria-*`, `role`, `:focus-visible` or `is-sr-only` affordance
  while restyling. Focus must stay visible on the near-black ground.
- Respect `prefers-reduced-motion`.
- Logical CSS properties only (`inline-size`, `margin-block-end`, `inset-inline-start`)
  — stylelint enforces this.
- Tailwind utilities are prefixed: `tw:flex`, `tw:bg-surface`. SCSS + tokens
  remain the primary tool; reach for Tailwind only for one-off layout.
