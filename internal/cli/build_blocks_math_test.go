package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
)

func TestBuildSidenotesAlignedMarginHTML(t *testing.T) {
	dir := t.TempDir()
	md := `---
title: Sidenote Monograph
lang: en
---

## Empirical Observations

Revenue rose sharply in the fourth quarter[^1] while operating costs remained stable[^method].

[^1]: **Seasonal Effect** Holiday demand accounted for 62% of the quarterly increase.

[^method]: **Methodology** Figures are seasonally adjusted using X-13ARIMA-SEATS.
    Additional verification confirmed zero restatements across prior quarters.
`
	if err := os.WriteFile(filepath.Join(dir, "sidenotes.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "build", "sidenotes.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "sidenotes.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)

	for _, want := range []string{
		`tufte-margin`,
		`href="#sn-1"`,
		`id="sn-1"`,
		`Seasonal Effect`,
		`Holiday demand accounted for 62% of the quarterly increase.`,
		`href="#sn-2"`,
		`id="sn-2"`,
		`Methodology`,
		`Figures are seasonally adjusted using X-13ARIMA-SEATS.`,
		`Additional verification confirmed zero restatements across prior quarters.`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected sidenote fragment %q\ngot:\n%s", want, html)
		}
	}

	if strings.Contains(html, `[^1]`) || strings.Contains(html, `[^method]`) {
		t.Errorf("raw footnote syntax should not leak into rendered HTML")
	}
}

func TestBuildMarginNotesFencedAndBlockquoteHTML(t *testing.T) {
	dir := t.TempDir()
	md := `---
title: Margin Note Monograph
lang: en
---

Main analytical paragraph preceding a fenced note.

:::note
Unnumbered context from fenced block.
:::

Another paragraph.

> **Margin Note:** Contextual observation from blockquote.

> A regular editorial quotation that stays in the main column.
`
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "build", "notes.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "notes.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)

	for _, want := range []string{
		`tufte-margin`,
		`df-margin-note`,
		`Unnumbered context from fenced block.`,
		`Contextual observation from blockquote.`,
		`df-blockquote`,
		`A regular editorial quotation that stays in the main column.`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected fragment %q\ngot:\n%s", want, html)
		}
	}
	body := html[strings.Index(html, "<body>"):]
	if strings.Contains(body, `df-sidenote-ref`) || strings.Contains(body, `<sup`) {
		t.Errorf("unnumbered margin notes must not generate inline superscript anchors")
	}
	if strings.Contains(html, `:::note`) {
		t.Errorf("raw :::note fence leaked into HTML")
	}

	// Verify Portuguese blockquote syntax (`> **Nota de Margem:**`)
	mdPT := `---
title: Notas em Português
lang: pt-BR
---

Parágrafo principal.

> **Nota de Margem:** Observação sem numeração no texto.
`
	if err := os.WriteFile(filepath.Join(dir, "notas_pt.md"), []byte(mdPT), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, dir, "build", "notas_pt.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("pt-BR build failed (%d): %s", code, errOut)
	}
	rawPT, err := os.ReadFile(filepath.Join(dir, "notas_pt.html"))
	if err != nil {
		t.Fatal(err)
	}
	htmlPT := string(rawPT)
	if !strings.Contains(htmlPT, `df-margin-note`) || !strings.Contains(htmlPT, `Nota de Margem`) || !strings.Contains(htmlPT, `Observação sem numeração no texto.`) {
		t.Errorf("pt-BR margin note missing expected output:\n%s", htmlPT)
	}
}

