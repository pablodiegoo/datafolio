package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
)

func TestTokensExportFromStandaloneDTCGYAMLAndJSONAllTargets(t *testing.T) {
	yamlContract := `
name: "Oxford Editorial"
color:
  $type: color
  brand:
    navy: { $value: "#1E3A8A" }
  light:
    paper: { $value: "#ffffff" }
    paper_subtle: { $value: "#f8fafc" }
    paper_alt: { $value: "#f1f5f9" }
    ink: { $value: "#0f172a" }
    ink_soft: { $value: "#334155" }
    muted: { $value: "#475569" }
    accent: { $value: "{color.brand.navy}" }
    ochre: { $value: "#854d0e" }
    forest: { $value: "#166534" }
    rule: { $value: "#e2e8f0" }
    rule_strong: { $value: "#475569" }
  dark:
    paper: { $value: "#0b0f17" }
    ink: { $value: "#f8fafc" }
    muted: { $value: "#94a3b8" }
    accent: { $value: "#60a5fa" }
font:
  serif: { $type: fontFamily, $value: "Source Serif 4" }
  mono: { $type: fontFamily, $value: "Fira Code" }
  sans: { $type: fontFamily, $value: "Source Sans 3" }
radius:
  default: { $type: dimension, $value: "0px" }
`
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "tokens.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContract), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "dist")
	code, out, errOut := runCLI(t, dir, "tokens", "export", "--from", "tokens.yaml", "--to", "css,tailwind,scss,typst,matplotlib", "--out", "dist")
	if code != cli.ExitSuccess {
		t.Fatalf("tokens export YAML failed (%d): stdout=%s stderr=%s", code, out, errOut)
	}

	expectedFiles := []string{
		"tokens.css",
		"tokens.tailwind.css",
		"_tokens.scss",
		"tokens.typ",
		"datafolio.mplstyle",
		"tokens.json",
	}
	for _, name := range expectedFiles {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("expected exported artifact %s: %v", name, err)
		}
	}

	cssRaw, _ := os.ReadFile(filepath.Join(outDir, "tokens.css"))
	twRaw, _ := os.ReadFile(filepath.Join(outDir, "tokens.tailwind.css"))
	scssRaw, _ := os.ReadFile(filepath.Join(outDir, "_tokens.scss"))
	typRaw, _ := os.ReadFile(filepath.Join(outDir, "tokens.typ"))
	mplRaw, _ := os.ReadFile(filepath.Join(outDir, "datafolio.mplstyle"))
	jsonRaw, _ := os.ReadFile(filepath.Join(outDir, "tokens.json"))

	// Synchronized resolved alias value (#1E3A8A) across all five targets.
	for target, content := range map[string]string{
		"tokens.css":          string(cssRaw),
		"tokens.tailwind.css": string(twRaw),
		"_tokens.scss":        string(scssRaw),
		"tokens.typ":          string(typRaw),
		"tokens.json":         string(jsonRaw),
	} {
		if !strings.Contains(strings.ToLower(content), "#1e3a8a") {
			t.Errorf("%s missing synchronized accent #1e3a8a:\n%s", target, content)
		}
	}
	if !strings.Contains(strings.ToLower(string(mplRaw)), "1e3a8a") {
		t.Errorf("datafolio.mplstyle missing accent 1e3a8a:\n%s", string(mplRaw))
	}
	if !strings.Contains(string(twRaw), "@theme {") {
		t.Errorf("tokens.tailwind.css missing Tailwind v4 @theme block")
	}

	// Verify standalone DTCG JSON input also works end-to-end.
	jsonInput := `{
  "color": {
    "paper": { "$type": "color", "$value": "#fffff8" },
    "ink": { "$type": "color", "$value": "#111111" },
    "accent": { "$type": "color", "$value": "#8b1e1e" }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "tokens.json"), []byte(jsonInput), 0o644); err != nil {
		t.Fatal(err)
	}
	jsonOut := filepath.Join(dir, "json-dist")
	code, _, errOut = runCLI(t, dir, "tokens", "export", "--from", "tokens.json", "--to", "css,matplotlib", "--out", "json-dist")
	if code != cli.ExitSuccess {
		t.Fatalf("tokens export JSON failed (%d): %s", code, errOut)
	}
	exportedJSON, err := os.ReadFile(filepath.Join(jsonOut, "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(exportedJSON, &parsed); err != nil {
		t.Fatalf("exported tokens.json invalid: %v", err)
	}
}

func TestTokensExportFromDesignMDFrontmatterAndFencedBlocks(t *testing.T) {
	frontmatterDesign := `---
