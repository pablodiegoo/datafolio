package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
)

func TestBuildBaselineHTMLSelfContainedAndPositionalFlags(t *testing.T) {
	src := `---
title: Archival Monograph Title
subtitle: Analytical Subtitle Here
kicker: RESEARCH NOTE
author: Ada Lovelace
date: 2026-11-23
---

## Introduction

This paragraph includes *italic*, **bold**, ` + "`inline_code`" + `, and a [local link](#section-2).

- First observation
- Second observation

> A crisp editorial quotation.

### Section 2

Final paragraph.
`

	for _, tc := range []struct {
		name string
		args []string
		out  string
	}{
		{name: "positional before flags", args: []string{"build", "report.md", "--to", "html"}, out: "report.html"},
		{name: "flags before positional", args: []string{"build", "--to", "html", "-o", "custom/out.html", "report.md"}, out: "custom/out.html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}

			code, _, errOut := runCLI(t, dir, tc.args...)
			if code != cli.ExitSuccess {
				t.Fatalf("build failed with %d: %s", code, errOut)
			}

			htmlBytes, err := os.ReadFile(filepath.Join(dir, tc.out))
			if err != nil {
				t.Fatalf("expected output %s: %v", tc.out, err)
			}
			html := string(htmlBytes)

			for _, want := range []string{
				"<!DOCTYPE html>",
				`<html lang="en"`,
				"Archival Monograph Title",
				"Analytical Subtitle Here",
				"RESEARCH NOTE",
				"Ada Lovelace",
				"2026-11-23",
				"<h2",
				"Introduction",
				"<em>italic</em>",
				"<strong>bold</strong>",
				"<code>inline_code</code>",
				`<a href="#section-2">local link</a>`,
				"First observation",
				"A crisp editorial quotation.",
				"#fffff8",
				"#111111",
				"#8b1e1e",
				"65%",
				"30%",
				"border-radius: 0px",
				"box-shadow: none",
			} {
				if !strings.Contains(html, want) {
					t.Errorf("HTML missing expected fragment %q", want)
				}
			}

			for _, forbidden := range []string{"https://", "http://", "<script", "cdn."} {
				if strings.Contains(strings.ToLower(html), forbidden) {
					t.Errorf("offline HTML must not contain external/CDN reference %q", forbidden)
				}
			}
		})
	}
}

func TestBuildUsageAndMissingInputExitCodes(t *testing.T) {
	dir := t.TempDir()

	for _, args := range [][]string{
		{"build"},
		{"build", "--to", "html"},
		{"build", "report.md", "--to", "docx"},
		{"build", "report.md", "--unknown-flag"},
	} {
		code, _, _ := runCLI(t, dir, args...)
		if code != cli.ExitUsage {
			t.Fatalf("args %v: got exit %d, want %d", args, code, cli.ExitUsage)
		}
	}

	code, _, _ := runCLI(t, dir, "build", "does-not-exist.md", "--to", "html")
	if code != cli.ExitError {
		t.Fatalf("missing input file: got exit %d, want %d", code, cli.ExitError)
	}
}
