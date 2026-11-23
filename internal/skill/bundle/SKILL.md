---
name: datafolio
description: Author, compile, and review editorial Source Documents, 16:9 slides, standalone web monographs, and W3C DTCG Token Contracts using DataFolio and the active School visual grammar.
---

# DataFolio Editorial & Visual Governance Skill

DataFolio separates semantic authoring from visual grammar through a pluggable `School` architecture (`tufte` in v1, supporting `paper`, `night`, and `print` `Theme`s). Always follow the canonical terms in `GLOSSARY.md`:

- **School**: Complete visual and editorial grammar (`tufte`).
- **Theme**: Surface polarity within a `School` (`paper`, `night`, `print`).
- **Token Contract**: W3C DTCG specification (standalone YAML/JSON or embedded inside `DESIGN.md`).
- **Source Document**: Extended Markdown file with YAML frontmatter compiled to a `Format`.
- **Format**: Publication target: `document` (portrait PDF), `slides` (16:9 PDF), or `html` (standalone offline web monograph).
- **Sidenote**: Numbered margin annotation (`[^1]`).
- **Margin Note**: Unnumbered contextual annotation (`:::note`, `> **Margin Note:**`, or `> **Nota de Margem:**`).
- **Running Chrome**: Repeating page `header` and `footer` slots (`left`, `center`, `right`, `skip_first`, `rule`).
- **Kicker**: Short uppercase or small-caps label above a document or slide title.
- **Slide Overflow**: Pagination defect when a logical slide exceeds the 16:9 canvas height.
- **Review** / **Finding**: Deterministic static audit and its structured diagnostics (`error` or `warn`).
- **Skill Bundle**: Versioned agent instructions, references, and optional visualization helper installed by `datafolio skill install`.

## CLI Commands

- **Scaffold project configuration**:
  ```bash
  datafolio init [--tokens] [--school tufte] [--lang en|pt-BR] [--theme paper|night|print] [--force]
  ```
- **Compile a Source Document**:
  ```bash
  datafolio build <source.md> [--to document|slides|html] [-o path] \
    [--school tufte] [--theme paper|night|print] [--lang en|pt-BR] [--paper a4|letter] \
    [--tokens path] [--math mathml|katex] [--cover] [--toc] [--allow-overflow] [--watch]
  ```
  Configuration resolves in four precedence levels (lowest to highest):
  1. Active `School` defaults
  2. Upward-discovered `datafolio.yaml`
  3. `Source Document` YAML frontmatter
  4. CLI flags
- **Validate and export a Token Contract**:
  ```bash
  datafolio tokens check [--from path] [--school tufte] [--theme paper]
  datafolio tokens export [--from path] [--to css,tailwind,scss,typst,matplotlib] [--out dir] [--school tufte] [--theme paper] [--check]
  ```
- **Run static visual and editorial Review**:
  ```bash
  datafolio review <path...> [--tokens path] [--school tufte] [--format text|json|md] [--strict]
  ```
- **Manage the installed Skill Bundle**:
  ```bash
  datafolio skill install [--target agents|claude|gemini|all] [--global] [--school tufte] [--force]
  datafolio skill status [--target agents|claude|gemini|all] [--global] [--json]
  ```
- **Verify Typst and bundled SIL OFL font health**:
  ```bash
  datafolio doctor [--install-typst] [--json]
  ```

## Authoring Source Documents

Read [references/markdown-grammar.md](references/markdown-grammar.md) for full syntax details.

1. Start every `Source Document` with YAML frontmatter (`title`, `subtitle`, `kicker`, `author`, `date`, `lang`, `school`, `theme`, `format`, `paper`, `cover`, `toc`, `header`, `footer`, `bibliography`, `csl`, `tokens`).
2. Use Markdown footnote syntax (`[^1]` inline + `[^1]: **Optional title** body) for numbered `Sidenote`s. Keep references and definitions strictly 1-to-1.
3. Use `:::note ... :::`, `> **Margin Note:** ...`, or `> **Nota de Margem:** ...` for unnumbered `Margin Note`s.
4. Place full-width tables or charts inside `:::wide ... :::`, metric strips inside `:::kpi ... :::`, and side-by-side slide blocks inside `:::columns ... :::`.
5. Precede every Markdown table with `Table: ...` (or `Tabela: ...`) and follow every table and figure with `*Source: ...*` (or `*Fonte: ...*`).
6. Resolve local `.svg` and `.png` figures relative to the `Source Document` directory.
7. Write standard LaTeX math (`$...$` currency-safe inline and `$$...$$` block); DataFolio translates math to Typst for PDFs and pure-Go MathML (or `--math katex`) for offline HTML.
8. Use `---` to separate slides in `--to slides`, with optional per-slide comments `<!-- kicker: ... -->`, `<!-- header: ... -->`, and `<!-- footer: ... -->`.

## Running Chrome (`header` and `footer`)

Configure `Running Chrome` in `datafolio.yaml` or `Source Document` frontmatter:

- Boolean shorthand: `header: false` or `footer: false` disables chrome; `true` enables defaults.
- Object form supports `left`, `center`, `right`, `skip_first`, and `rule`.
- Dynamic variables: `{title}`, `{subtitle}`, `{section}`, `{author}`, `{date}`, `{page}`, `{pages}`.
- Defaults: enabled with `skip_first: true` for `document`, disabled by default for `slides` (leaving the top clear for `Kicker`s), and rendered as static top/bottom bands with minimal `@page` rules in `html`.

## Token Contract & Analytical Plotting Workflow

Read [references/token-contract.md](references/token-contract.md) for schema and export details.

1. Author design tokens in W3C DTCG YAML/JSON (`$value`/`$type` or `value`/`type`) or inside a `DESIGN.md` `design_tokens:` block.
2. Validate WCAG 2.1 AA contrast (`>= 4.5:1` normal text, `>= 3.0:1` large text/UI) via `datafolio tokens check`.
3. Export synchronized targets (`css`, `tailwind`, `scss`, `typst`, `matplotlib`) via `datafolio tokens export`.
4. To produce aligned Matplotlib charts, export `--to matplotlib` (generating `datafolio.mplstyle` and `tokens.json`), then run the optional PEP 723 helper with `uv`:
   ```bash
   uv run scripts/plot_tufte.py --mplstyle datafolio.mplstyle --tokens tokens.json --out assets/chart.svg --demo
   ```
   DataFolio's Go binary never invokes or depends on Python; it embeds the resulting `.svg` or `.png` artifacts directly.

## Editorial & Visual Governance Rules

Read [references/tufte-school.md](references/tufte-school.md) for complete `tufte` `School` geometry, color tokens, and `Review` rule IDs.

- **Deletion Test**: Cut filler throat-clearing phrases in English, Portuguese, and Spanish (`"it is important to note that"`, `"based on the data provided"`, `"é importante notar que"`, `"com base nos dados fornecidos"`, `"es importante señalar que"`, etc.).
- **Attribution**: Never leave a table or figure without a caption and source attribution.
- **Zero Emojis**: Never emit emojis in prose, headings, tables, charts, SVGs, or UI templates.
- **Token Adherence**: Use exported tokens rather than hardcoded hex/RGB colors; maintain `0px` border radius and flat elevation (`box-shadow: none`) under `tufte`.
- **Structural Integrity**: Resolve all `Sidenote` orphans and `Slide Overflow` `Finding`s before publication (`datafolio review --strict`).
