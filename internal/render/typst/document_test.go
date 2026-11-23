package typst_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/config"
	"github.com/pablodiegoo/datafolio/internal/doc"
	"github.com/pablodiegoo/datafolio/internal/render/typst"
	_ "github.com/pablodiegoo/datafolio/internal/school/tufte"
)

func TestEscapeMarkupAndTypstHelpers(t *testing.T) {
	in := "Cost is $10 to $20, C#, a_b, *s*, `c`, u@x, <t>, [k], a\\b, ~n, http://ex.com\n1. ordered\n- dash\n= heading"
	esc := typst.EscapeMarkup(in)

	for _, want := range []string{
		`\$10 to \$20`,
		`C\#`,
		`a\_b`,
		`\*s\*`,
		"\\`c\\`",
		`u\@x`,
		`\<t\>`,
		`\[k\]`,
		`a\\b`,
		`\~n`,
		`http:\/\/ex.com`,
		"\n1\\. ordered",
		"\n\\- dash",
		"\n\\= heading",
	} {
		if !strings.Contains(esc, want) {
			t.Errorf("EscapeMarkup missing %q\ngot: %s", want, esc)
		}
	}

	if l, r := typst.TypstLangRegion("pt-BR"); l != "pt" || r != "BR" {
		t.Errorf("TypstLangRegion(pt-BR) = (%q, %q), want (pt, BR)", l, r)
	}
	if l, r := typst.TypstLangRegion("en"); l != "en" || r != "" {
		t.Errorf("TypstLangRegion(en) = (%q, %q), want (en, '')", l, r)
	}
}

func TestResolveCSLBuiltinAliasesAndFiles(t *testing.T) {
	dir := t.TempDir()
	cslFile := filepath.Join(dir, "custom.csl")
	if err := os.WriteFile(cslFile, []byte("<style/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, isFile, err := typst.ResolveCSL(filepath.Join(dir, "abnt"), dir)
	if err != nil || isFile || got != "associacao-brasileira-de-normas-tecnicas" {
		t.Fatalf("ResolveCSL(abnt) = (%q, %v, %v)", got, isFile, err)
	}

	got, isFile, err = typst.ResolveCSL(filepath.Join(dir, "chicago"), dir)
	if err != nil || isFile || got != "chicago-author-date" {
		t.Fatalf("ResolveCSL(chicago) = (%q, %v, %v)", got, isFile, err)
	}

	got, isFile, err = typst.ResolveCSL("custom.csl", dir)
	if err != nil || !isFile || got != cslFile {
		t.Fatalf("ResolveCSL(custom.csl) = (%q, %v, %v)", got, isFile, err)
	}

	if _, _, err := typst.ResolveCSL("missing.csl", dir); err == nil {
		t.Fatalf("expected error for missing .csl file")
	}
}

func TestPrepareWorkspaceRejectsMissingAssetsAndResolvesParentPaths(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "sub", "docs")
	sharedDir := filepath.Join(root, "shared")
	tmpDir := filepath.Join(root, "tmp")
	for _, d := range []string{docDir, sharedDir, tmpDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	svgPath := filepath.Join(sharedDir, "fig.svg")
	if err := os.WriteFile(svgPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644); err != nil {
		t.Fatal(err)
	}

	md := `---
title: Workspace Test
---

![Chart](../../shared/fig.svg)
`
	parsed, err := doc.Parse(filepath.Join(docDir, "doc.md"), []byte(md))
	if err != nil {
		t.Fatal(err)
	}

	cfg, s, tok, err := config.Resolve()
	if err != nil {
		t.Fatal(err)
	}

	rootDir, opts, err := typst.PrepareWorkspace(parsed, cfg, tmpDir)
	if err != nil {
		t.Fatalf("PrepareWorkspace failed: %v", err)
	}
	if rootDir == "" || opts.AssetMapper == nil {
		t.Fatalf("expected non-empty rootDir and AssetMapper")
	}
	out, err := typst.RenderDocument(parsed, cfg, tok, s.Labels(cfg.Lang), opts)
	if err != nil {
		t.Fatalf("RenderDocument failed: %v", err)
	}
	if !strings.Contains(out, `"/shared/fig.svg"`) {
		t.Errorf("expected root-relative figure path in rendered typst:\n%s", out)
	}
}
