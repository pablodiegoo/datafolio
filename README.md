# DataFolio

DataFolio (`datafolio`) is a single static Go binary (`CGO_ENABLED=0`, zero Python runtime dependencies) that compiles extended Markdown `Source Document`s and W3C DTCG `Token Contract`s into typeset PDFs, 16:9 presentation decks, standalone offline web monographs, synchronized design-system assets, and deterministic static design reviews.

## Core Concepts

DataFolio separates semantic authoring from visual grammar using a consistent domain model (defined in [`GLOSSARY.md`](GLOSSARY.md)):

- **School**: A complete visual and editorial grammar governing page geometry, typography, margin treatment, and review policies. Version 1 ships the `tufte` `School` (Archival Carmine baseline with a `65%`/`30%` asymmetric grid, `5%` column gap, `0px` radius, flat elevation, and booktabs tables).
- **Theme**: A surface polarity within a `School` (`paper`, `night`, or `print`) that adapts colors and contrast without altering structural layout.
- **Token Contract**: A project's canonical visual specification in W3C Design Tokens Community Group (DTCG) YAML/JSON format, or extracted from `DESIGN.md`.
- **Source Document**: An extended Markdown file with YAML frontmatter compiled to any target `Format`.
- **Format**: Target output medium — `document` (paginated portrait PDF via Typst), `slides` (16:9 widescreen PDF via Typst with `Slide Overflow` detection), or `html` (standalone offline HTML5 monograph with pure-Go native MathML).
- **Running Chrome**: Repeating page `header` and `footer` bands with `left`, `center`, and `right` slots, `skip_first`, `rule`, and dynamic metadata variables.
- **Review** & **Finding**: Deterministic static audits across Markdown, `Token Contract`s, compiled `slides`, and web templates/stylesheets, emitting structured `error` or `warn` diagnostics.
- **Skill Bundle**: Version-tracked agent instructions, editorial references, and an optional PEP 723 (`uv run`) plotting helper installed into repositories via `datafolio skill install`.

## Installation

### Prebuilt Binaries (GitHub Releases)

