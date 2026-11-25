package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

func runSlidesCLI(t *testing.T, workDir, cacheDir string, runner toolchain.TypstRunner, lookPath func(string) (string, error), args ...string) (int, string, string) {
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

func TestBuildSlidesFourLayoutsAndBoundaryMarkers(t *testing.T) {
	t.Run("renders 16:9 widescreen PDF with cover, 65/30 standard, wide, and columns layouts plus slide boundary markers", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 40"><rect width="120" height="40" fill="#8b1e1e"/></svg>`
		if err := os.WriteFile(filepath.Join(dir, "trend.svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}

		deck := `---
title: Causal Inference in Production
subtitle: Difference-in-Differences at Scale
kicker: QUANTITATIVE SEMINAR
author: Ada Lovelace
date: 2026-11-23
---

Prepared for the engineering leadership review[^cov].

[^cov]: **Scope** Covers Q1–Q3 telemetry cohorts.

---

<!-- kicker: IDENTIFICATION -->
# Parallel Trends Assumption

Treatment and control cohorts track together before rollout[^1], with $\alpha + \beta \le \gamma$.

[^1]: **Pre-Trend Test** Joint $F$-test yields $p = 0.42$.

> **Margin Note:** Unnumbered slide margin context.

---

<!-- kicker: EMPIRICAL ESTIMATES -->
# Regional Impact Matrix

:::wide
Table: Regional treatment lift

| Region | Lift | SE |
| --- | --- | --- |
| North | +4.2% | 0.8% |
| South | +2.9% | 0.7% |

*Source: Telemetry warehouse*
:::

---

<!-- kicker: ARCHITECTURE -->
# Two-Column Comparison

:::columns
:::column
### Batch Pipeline
Nightly aggregation over parquet partitions.
:::
:::column
### Streaming Path
Sub-second window updates via $$y_t = \rho y_{t-1} + \varepsilon_t$$.
:::
:::
`
		if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(deck), 0o644); err != nil {
			t.Fatal(err)
		}

		var capturedTyp string
		var compileReq toolchain.CompileRequest
		var queryReq toolchain.QueryRequest

		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				compileReq = req
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n%slides\n"), 0o644)
			},
			OnQuery: func(_ context.Context, req toolchain.QueryRequest) ([]byte, error) {
				queryReq = req
				return []byte(`[
					{"func":"metadata","value":{"kind":"start","index":1,"title":"Causal Inference in Production","page":1},"label":"<datafolio-slide>"},
					{"func":"metadata","value":{"kind":"end","index":1,"title":"Causal Inference in Production","page":1},"label":"<datafolio-slide>"},
					{"func":"metadata","value":{"kind":"start","index":2,"title":"Parallel Trends Assumption","page":2},"label":"<datafolio-slide>"},
					{"func":"metadata","value":{"kind":"end","index":2,"title":"Parallel Trends Assumption","page":2},"label":"<datafolio-slide>"},
					{"func":"metadata","value":{"kind":"start","index":3,"title":"Regional Impact Matrix","page":3},"label":"<datafolio-slide>"},
					{"func":"metadata","value":{"kind":"end","index":3,"title":"Regional Impact Matrix","page":3},"label":"<datafolio-slide>"},
					{"func":"metadata","value":{"kind":"start","index":4,"title":"Two-Column Comparison","page":4},"label":"<datafolio-slide>"},
					{"func":"metadata","value":{"kind":"end","index":4,"title":"Two-Column Comparison","page":4},"label":"<datafolio-slide>"}
				]`), nil
			},
		}

		code, out, errOut := runSlidesCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/local/bin/typst", nil },
			"build", "deck.md", "--to", "slides",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build --to slides failed (%d): %s", code, errOut)
		}

		wantPDF := filepath.Join(dir, "deck.pdf")
		if !strings.Contains(out, wantPDF) {
			t.Errorf("stdout %q missing %q", out, wantPDF)
		}
		if compileReq.OutputPath != wantPDF {
			t.Errorf("Compile OutputPath = %q, want %q", compileReq.OutputPath, wantPDF)
		}
		if queryReq.Selector != "<datafolio-slide>" {
			t.Errorf("Query Selector = %q, want <datafolio-slide>", queryReq.Selector)
		}

		for _, want := range []string{
			`#let df-page-w = 254mm`,
			`#let df-page-h = 142.875mm`,
			`width: df-page-w`,
			`height: df-page-h`,
			`header: none`,
			`footer: none`,
			`#fffff8`,
			`#8b1e1e`,
			`EB Garamond`,
			`QUANTITATIVE SEMINAR`,
			`Causal Inference in Production`,
			`Difference-in-Differences at Scale`,
			`Ada Lovelace`,
			`Prepared for the engineering leadership review`,
			`Scope`,
			`Covers Q1–Q3 telemetry cohorts.`,
			`IDENTIFICATION`,
			`Parallel Trends Assumption`,
			`#df-sidenote(`,
			`Pre-Trend Test`,
			`#df-margin-note[`,
			`Unnumbered slide margin context.`,
			`$alpha + beta <= gamma$`,
			`EMPIRICAL ESTIMATES`,
			`Regional Impact Matrix`,
			`#df-wide[`,
			`Regional treatment lift`,
			`ARCHITECTURE`,
			`Two-Column Comparison`,
			`Batch Pipeline`,
			`Streaming Path`,
			`<datafolio-slide>`,
			`kind: "start"`,
			`kind: "end"`,
			`here().page()`,
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("slides .typ missing %q\n--- .typ ---\n%s", want, capturedTyp)
			}
		}

		// Ensure slide title is not duplicated in body after consuming heading.
		if count := strings.Count(capturedTyp, "Parallel Trends Assumption"); count != 3 {
			// Expected once in start marker, once in slide header heading, once in end marker = 3!
			t.Errorf("expected 'Parallel Trends Assumption' to appear 3 times (start marker, header, end marker), got %d\n%s", count, capturedTyp)
		}
	})

	t.Run("missing typst returns ExitError with doctor hint", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "empty-cache")
		if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte("# Slide 1\n\nBody.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runner := &toolchain.FakeTypstRunner{}
		code, _, errOut := runSlidesCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "", exec.ErrNotFound },
			"build", "deck.md", "--to", "slides",
		)
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s)", code, cli.ExitError, errOut)
		}
		if !strings.Contains(errOut, "typst") || !strings.Contains(errOut, "doctor --install-typst") {
			t.Errorf("expected doctor hint in stderr, got %q", errOut)
		}
	})
}

