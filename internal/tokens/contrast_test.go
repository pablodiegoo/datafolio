package tokens_test

import (
	"math"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/tokens"
)

func TestContrastRatioKnownWCAGValuesAndAlphaCompositing(t *testing.T) {
	cases := []struct {
		name string
		fg   string
		bg   string
		want float64
		tol  float64
	}{
		{name: "black on white", fg: "#000000", bg: "#ffffff", want: 21.0, tol: 0.01},
		{name: "3-digit hex", fg: "#111", bg: "#fff", want: 18.88, tol: 0.1},
		{name: "tufte ink on paper", fg: "#111111", bg: "#fffff8", want: 18.88, tol: 0.1},
		{name: "tufte muted on paper", fg: "#6b6559", bg: "#fffff8", want: 5.79, tol: 0.1},
		{name: "tufte night accent on night paper", fg: "#e06c60", bg: "#121316", want: 5.74, tol: 0.1},
		{name: "rgb function", fg: "rgb(17, 17, 17)", bg: "rgb(255, 255, 248)", want: 18.88, tol: 0.1},
		{name: "rgba 50% black over white", fg: "rgba(0, 0, 0, 0.5)", bg: "#ffffff", want: 3.95, tol: 0.1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tokens.ContrastRatio(tc.fg, tc.bg)
			if err != nil {
				t.Fatalf("ContrastRatio(%q, %q) error: %v", tc.fg, tc.bg, err)
			}
			if math.Abs(got-tc.want) > tc.tol {
				t.Fatalf("ContrastRatio(%q, %q) = %.3f, want %.3f (±%.2f)", tc.fg, tc.bg, got, tc.want, tc.tol)
			}
		})
	}
}

func TestCheckContrastLightDarkNormalAndLargePairs(t *testing.T) {
	passingYAML := `
color:
  light:
    paper: { $type: color, $value: "#fffff8" }
    paper_subtle: { $type: color, $value: "#fbf9f2" }
    ink: { $type: color, $value: "#111111" }
    muted: { $type: color, $value: "#6b6559" }
    accent: { $type: color, $value: "#8b1e1e" }
    rule: { $type: color, $value: "#ded8ca" }
    rule_strong: { $type: color, $value: "#3d3932" }
    large_text: { $type: color, $value: "#888888" }
  dark:
    paper: { $type: color, $value: "#121316" }
    ink: { $type: color, $value: "#ece8de" }
    muted: { $type: color, $value: "#989284" }
    accent: { $type: color, $value: "#e06c60" }
    rule_strong: { $type: color, $value: "#78808f" }
  status:
    sanitized_dark_bg: { $type: color, $value: "rgba(30, 58, 138, 0.25)" }
    sanitized_dark_text: { $type: color, $value: "#93C5FD" }
`
	c, err := tokens.Parse([]byte(passingYAML), "pass.yaml")
	if err != nil {
		t.Fatal(err)
	}
	results, err := tokens.CheckContrast(c, 4.5, 3.0)
	if err != nil {
		t.Fatalf("CheckContrast error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected non-empty contrast results")
	}
	for _, r := range results {
		if !r.Pass {
			t.Errorf("expected passing contrast, got failure: %s", r.Format())
		}
	}

	failingYAML := `
color:
  light:
    paper: { $type: color, $value: "#fffff8" }
    ink: { $type: color, $value: "#111111" }
    muted: { $type: color, $value: "#aaaaaa" }
    rule_strong: { $type: color, $value: "#dedede" }
  dark:
    paper: { $type: color, $value: "#121316" }
    ink: { $type: color, $value: "#2a2b2e" }
`
	fc, err := tokens.Parse([]byte(failingYAML), "fail.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fResults, err := tokens.CheckContrast(fc, 4.5, 3.0)
	if err != nil {
		t.Fatalf("CheckContrast failed: %v", err)
	}

	var failedTokens []string
	for _, r := range fResults {
		if !r.Pass {
			failedTokens = append(failedTokens, r.ForegroundToken+"("+r.Level+")")
		}
	}
	joined := strings.Join(failedTokens, ", ")
	for _, want := range []string{
		"color.light.muted(normal)",
		"color.light.rule_strong(large)",
		"color.dark.ink(normal)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected failing pair %q in [%s]", want, joined)
		}
	}
}
