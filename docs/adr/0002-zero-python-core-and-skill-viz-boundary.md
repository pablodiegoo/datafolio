---
status: accepted
---

# Zero-Python Go core with analytical plotting confined to the Skill Bundle

DataFolio targets analytical and scientific workflows where users routinely plot data with `matplotlib`, `scipy`, and `pandas`, yet the CLI itself must remain a portable, zero-runtime-dependency Go binary. We drew a hard boundary: the Go binary never invokes or embeds Python, instead exporting `.mplstyle` and `tokens.json` from the Token Contract and embedding finished SVG/PNG figures, while optional PEP 723 (`uv`) Python chart helpers live strictly inside the installed Skill Bundle. This avoids reimplementing a statistical plotting library in Go without sacrificing visual alignment between data charts and typeset prose.
