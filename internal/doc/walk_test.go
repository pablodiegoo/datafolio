package doc_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/doc"
)

func TestWalkBlocksInlinesAndReferencedAssets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := `---
title: Traversal Fixture
---

# Heading ![icon](assets/icon.png)

Intro with **![bold-img](assets/bold.svg)** and [link ![link-img](assets/link.png)](https://example.com) and [remote](https://example.com)[^n1].

> ![quote-fig](assets/quote.png)

- List item with ![list-img](assets/icon.png)

:::wide
![wide-fig](assets/wide.svg)
:::

:::columns
:::col
![col-fig](assets/col.png)
:::
:::col
![remote-fig](https://cdn.example.com/skip.png)
:::
:::

| Metric | Visual |
| --- | --- |
| Throughput | ![cell-img](assets/cell.png) |

[^n1]: Sidenote with ![sn-img](assets/sidenote.png).
`

	parsed, err := doc.Parse(filepath.Join(dir, "doc.md"), []byte(src))
	if err != nil {
		t.Fatalf("doc.Parse failed: %v", err)
	}

	var seenBlocks int
	doc.WalkDocumentBlocks(parsed, func(blk doc.Block) {
		if blk != nil {
			seenBlocks++
		}
	})
	if seenBlocks == 0 {
		t.Fatalf("expected WalkDocumentBlocks to visit blocks")
	}

	refs := parsed.ReferencedAssets()
	gotPaths := make([]string, 0, len(refs))
	for _, ref := range refs {
		gotPaths = append(gotPaths, ref.ResolvedPath)
	}

	wantPaths := []string{
		filepath.Join(dir, "assets", "icon.png"),
		filepath.Join(dir, "assets", "bold.svg"),
		filepath.Join(dir, "assets", "link.png"),
		filepath.Join(dir, "assets", "quote.png"),
		filepath.Join(dir, "assets", "wide.svg"),
		filepath.Join(dir, "assets", "col.png"),
		filepath.Join(dir, "assets", "cell.png"),
		filepath.Join(dir, "assets", "sidenote.png"),
	}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("ReferencedAssets() = %v, want %v", gotPaths, wantPaths)
	}
}
