---
status: accepted
---

# Typst as sole PDF engine with SHA-256 managed binary cache

DataFolio compiles both paginated portrait `document` PDFs and 16:9 `slides` PDFs with repeatable running headers/footers, real margin sidenotes, and native mathematics. We chose Typst as the sole PDF backend — resolved from `PATH` or lazily fetched into a SHA-256-verified cache (`~/.cache/datafolio/`) — rather than pure-Go PDF libraries, headless Chromium HTML printing, or embedding Typst via CGO/WASM. This avoids fragile CSS print-frame hacks for running headers and multi-page sidenotes while keeping the Go binary `CGO_ENABLED=0` and lightweight.
