package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

func runDocumentCLI(t *testing.T, workDir, cacheDir string, runner toolchain.TypstRunner, lookPath func(string) (string, error), args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := cli.Options{
		Stdout:      &stdout,
		Stderr:      &stderr,
		WorkDir:     workDir,
		HomeDir:     workDir,
		CacheDir:    cacheDir,
		Version:     "0.1.0-test",
		TypstRunner: runner,
		LookPath:    lookPath,
		GOOS:        "linux",
		GOARCH:      "amd64",
	}
	code := cli.Run(context.Background(), args, opts)
	return code, stdout.String(), stderr.String()
}

func TestBuildDocumentBaselineAndMissingTypst(t *testing.T) {
	t.Run("compiles short memo via TypstRunner with bundled fonts", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		md := `---
title: Concise Technical Memo
subtitle: Latency Diagnostics
kicker: ENGINEERING MEMO
author: Ada Lovelace
date: 2026-11-23
---

## Summary

Throughput improved by **18%** under *steady* load with ` + "`zero_copy`" + ` enabled.

- First observation
- Second observation

> Keep charts free of chartjunk.
`
		if err := os.WriteFile(filepath.Join(dir, "memo.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}

		var capturedTyp string
		var capturedReq toolchain.CompileRequest
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				capturedReq = req
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n%fake\n"), 0o644)
			},
		}

		code, out, errOut := runDocumentCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/local/bin/typst", nil },
			"build", "memo.md", "--to", "document",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build --to document failed (%d): %s", code, errOut)
		}

		wantPDF := filepath.Join(dir, "memo.pdf")
		if !strings.Contains(out, wantPDF) {
			t.Errorf("stdout %q missing output path %q", out, wantPDF)
		}
		if capturedReq.TypstBin != "/usr/local/bin/typst" {
			t.Errorf("TypstBin = %q, want /usr/local/bin/typst", capturedReq.TypstBin)
		}
		if capturedReq.OutputPath != wantPDF {
			t.Errorf("OutputPath = %q, want %q", capturedReq.OutputPath, wantPDF)
		}
		if capturedReq.RootDir == "" {
			t.Errorf("RootDir must be populated")
		}
		if !strings.HasPrefix(capturedReq.FontPath, filepath.Join(cacheDir, "fonts")) {
			t.Errorf("FontPath = %q, expected under %s", capturedReq.FontPath, filepath.Join(cacheDir, "fonts"))
		}
		if _, err := os.Stat(filepath.Join(capturedReq.FontPath, "EBGaramond[wght].ttf")); err != nil {
			t.Errorf("expected extracted EBGaramond[wght].ttf in FontPath %q: %v", capturedReq.FontPath, err)
		}

		for _, want := range []string{
			`paper: "a4"`,
			`#fffff8`,
			`#111111`,
			`#8b1e1e`,
			`EB Garamond`,
			`JetBrains Mono`,
			`Source Sans 3`,
			`Concise Technical Memo`,
			`Latency Diagnostics`,
			`ENGINEERING MEMO`,
			`Ada Lovelace`,
			`2026-11-23`,
			`Summary`,
			`Throughput improved`,
			`zero_copy`,
			`First observation`,
			`Keep charts free of chartjunk.`,
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("generated .typ missing %q\n--- .typ ---\n%s", want, capturedTyp)
			}
		}
	})

	t.Run("missing typst returns ExitError with doctor hint", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "empty-cache")
		if err := os.WriteFile(filepath.Join(dir, "memo.md"), []byte("# Hello\n\nBody.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		runner := &toolchain.FakeTypstRunner{}
		code, _, errOut := runDocumentCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "", exec.ErrNotFound },
			"build", "memo.md", "--to", "document",
		)
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s)", code, cli.ExitError, errOut)
		}
		if !strings.Contains(errOut, "typst") || !strings.Contains(errOut, "doctor --install-typst") {
			t.Errorf("expected typst missing hint on stderr, got %q", errOut)
		}
		if len(runner.CompileCalls) != 0 {
			t.Errorf("Compile should not be called when typst is missing")
		}
	})
}

