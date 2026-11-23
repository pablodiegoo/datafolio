package tokens_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/school/tufte"
	"github.com/pablodiegoo/datafolio/internal/tokens"
)

func TestExportAllFiveTargetsSynchronizedAndGolden(t *testing.T) {
	inputYAML := `
name: "Archival Monograph Contract"
color:
  light:
    paper: { $type: color, $value: "#fffff8" }
    paper_subtle: { $type: color, $value: "#fbf9f2" }
    paper_alt: { $type: color, $value: "#f6f3ea" }
    ink: { $type: color, $value: "#111111" }
    ink_soft: { $type: color, $value: "#2b2926" }
    muted: { $type: color, $value: "#6b6559" }
    accent: { $type: color, $value: "#8b1e1e" }
    ochre: { $type: color, $value: "#854d0e" }
    forest: { $type: color, $value: "#166534" }
    rule: { $type: color, $value: "#ded8ca" }
    rule_strong: { $type: color, $value: "#3d3932" }
  dark:
    paper: { $type: color, $value: "#121316" }
    paper_subtle: { $type: color, $value: "#16181d" }
    paper_alt: { $type: color, $value: "#1a1c21" }
    ink: { $type: color, $value: "#ece8de" }
    ink_soft: { $type: color, $value: "#d2cdc0" }
    muted: { $type: color, $value: "#989284" }
    accent: { $type: color, $value: "#e06c60" }
    ochre: { $type: color, $value: "#d9a04e" }
    forest: { $type: color, $value: "#65c494" }
    rule: { $type: color, $value: "#2c3038" }
    rule_strong: { $type: color, $value: "#78808f" }
font:
  serif: { $type: fontFamily, $value: "EB Garamond" }
  mono: { $type: fontFamily, $value: "JetBrains Mono" }
  sans: { $type: fontFamily, $value: "Source Sans 3" }
radius:
  default: { $type: dimension, $value: "0px" }
`
	c, err := tokens.Parse([]byte(inputYAML), "tokens.yaml")
	if err != nil {
		t.Fatal(err)
	}
	base, err := tufte.New().Tokens("paper")
	if err != nil {
		t.Fatal(err)
	}

	artifacts, err := tokens.Export(c, tokens.ExportOptions{
		Base:    base,
		Targets: []string{"css", "tailwind", "scss", "typst", "matplotlib"},
	})
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	byName := make(map[string]string, len(artifacts))
	for _, a := range artifacts {
		byName[a.Filename] = string(a.Content)
	}

	wantFiles := []string{
		"tokens.css",
		"tokens.tailwind.css",
		"_tokens.scss",
		"tokens.typ",
		"datafolio.mplstyle",
		"tokens.json",
	}
	for _, wf := range wantFiles {
		if _, ok := byName[wf]; !ok {
			t.Fatalf("missing exported artifact %q", wf)
		}
	}

	// Verify semantic synchronization and format validity across all 5 targets.
	if !strings.Contains(byName["tokens.css"], "--df-paper: #fffff8;") ||
		!strings.Contains(byName["tokens.css"], `[data-theme="night"]`) ||
		!strings.Contains(byName["tokens.css"], "--df-accent: #e06c60;") {
		t.Errorf("tokens.css missing expected properties:\n%s", byName["tokens.css"])
	}
	if !strings.Contains(byName["tokens.tailwind.css"], "@theme {") ||
		!strings.Contains(byName["tokens.tailwind.css"], "--color-accent: #8b1e1e;") ||
		!strings.Contains(byName["tokens.tailwind.css"], "--font-serif:") {
		t.Errorf("tokens.tailwind.css missing expected Tailwind v4 @theme tokens:\n%s", byName["tokens.tailwind.css"])
	}
	if !strings.Contains(byName["_tokens.scss"], "$df-paper: #fffff8;") ||
		!strings.Contains(byName["_tokens.scss"], "$df-night-accent: #e06c60;") {
		t.Errorf("_tokens.scss missing expected SCSS variables:\n%s", byName["_tokens.scss"])
	}
	if !strings.Contains(byName["tokens.typ"], `#let df-paper = rgb("#fffff8")`) ||
		!strings.Contains(byName["tokens.typ"], `#let df-radius = 0pt`) ||
		!strings.Contains(byName["tokens.typ"], `#let df-night-accent = rgb("#e06c60")`) {
		t.Errorf("tokens.typ missing expected Typst variables:\n%s", byName["tokens.typ"])
	}
	if !strings.Contains(byName["datafolio.mplstyle"], "figure.facecolor: fffff8") ||
		!strings.Contains(byName["datafolio.mplstyle"], "cycler('color', ['8b1e1e'") {
		t.Errorf("datafolio.mplstyle missing expected matplotlib rcParams:\n%s", byName["datafolio.mplstyle"])
	}

	var jsonDoc map[string]any
	if err := json.Unmarshal([]byte(byName["tokens.json"]), &jsonDoc); err != nil {
		t.Fatalf("tokens.json is invalid JSON: %v\n%s", err, byName["tokens.json"])
	}
	colorsMap, ok := jsonDoc["colors"].(map[string]any)
	if !ok || colorsMap["accent"] != "#8b1e1e" || colorsMap["paper"] != "#fffff8" {
		t.Fatalf("unexpected tokens.json colors: %#v", jsonDoc["colors"])
	}

	// Verify golden snapshots when present.
	goldenDir := filepath.Join("testdata", "golden")
	for _, wf := range wantFiles {
		gPath := filepath.Join(goldenDir, wf)
		wantBytes, err := os.ReadFile(gPath)
		if err != nil {
			t.Fatalf("missing golden file %s: %v", gPath, err)
		}
		if byName[wf] != string(wantBytes) {
			t.Errorf("golden mismatch for %s:\ngot:\n%s\nwant:\n%s", wf, byName[wf], string(wantBytes))
		}
	}
}