func TestBuildSlidesRunningChromeGlobalCLIAndPerSlide(t *testing.T) {
	t.Run("deck frontmatter Running Chrome and per-slide overrides across slides", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		deck := `---
title: Monetary Policy Deck
subtitle: Quarterly Seminar
kicker: RESEARCH
author: Ada Lovelace
date: 2026-11-23
header:
  left: "{title} ({subtitle})"
  right: "{section}"
  skip_first: true
  rule: true
footer:
  left: "{author} · {date}"
  right: "{page} / {pages}"
  skip_first: true
  rule: false
---

# Slide One

<!-- header: false -->
<!-- footer: CS 101 | {section} | Slide {page} -->
First slide body.

---

<!-- footer: {"right": "Appendix Note", "rule": true} -->
# Slide Two

Second slide body.

---

# Slide Three

Inherits deck chrome cleanly.
`
		if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(deck), 0o644); err != nil {
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
			QueryOutput: []byte(`[]`),
		}

		code, _, errOut := runSlidesCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "deck.md", "--to", "slides",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build failed (%d): %s", code, errOut)
		}

		for _, want := range []string{
			`if here().page() == 1 { return }`,
			`Monetary Policy Deck (Quarterly Seminar)`,
			`#df-section()`,
			`Ada Lovelace · 2026-11-23`,
			`counter(page).get().first()`,
			`counter(page).final().first()`,
			`#page(header: none, footer:`,
			`CS 101`,
			`[#("Slide ")#str(counter(page).get().first())]`,
			`#page(footer:`,
			`Appendix Note`,
			"#page[\n#context [#metadata((kind: \"start\", index: 4, title: \"Slide Three\"",
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("slides .typ missing %q\n--- .typ ---\n%s", want, capturedTyp)
			}
		}
	})

	t.Run("CLI --header and --footer flags override frontmatter and reject invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		deck := `---
title: CLI Chrome Override
footer:
  left: "From Frontmatter"
  right: "{page}"
---

# Only Slide

Body.
`
		if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(deck), 0o644); err != nil {
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
			QueryOutput: []byte(`[]`),
		}

		code, _, errOut := runSlidesCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "deck.md", "--to", "slides",
			"--header", "CLI Lab | {section} | {page}/{pages}",
			"--header-rule",
			"--header-skip-first=false",
			"--footer", "false",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("build with CLI chrome flags failed (%d): %s", code, errOut)
		}

		for _, want := range []string{
			`CLI Lab`,
			`#df-section()`,
			`footer: none`,
			`#line(length: 100%, stroke: 0.5pt + df-rule)`,
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("CLI chrome override .typ missing %q\n--- .typ ---\n%s", want, capturedTyp)
			}
		}
		if strings.Contains(capturedTyp, "From Frontmatter") {
			t.Errorf("expected --footer false to override frontmatter footer")
		}
		if strings.Contains(capturedTyp, "if here().page() == 1") {
			t.Errorf("expected --header-skip-first=false to disable skip_first")
		}

		code, _, _ = runSlidesCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "deck.md", "--to", "slides", "--header", "{invalid",
		)
		if code != cli.ExitUsage {
			t.Fatalf("expected ExitUsage for malformed --header JSON, got %d", code)
		}
	})
}

