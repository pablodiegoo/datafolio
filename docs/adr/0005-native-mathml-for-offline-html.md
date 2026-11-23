---
status: accepted
---

# Pure-Go LaTeX-to-MathML compilation for offline HTML output

HTML monographs produced by `datafolio build --to html` must work completely offline without requiring Typst or external JavaScript CDNs. Rather than bundling KaTeX assets or shelling out to Typst for SVG equations, DataFolio compiles LaTeX math (`$...$` and `$$...$$`) directly to standard browser-native MathML in Go, with `--math katex` available as an opt-in flag. This trades slightly narrower LaTeX macro coverage for zero-JS, selectable, print-safe HTML output that stays independent of the PDF toolchain.
