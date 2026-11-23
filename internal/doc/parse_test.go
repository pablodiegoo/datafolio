package doc_test

import (
	"testing"

	"github.com/pablodiegoo/datafolio/internal/doc"
)

func TestParseSidenotesAndOrphanTracking(t *testing.T) {
	md := `---
title: Sidenote Spec
---

First paragraph[^alpha] and missing[^ghost].

[^alpha]: **Note Title** Line one
    continued on indented line.

[^unused]: Orphan definition body.
`
	parsed, err := doc.Parse("report.md", []byte(md))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(parsed.SidenoteRefs) != 2 {
		t.Fatalf("got %d SidenoteRefs, want 2: %+v", len(parsed.SidenoteRefs), parsed.SidenoteRefs)
	}
	if parsed.SidenoteRefs[0].ID != "alpha" || parsed.SidenoteRefs[0].Number != 1 || !parsed.SidenoteRefs[0].Defined || parsed.SidenoteRefs[0].Line != 5 {
		t.Errorf("unexpected first SidenoteRef: %+v", parsed.SidenoteRefs[0])
	}
	if parsed.SidenoteRefs[1].ID != "ghost" || parsed.SidenoteRefs[1].Defined || parsed.SidenoteRefs[1].Line != 5 {
		t.Errorf("unexpected orphan SidenoteRef: %+v", parsed.SidenoteRefs[1])
	}

	if len(parsed.Sidenotes) != 2 {
		t.Fatalf("got %d Sidenotes, want 2: %+v", len(parsed.Sidenotes), parsed.Sidenotes)
	}
	alpha := parsed.Sidenotes[0]
	if alpha.ID != "alpha" || alpha.Number != 1 || alpha.Title != "Note Title" || !alpha.Referenced || alpha.Line != 7 {
		t.Errorf("unexpected alpha SidenoteDef: %+v", alpha)
	}
	unused := parsed.Sidenotes[1]
	if unused.ID != "unused" || unused.Referenced || unused.Line != 10 {
		t.Errorf("unexpected orphan SidenoteDef: %+v", unused)
	}
}

func TestParseKPIAndColumnsVariants(t *testing.T) {
	md := `:::kpi
| Metric | Value | Note |
| --- | --- | --- |
| ARR | $2.4M | +18% |
| NRR | 114% | stable |
:::

:::columns
Col A

---

Col B
:::
`
	parsed, err := doc.Parse("blocks.md", []byte(md))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(parsed.Blocks) != 2 {
		t.Fatalf("got %d top-level blocks, want 2: %#v", len(parsed.Blocks), parsed.Blocks)
	}

	kpi, ok := parsed.Blocks[0].(*doc.KPIBlock)
	if !ok {
		t.Fatalf("block 0 is %T, want *doc.KPIBlock", parsed.Blocks[0])
	}
	if len(kpi.Items) != 2 || kpi.Items[0].Label != "ARR" || kpi.Items[0].Value != "$2.4M" || kpi.Items[0].Note != "+18%" {
		t.Errorf("unexpected KPI items: %+v", kpi.Items)
	}

	cols, ok := parsed.Blocks[1].(*doc.ColumnsBlock)
	if !ok {
		t.Fatalf("block 1 is %T, want *doc.ColumnsBlock", parsed.Blocks[1])
	}
	if len(cols.Columns) != 2 {
		t.Fatalf("got %d columns, want 2", len(cols.Columns))
	}
}

func TestParseBibEntriesAndLatexCleanup(t *testing.T) {
	bib := `
@string{j = "Journal of " # "Data"}
@article{silva2024,
  author = {Jo{\~a}o da Silva and Mar{\'i}a Gonz{\'a}lez},
  title = {An{\'a}lise de {Dados} Hist{\'o}ricos},
  journal = j,
  pages = {10--25},
  year = {2024},
}
`
	entries, err := doc.ParseBib([]byte(bib))
	if err != nil {
		t.Fatalf("ParseBib failed: %v", err)
	}
	e, ok := entries["silva2024"]
	if !ok {
		t.Fatalf("missing silva2024 entry: %+v", entries)
	}
	if e.Journal != "Journal of Data" {
		t.Errorf("Journal = %q, want %q", e.Journal, "Journal of Data")
	}
	if e.Pages != "10–25" {
		t.Errorf("Pages = %q, want %q", e.Pages, "10–25")
	}
	if len(e.Authors) != 2 || e.Authors[0] != "João da Silva" || e.Authors[1] != "María González" {
		t.Errorf("unexpected authors: %+v", e.Authors)
	}
	if e.Title != "Análise de Dados Históricos" {
		t.Errorf("Title = %q, want %q", e.Title, "Análise de Dados Históricos")
	}
}

