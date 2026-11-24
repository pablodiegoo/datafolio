# Tufte School & Editorial Governance Reference

The built-in `tufte` `School` implements the Archival Carmine editorial visual grammar across `paper`, `night`, and `print` `Theme`s.

## Archival Carmine Color Tokens

| Semantic Token | `paper` | `night` | `print` | Role |
| --- | --- | --- | --- | --- |
| `paper` | `#fffff8` | `#121316` | `#ffffff` | Primary page canvas |
| `paper_subtle` | `#fbf9f2` | `#16181d` | `#fafafa` | Subtle elevated surface |
| `paper_alt` | `#f6f3ea` | `#1a1c21` | `#f4f4f4` | Alternate band surface |
| `ink` | `#111111` | `#ece8de` | `#111111` | Primary body ink |
| `ink_soft` | `#2b2926` | `#d2cdc0` | `#2b2926` | Secondary prose ink |
| `muted` | `#6b6559` | `#989284` | `#595959` | Captions, ticks, metadata |
| `accent` | `#8b1e1e` | `#e06c60` | `#8b1e1e` | Primary archival carmine accent |
| `ochre` | `#854d0e` | `#d9a04e` | `#854d0e` | Secondary ochre data series |
| `forest` | `#166534` | `#65c494` | `#166534` | Tertiary forest data series |
| `rule` | `#ded8ca` | `#2c3038` | `#d4d4d4` | Hairline divider and subtle grid |
| `rule_strong` | `#3d3932` | `#78808f` | `#3d3932` | Booktabs and axis rule |

## Geometry, Typography & Rules

- **Asymmetric grid**: `65%` main prose column, `30%` margin annotation column, `5%` column gap.
- **Border radius & elevation**: `border_radius: 0px`, `box_shadow: none`.
- **Booktabs tables**: `1.5px` top and bottom rules, `1px` header rule, `0.5px` row dividers, zero vertical gridlines, right-aligned `tabular-nums` for numeric columns.
- **Bundled SIL OFL 1.1 typography**:
  - Serif: `EB Garamond`
  - Monospace: `JetBrains Mono`
  - Sans-serif: `Source Sans 3`

## Static Review Rules (`datafolio review`)

`error` `Finding`s exit with code `1`. `warn` `Finding`s exit with code `0` unless `--strict` is set (which promotes warnings to exit code `1`).

### Error Rules

- `token-contrast-aa`: Foreground/background contrast below WCAG 2.1 AA (`< 4.5:1` normal text, `< 3.0:1` large text/UI).
- `sidenote-orphan-ref`: Inline `[^id]` reference missing a matching `[^id]:` definition.
- `sidenote-orphan-def`: `[^id]:` definition never referenced in the `Source Document`.
- `emoji-disallowed`: Emoji character detected where the active `School` sets `DisallowEmojis: true`.
- `slide-overflow`: Logical slide in `slides` `Format` spilled onto an extra page.

### Warning Rules

- `prose-deletion-test`: Throat-clearing filler phrase detected in EN, PT, or ES prose.
- `table-missing-caption`: Table lacks a preceding caption line.
- `table-missing-source`: Table lacks a trailing `*Source: ...*` or `*Fonte: ...*` attribution.
- `figure-missing-caption`: Figure lacks descriptive alt/caption text.
- `figure-missing-source`: Figure lacks source attribution.
- `web-hardcoded-color`: Raw color literal outside the active `Token Contract` in `.html`, `.templ`, `.blade.php`, `.css`, or `.scss`.
- `web-off-contract-radius`: Non-zero `border-radius` violating the `School` or `Token Contract`.
- `web-off-contract-shadow`: Non-`none` `box-shadow` violating flat elevation policy.

## Deletion Test Forbidden Phrases

Remove these throat-clearing phrases rather than hedging analytical prose:

- **English (`en`)**: `"it's important to note that"`, `"it is important to note that"`, `"based on the data provided"`, `"interestingly"`, `"it is worth mentioning"`, `"one can observe that"`, `"the data suggests that"`, `"in conclusion"`, `"as we can see"`.
- **Portuguese (`pt-BR`)**: `"é importante notar que"`, `"é importante destacar que"`, `"com base nos dados fornecidos"`, `"interessantemente"`, `"vale mencionar que"`, `"vale destacar que"`, `"pode-se observar que"`, `"os dados sugerem que"`, `"em conclusão"`, `"como podemos ver"`.
- **Spanish (`es`)**: `"es importante notar que"`, `"es importante señalar que"`, `"con base en los datos proporcionados"`, `"interesantemente"`, `"vale la pena mencionar que"`, `"cabe mencionar que"`, `"se puede observar que"`, `"los datos sugieren que"`, `"en conclusión"`, `"como podemos ver"`.

## Analytical Visualization Discipline

- Maximize the data-ink ratio: hide top and right axes spines, lighten remaining rules (`rule_strong`), and use dotted hairlines (`rule`) only where reference gridlines aid reading.
- Prefer direct labels over separate legend boxes when comparing 2-4 series.
- Never place emojis inside chart titles, axis labels, annotations, or SVG metadata.
