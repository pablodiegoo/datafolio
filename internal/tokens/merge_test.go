package tokens_test

import (
	"testing"

	"github.com/pablodiegoo/datafolio/internal/school/tufte"
	"github.com/pablodiegoo/datafolio/internal/tokens"
)

func TestMergeOverlaysWithoutMutatingSchoolDefaults(t *testing.T) {
	school := tufte.New()
	paperBase, err := school.Tokens("paper")
	if err != nil {
		t.Fatal(err)
	}
	nightBase, err := school.Tokens("night")
	if err != nil {
		t.Fatal(err)
	}

	contractYAML := `
color:
  brand:
    primary: { $type: color, $value: "#1E3A8A" }
    primary_dark: { $type: color, $value: "#60A5FA" }
    accent: { $type: color, $value: "#2563EB" }
  light:
    surface_canvas: { $type: color, $value: "#FFFFFF" }
    text_primary: { $type: color, $value: "#0F172A" }
    text_muted: { $type: color, $value: "#475569" }
    border_subtle: { $type: color, $value: "#E2E8F0" }
    border_strong: { $type: color, $value: "#CBD5E1" }
  dark:
    surface_canvas: { $type: color, $value: "#0B0F17" }
    text_primary: { $type: color, $value: "#F8FAFC" }
    text_muted: { $type: color, $value: "#94A3B8" }
typography:
  family:
    heading: { $type: fontFamily, $value: "'Source Serif 4', Georgia, serif" }
    mono: { $type: fontFamily, $value: "'Fira Code', monospace" }
radius:
  default: { $type: dimension, $value: "4px" }
`
	c, err := tokens.Parse([]byte(contractYAML), "tokens.yaml")
	if err != nil {
		t.Fatal(err)
	}

	mergedPaper := tokens.Merge(paperBase, c)
	if mergedPaper.Paper != "#FFFFFF" || mergedPaper.Ink != "#0F172A" || mergedPaper.Muted != "#475569" {
		t.Fatalf("unexpected paper merge colors: %+v", mergedPaper)
	}
	if mergedPaper.Accent != "#2563EB" {
		t.Fatalf("mergedPaper.Accent = %q, want #2563EB", mergedPaper.Accent)
	}
	if mergedPaper.SerifFont != "Source Serif 4" || mergedPaper.MonoFont != "Fira Code" {
		t.Fatalf("expected primary font names extracted, got serif=%q mono=%q", mergedPaper.SerifFont, mergedPaper.MonoFont)
	}
	if mergedPaper.BorderRadius != "4px" {
		t.Fatalf("mergedPaper.BorderRadius = %q, want 4px", mergedPaper.BorderRadius)
	}
	if mergedPaper.SansFont != "Source Sans 3" {
		t.Fatalf("unset token SansFont should preserve tufte default, got %q", mergedPaper.SansFont)
	}

	mergedNight := tokens.Merge(nightBase, c)
	if mergedNight.Paper != "#0B0F17" || mergedNight.Ink != "#F8FAFC" || mergedNight.Accent != "#60A5FA" {
		t.Fatalf("unexpected night merge colors: %+v", mergedNight)
	}

	// Base tufte defaults must remain untouched.
	freshPaper, _ := school.Tokens("paper")
	if freshPaper.Paper != "#fffff8" || freshPaper.Accent != "#8b1e1e" {
		t.Fatalf("tufte defaults were mutated: %+v", freshPaper)
	}

	// Shorthand CSS border strings for rules extract the leading width dimension cleanly.
	shorthandYAML := `
shape:
  rules:
    booktabsTop: "2px solid #3d3932"
    booktabsMid: "1px solid #3d3932"
    hairline: "0.75px solid #ded8ca"
`
	sc, err := tokens.Parse([]byte(shorthandYAML), "rules.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mergedRules := tokens.Merge(paperBase, sc)
	if mergedRules.BooktabsTopRule != "2px" || mergedRules.BooktabsMidRule != "1px" || mergedRules.BooktabsRowRule != "0.75px" {
		t.Fatalf("unexpected booktabs rule widths: top=%q mid=%q row=%q", mergedRules.BooktabsTopRule, mergedRules.BooktabsMidRule, mergedRules.BooktabsRowRule)
	}
}
