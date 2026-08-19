# Yamale — “Monolith” identity kit

A solid block with the Y cut clean out of it, the stem re-filled in brass. The block is the
ledger: closed, permissioned, immovable. The cut is the transfer passing through it — two
counterparties in, one settled position out. Everything is machined at 45°, with flat terminals
and mitred joins, so the mark reads as milled rather than drawn.

Open `brand-sheet.html` in a browser for the visual version of everything below.

---

## Contents

```
svg/          15 vector files. Master artwork — scale these, never the PNGs.
png/          Raster exports, 16–1024 px, plus transparent lockups at 1600 px wide.
social/       og-1200x630.png · banner-1500x500.png · avatar-1000x1000.png
favicon.ico   Multi-resolution 16/32/48 px browser icon.
brand-sheet.html
README.md
```

## Which file to use

| Situation | File |
|---|---|
| Default, any background | `svg/yamale-mark.svg` |
| Navy or Deep Ink surfaces | `svg/yamale-mark-on-dark.svg` |
| Over photography or colour | `svg/yamale-mark-knockout.svg` |
| Single-ink print, filings | `svg/yamale-mark-mono-navy.svg` · `-black` · `-white` |
| 32 px and below | `svg/yamale-mark-small.svg` |
| Default signature | `svg/yamale-lockup-horizontal.svg` |
| Square / narrow spaces | `svg/yamale-lockup-stacked.svg` |

`yamale-mark-soft.svg` is the original round-cap drawing from the trial sheet, kept in case you
prefer it. Pick one cut and stay with it — do not use both in the same piece.

## Palette

| Name | Hex | RGB | Where it goes |
|---|---|---|---|
| Yamale Navy | `#12253F` | 18 37 63 | Primary. Block, wordmark, headings, dark UI surfaces. |
| Deep Ink | `#0B1523` | 11 21 35 | Deepest background. Site chrome, social cards, footers. |
| Brass | `#A87B3C` | 168 123 60 | Accent on light. The stem, rules, key figures. Sparingly. |
| Brass Light | `#D2A65E` | 210 166 94 | Accent on dark. Same role, lifted for contrast. |
| Signal Slate | `#5C6E88` | 92 110 136 | Secondary text, captions, inactive states. |
| Mist | `#E3E7EE` | 227 231 238 | Borders, dividers, table rules. |
| Light Block | `#E8EEF7` | 232 238 247 | Block fill in the dark-background mark only. |
| Paper | `#F6F7F9` | 246 247 249 | Default page background. |

Brass is an accent, not a second brand colour. If it covers more than a few percent of a layout,
something has gone wrong.

## Construction

The mark lives on a 64-unit grid. Block `4,4 → 60,60`, corner radius 7. Arms run at a true 45°
from `17,17` and `47,17` into the vertex at `32,32`; the stem drops to `49.5`. Cut weight 7.2,
butt caps, mitred joins. The small-size variant thickens the cut to 8.6 and tightens the radius
to 6 so it survives at 16 px.

The wordmark is drawn, not set — six letters built entirely from straight strokes on the same
logic. Cap height 20, stroke 3.2, side bearing 1, tracking 7 (0.35 em). There is no font
dependency; the letters are paths. For supporting type, use a neutral grotesk (Inter, Söhne,
Helvetica Neue) and never re-set the wordmark itself.

## Clear space and minimum size

Clear space is one quarter of the block height on all four sides. Nothing sets inside it —
type, rules, or the trim edge. Minimums: mark 16 px on screen or 6 mm in print; horizontal
lockup 120 px or 30 mm.

## Don't

- Rotate, skew, stretch or re-proportion the block.
- Recolour the stem, or fill it with the same ink as the arms.
- Add shadows, glows, bevels, outlines or gradients.
- Place the navy block on a navy or Deep Ink field — switch to the dark-background mark.
- Redraw the Y at another weight, or set the wordmark in a substitute typeface.

## Favicon markup

```html
<link rel="icon" href="/favicon.ico" sizes="any">
<link rel="icon" type="image/svg+xml" href="/yamale-mark-small.svg">
<link rel="apple-touch-icon" href="/icon-180.png">
<meta name="theme-color" content="#12253F">
<meta property="og:image" content="/og-1200x630.png">
```

---

Nothing here has been trademark-cleared or checked against existing marks in the payments
category. Run a search before this goes on anything binding.