func TestBuildSlidesOverflowDiagnosticsAndAllowOverflow(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	deck := `---
title: Overflow Diagnostics
---

# Normal Slide

Short body.

---

# Dense Matrix

Lots of rows that spill onto page 4.
`
	if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(deck), 0o644); err != nil {
		t.Fatal(err)
	}

	overflowQuery := []byte(`[
		{"func":"metadata","value":{"kind":"start","index":1,"title":"Overflow Diagnostics","page":1},"label":"<datafolio-slide>"},
		{"func":"metadata","value":{"kind":"end","index":1,"title":"Overflow Diagnostics","page":1},"label":"<datafolio-slide>"},
		{"func":"metadata","value":{"kind":"start","index":2,"title":"Normal Slide","page":2},"label":"<datafolio-slide>"},
		{"func":"metadata","value":{"kind":"end","index":2,"title":"Normal Slide","page":2},"label":"<datafolio-slide>"},
		{"func":"metadata","value":{"kind":"start","index":3,"title":"Dense Matrix","page":3},"label":"<datafolio-slide>"},
		{"func":"metadata","value":{"kind":"end","index":3,"title":"Dense Matrix","page":4},"label":"<datafolio-slide>"}
	]`)

	runner := &toolchain.FakeTypstRunner{
		OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
			return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n%overflow\n"), 0o644)
		},
		QueryOutput: overflowQuery,
	}

	outPDF := filepath.Join(dir, "deck.pdf")

	t.Run("fails with ExitError reporting exact slide number and title and removes PDF", func(t *testing.T) {
		code, _, errOut := runSlidesCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "deck.md", "--to", "slides",
		)
		if code != cli.ExitError {
			t.Fatalf("expected ExitError (%d), got %d (stderr=%s)", cli.ExitError, code, errOut)
		}
		for _, want := range []string{"slide 3", "Dense Matrix", "pages 3-4", "--allow-overflow"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("expected stderr %q to contain %q", errOut, want)
			}
		}
		if _, err := os.Stat(outPDF); !os.IsNotExist(err) {
			t.Errorf("expected overflowing PDF %q to be removed on failure, stat err=%v", outPDF, err)
		}
	})

	t.Run("allow-overflow downgrades to warning preserves PDF and exits zero", func(t *testing.T) {
		code, out, errOut := runSlidesCLI(
			t,
			dir,
			cacheDir,
			runner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "deck.md", "--to", "slides", "--allow-overflow",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("expected ExitSuccess with --allow-overflow, got %d (stderr=%s)", code, errOut)
		}
		if !strings.Contains(out, outPDF) {
			t.Errorf("expected stdout %q to mention %q", out, outPDF)
		}
		for _, want := range []string{"warning:", "slide 3", "Dense Matrix", "pages 3-4"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("expected warning stderr %q to contain %q", errOut, want)
			}
		}
		pdfBytes, err := os.ReadFile(outPDF)
		if err != nil || !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
			t.Fatalf("expected preserved PDF at %q, err=%v", outPDF, err)
		}
	})

	t.Run("query failure returns ExitError and cleans up PDF", func(t *testing.T) {
		failQueryRunner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			QueryErr: context.DeadlineExceeded,
		}
		_ = os.Remove(outPDF)
		code, _, errOut := runSlidesCLI(
			t,
			dir,
			cacheDir,
			failQueryRunner,
			func(string) (string, error) { return "/usr/bin/typst", nil },
			"build", "deck.md", "--to", "slides",
		)
		if code != cli.ExitError {
			t.Fatalf("expected ExitError on query failure, got %d (stderr=%s)", code, errOut)
		}
		if _, err := os.Stat(outPDF); !os.IsNotExist(err) {
			t.Errorf("expected PDF %q to be removed on query failure", outPDF)
		}
	})
}