func TestBuildDocumentCoverTOCRunningChromeThemesAndTokens(t *testing.T) {
	t.Run("long report with cover toc letter night theme pt-BR and custom Running Chrome", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		md := `---
title: Relatório de Transmissão Monetária
subtitle: Análise Empírica Trimestral
kicker: SÉRIE ANALÍTICA
author: Banco Central — Pesquisa
date: 2026-11-23
lang: pt-BR
theme: night
paper: letter
cover: true
toc: true
header:
  left: "{kicker} — {title}"
  center: "{section}"
  right: "{date}"
  skip_first: false
  rule: true
footer:
  left: "{author} ({subtitle})"
  right: "{page}/{pages}"
  skip_first: false
  rule: true
---

## Introdução

Texto inicial em português.
`
		if err := os.WriteFile(filepath.Join(dir, "relatorio.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}

		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
		}

		code, _, errOut := runDocumentCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "relatorio.md", "--to", "document",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build failed (%d): %s", code, errOut)
		}

		for _, want := range []string{
			`paper: "us-letter"`,
			`#121316`,
			`#ece8de`,
			`#e06c60`,
			`lang: "pt", region: "BR"`,
			`#let df-label-contents = "Sumário"`,
			`#let df-label-references = "Referências"`,
			`#let df-label-figure = "Figura"`,
			`#let df-label-table = "Tabela"`,
			`#let df-label-source = "Fonte"`,
			`#outline(title: [#df-label-contents]`,
			`#pagebreak(weak: true)`,
			`SÉRIE ANALÍTICA — Relatório de Transmissão Monetária`,
			`#df-section()`,
			`2026-11-23`,
			`Banco Central — Pesquisa (Análise Empírica Trimestral)`,
			`counter(page).get().first()`,
			`counter(page).final().first()`,
			`#line(length: 100%, stroke: 0.5pt + df-rule)`,
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("generated .typ missing %q\n--- .typ ---\n%s", want, capturedTyp)
			}
		}
		for _, leaked := range []string{"{title}", "{subtitle}", "{kicker}", "{author}", "{date}", "{section}", "{page}", "{pages}", "if here().page() == 1"} {
			if strings.Contains(capturedTyp, leaked) {
				t.Errorf("generated .typ must not contain uninterpolated or disabled fragment %q", leaked)
			}
		}
	})

	t.Run("header false and footer false cleanly disable Running Chrome and tokens override colors", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		tokensYAML := `color:
  accent:
    $type: color
    $value: "#1e3a8a"
font:
  serif:
    $type: fontFamily
    $value: "IBM Plex Serif"
`
		if err := os.WriteFile(filepath.Join(dir, "tokens.yaml"), []byte(tokensYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		md := `---
title: Print Contract Report
theme: print
header: false
footer: false
---

## Section One

Plain content.
`
		if err := os.WriteFile(filepath.Join(dir, "contract.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}

		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
		}

		code, _, errOut := runDocumentCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "contract.md", "--to", "document", "--tokens", "tokens.yaml", "--cover", "--toc",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build failed (%d): %s", code, errOut)
		}

		for _, want := range []string{
			`#ffffff`,
			`#595959`,
			`#1e3a8a`,
			`IBM Plex Serif`,
			`header: none`,
			`footer: none`,
			`#outline(title: [#df-label-contents]`,
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("generated .typ missing %q\n--- .typ ---\n%s", want, capturedTyp)
			}
		}
	})

	t.Run("default document enables skip_first chrome and propagates compile errors", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		md := `---
title: Default Chrome Memo
author: Grace Hopper
date: 2026-11-23
---

## Findings

Short memo.
`
		if err := os.WriteFile(filepath.Join(dir, "def.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}

		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
		}

		code, _, errOut := runDocumentCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "def.md", "--to", "document",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build failed (%d): %s", code, errOut)
		}
		if !strings.Contains(capturedTyp, "if here().page() == 1 { return }") {
			t.Errorf("expected default document Running Chrome to skip page 1")
		}

		failRunner := &toolchain.FakeTypstRunner{
			CompileErr: context.DeadlineExceeded,
		}
		code, _, errOut = runDocumentCLI(
			t,
			dir,
			cacheDir,
			failRunner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "def.md", "--to", "document",
		)
		if code != cli.ExitError {
			t.Fatalf("expected ExitError when typst compile fails, got %d (stderr=%s)", code, errOut)
		}
	})
}

