package tokens_test

import (
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/tokens"
)

func TestParseStandaloneDTCGYAMLAndJSONWithAliases(t *testing.T) {
	yamlInput := `
color:
  $type: color
  brand:
    carmine:
      $value: "#8b1e1e"
  paper:
    $value: "#fffff8"
  ink:
    $value: "#111111"
  accent:
    $value: "{color.brand.carmine}"
font:
  serif:
    $type: fontFamily
    $value: "EB Garamond"
radius:
  default:
    type: dimension
    value: "0px"
`
	c, err := tokens.Parse([]byte(yamlInput), "tokens.yaml")
	if err != nil {
		t.Fatalf("Parse YAML failed: %v", err)
	}

	accent, ok := c.Get("color.accent")
	if !ok {
		t.Fatalf("expected token color.accent to exist")
	}
	if accent.Value != "#8b1e1e" {
		t.Fatalf("color.accent = %q, want %q", accent.Value, "#8b1e1e")
	}
	if accent.Type != "color" {
		t.Fatalf("color.accent type = %q, want %q", accent.Type, "color")
	}

	rad, ok := c.Get("radius.default")
	if !ok || rad.Value != "0px" || rad.Type != "dimension" {
		t.Fatalf("unexpected radius.default token: %+v (ok=%v)", rad, ok)
	}

	jsonInput := `{
  "$schema": "https://design-tokens.github.io/community-group/format/",
  "color": {
    "base": {
      "navy": { "$type": "color", "$value": "#1E3A8A" }
    },
    "paper": { "type": "color", "value": "#FFFFFF" },
    "ink": { "type": "color", "value": "#0F172A" },
    "accent": { "$value": "{color.base.navy}" }
  },
  "font": {
    "serif": {
      "$type": "fontFamily",
      "$value": ["Source Serif 4", "Georgia", "serif"]
    }
  }
}`
	jc, err := tokens.Parse([]byte(jsonInput), "tokens.json")
	if err != nil {
		t.Fatalf("Parse JSON failed: %v", err)
	}
	jAccent, ok := jc.Get("color.accent")
	if !ok || jAccent.Value != "#1E3A8A" || jAccent.Type != "color" {
		t.Fatalf("unexpected JSON color.accent: %+v (ok=%v)", jAccent, ok)
	}
	jSerif, ok := jc.Get("font.serif")
	if !ok || !strings.Contains(jSerif.Value, "Source Serif 4") {
		t.Fatalf("unexpected JSON font.serif: %+v (ok=%v)", jSerif, ok)
	}
}