func TestRealTypstSlidesIntegration(t *testing.T) {
	typstBin, err := exec.LookPath("typst")
	if err != nil {
		if os.Getenv("DATAFOLIO_TEST_REAL_TYPST") == "1" {
			t.Fatalf("DATAFOLIO_TEST_REAL_TYPST=1 set, but typst binary not found on PATH: %v", err)
		}
		t.Skip("typst not found on PATH; set DATAFOLIO_TEST_REAL_TYPST=1 to enforce")
	}

	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	sharedDir := filepath.Join(root, "shared")
	deckDir := filepath.Join(root, "talks", "2026")
	for _, d := range []string{sharedDir, deckDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 60">
  <rect width="240" height="60" fill="#fbf9f2" stroke="#3d3932" stroke-width="1"/>
  <polyline points="12,48 72,34 132,38 192,16 228,12" fill="none" stroke="#8b1e1e" stroke-width="2.5"/>
</svg>`
	if err := os.WriteFile(filepath.Join(sharedDir, "series.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Clean multi-layout 16:9 deck (5 logical slides -> exactly 5 PDF pages).
	cleanDeck := `---
title: Archival Empirical Presentation
subtitle: Margin Annotations and Widescreen Typography
kicker: RESEARCH SEMINAR
author: Ada Lovelace
date: 2026-11-23
lang: en
theme: paper
footer:
  left: "{author}"
  right: "{page} / {pages}"
  skip_first: true
  rule: false
---

Prepared for the quantitative research colloquium[^cov].

[^cov]: **Archive Note** Data frozen as of November 2026.

---

<!-- kicker: IDENTIFICATION -->
# Standard 65/30 Layout

Parallel pre-trends hold across all treated cohorts[^1] with $\alpha + \beta \le \gamma$ and $O(n \log n)$ runtime.

[^1]: **Joint F-Test** Null of zero differential pre-trend cannot be rejected ($p = 0.42$).

> **Margin Note:** Margin notes stay aligned beside slide prose without colliding with slide headers.

$$
\hat{\beta} = \left(\mathbf{X}^T \mathbf{X}\right)^{-1} \mathbf{X}^T \mathbf{y}
$$

---

<!-- kicker: EMPIRICAL ESTIMATES -->
<!-- footer: {"left": "Seminar Series", "center": "{section}", "right": "Slide {page}/{pages}", "rule": true} -->
# Full-Width Wide Container

:::wide
Table: Quarterly cohort lift

| Cohort | Treated Lift | Std. Error |
| --- | --- | --- |
| Q1 2026 | +4.2% | 0.8% |
| Q2 2026 | +5.1% | 0.7% |

*Source: Telemetry panel*

![Cohort trend](../../shared/series.svg)
*Source: Internal measurement pipeline*
:::

---

<!-- kicker: SYSTEM DESIGN -->
# Side-by-Side Columns

:::columns
:::column
### Batch Ingestion
- Partitioned parquet snapshots
- Deterministic nightly validation
- Reproducible schema checks
:::
:::column
### Online Scoring
- Sub-millisecond feature lookup
- Continuous residual monitoring
- Native $\ell_2$-regularized updates
:::
:::

---

<!-- kicker: SUMMARY -->
# Key Metrics Strip

:::kpi
- **Throughput**: 48.2k req/s | +18% YoY
- **P99 Latency**: 14.8 ms | -27.3 ms
- **Availability**: 99.98% | SLA met
:::
`
	if err := os.WriteFile(filepath.Join(deckDir, "clean.md"), []byte(cleanDeck), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanRunner := &slidesStrictRealTypstRunner{t: t}
	code, _, errOut := runSlidesCLI(
		t,
		root,
		cacheDir,
		cleanRunner,
		func(string) (string, error) { return typstBin, nil },
		"build", "talks/2026/clean.md", "--to", "slides",
	)
	if code != cli.ExitSuccess {
		t.Fatalf("real typst clean slides failed (%d): %s", code, errOut)
	}

	cleanPDF, err := os.ReadFile(filepath.Join(deckDir, "clean.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(cleanPDF, []byte("%PDF-")) || len(cleanPDF) < 2000 {
		t.Fatalf("clean.pdf invalid PDF (len=%d)", len(cleanPDF))
	}
	if gotPages := countTypstPDFPages(cleanPDF); gotPages != 5 {
		t.Errorf("clean.pdf page count = %d, want 5", gotPages)
	}
	assertMarkerPages(t, cleanRunner.lastQueryJSON, 5)

	// Verify night Theme + pt-BR compiles cleanly with zero warnings too.
	code, _, errOut = runSlidesCLI(
		t,
		root,
		cacheDir,
		cleanRunner,
		func(string) (string, error) { return typstBin, nil },
		"build", "talks/2026/clean.md", "--to", "slides", "--theme", "night", "--lang", "pt-BR", "-o", "talks/2026/clean-night.pdf",
	)
	if code != cli.ExitSuccess {
		t.Fatalf("real typst night slides failed (%d): %s", code, errOut)
	}
	nightPDF, err := os.ReadFile(filepath.Join(deckDir, "clean-night.pdf"))
	if err != nil || !bytes.HasPrefix(nightPDF, []byte("%PDF-")) {
		t.Fatalf("clean-night.pdf invalid: err=%v len=%d", err, len(nightPDF))
	}
	if gotPages := countTypstPDFPages(nightPDF); gotPages != 5 {
		t.Errorf("clean-night.pdf page count = %d, want 5", gotPages)
	}

	// 2. Intentionally overflowing deck: Slide 3 spills onto extra page.
	var longBody strings.Builder
	for i := 1; i <= 26; i++ {
		longBody.WriteString("- Observation row ")
		longBody.WriteString(string(rune('A' + (i-1)%26)))
		longBody.WriteString(": dense analytical evidence extending past the 16:9 canvas height.\n")
	}
	overflowDeck := `---
title: Overflow Detection Deck
author: Ada Lovelace
---

# Clean Overview

Fits easily on a single 16:9 canvas.

---

# Overstuffed Appendix

` + longBody.String() + `

---

# Closing Notes

Final summary slide.
`
	if err := os.WriteFile(filepath.Join(deckDir, "overflow.md"), []byte(overflowDeck), 0o644); err != nil {
		t.Fatal(err)
	}

	overflowPDFPath := filepath.Join(deckDir, "overflow.pdf")
	code, _, errOut = runSlidesCLI(
		t,
		root,
		cacheDir,
		cleanRunner,
		func(string) (string, error) { return typstBin, nil },
		"build", "talks/2026/overflow.md", "--to", "slides",
	)
	if code != cli.ExitError {
		t.Fatalf("expected ExitError on overflowing deck, got %d (stderr=%s)", code, errOut)
	}
	for _, want := range []string{"slide 3", "Overstuffed Appendix", "pages 3-4"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("expected overflow stderr %q to contain %q", errOut, want)
		}
	}
	if _, err := os.Stat(overflowPDFPath); !os.IsNotExist(err) {
		t.Errorf("expected overflowing PDF %q to be deleted without --allow-overflow", overflowPDFPath)
	}

	// With --allow-overflow: exits 0, warns on stderr, preserves valid 5-page PDF (4 logical slides + 1 spilled page).
	code, out, errOut := runSlidesCLI(
		t,
		root,
		cacheDir,
		cleanRunner,
		func(string) (string, error) { return typstBin, nil },
		"build", "talks/2026/overflow.md", "--to", "slides", "--allow-overflow",
	)
	if code != cli.ExitSuccess {
		t.Fatalf("expected ExitSuccess with --allow-overflow, got %d (stderr=%s)", code, errOut)
	}
	if !strings.Contains(out, overflowPDFPath) {
		t.Errorf("expected stdout %q to mention %q", out, overflowPDFPath)
	}
	for _, want := range []string{"warning:", "slide 3", "Overstuffed Appendix", "pages 3-4"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("expected warning stderr %q to contain %q", errOut, want)
		}
	}
	overflowPDF, err := os.ReadFile(overflowPDFPath)
	if err != nil || !bytes.HasPrefix(overflowPDF, []byte("%PDF-")) {
		t.Fatalf("expected preserved overflow PDF at %q, err=%v", overflowPDFPath, err)
	}
	if gotPages := countTypstPDFPages(overflowPDF); gotPages != 5 {
		t.Errorf("overflow.pdf page count = %d, want 5 (4 slides + 1 overflow page)", gotPages)
	}
}

func TestBuildSlidesNoFrontmatterDirectFirstSlideAndBibliography(t *testing.T) {
	t.Run("no frontmatter skips cover slide", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		if err := os.WriteFile(filepath.Join(dir, "bare.md"), []byte("# Alpha\n\nOne.\n\n---\n\n# Beta\n\nTwo.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, _ := os.ReadFile(req.InputPath)
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			QueryOutput: []byte(`[]`),
		}
		code, _, errOut := runSlidesCLI(t, dir, cacheDir, runner, func(string) (string, error) { return "/usr/bin/typst", nil }, "build", "bare.md", "--to", "slides")
		if code != cli.ExitSuccess {
			t.Fatalf("failed (%d): %s", code, errOut)
		}
		if !strings.Contains(capturedTyp, `index: 1, title: "Alpha"`) || !strings.Contains(capturedTyp, `index: 2, title: "Beta"`) {
			t.Errorf("expected slides 1=Alpha, 2=Beta without cover, got:\n%s", capturedTyp)
		}
		if strings.Contains(capturedTyp, `index: 3`) {
			t.Errorf("unexpected 3rd slide in bare deck")
		}
	})

	t.Run("frontmatter followed immediately by heading emits cover plus content slides and references slide", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		if err := os.WriteFile(filepath.Join(dir, "refs.bib"), []byte("@book{tufte2001, author={Tufte, Edward}, title={Visual Display}, year={2001}}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		md := `---
title: Cited Deck
bibliography: refs.bib
---

# Empirical Findings

As shown in [@tufte2001].
`
		if err := os.WriteFile(filepath.Join(dir, "cited.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, _ := os.ReadFile(req.InputPath)
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			QueryOutput: []byte(`[]`),
		}
		code, _, errOut := runSlidesCLI(t, dir, cacheDir, runner, func(string) (string, error) { return "/usr/bin/typst", nil }, "build", "cited.md", "--to", "slides")
		if code != cli.ExitSuccess {
			t.Fatalf("failed (%d): %s", code, errOut)
		}
		for _, want := range []string{
			`index: 1, title: "Cited Deck"`,
			`index: 2, title: "Empirical Findings"`,
			`index: 3, title: "References"`,
			`#cite(label("tufte2001"))`,
			`#bibliography(`,
		} {
			if !strings.Contains(capturedTyp, want) {
				t.Errorf("missing %q in:\n%s", want, capturedTyp)
			}
		}
	})
}

