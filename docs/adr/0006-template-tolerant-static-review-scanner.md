---
status: accepted
---

# Template-tolerant static scanning for web Review across stacks

Real-world websites authored alongside DataFolio span heterogeneous server-side templates — including Go `templ`, Laravel Blade (`.blade.php`), and Jekyll Liquid (`.html`) alongside `.css` and `.scss`. Rather than embedding a full parser per web framework or requiring a rendered headless-browser DOM in v1, `datafolio review` strips template expressions (`{{ }}`, `{% %}`, `@directives`) before statically checking hardcoded colors, disallowed emojis, elevation/radius violations, and Token Contract contrast. This trades runtime conditional evaluation for immediate zero-setup auditing directly on source trees of any stack.
