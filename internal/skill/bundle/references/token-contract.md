# Token Contract Specification & Export Workflow

A `Token Contract` expresses a project's visual specification in W3C Design Tokens Community Group (DTCG) format and compiles synchronized assets across web, PDF, and Python plotting workflows.

## Supported Input Formats

DataFolio reads `Token Contract`s from:

1. **Standalone DTCG YAML or JSON** (`.yaml`, `.yml`, `.json`): supports both `$value`/`$type` and `value`/`type`, plus `{a.b.c}` alias references with cycle detection.
2. **Embedded `DESIGN.md` blocks**: extracts a `design_tokens:` YAML frontmatter block or fenced ```` ```yaml ```` blocks inside `DESIGN.md`.

Example DTCG YAML:

```yaml
name: "Oxford Editorial"
color:
  $type: color
  brand:
    navy: { $value: "#1E3A8A" }
  light:
    paper: { $value: "#ffffff" }
    paper_subtle: { $value: "#f8fafc" }
    paper_alt: { $value: "#f1f5f9" }
    ink: { $value: "#0f172a" }
    ink_soft: { $value: "#334155" }
    muted: { $value: "#475569" }
    accent: { $value: "{color.brand.navy}" }
    ochre: { $value: "#854d0e" }
    forest: { $value: "#166534" }
    rule: { $value: "#e2e8f0" }
    rule_strong: { $value: "#475569" }
  dark:
    paper: { $value: "#0b0f17" }
    ink: { $value: "#f8fafc" }
    muted: { $value: "#94a3b8" }
    accent: { $value: "#60a5fa" }
font:
  serif: { $type: fontFamily, $value: "Source Serif 4" }
  mono: { $type: fontFamily, $value: "Fira Code" }
  sans: { $type: fontFamily, $value: "Source Sans 3" }
radius:
  default: { $type: dimension, $value: "0px" }
```

## Contrast Validation & Non-Destructive Overrides

- `datafolio tokens check` and `datafolio tokens export` verify WCAG 2.1 AA contrast (`>= 4.5:1` for normal text, `>= 3.0:1` for large text and UI rules) across both light and dark surfaces, supporting `#RGB`, `#RRGGBB`, `rgb(...)`, and `rgba(...)` composited over the surface color.
- Pass `--tokens <path>` to `datafolio build` or `datafolio review` to apply an existing repository's `Token Contract` without mutating that repository's visual identity.

## Synchronized Export Targets

`datafolio tokens export --from tokens.yaml --to css,tailwind,scss,typst,matplotlib --out dist` generates:

| Target | Filename(s) | Output Format |
| --- | --- | --- |
| `css` | `tokens.css` | `:root` `--df-*` custom properties + `[data-theme="night"]` |
| `tailwind` | `tokens.tailwind.css` | Tailwind v4 `@theme` block |
| `scss` | `_tokens.scss` | `$df-*` SCSS variables |
| `typst` | `tokens.typ` | `#let df-* = ...` Typst declarations |
| `matplotlib` | `datafolio.mplstyle`, `tokens.json` | Matplotlib stylesheet + structured JSON contract |

## Matplotlib `tokens.json` & `.mplstyle` Contract

When exporting `--to matplotlib`, DataFolio writes:

1. **`datafolio.mplstyle`**: sets `figure.facecolor`, `axes.facecolor`, `savefig.facecolor`, `text.color`, `axes.labelcolor`, `xtick.color`, `ytick.color`, `axes.edgecolor`, `axes.spines.top: False`, `axes.spines.right: False`, `axes.grid: True`, `grid.color`, `font.family`, `font.serif`, `font.sans-serif`, `font.monospace`, and `axes.prop_cycle`.
2. **`tokens.json`**: exposes resolved color, font, and layout fields for programmatic chart scripts:
   - `colors.paper`, `colors.paper_subtle`, `colors.paper_alt`
   - `colors.ink`, `colors.ink_soft`, `colors.muted`
   - `colors.accent`, `colors.ochre`, `colors.forest`
   - `colors.rule`, `colors.rule_strong`
   - `colors.palette` (ordered array `[accent, ink, ochre, forest, muted]`)
   - `dark` (optional night-surface tokens if defined in the contract)
   - `fonts.serif`, `fonts.mono`, `fonts.sans`
   - `layout` (`main_width`, `margin_width`, `column_gap`, `border_radius`, `box_shadow`, `booktabs_top`, `booktabs_mid`, `booktabs_row`)

Use the Skill Bundle helper `scripts/plot_tufte.py` via `uv run` to load both files and write clean `.svg` or `.png` figures for embedding in your `Source Document`:

```bash
datafolio tokens export --to matplotlib --out dist
uv run scripts/plot_tufte.py --mplstyle dist/datafolio.mplstyle --tokens dist/tokens.json --out assets/chart.svg --demo
```