func TestParseRejectsAliasCyclesAndUnresolvedRefs(t *testing.T) {
	cycleYAML := `
color:
  a:
    $type: color
    $value: "{color.b}"
  b:
    $type: color
    $value: "{color.a}"
`
	_, err := tokens.Parse([]byte(cycleYAML), "cycle.yaml")
	if err == nil {
		t.Fatalf("expected error on reference cycle, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cycle") {
		t.Fatalf("expected cycle error diagnostic, got: %v", err)
	}

	missingYAML := `
color:
  accent:
    $type: color
    $value: "{color.nonexistent}"
`
	_, err = tokens.Parse([]byte(missingYAML), "missing.yaml")
	if err == nil {
		t.Fatalf("expected error on unresolved alias reference, got nil")
	}
	if !strings.Contains(err.Error(), "color.nonexistent") {
		t.Fatalf("expected missing reference name in error, got: %v", err)
	}
}

func TestParseDesignMDFrontmatterAndFencedBlocks(t *testing.T) {
	frontmatterMD := `---
design_tokens:
  version: "1.1.0"
  format: "DTCG"
  name: "Portfolio Design Tokens"
  color:
    brand:
      primary:
        value: "#1E3A8A"
        type: "color"
      accent:
        value: "{color.brand.primary}"
        type: "color"
    light:
      surface_canvas:
        value: "#FFFFFF"
        type: "color"
      text_primary:
        value: "#0F172A"
        type: "color"
    dark:
      surface_canvas:
        value: "#0B0F17"
        type: "color"
      text_primary:
        value: "#F8FAFC"
        type: "color"
    status:
      sanitized_dark_bg:
        value: "rgba(30, 58, 138, 0.25)"
        type: "color"
  typography:
    family:
      heading:
        value: "'Source Serif 4', Georgia, serif"
        type: "fontFamily"
  contrast_matrix:
    light:
      body: { ratio: "16.5:1", wcag: "AAA", fg: "#0F172A", bg: "#FFFFFF" }
---

# Visual Contract
`
	c, err := tokens.Parse([]byte(frontmatterMD), "DESIGN.md")
	if err != nil {
		t.Fatalf("Parse frontmatter DESIGN.md failed: %v", err)
	}
	if c.Name != "Portfolio Design Tokens" {
		t.Fatalf("Name = %q, want %q", c.Name, "Portfolio Design Tokens")
	}
	if tok, ok := c.Get("color.brand.accent"); !ok || tok.Value != "#1E3A8A" {
		t.Fatalf("expected resolved color.brand.accent=#1E3A8A, got %+v (ok=%v)", tok, ok)
	}
	if tok, ok := c.Get("color.status.sanitized_dark_bg"); !ok || tok.Value != "rgba(30, 58, 138, 0.25)" {
		t.Fatalf("unexpected sanitized_dark_bg: %+v", tok)
	}
	if _, ok := c.Get("contrast_matrix.light.body.fg"); ok {
		t.Fatalf("contrast_matrix metadata should not be emitted as design token")
	}

	multiBlockMD := "# Design System\n\n" +
		"```yaml\n" +
		"colors:\n" +
		"  paper_theme:\n" +
		"    paper:\n" +
		"      value: \"#fffff8\"\n" +
		"      cssVar: \"--tufte-paper\"\n" +
		"      role: \"Primary page canvas\"\n" +
		"    ink:\n" +
		"      value: \"#111111\"\n" +
		"    accent:\n" +
		"      value: \"#8b1e1e\"\n" +
		"  night_theme:\n" +
		"    paper: \"#121316\"\n" +
		"    ink: \"#ece8de\"\n" +
		"    accent: \"#e06c60\"\n" +
		"```\n\n" +
		"```yaml\n" +
		"typography:\n" +
		"  fontFamily:\n" +
		"    serif: \"'ET Book', Georgia, serif\"\n" +
		"    mono: \"'JetBrains Mono', monospace\"\n" +
		"layout:\n" +
		"  maxWidth: \"1320px\"\n" +
		"  asymmetricGrid:\n" +
		"    columnGap: \"5%\"\n" +
		"shape:\n" +
		"  borderRadius:\n" +
		"    default: \"0px\"\n" +
		"    note: \"Sharp rectangular edges\"\n" +
		"elevation:\n" +
		"  surfaces: \"Flat print hierarchy\"\n" +
		"  popoversOnly:\n" +
		"    boxShadow: \"0 14px 34px rgba(17, 17, 17, 0.16)\"\n" +
		"```\n"

	mc, err := tokens.Parse([]byte(multiBlockMD), "DESIGN.md")
	if err != nil {
		t.Fatalf("Parse multi-block DESIGN.md failed: %v", err)
	}
	for path, want := range map[string]string{
		"colors.paper_theme.paper":         "#fffff8",
		"colors.night_theme.paper":         "#121316",
		"colors.night_theme.accent":        "#e06c60",
		"typography.fontFamily.serif":      "'ET Book', Georgia, serif",
		"layout.asymmetricGrid.columnGap":  "5%",
		"shape.borderRadius.default":       "0px",
		"elevation.popoversOnly.boxShadow": "0 14px 34px rgba(17, 17, 17, 0.16)",
	} {
		tok, ok := mc.Get(path)
		if !ok || tok.Value != want {
			t.Errorf("token %q = %q (ok=%v), want %q", path, tok.Value, ok, want)
		}
	}
	if _, ok := mc.Get("shape.borderRadius.note"); ok {
		t.Errorf("metadata key shape.borderRadius.note should not become a token")
	}

	fencedDTCG := "# Tokens\n\n```yaml\ndesign_tokens:\n  color:\n    paper: { $value: \"#fffff8\", $type: \"color\" }\n    ink: { $value: \"#111111\", $type: \"color\" }\n```\n"
	fc, err := tokens.Parse([]byte(fencedDTCG), "DESIGN.md")
	if err != nil {
		t.Fatalf("Parse fenced design_tokens failed: %v", err)
	}
	if tok, ok := fc.Get("color.paper"); !ok || tok.Value != "#fffff8" {
		t.Fatalf("expected color.paper=#fffff8, got %+v", tok)
	}

	if _, err := tokens.Parse([]byte("# Plain Markdown\n\nNo tokens here.\n"), "DESIGN.md"); err == nil {
		t.Fatalf("expected error when DESIGN.md contains no token blocks")
	}
}