design_tokens:
  name: "Portfolio Design System"
  color:
    brand:
      primary: { value: "#1E3A8A", type: "color" }
      primary_dark: { value: "#60A5FA", type: "color" }
      accent: { value: "#2563EB", type: "color" }
    light:
      surface_canvas: { value: "#FFFFFF", type: "color" }
      text_primary: { value: "#0F172A", type: "color" }
      text_muted: { value: "#475569", type: "color" }
    dark:
      surface_canvas: { value: "#0B0F17", type: "color" }
      text_primary: { value: "#F8FAFC", type: "color" }
      text_muted: { value: "#94A3B8", type: "color" }
    status:
      sanitized_bg: { value: "#EFF6FF", type: "color" }
      sanitized_text: { value: "#1E3A8A", type: "color" }
      sanitized_dark_bg: { value: "rgba(30, 58, 138, 0.25)", type: "color" }
      sanitized_dark_text: { value: "#93C5FD", type: "color" }
  typography:
    family:
      heading: { value: "'Source Serif 4', Georgia, serif", type: "fontFamily" }
      mono: { value: "'Fira Code', monospace", type: "fontFamily" }
---

# Design Contract
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "DESIGN.md"), []byte(frontmatterDesign), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "tokens", "export", "--from", "DESIGN.md", "--to", "css,typst,matplotlib", "--out", "out-fm")
	if code != cli.ExitSuccess {
		t.Fatalf("tokens export DESIGN.md failed (%d): %s", code, errOut)
	}
	cssBytes, err := os.ReadFile(filepath.Join(dir, "out-fm", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cssBytes), "--df-accent: #2563EB;") || !strings.Contains(string(cssBytes), "--df-accent: #60A5FA;") {
		t.Fatalf("expected light and dark accent tokens in CSS:\n%s", string(cssBytes))
	}

	fencedDesign := "# Editorial Design System\n\n```yaml\ncolors:\n  paper_theme:\n    paper: { value: \"#fffff8\" }\n    ink: { value: \"#111111\" }\n    accent: { value: \"#8b1e1e\" }\n  night_theme:\n    paper: \"#121316\"\n    ink: \"#ece8de\"\n    accent: \"#e06c60\"\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "FENCED_DESIGN.md"), []byte(fencedDesign), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, dir, "tokens", "export", "--from", "FENCED_DESIGN.md", "--to", "scss", "--out", "out-fenced")
	if code != cli.ExitSuccess {
		t.Fatalf("tokens export FENCED_DESIGN.md failed (%d): %s", code, errOut)
	}
	scssBytes, err := os.ReadFile(filepath.Join(dir, "out-fenced", "_tokens.scss"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scssBytes), "$df-night-accent: #e06c60;") {
		t.Fatalf("expected night accent in SCSS:\n%s", string(scssBytes))
	}
}

func TestTokensContrastValidationPassAndFailDiagnostics(t *testing.T) {
	dir := t.TempDir()

	badContract := `
color:
  light:
    paper: { $type: color, $value: "#fffff8" }
    ink: { $type: color, $value: "#111111" }
    muted: { $type: color, $value: "#b0b0b0" }
    rule_strong: { $type: color, $value: "#e5e5e5" }
  dark:
    paper: { $type: color, $value: "#121316" }
    ink: { $type: color, $value: "#2d3036" }
`
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(badContract), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "bad-out")
	code, _, errOut := runCLI(t, dir, "tokens", "export", "--from", "bad.yaml", "--to", "css", "--out", "bad-out")
	if code != cli.ExitError {
		t.Fatalf("expected ExitError (%d) for failing contrast, got %d (stderr: %s)", cli.ExitError, code, errOut)
	}
	for _, want := range []string{"color.light.muted", "color.light.rule_strong", "color.dark.ink", "4.5:1", "3.0:1"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr diagnostic missing %q:\n%s", want, errOut)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "tokens.css")); !os.IsNotExist(err) {
		t.Fatalf("failing contrast check must not write tokens.css")
	}

	// --check without --to / --out validates without writing files.
	code, _, errOut = runCLI(t, dir, "tokens", "export", "--from", "bad.yaml", "--check")
	if code != cli.ExitError {
		t.Fatalf("expected ExitError on --check failure, got %d", code)
	}

	// Default school tokens pass --check without writing files.
	code, out, errOut := runCLI(t, dir, "tokens", "export", "--check")
	if code != cli.ExitSuccess {
		t.Fatalf("expected ExitSuccess on default tufte --check, got %d (stderr: %s)", code, errOut)
	}
	if !strings.Contains(out, "passed") {
		t.Errorf("expected pass summary on stdout, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "tokens.css")); !os.IsNotExist(err) {
		t.Fatalf("--check alone should not write tokens.css")
	}
}

