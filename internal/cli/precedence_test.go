package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
)

func TestBuildFourLevelConfigPrecedenceAndThemes(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "reports", "q4")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Level 2: upward-discovered datafolio.yaml in project root.
	projectYAML := `school: tufte
theme: night
lang: pt-BR
format: html
cover: true
`
	if err := os.WriteFile(filepath.Join(root, "datafolio.yaml"), []byte(projectYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Case A: datafolio.yaml overrides School defaults; unset CLI flags do not clobber.
	docOnlyTitle := `---
title: Night Report
---

Hello world.
`
	docAPath := filepath.Join(subDir, "a.md")
	if err := os.WriteFile(docAPath, []byte(docOnlyTitle), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, root, "build", "reports/q4/a.md")
	if code != cli.ExitSuccess {
		t.Fatalf("build a.md failed (%d): %s", code, errOut)
	}
	htmlA, err := os.ReadFile(filepath.Join(subDir, "a.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlA), `data-theme="night"`) || !strings.Contains(string(htmlA), "#121316") || !strings.Contains(string(htmlA), "#e06c60") {
		t.Fatalf("expected night theme from upward datafolio.yaml, got:\n%s", string(htmlA))
	}
	if !strings.Contains(string(htmlA), `<html lang="pt-BR"`) {
		t.Fatalf("expected lang pt-BR from upward datafolio.yaml")
	}

	// Case B: frontmatter overrides upward datafolio.yaml.
	docFrontmatter := `---
title: Print Override
theme: print
lang: en
---

Hello print.
`
	docBPath := filepath.Join(subDir, "b.md")
	if err := os.WriteFile(docBPath, []byte(docFrontmatter), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut = runCLI(t, root, "build", "reports/q4/b.md")
	if code != cli.ExitSuccess {
		t.Fatalf("build b.md failed (%d): %s", code, errOut)
	}
	htmlB, err := os.ReadFile(filepath.Join(subDir, "b.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlB), `data-theme="print"`) || !strings.Contains(string(htmlB), "#ffffff") {
		t.Fatalf("expected print theme from frontmatter")
	}
	if !strings.Contains(string(htmlB), `<html lang="en"`) {
		t.Fatalf("expected lang en from frontmatter")
	}

	// Case C: CLI flags override frontmatter and datafolio.yaml.
	code, _, errOut = runCLI(t, root, "build", "reports/q4/b.md", "--theme", "night", "--lang", "pt-BR")
	if code != cli.ExitSuccess {
		t.Fatalf("build b.md with CLI overrides failed (%d): %s", code, errOut)
	}
	htmlC, err := os.ReadFile(filepath.Join(subDir, "b.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlC), `data-theme="night"`) || !strings.Contains(string(htmlC), `<html lang="pt-BR"`) {
		t.Fatalf("expected CLI flags to override frontmatter")
	}
}

func TestBuildInvalidConfigYAMLReturnsUsageExitCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "datafolio.yaml"), []byte("theme: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("---\ntitle: Hi\n---\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, _ := runCLI(t, dir, "build", "doc.md", "--to", "html")
	if code != cli.ExitUsage {
		t.Fatalf("expected exit %d for invalid datafolio.yaml, got %d", cli.ExitUsage, code)
	}

	// Valid datafolio.yaml, invalid frontmatter chrome spec.
	if err := os.WriteFile(filepath.Join(dir, "datafolio.yaml"), []byte("format: html\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad_chrome.md"), []byte("---\nheader: not-a-bool-or-map\n---\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, _ = runCLI(t, dir, "build", "bad_chrome.md")
	if code != cli.ExitUsage {
		t.Fatalf("expected exit %d for invalid header spec, got %d", cli.ExitUsage, code)
	}
}
