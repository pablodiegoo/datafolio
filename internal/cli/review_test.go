package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/review"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

func runReviewCLI(t *testing.T, workDir string, runner toolchain.TypstRunner, lookPath func(string) (string, error), args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := cli.Options{
		Stdout:      &stdout,
		Stderr:      &stderr,
		WorkDir:     workDir,
		HomeDir:     workDir,
		CacheDir:    filepath.Join(workDir, ".cache"),
		Version:     "0.1.0-test",
		TypstRunner: runner,
		LookPath:    lookPath,
		GOOS:        "linux",
		GOARCH:      "amd64",
	}
	code := cli.Run(context.Background(), args, opts)
	return code, stdout.String(), stderr.String()
}

func TestReviewCLIUsageAndFlags(t *testing.T) {
	dir := t.TempDir()
	cleanDoc := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(cleanDoc, []byte("# Clean Title\n\nConcise empirical prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("help exits zero", func(t *testing.T) {
		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "--help")
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s)", code, cli.ExitSuccess, errOut)
		}
		if !strings.Contains(out, "--format") || !strings.Contains(out, "--strict") || !strings.Contains(out, "--tokens") {
			t.Fatalf("help output missing expected flags: %q", out)
		}
	})

	t.Run("invalid arguments, flags, format, school, missing target, missing tokens return ExitUsage", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
		}{
			{name: "no paths", args: []string{"review"}},
			{name: "unknown flag", args: []string{"review", "--bogus-flag", "clean.md"}},
			{name: "invalid format", args: []string{"review", "--format", "xml", "clean.md"}},
			{name: "invalid school", args: []string{"review", "--school", "unknown", "clean.md"}},
			{name: "missing target path", args: []string{"review", "does-not-exist.md"}},
			{name: "missing tokens path", args: []string{"review", "--tokens", "missing-tokens.yaml", "clean.md"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				code, _, errOut := runReviewCLI(t, dir, nil, nil, tc.args...)
				if code != cli.ExitUsage {
					t.Fatalf("args %v: got exit %d, want %d (stderr=%s)", tc.args, code, cli.ExitUsage, errOut)
				}
				if strings.TrimSpace(errOut) == "" {
					t.Fatalf("args %v: expected diagnostic on stderr", tc.args)
				}
			})
		}
	})
}

func decodeReviewReport(t *testing.T, out string) review.Report {
	t.Helper()
	var rep review.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("failed to decode JSON review report: %v\noutput:\n%s", err, out)
	}
	return rep
}