func TestTokensExportFromOmittedUsesDatafolioYAMLOrSchoolDefault(t *testing.T) {
	// Case 1: no datafolio.yaml -> defaults to active School (tufte, paper/night).
	bareDir := t.TempDir()
	code, _, errOut := runCLI(t, bareDir, "tokens", "export", "--school", "tufte", "--theme", "night", "--to", "css", "--out", "night-out")
	if code != cli.ExitSuccess {
		t.Fatalf("default school export failed (%d): %s", code, errOut)
	}
	nightCSS, err := os.ReadFile(filepath.Join(bareDir, "night-out", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nightCSS), "--df-paper: #121316;") || !strings.Contains(string(nightCSS), "--df-accent: #e06c60;") {
		t.Fatalf("expected night theme tufte defaults, got:\n%s", string(nightCSS))
	}

	// Case 2: datafolio.yaml specifies `tokens:` -> picked up when --from is omitted.
	projDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projDir, "datafolio.yaml"), []byte("tokens: brand.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	brandYAML := `
color:
  paper: { $type: color, $value: "#ffffff" }
  ink: { $type: color, $value: "#0f172a" }
  accent: { $type: color, $value: "#1E3A8A" }
`
	if err := os.WriteFile(filepath.Join(projDir, "brand.yaml"), []byte(brandYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, projDir, "tokens", "export", "--to", "css", "--out", "proj-out")
	if code != cli.ExitSuccess {
		t.Fatalf("export from datafolio.yaml failed (%d): %s", code, errOut)
	}
	projCSS, err := os.ReadFile(filepath.Join(projDir, "proj-out", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projCSS), "--df-accent: #1E3A8A;") {
		t.Fatalf("expected datafolio.yaml tokens override in CSS, got:\n%s", string(projCSS))
	}
}

func TestBuildTokensOverridePrecedenceDoesNotMutateContract(t *testing.T) {
	dir := t.TempDir()

	yamlTokens := `
color:
  paper: { $type: color, $value: "#ffffff" }
  ink: { $type: color, $value: "#0f172a" }
  accent: { $type: color, $value: "#1E3A8A" }
font:
  serif: { $type: fontFamily, $value: "Source Serif 4" }
`
	cliTokens := `
color:
  paper: { $type: color, $value: "#f8fafc" }
  accent: { $type: color, $value: "#166534" }
`
	fmTokens := `
color:
  accent: { $type: color, $value: "#7c2d12" }
`
	if err := os.WriteFile(filepath.Join(dir, "yaml-tokens.yaml"), []byte(yamlTokens), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fm-tokens.yaml"), []byte(fmTokens), 0o644); err != nil {
		t.Fatal(err)
	}
	cliTokPath := filepath.Join(dir, "cli-tokens.yaml")
	if err := os.WriteFile(cliTokPath, []byte(cliTokens), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "datafolio.yaml"), []byte("format: html\ntokens: yaml-tokens.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Doc 1: inherits datafolio.yaml tokens.
	if err := os.WriteFile(filepath.Join(dir, "doc1.md"), []byte("---\ntitle: Doc One\n---\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, dir, "build", "doc1.md")
	if code != cli.ExitSuccess {
		t.Fatalf("build doc1 failed (%d): %s", code, errOut)
	}
	html1, _ := os.ReadFile(filepath.Join(dir, "doc1.html"))
	if !strings.Contains(string(html1), "#1E3A8A") || !strings.Contains(string(html1), "Source Serif 4") {
		t.Fatalf("expected datafolio.yaml tokens in doc1.html, got:\n%s", string(html1))
	}

	// Doc 2: frontmatter overrides datafolio.yaml tokens.
	if err := os.WriteFile(filepath.Join(dir, "doc2.md"), []byte("---\ntitle: Doc Two\ntokens: fm-tokens.yaml\n---\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, dir, "build", "doc2.md")
	if code != cli.ExitSuccess {
		t.Fatalf("build doc2 failed (%d): %s", code, errOut)
	}
	html2, _ := os.ReadFile(filepath.Join(dir, "doc2.html"))
	if !strings.Contains(string(html2), "#7c2d12") {
		t.Fatalf("expected frontmatter token override #7c2d12 in doc2.html")
	}

	// Doc 2 with --tokens CLI flag: CLI overrides frontmatter and datafolio.yaml without mutating source file.
	code, _, errOut = runCLI(t, dir, "build", "doc2.md", "--tokens", "cli-tokens.yaml")
	if code != cli.ExitSuccess {
		t.Fatalf("build doc2 --tokens failed (%d): %s", code, errOut)
	}
	html3, _ := os.ReadFile(filepath.Join(dir, "doc2.html"))
	if !strings.Contains(string(html3), "#166534") || !strings.Contains(string(html3), "#f8fafc") {
		t.Fatalf("expected CLI --tokens override in doc2.html")
	}
	rawAfter, _ := os.ReadFile(cliTokPath)
	if string(rawAfter) != cliTokens {
		t.Fatalf("cli-tokens.yaml was mutated during build")
	}
}

func TestTokensUsageAndErrorExitCodes(t *testing.T) {
	dir := t.TempDir()

	code, out, _ := runCLI(t, dir, "tokens", "--help")
	if code != cli.ExitSuccess || !strings.Contains(out, "export") {
		t.Fatalf("tokens --help unexpected: code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, dir, "tokens", "export", "--help")
	if code != cli.ExitSuccess || !strings.Contains(out, "--from") {
		t.Fatalf("tokens export --help unexpected: code=%d out=%q", code, out)
	}

	for _, args := range [][]string{
		{"tokens"},
		{"tokens", "bogus"},
		{"tokens", "export", "--to", "less"},
		{"tokens", "export", "--school", "unknown"},
		{"tokens", "export", "--unknown-flag"},
		{"tokens", "check", "--out", "dist"},
		{"tokens", "check", "--to", "css"},
	} {
		code, _, _ := runCLI(t, dir, args...)
		if code != cli.ExitUsage {
			t.Fatalf("args %v: got exit %d, want %d", args, code, cli.ExitUsage)
		}
	}

	code, _, _ = runCLI(t, dir, "tokens", "export", "--from", "missing.yaml")
	if code != cli.ExitError {
		t.Fatalf("missing --from file: got exit %d, want %d", code, cli.ExitError)
	}
}

func TestBuildHTMLTokensRadiusAndShadowOverrides(t *testing.T) {
	dir := t.TempDir()

	tokensYAML := `
color:
  paper: { $type: color, $value: "#ffffff" }
  ink: { $type: color, $value: "#0f172a" }
  accent: { $type: color, $value: "#1e3a8a" }
radius:
  default: { $type: dimension, $value: "8px" }
shadow:
  default: { $type: shadow, $value: "0 4px 12px rgba(15, 23, 42, 0.12)" }
`
	if err := os.WriteFile(filepath.Join(dir, "tokens.yaml"), []byte(tokensYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chart.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}

	docMD := `---
title: Token Radius & Shadow Monograph
---

Intro paragraph[^n1].

[^n1]: Sidenote body.

:::note
Margin note body.
:::

:::kpi
- Metric | 42 | Baseline
:::

> Blockquote surface.

` + "```go\nvar x = 1\n```" + `

![Chart caption](chart.svg)

*Source: Test fixture*
`
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte(docMD), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "build", "doc.md", "--to", "html", "--tokens", "tokens.yaml")
	if code != cli.ExitSuccess {
		t.Fatalf("build --to html --tokens failed (%d): %s", code, errOut)
	}

	htmlBytes, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)

	// Extract universal reset block to ensure border-radius: 0px / box-shadow: none do not clobber elements.
	resetStart := strings.Index(html, "*, *::before, *::after")
	if resetStart < 0 {
		t.Fatalf("expected universal box-sizing reset in HTML stylesheet")
	}
	resetEnd := strings.Index(html[resetStart:], "}")
	if resetEnd < 0 {
		t.Fatalf("unterminated universal reset rule in HTML stylesheet")
	}
	resetBlock := html[resetStart : resetStart+resetEnd+1]
	if strings.Contains(resetBlock, "border-radius") || strings.Contains(resetBlock, "box-shadow") {
		t.Fatalf("universal reset must not hardcode border-radius or box-shadow, got:\n%s", resetBlock)
	}
	if !strings.Contains(resetBlock, "box-sizing: border-box;") {
		t.Fatalf("universal reset missing box-sizing: border-box, got:\n%s", resetBlock)
	}

	for _, want := range []string{
		"--df-radius: 8px;",
		"--df-shadow: 0 4px 12px rgba(15, 23, 42, 0.12);",
		"border-radius: var(--df-radius);",
		"box-shadow: var(--df-shadow);",
		".df-margin-note",
		".df-sidenote",
		".df-kpi-strip",
		".df-blockquote",
		".df-code",
		".df-figure img",
		".df-figure svg",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected fragment %q", want)
		}
	}
}
