# DataFolio

An editorial publishing and visual-governance system that turns structured Markdown and design-token contracts into typeset documents, presentations, web monographs, and automated design reviews across pluggable visual schools.

## Identity & Design System

**School**:
A complete visual and editorial grammar — covering page geometry, margin annotation treatment, typographic hierarchy, and review rules (such as `tufte`, and future schools like `few`, `cairo`, `swiss`, or `editorial`).
_Avoid_: Style, template, skin, preset

**Theme**:
A surface and color mode within a School (such as `paper`, `night`, or `print`) that adapts polarity and contrast without changing the School's structural grammar.
_Avoid_: Style, dark mode, palette, skin

**Token Contract**:
A project's canonical visual specification expressed in W3C Design Tokens Community Group (DTCG) format, either as a standalone file or embedded inside a `DESIGN.md`.
_Avoid_: Config file, stylesheet, theme file, palette file

## Authoring & Outputs

**Source Document**:
An extended Markdown file containing editorial prose, semantic blocks, math, and YAML frontmatter metadata from which one or more rendered artifacts are compiled.
_Avoid_: Draft, input file, script, template

**Format**:
The target publication medium produced from a Source Document: `document` (paginated portrait PDF), `slides` (16:9 widescreen PDF), or `html` (standalone web monograph).
_Avoid_: Mode, output type, layout, export

**Sidenote**:
A numbered margin annotation anchored to a specific point in the main text, rendered in the margin column by Schools that use margin layouts or adapted to an inline diagnostic block by other Schools.
_Avoid_: Footnote, comment, tooltip, sidebar

**Margin Note**:
An unnumbered contextual annotation attached to a section, table, or figure without an inline superscript reference.
_Avoid_: Callout, alert box, footnote, sidenote

**Running Chrome**:
Optional repeating top headers and bottom footers placed across pages or slides using left, center, and right slots with dynamic variables such as page numbers and section titles.
_Avoid_: Banner, navbar, watermark, masthead

**Kicker**:
A short uppercase or small-caps label placed above a document or slide title to orient the reader within a section, course, or publication series.
_Avoid_: Eyebrow, badge, tag, supertitle

**Slide Overflow**:
A pagination defect in the `slides` Format where a single logical slide exceeds the 16:9 canvas height and spills onto an unintended extra page.
_Avoid_: Clipping, bleed, page break error

## Governance & Agent Integration

**Review**:
A static, deterministic audit of Markdown prose, web templates/stylesheets, token contracts, or compiled slides against a School's editorial rules and a project's Token Contract.
_Avoid_: Linter, check, test, critique

**Finding**:
A single diagnostic item emitted by a Review, carrying a rule identifier, source location, severity (`error` or `warn`), and remediation hint.
_Avoid_: Issue, alert, violation, lint error

**Skill Bundle**:
A versioned package of agent instructions, editorial references, and optional visualization helpers installed into a target repository so AI coding agents author and review artifacts consistently.
_Avoid_: Plugin, prompt, extension, preset