func TestParseSlideSplitsAndSlideComments(t *testing.T) {
	md := `---
title: Deck Title
kicker: SERIES 01
---

Cover note[^cov].

[^cov]: Cover sidenote.

---

<!-- kicker: SECTION ONE -->
<!-- footer: CS 101 | | Slide {page} -->
# First Content Slide

Main slide bullet.

---

<!-- kicker: TWO COLUMNS -->
<!-- header: false -->
# Comparison Slide

:::columns
Left col

---

Right col
:::
`
	parsed, err := doc.Parse("deck.md", []byte(md))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(parsed.Slides) != 3 {
		t.Fatalf("got %d slides, want 3: %+v", len(parsed.Slides), parsed.Slides)
	}

	s0 := parsed.Slides[0]
	if s0.Index != 1 || len(s0.Blocks) != 1 {
		t.Errorf("unexpected slide 0: %+v", s0)
	}

	s1 := parsed.Slides[1]
	if s1.Index != 2 || s1.Kicker != "SECTION ONE" || s1.Title != "First Content Slide" {
		t.Errorf("unexpected slide 1 metadata: %+v", s1)
	}
	if !s1.Footer.Set || s1.Footer.Left != "CS 101" || s1.Footer.Right != "Slide {page}" {
		t.Errorf("unexpected slide 1 footer override: %+v", s1.Footer)
	}

	s2 := parsed.Slides[2]
	if s2.Index != 3 || s2.Kicker != "TWO COLUMNS" || s2.Title != "Comparison Slide" {
		t.Errorf("unexpected slide 2 metadata: %+v", s2)
	}
	if !s2.Header.Set || s2.Header.Enabled == nil || *s2.Header.Enabled {
		t.Errorf("unexpected slide 2 header override (want enabled=false): %+v", s2.Header)
	}
	// Ensure `---` inside :::columns did NOT split slide 2 into another slide!
	if len(s2.Blocks) != 2 {
		t.Errorf("expected 2 blocks (heading + columns) on slide 2, got %d: %#v", len(s2.Blocks), s2.Blocks)
	}
}

func TestParseSlideChromeOverrideJSONAndPipeAttributes(t *testing.T) {
	md := `---
title: Chrome Deck
---

<!-- header: {"left": "Course 101", "right": "{page} / {pages}", "rule": true, "skip_first": false} -->
<!-- footer: left: Dept | right: Slide {page} | rule: true -->
# Slide One

Body.
`
	parsed, err := doc.Parse("chrome.md", []byte(md))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(parsed.Slides) != 1 {
		t.Fatalf("got %d slides, want 1", len(parsed.Slides))
	}
	s := parsed.Slides[0]
	if !s.Header.Set || s.Header.Enabled == nil || !*s.Header.Enabled {
		t.Fatalf("expected header enabled=true, got %+v", s.Header)
	}
	if !s.Header.HasLeft || s.Header.Left != "Course 101" || s.Header.HasCenter || !s.Header.HasRight || s.Header.Right != "{page} / {pages}" {
		t.Errorf("unexpected JSON header slots: %+v", s.Header)
	}
	if s.Header.Rule == nil || !*s.Header.Rule {
		t.Errorf("expected header Rule=true, got %+v", s.Header.Rule)
	}
	if s.Header.SkipFirst == nil || *s.Header.SkipFirst {
		t.Errorf("expected header SkipFirst=false, got %+v", s.Header.SkipFirst)
	}

	if !s.Footer.Set || !s.Footer.HasLeft || s.Footer.Left != "Dept" || s.Footer.HasCenter || !s.Footer.HasRight || s.Footer.Right != "Slide {page}" {
		t.Errorf("unexpected pipe-attribute footer slots: %+v", s.Footer)
	}
	if s.Footer.Rule == nil || !*s.Footer.Rule {
		t.Errorf("expected footer Rule=true, got %+v", s.Footer.Rule)
	}
}
