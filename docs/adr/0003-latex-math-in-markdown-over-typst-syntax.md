---
status: accepted
---

# Standard LaTeX math in Markdown translated to Typst at compile time

Although Typst has its own native math syntax, Source Documents must remain portable across `html` output, GitHub, and Obsidian. Authors therefore write standard LaTeX math (`$...$` and `$$...$$`) in Markdown, and DataFolio deterministically translates it to Typst math when compiling PDFs rather than exposing Typst-specific math syntax to authors. This adds a translation layer in the compiler in exchange for a single, ecosystem-standard authoring grammar across every Format.