func TestBuildSlidesAsymmetricCoverMarginAndFullWidthKPI(t *testing.T) {
	t.Run("cover with margin annotations uses main width and places margin notes before cover block", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		md := `---
title: Asymmetric Cover Deck
author: Ada Lovelace
---

:::note
**Scope** Right-column cover margin note.
:::

Intro paragraph[^cov].

[^cov]: **Footnote** Cover sidenote.

---

# Second Slide

Body.
`
		if err := os.WriteFile(filepath.Join(dir, "cover.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, _ := os.ReadFile(req.InputPath)
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			QueryOutput: []byte(`[]`),
		}
		code, _, errOut := runSlidesCLI(t, dir, cacheDir, runner, func(string) (string, error) { return "/usr/bin/typst", nil }, "build", "cover.md", "--to", "slides")
		if code != cli.ExitSuccess {
			t.Fatalf("failed (%d): %s", code, errOut)
		}
		noteIdx := strings.Index(capturedTyp, "#df-margin-note[")
		coverIdx := strings.Index(capturedTyp, "#block(width: df-main-w)[")
		if noteIdx < 0 || coverIdx < 0 || noteIdx > coverIdx {
			t.Fatalf("expected #df-margin-note before #block(width: df-main-w), got noteIdx=%d coverIdx=%d\n%s", noteIdx, coverIdx, capturedTyp)
		}
	})

	t.Run("cover without margin annotations keeps wide cover and expands KPI strip", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "cache")
		md := `---
title: Minimalist Wide Cover
---

:::kpi
- **Slides**: 10 | 10 min
- **Assets**: 110 | B3 IBrX-100
:::

---

# Next Slide

Body.
`
		if err := os.WriteFile(filepath.Join(dir, "widecover.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		var capturedTyp string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, _ := os.ReadFile(req.InputPath)
				capturedTyp = string(raw)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			QueryOutput: []byte(`[]`),
		}
		code, _, errOut := runSlidesCLI(t, dir, cacheDir, runner, func(string) (string, error) { return "/usr/bin/typst", nil }, "build", "widecover.md", "--to", "slides")
		if code != cli.ExitSuccess {
			t.Fatalf("failed (%d): %s", code, errOut)
		}
		if !strings.Contains(capturedTyp, "#df-wide[\n#df-kpi[") {
			t.Errorf("expected wide KPI strip on cover without margin annotations, got:\n%s", capturedTyp)
		}
		if strings.Contains(capturedTyp, `index: 2, title: ""`) {
			t.Errorf("expected KPI block to stay in cover slide 1 rather than creating an extra slide:\n%s", capturedTyp)
		}
		if !strings.Contains(capturedTyp, `#show figure.caption: it => block(width: 100%, align(left)[#it])`) {
			t.Errorf("expected left-aligned figure.caption rule in slides output")
		}
	})
}