func TestBuildDocumentSidenotesBlocksMathBooktabsFiguresAndBib(t *testing.T) {
	t.Run("renders margin sidenotes structural blocks math booktabs relative parent assets and bib+csl", func(t *testing.T) {
		root := t.TempDir()
		cacheDir := filepath.Join(root, "cache")
		docDir := filepath.Join(root, "reports", "q4")
		sharedDir := filepath.Join(root, "reports", "shared")
		if err := os.MkdirAll(docDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(sharedDir, 0o755); err != nil {
			t.Fatal(err)
		}

		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 40"><rect width="120" height="40" fill="#8b1e1e"/></svg>`
		if err := os.WriteFile(filepath.Join(sharedDir, "trend.svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sharedDir, "refs.bib"), []byte(`@book{tufte2001,
  author = {Tufte, Edward R.},
  title = {The Visual Display of Quantitative Information},
  publisher = {Graphics Press},
  year = {2001},
}
`), 0o644); err != nil {
			t.Fatal(err)
		}
		cslXML := `<?xml version="1.0" encoding="utf-8"?>
<style xmlns="http://purl.org/net/xbiblio/csl" class="in-text" version="1.0">
  <info><title>Custom</title><id>http://example.com/custom</id><updated>2020-01-01T00:00:00Z</updated></info>
  <citation><layout prefix="(" suffix=")" delimiter="; "><text variable="title"/></layout></citation>
  <bibliography><layout><text variable="title"/></layout></bibliography>
</style>`
		if err := os.WriteFile(filepath.Join(sharedDir, "custom.csl"), []byte(cslXML), 0o644); err != nil {
			t.Fatal(err)
		}

		md := `---
title: Full Analytical Report
lang: en
bibliography: ../shared/refs.bib
csl: ../shared/custom.csl
---

## Empirical Analysis

Revenue grew by $10 to $20 per unit[^1], and margin efficiency followed [@tufte2001, p. 12] again[^1].

[^1]: **Seasonal Effect** Fourth-quarter demand accounted for 62% of annual growth.

:::note
Unnumbered contextual annotation.
:::

> **Margin Note:** Blockquote margin observation.

:::kpi
- **Revenue**: $4.82M | +14% YoY
- **Margin**: 22.4%
:::

:::wide
Table: Quarterly performance

| Region | Revenue | Growth |
| --- | --- | --- |
| North | $1,240.50 | 18.4% |
| South | $980.00 | -2.1% |

*Source: ERP Ledger*
:::

:::columns
:::column
### Hypothesis
Demand scales as $\alpha + \beta \le \gamma$.
:::
:::column
### Evidence
$$
\hat{\beta} = \left(\mathbf{X}^T \mathbf{X}\right)^{-1} \mathbf{X}^T \mathbf{y}
$$
:::
:::

![Quarterly trajectory](../shared/trend.svg)
*Source: Telemetry pipeline*
`
		if err := os.WriteFile(filepath.Join(docDir, "report.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}

		var capturedTyp string
		var capturedReq toolchain.CompileRequest
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				capturedReq = req
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
		}

		code, _, errOut := runDocumentCLI(
			t,
			root,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "reports/q4/report.md", "--to", "document",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build failed (%d): %s", code, errOut)
		}

		// Verify RootDir contains BOTH InputPath and ../shared assets!
		for _, mustBeInsideRoot := range []string{
			capturedReq.InputPath,
			filepath.Join(sharedDir, "trend.svg"),
			filepath.Join(sharedDir, "refs.bib"),
			filepath.Join(sharedDir, "custom.csl"),
		} {
			rel, err := filepath.Rel(capturedReq.RootDir, mustBeInsideRoot)
			if err != nil || strings.HasPrefix(rel, "..") {
				t.Errorf("asset %q is outside RootDir %q (rel=%q, err=%v)", mustBeInsideRoot, capturedReq.RootDir, rel, err)
			}
		}

		for _, want := range []string{
			`#df-sidenote(1)[`,
			`Seasonal Effect`,
			`#df-margin-note[`,
			`Unnumbered contextual annotation.`,
			`Blockquote margin observation.`,
			`#df-kpi[`,
			`#df-wide[`,
			`table.header(`,
			`repeat: true`,
			`align: (left, right, right)`,
			`stroke: none`,
			`Quarterly performance`,
			`ERP Ledger`,
			`$alpha + beta <= gamma$`,
			`hat(beta) = lr((bold(X)^T bold(X)))^(- 1) bold(X)^T bold(y)`,
			`\$10 to \$20`,
			`#cite(label("tufte2001"), supplement: [p. 12])`,
			`#bibliography(`,
			`style: "/`,
			`trend.svg`,
			`refs.bib`,
			`custom.csl`,
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("generated .typ missing %q\n--- .typ ---\n%s", want, capturedTyp)
			}
		}
	})

	t.Run("built-in csl aliases and missing local assets return ExitError", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		if err := os.WriteFile(filepath.Join(dir, "refs.bib"), []byte("@book{k1, author={Ada}, title={Work}, year={2020}}"), 0o644); err != nil {
			t.Fatal(err)
		}

		mdABNT := `---
title: ABNT Builtin
lang: pt-BR
bibliography: refs.bib
csl: abnt
---

Conforme [@k1].
`
		if err := os.WriteFile(filepath.Join(dir, "abnt.md"), []byte(mdABNT), 0o644); err != nil {
			t.Fatal(err)
		}

		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
		}

		code, _, errOut := runDocumentCLI(t, dir, cacheDir, runner, func(string) (string, error) { return "/usr/bin/typst", nil }, "build", "abnt.md", "--to", "document")
		if code != cli.ExitSuccess {
			t.Fatalf("abnt build failed (%d): %s", code, errOut)
		}
		if !strings.Contains(capturedTyp, `style: "associacao-brasileira-de-normas-tecnicas"`) {
			t.Errorf("expected abnt alias to resolve to associacao-brasileira-de-normas-tecnicas, got:\n%s", capturedTyp)
		}

		// Missing local figure must return ExitError mentioning the file name.
		if err := os.WriteFile(filepath.Join(dir, "missing_fig.md"), []byte("![Chart](assets/nope.svg)\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, errOut = runDocumentCLI(t, dir, cacheDir, runner, func(string) (string, error) { return "/usr/bin/typst", nil }, "build", "missing_fig.md", "--to", "document")
		if code != cli.ExitError || !strings.Contains(errOut, "nope.svg") {
			t.Fatalf("expected ExitError mentioning nope.svg, got (%d, %q)", code, errOut)
		}

		// Missing .bib must return ExitError mentioning the file name.
		if err := os.WriteFile(filepath.Join(dir, "missing_bib.md"), []byte("---\nbibliography: nope.bib\n---\n[@k1]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, errOut = runDocumentCLI(t, dir, cacheDir, runner, func(string) (string, error) { return "/usr/bin/typst", nil }, "build", "missing_bib.md", "--to", "document")
		if code != cli.ExitError || !strings.Contains(errOut, "nope.bib") {
			t.Fatalf("expected ExitError mentioning nope.bib, got (%d, %q)", code, errOut)
		}
	})
}

func TestRealTypstDocumentIntegration(t *testing.T) {
	typstBin, err := exec.LookPath("typst")
	if err != nil {
		if os.Getenv("DATAFOLIO_TEST_REAL_TYPST") == "1" {
			t.Fatalf("DATAFOLIO_TEST_REAL_TYPST=1 set, but typst binary not found on PATH: %v", err)
		}
		t.Skip("typst not found on PATH; set DATAFOLIO_TEST_REAL_TYPST=1 to enforce")
	}

	strictRunner := &strictRealTypstRunner{t: t}

	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	sharedDir := filepath.Join(root, "shared")
	memoDir := filepath.Join(root, "memo")
	reportDir := filepath.Join(root, "report", "2026")
	for _, d := range []string{sharedDir, memoDir, reportDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 80">
  <rect width="240" height="80" fill="#fbf9f2" stroke="#3d3932" stroke-width="1"/>
  <polyline points="16,64 72,48 128,52 184,22 224,14" fill="none" stroke="#8b1e1e" stroke-width="2.5"/>
</svg>`
	if err := os.WriteFile(filepath.Join(sharedDir, "series.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "snap.png"), makeTestPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	bib := `@book{tufte2001,
  author = {Tufte, Edward R.},
  title = {The Visual Display of Quantitative Information},
  publisher = {Graphics Press},
  year = {2001},
}

@article{shannon1948,
  author = {Shannon, Claude E.},
  title = {A Mathematical Theory of Communication},
  journal = {Bell System Technical Journal},
  volume = {27},
  number = {3},
  pages = {379--423},
  year = {1948},
}

@book{cleveland1985,
  author = {Cleveland, William S. and McGill, Robert},
  title = {Graphical Perception},
  publisher = {Wadsworth},
  year = {1985},
}
`
	if err := os.WriteFile(filepath.Join(sharedDir, "refs.bib"), []byte(bib), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Short memo (cover: false, toc: false, letter, paper, en)
	memoMD := `---
title: Latency Reduction Brief
subtitle: Zero-Copy Buffer Path
kicker: PERFORMANCE NOTE
author: Ada Lovelace
date: 2026-11-23
theme: paper
paper: letter
lang: en
cover: false
toc: false
---

## Executive Summary

Tail latency dropped from 42.1 ms to 14.8 ms[^1], saving $12 to $18 per node-hour.

[^1]: **Measurement Protocol** Measured across 10,000 steady-state requests with ` + "`p99`" + ` bucketing.

> **Margin Note:** All measurements exclude cold-start JIT warmup.

Inline complexity scales as $O(n \log n)$ with $\alpha + \beta \le \gamma$.

Table: Benchmark Comparison

| Mode | Latency (ms) | Cost / Hr |
| --- | --- | --- |
| Baseline | 42.1 | $24.00 |
| Zero-Copy | 14.8 | $11.50 |

*Source: Internal benchmark suite*

![Snapshot](../shared/snap.png)
*Source: Telemetry capture*
`
	if err := os.WriteFile(filepath.Join(memoDir, "memo.md"), []byte(memoMD), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runDocumentCLI(
		t,
		root,
		cacheDir,
		strictRunner,
		func(string) (string, error) { return typstBin, nil },
		"build", "memo/memo.md", "--to", "document",
	)
	if code != cli.ExitSuccess {
		t.Fatalf("real typst memo compilation failed (%d): %s", code, errOut)
	}
	memoPDF, err := os.ReadFile(filepath.Join(memoDir, "memo.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(memoPDF, []byte("%PDF-")) || len(memoPDF) < 1000 {
		t.Fatalf("memo.pdf invalid PDF (len=%d)", len(memoPDF))
	}

	// 2. Long report (cover: true, toc: true, a4, pt-BR, Running Chrome, collisions, wide, kpi, columns, multi-page table, math, bib+csl, ../svg)
	var rows strings.Builder
	for i := 1; i <= 45; i++ {
		rows.WriteString("| Região ")
		rows.WriteString(string(rune('A' + (i-1)%26)))
		rows.WriteString(" | 1.240,50 | 18,4% |\n")
	}

	reportMD := `---
title: Relatório Analítico de Transmissão Econômica
subtitle: Evidências Empíricas e Formulação Matemática
kicker: MONOGRAFIA TÉCNICA
author: Núcleo de Pesquisa Quantitativa
date: 2026-11-23
lang: pt-BR
theme: paper
paper: a4
cover: true
toc: true
bibliography: ../../shared/refs.bib
csl: apa
header:
  left: "{title}"
  center: "{section}"
  right: "{date}"
  skip_first: true
  rule: true
footer:
  left: "{author}"
  right: "{page} / {pages}"
  skip_first: true
  rule: true
---

## Fundamentação Teórica

A clareza gráfico-analítica[^1] reduz ruído visual e melhora a precisão perceptual[^2] conforme descrito na literatura [@tufte2001; @cleveland1985] e na teoria da informação [ver @shannon1948, p. 380].

[^1]: **Densidade de Informação** A razão dados-tinta prioriza sinais empíricos sobre ornamentos supérfluos.

[^2]: **Alinhamento em Margem** Notas adjacentes na mesma linha devem empilhar sem colisão vertical no PDF final.

:::note
Nota de margem sem numeração ancorada na primeira seção do relatório.
:::

> **Nota de Margem:** Observação complementar em português via citação formatada.

:::kpi
- **Receita Anual**: R$ 4,82M | +14% a/a
- **Margem Operacional**: 22,4% | +1,8 pp
- **Disponibilidade**: 99,98% | SLA atendido
:::

:::columns
:::column
### Hipótese Estrutural
A elasticidade de demanda segue $\hat{y}_i \in \mathbb{R}$ com $\sqrt[3]{x^2 + 1}$ e $\alpha + \beta \le \gamma$.
:::
:::column
### Formulação Matricial
$$
\hat{\beta} = \left( \mathbf{X}^T \mathbf{X} \right)^{-1} \mathbf{X}^T \mathbf{y}, \quad \lim_{n \to \infty} \sum_{i=1}^n \frac{x_i}{n} \approx \int_0^\infty f(t)\,dt
$$
:::
:::

$$
\begin{aligned}
a &\neq b \pm c \times d \cdot e \\
f(x) &\Rightarrow \begin{cases} x^2, & x \ge 0 \\ 0, & \text{caso contrário} \end{cases}
\end{aligned}
$$

$$
A = \begin{pmatrix} 1 & 2 \\ 3 & 4 \end{pmatrix}, \quad B = \begin{bmatrix} \bar{x} & \tilde{y} \\ \vec{v} & \prod_{j=1}^k p_j \end{bmatrix}
$$

:::wide
![Trajetória trimestral de desempenho](../../shared/series.svg)
*Fonte: Sistema de telemetria interna*
:::

## Série Histórica Multipágina

Tabela: Desempenho regional detalhado

| Região | Receita (R$ mil) | Margem |
| --- | --- | --- |
` + rows.String() + `
*Fonte: Razão contábil consolidada*
`
	if err := os.WriteFile(filepath.Join(reportDir, "relatorio.md"), []byte(reportMD), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut = runDocumentCLI(
		t,
		root,
		cacheDir,
		strictRunner,
		func(string) (string, error) { return typstBin, nil },
		"build", "report/2026/relatorio.md", "--to", "document",
	)
	if code != cli.ExitSuccess {
		t.Fatalf("real typst report compilation failed (%d): %s", code, errOut)
	}
	reportPDF, err := os.ReadFile(filepath.Join(reportDir, "relatorio.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(reportPDF, []byte("%PDF-")) || len(reportPDF) < 5000 {
		t.Fatalf("relatorio.pdf invalid PDF (len=%d)", len(reportPDF))
	}

	// 3. Also verify night Theme compiles cleanly with real Typst.
	code, _, errOut = runDocumentCLI(
		t,
		root,
		cacheDir,
		strictRunner,
		func(string) (string, error) { return typstBin, nil },
		"build", "report/2026/relatorio.md", "--to", "document", "--theme", "night", "-o", "report/2026/relatorio-night.pdf",
	)
	if code != cli.ExitSuccess {
		t.Fatalf("real typst night report compilation failed (%d): %s", code, errOut)
	}
	nightPDF, err := os.ReadFile(filepath.Join(reportDir, "relatorio-night.pdf"))
	if err != nil || !bytes.HasPrefix(nightPDF, []byte("%PDF-")) {
		t.Fatalf("relatorio-night.pdf invalid (err=%v len=%d)", err, len(nightPDF))
	}
}

type strictRealTypstRunner struct {
	t *testing.T
}

func (s *strictRealTypstRunner) Compile(ctx context.Context, req toolchain.CompileRequest) error {
	var stdout, stderr bytes.Buffer
	args := []string{"compile", "--ignore-system-fonts", "--root", req.RootDir, "--font-path", req.FontPath, req.InputPath, req.OutputPath}
	cmd := exec.CommandContext(ctx, req.TypstBin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if strings.TrimSpace(stderr.String()) != "" {
		s.t.Errorf("unexpected Typst warning/stderr on %s:\n%s", req.OutputPath, stderr.String())
	}
	return nil
}

func (s *strictRealTypstRunner) Query(ctx context.Context, req toolchain.QueryRequest) ([]byte, error) {
	return toolchain.RealTypstRunner{}.Query(ctx, req)
}

func (s *strictRealTypstRunner) Version(ctx context.Context, typstBin string) (string, error) {
	return toolchain.RealTypstRunner{}.Version(ctx, typstBin)
}



