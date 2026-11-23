package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/review"
)

func runWebReview(t *testing.T, workDir string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := cli.Options{
		Stdout:   &stdout,
		Stderr:   &stderr,
		WorkDir:  workDir,
		HomeDir:  workDir,
		CacheDir: filepath.Join(workDir, ".cache"),
		Version:  "0.1.0-test",
		GOOS:     "linux",
		GOARCH:   "amd64",
	}
	code := cli.Run(context.Background(), args, opts)
	return code, stdout.String(), stderr.String()
}

func decodeWebReport(t *testing.T, raw string) review.Report {
	t.Helper()
	var rep review.Report
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("decode JSON review report: %v\noutput:\n%s", err, raw)
	}
	return rep
}

func TestReviewWebCSSAndSCSS(t *testing.T) {
	t.Run("passing tufte tokens, var references, zero radius, none shadow, and ignored comments exit zero", func(t *testing.T) {
		dir := t.TempDir()
		cssSrc := `/* Commented out violation: color: #ff00aa; border-radius: 12px; 🚀 */
:root {
  --df-paper: #fffff8;
  --df-ink: #111111;
  --df-accent: #8b1e1e;
}

[data-theme="night"] {
  --df-paper: #121316;
  --df-accent: #e06c60;
}

.card {
  background-color: var(--df-paper);
  color: var(--df-ink);
  border-bottom: 1px solid #ded8ca;
  border-radius: 0px;
  box-shadow: none;
}

#header:hover {
  color: #8b1e1e;
}
`
		scssSrc := `// Line comment with #ff00aa and box-shadow: 0 4px 8px #000000; 🎉
$brand-accent: #8b1e1e;
$paper-bg: #fffff8;

@mixin rule-band {
  border-top: 1px solid #3d3932;
  background: url(https://example.com/assets/bg.svg);
}

.panel {
  @include rule-band;
  color: $brand-accent;
  background-color: $paper-bg;
  border-radius: 0;
  box-shadow: none;
}
`
		if err := os.WriteFile(filepath.Join(dir, "site.css"), []byte(cssSrc), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "theme.scss"), []byte(scssSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runWebReview(t, dir, "review", "site.css", "theme.scss", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeWebReport(t, out)
		if rep.Files != 2 || rep.ErrorCount != 0 || rep.WarnCount != 0 || len(rep.Findings) != 0 {
			t.Fatalf("expected 2 clean files with 0 findings, got %+v", rep)
		}
	})

	t.Run("hardcoded colors, off-contract radius and shadow, and disallowed emojis emit exact lines and promote under --strict", func(t *testing.T) {
		dir := t.TempDir()
		cssSrc := `/* Ignored comment #ff00aa */
.hero {
  color: #ff00aa;
  background: rgb(59, 130, 246);
  border-color: hsl(300, 100%, 50%);
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}
`
		scssSrc := `$bad-color: #3b82f6;
.badge::before {
  content: "🚀";
  border-top-left-radius: 4px;
}
`
		if err := os.WriteFile(filepath.Join(dir, "bad.css"), []byte(cssSrc), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bad.scss"), []byte(scssSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		// Warning-only CSS file should exit 0 by default and exit 1 under --strict.
		warnCode, warnOut, warnErr := runWebReview(t, dir, "review", "bad.css", "--format", "json")
		if warnCode != cli.ExitSuccess {
			t.Fatalf("non-strict warnings: got exit %d, want %d (stderr=%s, stdout=%s)", warnCode, cli.ExitSuccess, warnErr, warnOut)
		}
		warnRep := decodeWebReport(t, warnOut)
		if warnRep.ErrorCount != 0 || warnRep.WarnCount != 5 || len(warnRep.Findings) != 5 {
			t.Fatalf("bad.css findings mismatch: %+v", warnRep)
		}

		wantCSS := []struct {
			line   int
			ruleID string
		}{
			{line: 3, ruleID: "web-hardcoded-color"},
			{line: 4, ruleID: "web-hardcoded-color"},
			{line: 5, ruleID: "web-hardcoded-color"},
			{line: 6, ruleID: "web-off-contract-radius"},
			{line: 7, ruleID: "web-off-contract-shadow"},
		}
		for i, w := range wantCSS {
			got := warnRep.Findings[i]
			if got.File != "bad.css" || got.Line != w.line || got.RuleID != w.ruleID || got.Severity != review.SeverityWarn || strings.TrimSpace(got.Hint) == "" {
				t.Errorf("bad.css finding[%d] = %+v, want line %d rule %s warn", i, got, w.line, w.ruleID)
			}
		}

		strictCode, _, _ := runWebReview(t, dir, "review", "--strict", "bad.css")
		if strictCode != cli.ExitError {
			t.Fatalf("--strict on warnings: got exit %d, want %d", strictCode, cli.ExitError)
		}

		// SCSS file with emoji error exits 1 even without --strict.
		scssCode, scssOut, scssErr := runWebReview(t, dir, "review", "bad.scss", "--format", "json")
		if scssCode != cli.ExitError {
			t.Fatalf("bad.scss: got exit %d, want %d (stderr=%s, stdout=%s)", scssCode, cli.ExitError, scssErr, scssOut)
		}
		scssRep := decodeWebReport(t, scssOut)
		if scssRep.ErrorCount != 1 || scssRep.WarnCount != 2 || len(scssRep.Findings) != 3 {
			t.Fatalf("bad.scss findings mismatch: %+v", scssRep)
		}
		wantSCSS := []struct {
			line     int
			ruleID   string
			severity review.Severity
		}{
			{line: 1, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
			{line: 3, ruleID: "emoji-disallowed", severity: review.SeverityError},
			{line: 4, ruleID: "web-off-contract-radius", severity: review.SeverityWarn},
		}
		for i, w := range wantSCSS {
			got := scssRep.Findings[i]
			if got.File != "bad.scss" || got.Line != w.line || got.RuleID != w.ruleID || got.Severity != w.severity {
				t.Errorf("bad.scss finding[%d] = %+v, want %+v", i, got, w)
			}
		}
	})
}

func TestReviewWebJekyllLiquidHTML(t *testing.T) {
	t.Run("strips Jekyll frontmatter, Liquid tags/comments/interpolations, HTML comments, and scripts with 1:1 line numbers", func(t *testing.T) {
		dir := t.TempDir()
		htmlSrc := `---
layout: default
title: Quarterly Benchmark 🚀
---
<!DOCTYPE html>
<html lang="en">
<head>
  <style>
    /* Multi-line CSS comment
       #ff00aa and border-radius: 12px should be ignored */
    @media (prefers-color-scheme: dark) {
      body { background: #121316; color: #ece8de; }
    }
    .callout {
      background: #ff00aa;
      border-radius: 10px;
      box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
    }
  </style>
  <script>
    const ignoredHex = "#ff00aa";
    const ignoredEmoji = "🚀";
  </script>
</head>
<body>
  <!-- HTML comment with #ff00aa and 🚀
       and border-radius: 12px -->
  {% comment %}
    Liquid comment with 🚀 and #ff00aa
  {% endcomment %}
  {# Twig comment with 🚀 #}
  <header>
    <h1>{{ page.title | default: "Fallback" }} 🚀</h1>
  </header>
  {% for item in site.posts %}
    <article class="p-4 bg-[#3b82f6] hover:text-[#8b1e1e]" style="color: {{ item.color }}; border-radius: 6px;">
      <a href="#facade">Jump to section &#123;</a>
      <svg viewBox="0 0 10 10"><path fill="#ff00aa" clip-path="url(#deadbeef)" /></svg>
    </article>
  {% endfor %}
</body>
</html>
`
		if err := os.WriteFile(filepath.Join(dir, "report.html"), []byte(htmlSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runWebReview(t, dir, "review", "report.html", "--format", "json")
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeWebReport(t, out)

		want := []struct {
			line     int
			ruleID   string
			severity review.Severity
		}{
			{line: 15, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
			{line: 16, ruleID: "web-off-contract-radius", severity: review.SeverityWarn},
			{line: 17, ruleID: "web-off-contract-shadow", severity: review.SeverityWarn},
			{line: 33, ruleID: "emoji-disallowed", severity: review.SeverityError},
			{line: 36, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
			{line: 36, ruleID: "web-off-contract-radius", severity: review.SeverityWarn},
			{line: 38, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
		}
		if len(rep.Findings) != len(want) {
			t.Fatalf("got %d findings, want %d:\n%+v", len(rep.Findings), len(want), rep.Findings)
		}
		for i, w := range want {
			got := rep.Findings[i]
			if got.File != "report.html" || got.Line != w.line || got.RuleID != w.ruleID || got.Severity != w.severity || strings.TrimSpace(got.Hint) == "" {
				t.Errorf("finding[%d] = %+v, want %+v", i, got, w)
			}
		}
	})
}

func TestReviewWebLaravelBlade(t *testing.T) {
	t.Run("strips Blade directives, comments, echoes, and PHP blocks while preserving @media, emails, Alpine attrs, and exact lines", func(t *testing.T) {
		dir := t.TempDir()
		bladeSrc := `{{-- Multi-line Blade comment:
     ignore #ff00aa, border-radius: 12px, and 🚀 --}}
@extends('layouts.editorial')
@props(['title' => 'Quarterly Note', 'items' => []])

@php
  $ignoredColor = '#ff00aa';
  $ignoredEmoji = '🚀';
  $closingInString = "?>";
@endphp

<?php
  // PHP tag with #ff00aa and 🚀
  $x = "?> still inside PHP";
?>

@section('content')
  <style>
    @media (min-width: 768px) {
      .sheet {
        background-color: #fffff8;
        color: #111111;
      }
      .offending {
        color: #ff00aa;
        border-radius: 10px;
      }
    }
  </style>

  <header>
    <h1>{{ $title }} ⚠️</h1>
    <p>Contact <a href="mailto:editor@example.com">editor@example.com</a></p>
  </header>

  @if (count($items) > 0)
    @foreach ($items as $item)
      <div class="p-4 @if($item->active) bg-[#3b82f6] @endif" @click="toggle({{ $item->id }})">
        {!! $item->summary !!}
        <span style="box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);"><?= $item->score ?></span>
      </div>
    @endforeach
  @elseif ($fallback)
    @include('partials.empty', ['label' => 'None'])
  @else
    <p>No records.</p>
  @endif
@endsection
`
		if err := os.WriteFile(filepath.Join(dir, "dashboard.blade.php"), []byte(bladeSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runWebReview(t, dir, "review", "dashboard.blade.php", "--format", "json")
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeWebReport(t, out)

		want := []struct {
			line     int
			ruleID   string
			severity review.Severity
		}{
			{line: 25, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
			{line: 26, ruleID: "web-off-contract-radius", severity: review.SeverityWarn},
			{line: 32, ruleID: "emoji-disallowed", severity: review.SeverityError},
			{line: 38, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
			{line: 40, ruleID: "web-off-contract-shadow", severity: review.SeverityWarn},
		}
		if len(rep.Findings) != len(want) {
			t.Fatalf("got %d findings, want %d:\n%+v", len(rep.Findings), len(want), rep.Findings)
		}
		for i, w := range want {
			got := rep.Findings[i]
			if got.File != "dashboard.blade.php" || got.Line != w.line || got.RuleID != w.ruleID || got.Severity != w.severity || strings.TrimSpace(got.Hint) == "" {
				t.Errorf("finding[%d] = %+v, want %+v", i, got, w)
			}
		}
	})
}

func TestReviewWebGoTempl(t *testing.T) {
	t.Run("strips Go preamble, comments, control blocks, and { expr } interpolations while auditing css/style/markup with exact lines", func(t *testing.T) {
		dir := t.TempDir()
		templSrc := `package components

import (
	"fmt"
	"strings"
)

/* Go block comment with #ff00aa and 🚀 */
// Go line comment with border-radius: 12px and 🚀

type MetricCardProps struct {
	Title  string
	Value  string
	Active bool
}

func helperColor() string {
	return "#ff00aa" // Go helper must not trigger
}

css metricBox() {
	background-color: #fffff8;
	border-color: #ff00aa;
	border-radius: 8px;
}

script trackHover() {
	const ignored = "#ff00aa 🚀";
}

templ MetricCard(p MetricCardProps, items []string) {
	<style>
		.inline-card { box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2); }
	</style>
	<article class="p-4 bg-[#3b82f6]" style="color: #111111;">
		<!-- HTML comment with 🚀 and #ff00aa -->
		<h2>{ strings.TrimSpace(p.Title) } 📈</h2>
		if p.Active {
			<p>
				if the series rises, hold steady.
			</p>
		} else if len(items) == 0 {
			<p>
				for complete coverage see appendix.
			</p>
		} else {
			<span>{ p.Value }</span>
		}
		for i, item := range items {
			<span data-idx={ fmt.Sprintf("%d", i) }>{ item }</span>
		}
		switch len(items) {
			case 0:
				<span>Empty</span>
			default:
				@BadgeTag(p.Title) {
					<span>Nested</span>
				}
		}
		@FooterNote("v1")
	</article>
}
`
		if err := os.WriteFile(filepath.Join(dir, "metric_card.templ"), []byte(templSrc), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runWebReview(t, dir, "review", "metric_card.templ", "--format", "json")
		if code != cli.ExitError {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitError, errOut, out)
		}
		rep := decodeWebReport(t, out)

		want := []struct {
			line     int
			ruleID   string
			severity review.Severity
		}{
			{line: 23, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
			{line: 24, ruleID: "web-off-contract-radius", severity: review.SeverityWarn},
			{line: 33, ruleID: "web-off-contract-shadow", severity: review.SeverityWarn},
			{line: 35, ruleID: "web-hardcoded-color", severity: review.SeverityWarn},
			{line: 37, ruleID: "emoji-disallowed", severity: review.SeverityError},
		}
		if len(rep.Findings) != len(want) {
			t.Fatalf("got %d findings, want %d:\n%+v", len(rep.Findings), len(want), rep.Findings)
		}
		for i, w := range want {
			got := rep.Findings[i]
			if got.File != "metric_card.templ" || got.Line != w.line || got.RuleID != w.ruleID || got.Severity != w.severity || strings.TrimSpace(got.Hint) == "" {
				t.Errorf("finding[%d] = %+v, want %+v", i, got, w)
			}
		}
	})
}

func TestReviewWebCustomTokensAndUpwardConfig(t *testing.T) {
	oxfordDesignMD := `---
design_tokens:
  name: "Oxford Navy Contract"
  color:
    brand:
      primary: { value: "#1E3A8A", type: "color" }
    light:
      paper: { value: "#FFFFFF", type: "color" }
      surface_card: { value: "#F8FAFC", type: "color" }
      ink: { value: "#0F172A", type: "color" }
      muted: { value: "#475569", type: "color" }
      accent: { value: "{color.brand.primary}", type: "color" }
    dark:
      paper: { value: "#0B0F17", type: "color" }
      ink: { value: "#F8FAFC", type: "color" }
      muted: { value: "#94A3B8", type: "color" }
      accent: { value: "#60A5FA", type: "color" }
    status:
      sanitized_dark_bg: { value: "rgba(30, 58, 138, 0.25)", type: "color" }
      sanitized_dark_text: { value: "#93C5FD", type: "color" }
  radius:
    none: { value: "0px" }
    subtle: { value: "4px" }
    card: { value: "8px" }
    pill: { value: "9999px" }
  elevation:
    card: { value: "0 1px 2px rgba(15, 23, 42, 0.08)", type: "shadow" }
---

# Oxford Visual Contract
`

	t.Run("explicit --tokens DESIGN.md allows contract colors, radii, and shadows across all 5 web extensions while catching off-contract drift", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "DESIGN.md"), []byte(oxfordDesignMD), 0o644); err != nil {
			t.Fatal(err)
		}

		// Clean multi-stack files conforming to DESIGN.md
		cleanCSS := `:root {
  --brand: #1E3A8A;
  --canvas: hsl(0, 0%, 100%);
  --ink: #0F172A;
}
.card {
  background: rgb(248, 250, 252);
  color: #0F172A;
  border-radius: 8px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
}
`
		cleanSCSS := `$pill-bg: rgba(30, 58, 138, 0.25);
.pill {
  background: $pill-bg;
  color: #93C5FD;
  border-radius: 9999px;
}
`
		cleanHTML := `---
title: Oxford Essay
---
<article class="bg-[#1E3A8A] text-[#F8FAFC]" style="border-radius: 4px;">
  <h1>{{ page.title }}</h1>
</article>
`
		cleanBlade := `@extends('layouts.app')
@section('content')
<div class="bg-[#0B0F17] text-[#F8FAFC]" style="color: #60A5FA; border-radius: 8px;">
  {{ $heading }}
</div>
@endsection
`
		cleanTempl := `package ui

css oxfordPanel() {
	background-color: #FFFFFF;
	border-radius: 8px;
}

templ OxfordPanel(title string) {
	<section class="text-[#1E3A8A]">
		<h2>{ title }</h2>
	</section>
}
`
		// File with off-contract drift even under DESIGN.md
		driftCSS := `.rogue {
  color: #ff00aa;
  border-radius: 24px;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
}
`
		files := map[string]string{
			"web/clean.css":       cleanCSS,
			"web/clean.scss":      cleanSCSS,
			"web/clean.html":      cleanHTML,
			"web/clean.blade.php": cleanBlade,
			"web/clean.templ":     cleanTempl,
			"web/drift.css":       driftCSS,
		}
		for rel, content := range files {
			full := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		// Clean files pass cleanly with zero findings.
		cleanCode, cleanOut, cleanErr := runWebReview(
			t, dir,
			"review", "--tokens", "DESIGN.md",
			"web/clean.css", "web/clean.scss", "web/clean.html", "web/clean.blade.php", "web/clean.templ",
			"--format", "json",
		)
		if cleanCode != cli.ExitSuccess {
			t.Fatalf("clean files with --tokens: got exit %d, want %d (stderr=%s, stdout=%s)", cleanCode, cli.ExitSuccess, cleanErr, cleanOut)
		}
		cleanRep := decodeWebReport(t, cleanOut)
		if cleanRep.Files != 6 || cleanRep.ErrorCount != 0 || cleanRep.WarnCount != 0 || len(cleanRep.Findings) != 0 {
			t.Fatalf("expected 6 clean files (DESIGN.md + 5 web files) with 0 findings, got %+v", cleanRep)
		}

		// Off-contract drift file warns on lines 2, 3, 4 and promotes under --strict.
		driftCode, driftOut, driftErr := runWebReview(t, dir, "review", "--tokens", "DESIGN.md", "web/drift.css", "--format", "json")
		if driftCode != cli.ExitSuccess {
			t.Fatalf("drift non-strict: got exit %d, want %d (stderr=%s, stdout=%s)", driftCode, cli.ExitSuccess, driftErr, driftOut)
		}
		driftRep := decodeWebReport(t, driftOut)
		if driftRep.WarnCount != 3 || len(driftRep.Findings) != 3 {
			t.Fatalf("expected 3 drift warnings, got %+v", driftRep)
		}
		wantDrift := []struct {
			line   int
			ruleID string
		}{
			{line: 2, ruleID: "web-hardcoded-color"},
			{line: 3, ruleID: "web-off-contract-radius"},
			{line: 4, ruleID: "web-off-contract-shadow"},
		}
		for i, w := range wantDrift {
			if driftRep.Findings[i].Line != w.line || driftRep.Findings[i].RuleID != w.ruleID {
				t.Errorf("drift finding[%d] = %+v, want line %d rule %s", i, driftRep.Findings[i], w.line, w.ruleID)
			}
		}

		strictCode, _, _ := runWebReview(t, dir, "review", "--strict", "--tokens", "DESIGN.md", "web/drift.css")
		if strictCode != cli.ExitError {
			t.Fatalf("--strict on drift.css: got exit %d, want %d", strictCode, cli.ExitError)
		}
	})

	t.Run("upward datafolio.yaml tokens contract is honored automatically when --tokens is omitted", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "DESIGN.md"), []byte(oxfordDesignMD), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "datafolio.yaml"), []byte("school: tufte\ntokens: DESIGN.md\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		sub := filepath.Join(dir, "templates")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		bladePage := `<div class="bg-[#1E3A8A]" style="border-radius: 8px;">{{ $title }}</div>`
		if err := os.WriteFile(filepath.Join(sub, "card.blade.php"), []byte(bladePage), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runWebReview(t, dir, "review", "templates/card.blade.php", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("upward datafolio.yaml tokens: got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeWebReport(t, out)
		if rep.ErrorCount != 0 || rep.WarnCount != 0 || len(rep.Findings) != 0 {
			t.Fatalf("expected 0 findings via upward datafolio.yaml tokens, got %+v", rep)
		}
	})

	t.Run("exported CSS and SCSS from datafolio tokens export pass review with zero findings", func(t *testing.T) {
		dir := t.TempDir()
		expCode, _, expErr := runWebReview(t, dir, "tokens", "export", "--to", "css,tailwind,scss", "--out", "dist")
		if expCode != cli.ExitSuccess {
			t.Fatalf("tokens export failed (%d): %s", expCode, expErr)
		}

		revCode, revOut, revErr := runWebReview(t, dir, "review", "--strict", "dist", "--format", "json")
		if revCode != cli.ExitSuccess {
			t.Fatalf("review exported dist failed (%d): stderr=%s\nstdout=%s", revCode, revErr, revOut)
		}
		rep := decodeWebReport(t, revOut)
		if rep.Files != 3 || rep.ErrorCount != 0 || rep.WarnCount != 0 {
			t.Fatalf("expected 3 clean exported web files, got %+v", rep)
		}
	})

	t.Run("templ receiver methods and malformed/unclosed template edges never hang or panic", func(t *testing.T) {
		dir := t.TempDir()
		recvTempl := `package ui

type Panel struct {
	Title string
}

templ (p *Panel) Render() {
	<div style="color: #ff00aa">{ p.Title }</div>
}
`
		unclosedBlade := `{{-- unclosed comment`
		unclosedHTML := `<!-- unclosed`
		unclosedCSS := `/* unclosed`
		if err := os.WriteFile(filepath.Join(dir, "recv.templ"), []byte(recvTempl), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "broken.blade.php"), []byte(unclosedBlade), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "broken.html"), []byte(unclosedHTML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "broken.css"), []byte(unclosedCSS), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := runWebReview(t, dir, "review", "recv.templ", "broken.blade.php", "broken.html", "broken.css", "--format", "json")
		if code != cli.ExitSuccess {
			t.Fatalf("got exit %d, want %d (stderr=%s, stdout=%s)", code, cli.ExitSuccess, errOut, out)
		}
		rep := decodeWebReport(t, out)
		if rep.Files != 4 || rep.WarnCount != 1 || len(rep.Findings) != 1 {
			t.Fatalf("expected 1 warning across 4 files, got %+v", rep)
		}
		if rep.Findings[0].File != "recv.templ" || rep.Findings[0].Line != 8 || rep.Findings[0].RuleID != "web-hardcoded-color" {
			t.Fatalf("unexpected finding: %+v", rep.Findings[0])
		}
	})
}