func TestBuildWideKPIColumnsStructuralBlocksHTML(t *testing.T) {
	dir := t.TempDir()
	md := `---
title: Structural Blocks Report
lang: en
---

:::kpi
- **Annual Revenue**: $4.82M (+14% YoY)
- **Operating Margin**: 22.4%
Churn Rate: 1.9%
:::

:::wide
Table: Full-width benchmark comparison

| Model | Latency (ms) | Throughput (req/s) | Accuracy |
| --- | --- | --- | --- |
| Baseline | 42.1 | 1,200 | 91.4% |
| Optimized | 14.8 | 3,850 | 94.2% |

*Source: Internal benchmark harness*
:::

:::kpi
| Users | Net Retention | Uptime |
| --- | --- | --- |
| 12,400 | 118% | 99.98% |
:::

:::columns
:::column
### Left Hypothesis

Demand scales sublinearly with city population.
:::
:::column
### Right Evidence

Empirical elasticity measured at $\beta = 0.84$ across 40 metros.
:::
:::

:::columns
Left panel via separator.

---

Right panel via separator.
:::
`
	if err := os.WriteFile(filepath.Join(dir, "blocks.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "build", "blocks.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "blocks.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)

	for _, want := range []string{
		`df-kpi-strip`,
		`Annual Revenue`,
		`$4.82M (+14% YoY)`,
		`Operating Margin`,
		`22.4%`,
		`Churn Rate`,
		`1.9%`,
		`Users`,
		`12,400`,
		`Net Retention`,
		`118%`,
		`df-wide`,
		`Full-width benchmark comparison`,
		`df-columns`,
		`df-column`,
		`Left Hypothesis`,
		`Right Evidence`,
		`Left panel via separator.`,
		`Right panel via separator.`,
		`tabular-nums`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected fragment %q\ngot:\n%s", want, html)
		}
	}

	for _, leaked := range []string{":::kpi", ":::", ":::wide", ":::columns", ":::column"} {
		if strings.Contains(html, leaked) {
			t.Errorf("raw fenced container marker %q leaked into HTML", leaked)
		}
	}
}

func TestBuildMathMLAndKaTeXAndCurrencySafetyHTML(t *testing.T) {
	dir := t.TempDir()
	md := `---
title: Mathematical Monograph
lang: en
---

Budget rose from $10 to $20 per unit ($50 total, or $5-$10 discount, and $ 50 $ spaced, and $10$20 adjacent, plus code ` + "`$x^2$`" + `).

Inline identity: $\alpha + \beta \le \gamma$, with $\hat{y}_i \in \mathbb{R}$, $\sqrt{a^2 + b^2}$, and $\sqrt[3]{x^2 + 1}$.

$$
\hat{\beta} = \left( \mathbf{X}^T \mathbf{X} \right)^{-1} \mathbf{X}^T \mathbf{y}, \quad \lim_{n \to \infty} \sum_{i=1}^n \frac{x_i}{n} \approx \int_0^\infty f(t)\,dt
$$

$$
\begin{aligned}
a &\neq b \pm c \times d \cdot e \\
f(x) &\Rightarrow \begin{cases} x^2, & x \ge 0 \\ 0, & \text{otherwise} \end{cases}
\end{aligned}
$$

$$
A = \begin{pmatrix} 1 & 2 \\ 3 & 4 \end{pmatrix}, \quad B = \begin{bmatrix} \bar{x} & \tilde{y} \\ \vec{v} & \prod_{j=1}^k p_j \end{bmatrix}
$$
`
	if err := os.WriteFile(filepath.Join(dir, "math.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	// Default mode: pure-Go MathML
	code, _, errOut := runCLI(t, dir, "build", "math.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("mathml build failed (%d): %s", code, errOut)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "math.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)

	for _, want := range []string{
		`<math xmlns="http://www.w3.org/1998/Math/MathML" display="inline">`,
		`<math xmlns="http://www.w3.org/1998/Math/MathML" display="block">`,
		`<mfrac>`,
		`<msqrt>`,
		`<mroot>`,
		`<munderover>`,
		`<mtable>`,
		`ℝ`,
		`α`,
		`β`,
		`≤`,
		`≥`,
		`≠`,
		`≈`,
		`±`,
		`×`,
		`⋅`,
		`∈`,
		`→`,
		`⇒`,
		`∑`,
		`∏`,
		`∫`,
		`<mtext>otherwise</mtext>`,
		`$10 to $20 per unit ($50 total, or $5-$10 discount, and $ 50 $ spaced, and $10$20 adjacent, plus code <code>$x^2$</code>).`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("MathML HTML missing expected fragment %q\ngot:\n%s", want, html)
		}
	}
	offlineCheck := strings.ReplaceAll(html, "http://www.w3.org/1998/Math/MathML", "")
	for _, forbidden := range []string{"https://", "http://", "<script", "cdn."} {
		if strings.Contains(strings.ToLower(offlineCheck), forbidden) {
			t.Errorf("MathML HTML must remain offline without %q", forbidden)
		}
	}

	// Opt-in --math katex mode
	code, _, errOut = runCLI(t, dir, "build", "math.md", "--to", "html", "--math", "katex", "-o", "math_katex.html")
	if code != cli.ExitSuccess {
		t.Fatalf("katex build failed (%d): %s", code, errOut)
	}
	rawK, err := os.ReadFile(filepath.Join(dir, "math_katex.html"))
	if err != nil {
		t.Fatal(err)
	}
	htmlK := string(rawK)
	if strings.Contains(htmlK, `<math xmlns="http://www.w3.org/1998/Math/MathML"`) {
		t.Errorf("--math katex output should emit KaTeX delimiters instead of MathML")
	}
	for _, want := range []string{
		`class="math inline katex"`,
		`\(`,
		`\)`,
		`katex`,
		`\[`,
		`\]`,
		`$10 to $20 per unit`,
	} {
		if !strings.Contains(htmlK, want) {
			t.Errorf("KaTeX HTML missing expected fragment %q\ngot:\n%s", want, htmlK)
		}
	}
	for _, forbidden := range []string{"https://", "http://", "<script", "cdn."} {
		if strings.Contains(strings.ToLower(htmlK), forbidden) {
			t.Errorf("KaTeX HTML must still have zero external CDN requests (%q)", forbidden)
		}
	}
}

func TestBuildCitationsAndBibliographyHTML(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "papers")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}

	bib := `% Sample bibliography
@book{tufte2001,
  author = {Tufte, Edward R.},
  title = {The {Visual} Display of Quantitative Information},
  publisher = {Graphics Press},
  year = {2001},
}

@article{shannon1948,
  author = "Shannon, Claude E.",
  title = "A Mathematical Theory of Communication",
  journal = "Bell System Technical Journal",
  volume = "27",
  number = "3",
  pages = "379--423",
  year = 1948,
}

@book{cleveland1985,
  author = {William S. Cleveland and Robert McGill},
  title = {Graphical Perception},
  publisher = {Wadsworth},
  year = {1985},
}
`
	if err := os.WriteFile(filepath.Join(docDir, "refs.bib"), []byte(bib), 0o644); err != nil {
		t.Fatal(err)
	}

	md := `---
title: Citation Monograph
lang: en
bibliography: refs.bib
---

Data-ink clarity is foundational [@tufte2001] and connects to channel capacity [@tufte2001; @shannon1948] as well as perceptual accuracy [see @cleveland1985, p. 42].

Contact [user@example.com] should stay literal text.
`
	if err := os.WriteFile(filepath.Join(docDir, "paper.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build from root (not docDir) to ensure relative bibliography resolves against docDir.
	code, _, errOut := runCLI(t, root, "build", "papers/paper.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}

	raw, err := os.ReadFile(filepath.Join(docDir, "paper.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)

	for _, want := range []string{
		`href="#ref-tufte2001"`,
		`Tufte, 2001`,
		`Shannon, 1948`,
		`Cleveland &amp; McGill, 1985`,
		`p. 42`,
		`id="ref-tufte2001"`,
		`id="ref-shannon1948"`,
		`id="ref-cleveland1985"`,
		`References`,
		`The Visual Display of Quantitative Information`,
		`Graphics Press`,
		`Bell System Technical Journal`,
		`379–423`,
		`[user@example.com]`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected citation/reference fragment %q\ngot:\n%s", want, html)
		}
	}

	// Portuguese localized heading (`Referências`)
	mdPT := `---
title: Artigo
lang: pt-BR
bibliography: refs.bib
---

Conforme demostrado em [@tufte2001].
`
	if err := os.WriteFile(filepath.Join(docDir, "artigo.md"), []byte(mdPT), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, root, "build", "papers/artigo.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("pt-BR citation build failed (%d): %s", code, errOut)
	}
	rawPT, err := os.ReadFile(filepath.Join(docDir, "artigo.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawPT), "Referências") {
		t.Errorf("expected pt-BR Referências heading in HTML")
	}

	// Missing bibliography file must return ExitError
	mdMissing := `---
title: Missing Bib
bibliography: does-not-exist.bib
---

Reference [@tufte2001].
`
	if err := os.WriteFile(filepath.Join(docDir, "missing_bib.md"), []byte(mdMissing), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, root, "build", "papers/missing_bib.md", "--to", "html")
	if code != cli.ExitError {
		t.Fatalf("expected ExitError for missing .bib, got %d (stderr: %s)", code, errOut)
	}
}

func TestBuildHTMLRunningChromeAndPrintCSS(t *testing.T) {
	dir := t.TempDir()
	md := `---
title: Annual Monetary Review
subtitle: Policy Transmission Analysis
kicker: MONETARY BRIEF
author: Central Research Desk
date: 2026-11-23
header:
  left: "{kicker}"
  center: "{title}"
  right: "{date}"
  rule: true
  skip_first: true
footer:
  left: "{author}"
  center: "{subtitle}"
  right: "Archival Copy"
  rule: true
---

## Policy Channels

Transmission lags averaged four quarters.
`
	if err := os.WriteFile(filepath.Join(dir, "chrome.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "build", "chrome.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "chrome.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)

	for _, want := range []string{
		`df-chrome-header`,
		`df-chrome-footer`,
		`df-chrome-rule`,
		`MONETARY BRIEF`,
		`Annual Monetary Review`,
		`2026-11-23`,
		`Central Research Desk`,
		`Policy Transmission Analysis`,
		`Archival Copy`,
		`@page`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected Running Chrome fragment %q\ngot:\n%s", want, html)
		}
	}

	for _, forbidden := range []string{
		`table-header-group`,
		`table-footer-group`,
		`{title}`,
		`{subtitle}`,
		`{author}`,
		`{date}`,
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("HTML must not contain %q", forbidden)
		}
	}

	// Default HTML (without header/footer configured) must omit chrome bands in <body>.
	bareMD := `---
title: Bare Memo
---

Short memo body.
`
	if err := os.WriteFile(filepath.Join(dir, "bare.md"), []byte(bareMD), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, dir, "build", "bare.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("bare build failed (%d): %s", code, errOut)
	}
	bareRaw, err := os.ReadFile(filepath.Join(dir, "bare.html"))
	if err != nil {
		t.Fatal(err)
	}
	bareHTML := string(bareRaw)
	body := bareHTML[strings.Index(bareHTML, "<body>"):]
	if strings.Contains(body, `df-chrome-header`) || strings.Contains(body, `df-chrome-footer`) {
		t.Errorf("bare HTML should not render Running Chrome bands when disabled:\n%s", body)
	}
}
