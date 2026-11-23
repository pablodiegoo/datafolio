# Source Document Extended Markdown Grammar

A DataFolio `Source Document` is an extended CommonMark/GFM file with optional YAML frontmatter. The same file compiles across all three `Format`s (`document`, `slides`, `html`).

## YAML Frontmatter

Supported frontmatter keys:

```yaml
---
title: Quarterly Revenue Review
subtitle: Regional Breakdown and Margin Attribution
kicker: ANALYTICAL REPORT
author: Ada Lovelace
date: 2026-11-23
lang: en                 # en or pt-BR
school: tufte            # active School
theme: paper             # paper, night, or print
format: document         # document, slides, or html
paper: a4                # a4 or letter (PDF document)
cover: true              # true/false
toc: true                # true/false
tokens: tokens.yaml      # optional Token Contract path (relative to Source Document)
bibliography: refs.bib   # optional BibTeX file
csl: apa.csl             # optional citation style
math: mathml             # mathml or katex (HTML Format)
header:
  left: "{kicker}"
  center: ""
  right: "{section}"
  skip_first: true
  rule: true
footer:
  left: "{author}"
  center: "{date}"
  right: "{page} / {pages}"
  skip_first: true
  rule: false
---
```

Precedence order: `School` defaults -> upward-discovered `datafolio.yaml` -> `Source Document` YAML frontmatter -> CLI flags.

## Running Chrome (`header` and `footer`)

- Set `header: false` or `footer: false` to disable chrome entirely; set `true` to enable defaults.
- Slot placeholders: `{title}`, `{subtitle}`, `{section}`, `{author}`, `{date}`, `{page}`, `{pages}`.
- `document` defaults to enabled with `skip_first: true`.
- `slides` defaults to disabled so the top edge stays clear for slide `Kicker`s.
- Override `Running Chrome` or `Kicker` on an individual slide using HTML comments inside that slide:
  ```markdown
  <!-- kicker: METHODOLOGY -->
  <!-- header: false -->
  <!-- footer: {"left": "Course 101", "right": "{page} / {pages}"} -->
  ```

## Numbered Sidenotes

Write numbered `Sidenote`s with standard Markdown footnote syntax so the source stays readable in GitHub and Obsidian:

```markdown
The margin ratio preserves reading rhythm[^1] across pages.

[^1]: **Asymmetric grid** In the `tufte` `School`, 65% hosts prose and 30% hosts annotations.
```

Rules:
- Every `[^id]` reference must match a `[^id]:` definition (`sidenote-orphan-ref` / `sidenote-orphan-def` are `error` `Finding`s).

## Unnumbered Margin Notes

Attach contextual annotations without inline superscript markers using any of these equivalent forms:

```markdown
:::note
**Note title** Unnumbered contextual commentary aligned in the margin column.
:::
```

Compatibility blockquote forms:

```markdown
> **Margin Note:** Contextual commentary for English Source Documents.

> **Nota de Margem:** Comentario contextual para documentos em pt-BR.
```

## Structural Containers

- **`:::wide`**: Spans 100% width across main and margin columns for dense tables or multi-panel figures.
- **`:::kpi`**: Compact metric strip rendered with tabular numerals and subtle vertical dividers.
- **`:::columns`**: Side-by-side two-column content (especially in `slides`).

```markdown
:::wide
![Regional heatmap](assets/heatmap.svg)
*Source: Telemetry warehouse*
:::
```

## Booktabs Tables

Place a caption paragraph immediately before the GFM table and a source paragraph immediately after:

```markdown
Table: Regional performance summary

| Region | Revenue | Margin |
| --- | --- | --- |
| North | $1,240.50 | 18.4% |
| South | $980.00 | -2.1% |

*Source: Internal ERP ledger*
```

- Accepted caption prefixes (case-insensitive): `Table:`, `Tabela:`, `Caption:`, `Legenda:`, `:`.
- Accepted source prefixes (case-insensitive): `Source:`, `Fonte:`.
- Numeric columns auto-align right with `tabular-nums`; vertical gridlines are omitted in booktabs typography.

## Local Figures

Reference SVG or PNG images relative to the `Source Document` directory (never relative to the current working directory if the document lives in a subdirectory):

```markdown
![Revenue trajectory across quarters](assets/trend.svg)
*Source: Telemetry pipeline*
```

Or pass the source attribution via the Markdown image title attribute:

```markdown
![System snapshot](assets/snapshot.png "Source: Operations log")
```

## LaTeX Math

- **Inline math**: `$E = mc^2$` follows Pandoc currency-safe delimiters (no whitespace right after the opening `$`, no whitespace right before the closing `$`, and the closing `$` is not immediately followed by a digit, so `$1,240.50` and `$50 to $80` stay literal currency).
- **Display math**: `$$\int_0^\infty e^{-x^2}\,dx = \frac{\sqrt{\pi}}{2}$$`
- Compiled deterministically in pure Go to Typst math for `document` / `slides` and native MathML (or `--math katex` passthrough) for `html`.

## Citations & Bibliography

Set `bibliography: refs.bib` (and optional `csl: abnt.csl`) in frontmatter and cite keys inline using `[@smith2024]`.

## Slides (`--to slides`)

Separate logical slides with `---` horizontal rules. Four structural layouts are supported:
1. Cover slide generated from YAML frontmatter (with optional cover `Sidenote`s).
2. Standard 65%/30% slide with `Kicker`, slide title, horizontal rule, prose, and margin `Sidenote`s.
3. Full-width `:::wide` slide content.
4. Two-column `:::columns` slide content.

DataFolio checks each slide for `Slide Overflow` via `typst query` and fails the build by default (pass `--allow-overflow` only for diagnostics).

## Bilingual Labels (`lang: en` vs `lang: pt-BR`)

| Label | `en` | `pt-BR` |
| --- | --- | --- |
| Figure | `Figure` | `Figura` |
| Table | `Table` | `Tabela` |
| Source | `Source` | `Fonte` |
| Margin Note | `Margin Note` | `Nota de Margem` |
| References | `References` | `Referências` |
| Contents | `Contents` | `Sumário` |