Download the static archive for your platform (`linux`, `darwin`, or `windows` on `amd64` or `arm64`) from [GitHub Releases](https://github.com/pablodiegoo/datafolio/releases), verify against `checksums.txt`, and place `datafolio` on your `PATH`.

### Install with Go

Requires Go 1.24 or newer:

```bash
go install github.com/pablodiegoo/datafolio/cmd/datafolio@latest
```

### Build from Source

```bash
CGO_ENABLED=0 go build -trimpath -o ./bin/datafolio ./cmd/datafolio
```

### Typst Toolchain & Bundled Fonts

DataFolio uses [Typst](https://github.com/typst/typst) (`v0.15.1`) to compile `document` and `slides` PDFs, while `html`, `tokens`, `review` (on Markdown/tokens/web), `init`, and `skill` work completely offline without Typst.

DataFolio checks `PATH` first and then `~/.cache/datafolio/`. If `typst` is not installed, run `datafolio doctor --install-typst` to download and SHA-256 verify the pinned `v0.15.1` release into `~/.cache/datafolio/`. SIL OFL 1.1 fonts (`EB Garamond`, `JetBrains Mono`, and `Source Sans 3`) are embedded in the binary and automatically extracted to the cache and passed to Typst via `--font-path`.

```bash
datafolio doctor --install-typst
datafolio doctor
```

Verified output:

```text
DataFolio Toolchain Doctor
  typst:  0.15.1 (PATH: /home/user/.cache/datafolio/typst/v0.15.1/typst)
  fonts:  ok — EB Garamond, JetBrains Mono, Source Sans 3 (9 assets in /home/user/.cache/datafolio/fonts/0b05fe04e467c3a0)
  cache:  ok (/home/user/.cache/datafolio)
```

## Quickstart

Scaffold project defaults and a starter W3C DTCG `Token Contract`, validate contrast, and compile:

```bash
datafolio init --tokens
datafolio tokens check --from tokens.yaml
datafolio build report.md --to html
datafolio build report.md --to document
datafolio review report.md --strict
```

Verified `init` and `tokens check` output:

```text
created /path/to/project/tokens.yaml
created /path/to/project/datafolio.yaml
tokens: contrast check passed (13 pairs evaluated)
```

## CLI Reference

Every command maps outcomes to deterministic exit codes: `0` on success, `1` on build/runtime/review error, and `2` on usage or configuration validation errors.

### `datafolio init`

Scaffold `datafolio.yaml` and an optional starter W3C DTCG `tokens.yaml`:

```bash
datafolio init [--tokens] [--school tufte] [--lang en|pt-BR] [--theme paper|night|print] [--force]
```

### `datafolio build`

Compile a `Source Document` to `document`, `slides`, or `html`:

```bash
datafolio build <source.md> [--to document|slides|html] [-o path] [--output path] \
  [--school tufte] [--theme paper|night|print] [--lang en|pt-BR] [--paper a4|letter] \
  [--tokens path] [--math mathml|katex] \
  [--header spec] [--footer spec] [--header-rule] [--footer-rule] \
  [--header-skip-first] [--footer-skip-first] \
  [--cover] [--toc] [--allow-overflow] [--watch]
```

Configuration resolves across four precedence levels (lowest to highest):

1. Active `School` defaults (`tufte`)
2. Upward-discovered `datafolio.yaml`
3. `Source Document` YAML frontmatter
4. CLI flags (explicit pointer/presence tracking ensures unset flags never clobber lower levels)

With `--watch`, DataFolio monitors the `Source Document`, referenced local figures, `.bib`/`.csl` files, `datafolio.yaml`, and referenced `Token Contract`s for debounced incremental rebuilds.

Verified output:

```text
wrote /path/to/project/memo.html
wrote /path/to/project/memo.pdf
```

### `datafolio tokens check` & `datafolio tokens export`

Validate WCAG 2.1 AA contrast on a `Token Contract` and compile synchronized targets:

```bash
datafolio tokens check [--from path] [--school tufte] [--theme paper|night|print]
datafolio tokens export [--from path] [--to css,tailwind,scss,typst,matplotlib] [--out dir] [--school tufte] [--theme paper|night|print] [--check]
```

Verified export output:

```text
wrote /path/to/project/dist/tokens.css
wrote /path/to/project/dist/tokens.tailwind.css
wrote /path/to/project/dist/_tokens.scss
wrote /path/to/project/dist/tokens.typ
wrote /path/to/project/dist/datafolio.mplstyle
wrote /path/to/project/dist/tokens.json
```

### `datafolio review`

Run deterministic static visual and editorial `Review` checks across Markdown files, directories, web templates/stylesheets, and `Token Contract`s:

```bash
datafolio review <path...> [--tokens path] [--school tufte] [--format text|json|md] [--strict]
```

Verified output:

```text
memo.md:7: warn [prose-deletion-test] throat-clearing phrase "interestingly" fails the editorial Deletion Test
  hint: Delete "interestingly" and start directly with the empirical claim or observation.

Summary: 0 errors, 1 warning (1 file checked)
```

### `datafolio skill install` & `datafolio skill status`

Install the embedded `Skill Bundle` into agent directories and inspect version/hash drift via `.datafolio-manifest.json`:

```bash
datafolio skill install [--target agents|claude|gemini|all] [--global] [--school tufte] [--force]
datafolio skill status [--target agents|claude|gemini|all] [--global] [--json]
```

Verified output:

```text
installed agents Skill Bundle (6 files, school=tufte, version=0.1.0-dev) -> /path/to/project/.agents/skills/datafolio
DataFolio Skill Bundle Status (current binary: 0.1.0-dev)
  agents  [ok]  /path/to/project/.agents/skills/datafolio
          school: tufte
          version: installed 0.1.0-dev vs current 0.1.0-dev
```

### `datafolio doctor` & `datafolio version`

Inspect Typst toolchain availability, bundled SIL OFL fonts, and cache health, or print the binary version:

```bash
datafolio doctor [--install-typst] [--json]
datafolio version
```

## Extended Markdown `Source Document` Syntax

A single `Source Document` compiles cleanly across `document`, `slides`, and `html`.

### YAML Frontmatter

```yaml
---
title: Quarterly Revenue Review
subtitle: Regional Attribution and Margin Analysis
kicker: ANALYTICAL REPORT
author: Ada Lovelace
date: 2026-11-23
lang: en                 # en or pt-BR
school: tufte
theme: paper             # paper, night, or print
format: document         # document, slides, or html
paper: a4                # a4 or letter
cover: true
toc: true
tokens: tokens.yaml
bibliography: refs.bib
csl: apa.csl
math: mathml             # mathml or katex (HTML Format)
header:
  left: "{kicker}"
  right: "{section}"
  skip_first: true
  rule: true
footer:
  left: "{author}"
  right: "{page} / {pages}"
  skip_first: true
  rule: false
---
```

### `Sidenote`s & `Margin Note`s

Write numbered `Sidenote`s using standard Markdown footnote syntax so files remain readable in GitHub and Obsidian:

```markdown
The margin ratio preserves reading rhythm[^1] across dense analytical prose.

[^1]: **Asymmetric grid** In the `tufte` `School`, 65% holds main prose and 30% holds margin notes.
```

Write unnumbered `Margin Note`s with a fenced `:::note` block (or blockquote forms `> **Margin Note:** ...` and `> **Nota de Margem:** ...`):

```markdown
:::note
**Methodology** Seasonally adjusted using X-13ARIMA-SEATS.
:::
```

### Containers: `:::wide`, `:::kpi`, `:::columns`

- **`:::wide`**: Spans 100% page width across both main and margin columns for dense tables or multi-panel charts.
- **`:::kpi`**: Renders a compact booktabs metric strip with tabular numerals and subtle vertical separators.
- **`:::columns`**: Renders side-by-side columns using nested `:::column` fences or a `---` separator inside the block.

```markdown
:::kpi
- **Annual Revenue**: $4.82M (+14% YoY)
- **Operating Margin**: 22.4%
:::

:::columns
:::column
### Hypothesis
Demand scales sublinearly with city population.
:::
:::column
### Empirical Fit
Elasticity $\beta = 0.84$ across 40 metropolitan areas.
:::
:::
```

### Booktabs Tables & Local Figures

Precede tables with `Table: ...` (or `Tabela: ...`) and follow tables and figures with `*Source: ...*` (or `*Fonte: ...*`). Numeric columns automatically right-align with `tabular-nums`, vertical gridlines are omitted, and PDF table headers repeat across page breaks. Local `.svg` and `.png` image paths resolve relative to the `Source Document`'s directory.

```markdown
Table: Regional performance summary

| Region | Revenue | Margin |
| --- | --- | --- |
| North | $1,240.50 | 18.4% |
| South | $980.00 | -2.1% |

*Source: Internal ERP ledger*

![Revenue trajectory](assets/trend.svg)
*Source: Telemetry pipeline*
```

### LaTeX Math & Citations

- **Inline math**: `$E = mc^2$` follows Pandoc currency-safe rules (no space after opening `$`, no space before closing `$`, closing `$` not followed by a digit, keeping `$1,240.50` and `$50 to $80` literal).
- **Display math**: `$$\int_0^\infty e^{-x^2}\,dx = \frac{\sqrt{\pi}}{2}$$`
- **Translation**: Compiled in pure Go to Typst math for `document`/`slides`, and browser-native MathML for offline `html` (with `--math katex` passthrough available).
- **Citations**: Set `bibliography: refs.bib` (and optional `csl`) in frontmatter and cite entries using `[@key]`.

### Slides (`--to slides`)

Separate slides with `---`. Four layouts are supported: automatic cover slide from frontmatter (with optional cover `Sidenote`s), standard `65%`/`30%` slide (`Kicker`, title, rule, prose, margin `Sidenote`s), `:::wide`, and `:::columns`. Per-slide HTML comments override metadata on individual slides:

```markdown
<!-- kicker: METHODOLOGY -->
<!-- header: false -->
<!-- footer: {"left": "Course 101", "right": "{page} / {pages}"} -->
```

DataFolio queries page locations with `typst query` after compiling `slides` and fails the build if `Slide Overflow` occurs (reporting exact slide number and title; override with `--allow-overflow`).

### Bilingual Support (`lang: en` | `pt-BR`)

Setting `lang: pt-BR` switches Typst hyphenation, generated labels (`Figura`, `Tabela`, `Fonte`, `Nota de Margem`, `Referências`, `Sumário`), and Deletion Test phrase lists.

## `Running Chrome` (`header` & `footer`)

Configure `Running Chrome` in `datafolio.yaml`, YAML frontmatter, or CLI flags (`--header`, `--footer`, `--header-rule`, `--footer-rule`, `--header-skip-first`, `--footer-skip-first`):

- **Shorthand**: `header: false` / `footer: false` disables chrome; `true` enables defaults. CLI flags also accept `"left|center|right"` or JSON (`'{"left":"Report","right":"{page} / {pages}","rule":true}'`).
- **Object fields**: `left`, `center`, `right`, `skip_first`, `rule`.
- **Dynamic variables**: `{title}`, `{subtitle}`, `{kicker}`, `{section}`, `{author}`, `{date}`, `{page}`, `{pages}`.
- **Per-Format defaults**: Enabled with `skip_first: true` in `document`, disabled by default in `slides` (leaving room for slide `Kicker`s), and rendered as clean static top/bottom editorial bands with minimal `@page` rules in `html`.

## `Token Contract` Exports & `DESIGN.md` Extraction

DataFolio reads standalone W3C DTCG YAML/JSON files (`$value`/`$type` or `value`/`type`) with `{a.b.c}` alias resolution and cycle detection, as well as `design_tokens:` YAML frontmatter or fenced YAML blocks inside `DESIGN.md`.

Exported artifacts for `--to css,tailwind,scss,typst,matplotlib`:

- `css` -> `tokens.css` (`:root` `--df-*` custom properties + `[data-theme="night"]`)
- `tailwind` -> `tokens.tailwind.css` (Tailwind v4 `@theme` block)
- `scss` -> `_tokens.scss` (`$df-*` SCSS variables)
- `typst` -> `tokens.typ` (`#let df-* = ...` Typst declarations)
- `matplotlib` -> `datafolio.mplstyle` + `tokens.json`

Both `datafolio tokens check` and `datafolio tokens export` enforce WCAG 2.1 AA contrast (`>= 4.5:1` normal text, `>= 3.0:1` large text/UI) across light and dark surfaces before writing artifacts. Pass `--tokens <path>` to `build` or `review` to apply an existing repository's `Token Contract` without mutating it.

## Static `Review` Rules

`datafolio review` exits with `1` on any `error` `Finding`, or `0` when only `warn` `Finding`s exist (`--strict` promotes warnings to exit `1`).

### Error Rules

- `token-contrast-aa`: Contrast ratio below WCAG 2.1 AA (`4.5:1` normal, `3.0:1` large/UI).
- `sidenote-orphan-ref`: Inline `[^id]` reference missing a matching `[^id]:` definition.
- `sidenote-orphan-def`: `[^id]:` definition never referenced in the document.
- `emoji-disallowed`: Emoji detected where the active `School` sets `DisallowEmojis: true`.
- `slide-overflow`: Logical slide exceeds 16:9 canvas height in `slides` `Format`.

### Warning Rules

- `prose-deletion-test`: Throat-clearing filler phrases in English, Portuguese, or Spanish prose.
- `table-missing-caption` / `table-missing-source`: Table lacks a caption or `*Source: ...*` / `*Fonte: ...*` attribution.
- `figure-missing-caption` / `figure-missing-source`: Figure lacks descriptive caption or source attribution.
- `web-hardcoded-color`: Raw color literal outside the active `Token Contract`.
- `web-off-contract-radius`: Non-zero `border-radius` violating the `School` or `Token Contract`.
- `web-off-contract-shadow`: Non-`none` `box-shadow` violating flat elevation policy.

### Template-Tolerant Web Scanning

`datafolio review` strips server-side template expressions (`{{ }}`, `{% %}`, `@directives`, Go expressions) before scanning `.html` (Jekyll/Liquid), `.templ` (Go), `.blade.php` (Laravel), `.css`, and `.scss` files in place with zero headless-browser dependencies.

## `Skill Bundle` & Optional Python Plotting

Install the embedded `Skill Bundle` (`SKILL.md`, `references/*.md`, and `scripts/plot_tufte.py`) into `.agents/skills/datafolio/` (or `claude`, `gemini`, `all`; add `--global` for user-home targets):

```bash
datafolio skill install --target agents
datafolio skill status
```

Table: Skill Bundle installation paths by target

| Target | Local Path | Global Path (`--global`) |
| --- | --- | --- |
| `agents` | `.agents/skills/datafolio` | `~/.agents/skills/datafolio` |
| `claude` | `.claude/skills/datafolio` | `~/.claude/skills/datafolio` |
| `gemini` | `.gemini/skills/datafolio` | `~/.gemini/config/skills/datafolio` |

*Source: DataFolio CLI installer (`internal/skill/installer.go`)*

To generate Tufte-styled SVG or PNG figures using Python without adding any Python dependency to the `datafolio` Go binary, export `matplotlib` tokens and invoke the self-contained PEP 723 helper with `uv`:

```bash
datafolio tokens export --to matplotlib --out dist
uv run .agents/skills/datafolio/scripts/plot_tufte.py \
  --mplstyle dist/datafolio.mplstyle \
  --tokens dist/tokens.json \
  --out assets/chart.svg \
  --demo
```

## Licensing & Third-Party Notices

- DataFolio source code is licensed under the Apache License, Version 2.0 ([`LICENSE`](LICENSE)).
- Bundled SIL Open Font License 1.1 fonts (`EB Garamond`, `JetBrains Mono`, `Source Sans 3`) and Go module dependency notices are consolidated in [`THIRD_PARTY_LICENSES`](THIRD_PARTY_LICENSES).