func TestReviewTokenContractsWCAGAA(t *testing.T) {
	t.Run("passing standalone DTCG YAML and JSON exit zero", func(t *testing.T) {
		dir := t.TempDir()
		passYAML := `color:
  light:
    paper: { $type: color, $value: "#fffff8" }
    ink: { $type: color, $value: "#111111" }
    muted: { $type: color, $value: "#6b6559" }
    rule_strong: { $type: color, $value: "#3d3932" }
  dark:
    paper: { $type: color, $value: "#121316" }
    ink: { $type: color, $value: "#ece8de" }
    accent: { $type: color, $value: "#e06c60" }
`
		passJSON := `{
  "color": {
    "paper": { "$type": "color", "$value": "#ffffff" },
    "ink": { "$type": "color", "$value": "#111111" },
    "accent": { "$type": "color", "$value": "#8b1e1e" }
  }
}`
		if err := os.WriteFile(filepath.Join(dir, "pass.yaml"), []byte(passYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pass.json"), []byte(passJSON), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "pass.yaml", "pass.json", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 0 || rep.WarnCount != 0 || len(rep.Findings) != 0 {
			t.Fatalf("expected 0 findings, got %+v", rep)
		}
		if rep.Files != 2 {
			t.Fatalf("Files = %d, want 2", rep.Files)
		}
	})

	t.Run("failing DTCG YAML contrast reports token-contrast-aa errors with exact lines", func(t *testing.T) {
		dir := t.TempDir()
		failYAML := `color:
  light:
    paper: { $type: color, $value: "#fffff8" }
    ink: { $type: color, $value: "#111111" }
    muted: { $type: color, $value: "#aaaaaa" }
    rule_strong: { $type: color, $value: "#dedede" }
  dark:
    paper: { $type: color, $value: "#121316" }
    ink: { $type: color, $value: "#2a2b2e" }
`
		if err := os.WriteFile(filepath.Join(dir, "tokens.yaml"), []byte(failYAML), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "tokens.yaml", "--format", "json")
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 3 || len(rep.Findings) != 3 {
			t.Fatalf("got %d errors (%d findings), want 3: %+v", rep.ErrorCount, len(rep.Findings), rep)
		}

		wantByLine := map[int]string{
			5: "color.light.muted",
			6: "color.light.rule_strong",
			9: "color.dark.ink",
		}
		for _, f := range rep.Findings {
			if f.RuleID != "token-contrast-aa" || f.Severity != review.SeverityError || f.File != "tokens.yaml" {
				t.Errorf("unexpected finding metadata: %+v", f)
			}
			wantToken, ok := wantByLine[f.Line]
			if !ok {
				t.Errorf("unexpected finding line %d: %+v", f.Line, f)
				continue
			}
			if !strings.Contains(f.Message, wantToken) || strings.TrimSpace(f.Hint) == "" {
				t.Errorf("finding at line %d missing token %q or hint: %+v", f.Line, wantToken, f)
			}
			delete(wantByLine, f.Line)
		}
		if len(wantByLine) != 0 {
			t.Errorf("missing expected line findings: %+v", wantByLine)
		}
	})

	t.Run("DESIGN.md frontmatter and fenced blocks audited via direct target and --tokens flag", func(t *testing.T) {
		dir := t.TempDir()
		designMD := `# Design Contract

` + "```yaml\n" + `colors:
  paper_theme:
    paper:
      value: "#fffff8"
    muted:
      value: "#b5b5b5"
  night_theme:
    paper: "#121316"
    ink: "#1f2024"
` + "```\n"
		if err := os.WriteFile(filepath.Join(dir, "DESIGN.md"), []byte(designMD), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte("# Clean\n\nAll good.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "report.md", "--tokens", "DESIGN.md", "--format", "json")
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 2 {
			t.Fatalf("got %d errors, want 2: %+v", rep.ErrorCount, rep)
		}
		if rep.Findings[0].File != "DESIGN.md" || rep.Findings[0].Line != 8 {
			t.Errorf("expected first finding at DESIGN.md:8, got %+v", rep.Findings[0])
		}
		if rep.Findings[1].File != "DESIGN.md" || rep.Findings[1].Line != 12 {
			t.Errorf("expected second finding at DESIGN.md:12, got %+v", rep.Findings[1])
		}
	})
}

func TestReviewMarkdownSidenoteOrphans(t *testing.T) {
	t.Run("paired sidenotes pass and code examples do not trigger false positives", func(t *testing.T) {
		dir := t.TempDir()
		docSrc := `---
title: Paired Sidenotes
---

Valid reference[^ok] and code example ` + "`[^not-a-ref]`" + `.

` + "```markdown\n[^also-not-a-ref]\n```" + `

[^ok]: **Note** Properly paired definition.
`
		if err := os.WriteFile(filepath.Join(dir, "ok.md"), []byte(docSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "ok.md", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if len(rep.Findings) != 0 {
			t.Fatalf("expected 0 findings, got %+v", rep.Findings)
		}
	})

	t.Run("orphan ref and orphan def emit exact line errors", func(t *testing.T) {
		dir := t.TempDir()
		docSrc := `---
title: Broken Sidenotes
---

First line[^ok] and broken[^ghost].

[^ok]: **OK** Paired note.

[^unused]: Unused definition on line 9.
`
		if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte(docSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "broken.md", "--format", "json")
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 2 || len(rep.Findings) != 2 {
			t.Fatalf("got %+v, want 2 errors", rep)
		}

		f0, f1 := rep.Findings[0], rep.Findings[1]
		if f0.RuleID != "sidenote-orphan-ref" || f0.Line != 5 || f0.Severity != review.SeverityError || !strings.Contains(f0.Message, "ghost") {
			t.Errorf("unexpected first finding: %+v", f0)
		}
		if f1.RuleID != "sidenote-orphan-def" || f1.Line != 9 || f1.Severity != review.SeverityError || !strings.Contains(f1.Message, "unused") {
			t.Errorf("unexpected second finding: %+v", f1)
		}
	})
}

func TestReviewMarkdownEmojiDisallowed(t *testing.T) {
	t.Run("allows Latin/PT-BR/ES punctuation, typographic marks, math symbols, and code emojis", func(t *testing.T) {
		dir := t.TempDir()
		docSrc := `---
title: Análise Estatística — § 2.1 · Série Histórica
author: João da Silva & María González
---

# Equações e Pontuação (§ 1 · Resumo)

Em português e espanhol: ação, coração, informação, año, ¿cómo?, ¡bueno! © 2026 ® ™.
Pontuação editorial: travessão — meia-risca – reticências… marcadores • † ‡ → ← ✓.
Matemática: ∑ x_i ≤ 10 × μ ± σ ÷ 2 ≠ ∞, Δ = 42°.
Exemplo em código inline ` + "`🚀 ⚠️ 👨‍💻`" + ` não deve falhar.

` + "```text\n🚀 🔥 ✅ 1️⃣\n```" + `
`
		if err := os.WriteFile(filepath.Join(dir, "clean_unicode.md"), []byte(docSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "clean_unicode.md", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 0 || len(rep.Findings) != 0 {
			t.Fatalf("expected 0 findings for valid Unicode/math/code, got %+v", rep.Findings)
		}
	})

	t.Run("flags emojis in headings, prose, tables, captions, sources, sidenotes, and margin notes with exact lines", func(t *testing.T) {
		dir := t.TempDir()
		docSrc := `---
title: Clean Frontmatter
---

# Roadmap 🚀

First clean line
with warning ⚠️ and developer 👨‍💻 and flag 🇧🇷 and keycap 1️⃣
and sidenote ref[^1].

[^1]: **Note 💡** Sidenote text ✅.

> **Margin Note:** Margin body 🎯.

:::note Note Title 📌
Container note body 🔥.
:::

Table: Table caption 📊

| Metric | Value |
| --- | --- |
| Status | Pass 🟢 |

*Source: Telemetry 🛰️*

![Figure caption 📈](chart.svg)

*Source: Lab 🔬*
`
		if err := os.WriteFile(filepath.Join(dir, "emojis.md"), []byte(docSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "emojis.md", "--format", "json")
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeReviewReport(t, out)

		// Count findings by line to verify exact line accuracy and single-finding-per-cluster behavior.
		countsByLine := make(map[int]int)
		for _, f := range rep.Findings {
			if f.RuleID != "emoji-disallowed" || f.Severity != review.SeverityError || f.File != "emojis.md" {
				t.Errorf("unexpected finding metadata: %+v", f)
			}
			countsByLine[f.Line]++
		}

		wantCounts := map[int]int{
			5:  1, // 🚀 in heading
			8:  4, // ⚠️, 👨‍💻, 🇧🇷, 1️⃣ on paragraph line 8
			11: 2, // 💡 in sidenote title, ✅ in sidenote body
			13: 1, // 🎯 in blockquote margin note
			15: 1, // 📌 in :::note title
			16: 1, // 🔥 in :::note body
			19: 1, // 📊 in table caption
			23: 1, // 🟢 in table cell
			25: 1, // 🛰️ in table source
			27: 1, // 📈 in figure caption
			29: 1, // 🔬 in figure source
		}
		for line, want := range wantCounts {
			if got := countsByLine[line]; got != want {
				t.Errorf("line %d: got %d emoji findings, want %d (all findings: %+v)", line, got, want, rep.Findings)
			}
		}
		if len(countsByLine) != len(wantCounts) {
			t.Errorf("unexpected lines flagged: got %v, want %v", countsByLine, wantCounts)
		}
	})
}

func TestReviewWarningsDeletionTestTablesFiguresAndStrict(t *testing.T) {
	t.Run("EN, PT, and ES Deletion Test phrases emit warn findings with exact lines and skip code", func(t *testing.T) {
		dir := t.TempDir()
		docSrc := `---
title: Multilingual Editorial Audit
---

## In Conclusion

It's important to note that baseline throughput doubled[^1].
Interestingly, uninterestingly is not a match, nor is ` + "`as we can see`" + `.

It is important
to note that latency fell across cohorts.

É importante destacar que os dados sugerem que a variância caiu.

Vale la pena mencionar que los datos sugieren que la tendencia persiste.

[^1]: **Scope** Based on the data provided, results hold across regions.

> **Margin Note:** Como podemos ver, cabe mencionar que o modelo convergiu.

` + "```markdown\nIn conclusion, this is inside a fenced code block.\n```" + `
`
		if err := os.WriteFile(filepath.Join(dir, "deletion.md"), []byte(docSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		// Default (non-strict): warnings exist -> ExitSuccess (0)
		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "deletion.md", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("default mode with warnings: got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 0 {
			t.Fatalf("expected 0 errors, got %d: %+v", rep.ErrorCount, rep.Findings)
		}

		// Expect findings:
		// line 7: "It's important to note that"
		// line 8: "Interestingly"
		// line 10: "It is important\nto note that" (starts on line 10)
		// line 13: "É importante destacar que", "os dados sugerem que" (2)
		// line 15: "Vale la pena mencionar que", "los datos sugieren que" (2)
		// line 17: "Based on the data provided"
		// line 19: "Como podemos ver", "cabe mencionar que" (2)
		wantByLine := map[int]int{
			7:  1,
			8:  1,
			10: 1,
			13: 2,
			15: 2,
			17: 1,
			19: 2,
		}
		gotByLine := make(map[int]int)
		for _, f := range rep.Findings {
			if f.RuleID != "prose-deletion-test" || f.Severity != review.SeverityWarn || strings.TrimSpace(f.Hint) == "" {
				t.Errorf("unexpected deletion-test finding: %+v", f)
			}
			gotByLine[f.Line]++
		}
		for line, want := range wantByLine {
			if gotByLine[line] != want {
				t.Errorf("line %d: got %d deletion-test findings, want %d (all: %+v)", line, gotByLine[line], want, rep.Findings)
			}
		}
		if len(gotByLine) != len(wantByLine) {
			t.Errorf("unexpected lines flagged: got %v, want %v", gotByLine, wantByLine)
		}

		// Under --strict: warnings promote exit code to ExitError (1)
		strictCode, strictOut, strictErr := runReviewCLI(t, dir, nil, nil, "review", "--strict", "deletion.md", "--format", "json")
		if strictCode != cli.ExitError {
			t.Fatalf("--strict with warnings: got exit %d, want %d (stderr=%s, stdout=%s)", strictCode, cli.ExitError, strictErr, strictOut)
		}
	})

	t.Run("tables and figures missing captions or sources emit warn findings and promote under --strict", func(t *testing.T) {
		dir := t.TempDir()
		docSrc := `---
title: Attribution Audit
---

Table: Complete Table

| Col | Val |
| --- | --- |
| A | 1 |

*Source: Census Bureau*

:::wide
| Missing Both | Val |
| --- | --- |
| B | 2 |
:::

![Complete chart](ok.svg)

*Source: Telemetry*

![](missing-both.svg)
`
		if err := os.WriteFile(filepath.Join(dir, "attrs.md"), []byte(docSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runReviewCLI(t, dir, nil, nil, "review", "attrs.md", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 0 || rep.WarnCount != 4 || len(rep.Findings) != 4 {
			t.Fatalf("expected 0 errors and 4 warnings, got %+v", rep)
		}

		wantRules := map[string]int{
			"table-missing-caption":  14,
			"table-missing-source":   14,
			"figure-missing-caption": 23,
			"figure-missing-source":  23,
		}
		for _, f := range rep.Findings {
			if f.Severity != review.SeverityWarn || strings.TrimSpace(f.Hint) == "" {
				t.Errorf("unexpected finding metadata: %+v", f)
			}
			wantLine, ok := wantRules[f.RuleID]
			if !ok {
				t.Errorf("unexpected rule %q: %+v", f.RuleID, f)
				continue
			}
			if f.Line != wantLine {
				t.Errorf("rule %s: got line %d, want %d", f.RuleID, f.Line, wantLine)
			}
			delete(wantRules, f.RuleID)
		}
		if len(wantRules) != 0 {
			t.Errorf("missing expected rules: %+v", wantRules)
		}

		strictCode, _, _ := runReviewCLI(t, dir, nil, nil, "review", "--strict", "attrs.md")
		if strictCode != cli.ExitError {
			t.Fatalf("--strict with missing captions/sources: got exit %d, want %d", strictCode, cli.ExitError)
		}
	})
}

func TestReviewSlideOverflow(t *testing.T) {
	deckSrc := `---
title: Quarterly Telemetry Deck
format: slides
author: Ada Lovelace
---

Prepared for the engineering leadership review.

---

# Methodology

Cohort comparison across control and rollout regions.

---

# Dense Matrix

Heavy multi-row empirical table exceeding 16:9 canvas.
`

	t.Run("clean slides deck compiles into temp dir and exits zero", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(deckSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		var compiledOut string
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				compiledOut = req.OutputPath
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			QueryOutput: []byte(`[
				{"func":"metadata","value":{"kind":"start","index":1,"title":"Quarterly Telemetry Deck","page":1},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"end","index":1,"title":"Quarterly Telemetry Deck","page":1},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"start","index":2,"title":"Methodology","page":2},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"end","index":2,"title":"Methodology","page":2},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"start","index":3,"title":"Dense Matrix","page":3},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"end","index":3,"title":"Dense Matrix","page":3},"label":"<datafolio-slide>"}
			]`),
		}

		code, out, errOut := runReviewCLI(
			t,
			dir,
			runner,
			func(string) (string, error) { return "/usr/local/bin/typst", nil },
			"review", "deck.md", "--format", "json",
		)
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		if len(runner.CompileCalls) != 1 || len(runner.QueryCalls) != 1 {
			t.Fatalf("expected 1 compile and 1 query call, got %d compile / %d query", len(runner.CompileCalls), len(runner.QueryCalls))
		}
		// Temp dir and PDF must be cleaned up and never written into WorkDir.
		if _, err := os.Stat(filepath.Join(dir, "deck.pdf")); !os.IsNotExist(err) {
			t.Errorf("review must not write deck.pdf into working directory")
		}
		if _, err := os.Stat(filepath.Dir(compiledOut)); !os.IsNotExist(err) {
			t.Errorf("expected temp workspace %q to be removed after review", filepath.Dir(compiledOut))
		}
	})

	t.Run("overflowing slides emit slide-overflow error findings with slide number, title, and line", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(deckSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			QueryOutput: []byte(`[
				{"func":"metadata","value":{"kind":"start","index":1,"title":"Quarterly Telemetry Deck","page":1},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"end","index":1,"title":"Quarterly Telemetry Deck","page":1},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"start","index":2,"title":"Methodology","page":2},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"end","index":2,"title":"Methodology","page":3},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"start","index":3,"title":"Dense Matrix","page":4},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"end","index":3,"title":"Dense Matrix","page":5},"label":"<datafolio-slide>"}
			]`),
		}

		code, out, errOut := runReviewCLI(
			t,
			dir,
			runner,
			func(string) (string, error) { return "/usr/local/bin/typst", nil },
			"review", "deck.md", "--format", "json",
		)
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeReviewReport(t, out)
		if rep.ErrorCount != 2 || len(rep.Findings) != 2 {
			t.Fatalf("expected 2 slide-overflow errors, got %+v", rep)
		}

		f0 := rep.Findings[0]
		if f0.RuleID != "slide-overflow" || f0.File != "deck.md" || f0.Line != 11 || f0.Severity != review.SeverityError {
			t.Errorf("unexpected finding[0]: %+v", f0)
		}
		if !strings.Contains(f0.Message, "slide 2") || !strings.Contains(f0.Message, "Methodology") || strings.TrimSpace(f0.Hint) == "" {
			t.Errorf("finding[0] missing slide details or hint: %+v", f0)
		}

		f1 := rep.Findings[1]
		if f1.RuleID != "slide-overflow" || f1.File != "deck.md" || f1.Line != 17 || f1.Severity != review.SeverityError {
			t.Errorf("unexpected finding[1]: %+v", f1)
		}
		if !strings.Contains(f1.Message, "slide 3") || !strings.Contains(f1.Message, "Dense Matrix") || strings.TrimSpace(f1.Hint) == "" {
			t.Errorf("finding[1] missing slide details or hint: %+v", f1)
		}
	})

	t.Run("missing typst binary when reviewing slides returns ExitError with doctor hint", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "deck.md"), []byte(deckSrc), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := runReviewCLI(
			t,
			dir,
			&toolchain.FakeTypstRunner{},
			func(string) (string, error) { return "", exec.ErrNotFound },
			"review", "deck.md",
		)
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s)", code, cli.ExitError, errOut)
		}
		if !strings.Contains(errOut, "doctor --install-typst") {
			t.Errorf("expected doctor hint on stderr, got %q", errOut)
		}
	})
}

func TestReviewOutputFormatsAndDirectoryWalk(t *testing.T) {
	dir := t.TempDir()

	// Write datafolio.yaml (must NOT be misparsed as a Token Contract during directory walk).
	if err := os.WriteFile(filepath.Join(dir, "datafolio.yaml"), []byte("school: tufte\ntheme: paper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write DESIGN.md with frontmatter contrast failure (line 9).
	designMD := `---
design_tokens:
  color:
    light:
      paper: { value: "#fffff8", type: "color" }
      ink: { value: "#111111", type: "color" }
      muted: { value: "#bbbbbb", type: "color" }
---

# Design System
`
	if err := os.WriteFile(filepath.Join(dir, "DESIGN.md"), []byte(designMD), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write JSON token contract with failing contrast (line 6).
	tokensJSON := `{
  "color": {
    "paper": { "$type": "color", "$value": "#ffffff" },
    "ink": { "$type": "color", "$value": "#cccccc" }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "palette.json"), []byte(tokensJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write Markdown Source Document in nested dir with an error and a warning.
	subDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportMD := `# Report

In conclusion, see missing[^ghost].
`
	if err := os.WriteFile(filepath.Join(subDir, "report.md"), []byte(reportMD), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write web template/stylesheet files to verify clean extension dispatch during directory walks.
	for _, rel := range []string{"web/index.html", "web/card.templ", "web/view.blade.php", "web/site.css", "web/theme.scss"} {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("/* placeholder */\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Hidden agent dirs and non-contract JSON arrays should be ignored during directory walk.
	agentSkill := filepath.Join(dir, ".agents", "skills", "datafolio", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(agentSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentSkill, []byte("# Skill 🚀\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "markers.json"), []byte(`[{"kind":"start"}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Audit "." while ALSO passing --tokens DESIGN.md and --school tufte; DESIGN.md must not be duplicated.
	codeJSON, outJSON, errJSON := runReviewCLI(t, dir, nil, nil, "review", ".", "--tokens", "DESIGN.md", "--school", "tufte", "--format", "json")
	if codeJSON != cli.ExitError {
		t.Fatalf("json format: got exit %d, want %d (stderr=%s, stdout=%s)", codeJSON, cli.ExitError, errJSON, outJSON)
	}
	rep := decodeReviewReport(t, outJSON)
	// Expected findings (sorted by file, line, rule ID):
	// 1. DESIGN.md:7 error token-contrast-aa
	// 2. docs/report.md:3 warn prose-deletion-test
	// 3. docs/report.md:3 error sidenote-orphan-ref
	// 4. palette.json:4 error token-contrast-aa
	if rep.ErrorCount != 3 || rep.WarnCount != 1 || len(rep.Findings) != 4 {
		t.Fatalf("unexpected report counts/findings: %+v", rep)
	}

	wantOrder := []struct {
		file     string
		line     int
		ruleID   string
		severity review.Severity
	}{
		{file: "DESIGN.md", line: 7, ruleID: "token-contrast-aa", severity: review.SeverityError},
		{file: "docs/report.md", line: 3, ruleID: "prose-deletion-test", severity: review.SeverityWarn},
		{file: "docs/report.md", line: 3, ruleID: "sidenote-orphan-ref", severity: review.SeverityError},
		{file: "palette.json", line: 4, ruleID: "token-contrast-aa", severity: review.SeverityError},
	}
	for i, w := range wantOrder {
		got := rep.Findings[i]
		if got.File != w.file || got.Line != w.line || got.RuleID != w.ruleID || got.Severity != w.severity || strings.TrimSpace(got.Message) == "" || strings.TrimSpace(got.Hint) == "" {
			t.Errorf("finding[%d] = %+v, want %+v", i, got, w)
		}
	}

	// Verify --format text (default) and --format md both include rule ID, file, line, severity, message, and hint for every finding.
	codeText, outText, errText := runReviewCLI(t, dir, nil, nil, "review", ".", "--tokens", "DESIGN.md")
	if codeText != cli.ExitError {
		t.Fatalf("text format: got exit %d, want %d (stderr=%s)", codeText, cli.ExitError, errText)
	}
	codeMD, outMD, errMD := runReviewCLI(t, dir, nil, nil, "review", "--format", "md", ".", "--tokens", "DESIGN.md")
	if codeMD != cli.ExitError {
		t.Fatalf("md format: got exit %d, want %d (stderr=%s)", codeMD, cli.ExitError, errMD)
	}

	if !strings.Contains(outMD, "| Severity | Rule | File | Line | Message | Hint |") {
		t.Errorf("md format missing table header:\n%s", outMD)
	}

	for _, f := range rep.Findings {
		for fmtName, body := range map[string]string{"text": outText, "md": outMD} {
			for _, substr := range []string{f.RuleID, f.File, string(f.Severity), f.Message, f.Hint} {
				if !strings.Contains(body, substr) {
					t.Errorf("%s output missing %q for finding %+v\n--- output ---\n%s", fmtName, substr, f, body)
				}
			}
			lineNeedle := strings.Contains(body, ":3:") || strings.Contains(body, "| 3 |") || strings.Contains(body, ":7:") || strings.Contains(body, "| 7 |") || strings.Contains(body, ":4:") || strings.Contains(body, "| 4 |")
			if !lineNeedle {
				t.Errorf("%s output missing line numbers:\n%s", fmtName, body)
			}
		}
	}
}