type slidesStrictRealTypstRunner struct {
	t             *testing.T
	lastQueryJSON []byte
}

func (s *slidesStrictRealTypstRunner) Compile(ctx context.Context, req toolchain.CompileRequest) error {
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

func (s *slidesStrictRealTypstRunner) Query(ctx context.Context, req toolchain.QueryRequest) ([]byte, error) {
	out, err := toolchain.RealTypstRunner{}.Query(ctx, req)
	if err == nil {
		s.lastQueryJSON = append([]byte(nil), out...)
	}
	return out, err
}

func (s *slidesStrictRealTypstRunner) Version(ctx context.Context, typstBin string) (string, error) {
	return toolchain.RealTypstRunner{}.Version(ctx, typstBin)
}

var pdfPageLeafRe = regexp.MustCompile(`/Type\s*/Page(?:\b|/)`)

func countTypstPDFPages(pdf []byte) int {
	return len(pdfPageLeafRe.FindAll(pdf, -1))
}

func assertMarkerPages(t *testing.T, raw []byte, wantSlides int) {
	t.Helper()
	var items []struct {
		Value struct {
			Kind  string `json:"kind"`
			Index int    `json:"index"`
			Title string `json:"title"`
			Page  int    `json:"page"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("unmarshal query markers: %v\nraw=%s", err, string(raw))
	}
	if len(items) != wantSlides*2 {
		t.Fatalf("got %d boundary markers, want %d: %s", len(items), wantSlides*2, string(raw))
	}
	for i := 0; i < wantSlides; i++ {
		wantIdx := i + 1
		start := items[i*2].Value
		end := items[i*2+1].Value
		if start.Kind != "start" || start.Index != wantIdx || start.Page != wantIdx {
			t.Errorf("slide %d start marker mismatch: %+v", wantIdx, start)
		}
		if end.Kind != "end" || end.Index != wantIdx || end.Page != wantIdx {
			t.Errorf("slide %d end marker mismatch: %+v", wantIdx, end)
		}
	}
}
