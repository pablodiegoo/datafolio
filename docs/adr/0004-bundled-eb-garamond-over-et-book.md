---
status: accepted
---

# Bundled EB Garamond, JetBrains Mono, and Source Sans 3 over ET Book or system fonts

Minimal Linux hosts and CI runners lack classical serif and tabular monospace fonts, causing silent fallback and layout drift across machines, and Edward Tufte's MIT-licensed `ET Book` has incomplete Portuguese diacritic coverage across weights. DataFolio bundles a compact SIL OFL font set (`EB Garamond`, `JetBrains Mono`, and `Source Sans 3`) passed to Typst via `--font-path` and overridable via the Token Contract. This trades a few megabytes of bundled assets for deterministic cross-OS typography and full `pt-BR` and `en` diacritic and small-caps support.
