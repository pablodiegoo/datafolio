package doc

import (
	"path/filepath"
	"strings"
)

// AssetRef pairs a local asset's original source reference with its resolved filesystem path.
type AssetRef struct {
	Src          string
	ResolvedPath string
}

// WalkBlocks traverses blocks in pre-order, descending into container blocks
// (lists, blockquotes, margin notes, wide sections, and columns).
func WalkBlocks(blocks []Block, fn func(Block)) {
	if fn == nil {
		return
	}
	for _, blk := range blocks {
		if blk == nil {
			continue
		}
		fn(blk)
		switch node := blk.(type) {
		case *ListBlock:
			for _, item := range node.Items {
				WalkBlocks(item, fn)
			}
		case *BlockquoteBlock:
			WalkBlocks(node.Blocks, fn)
		case *MarginNoteBlock:
			WalkBlocks(node.Blocks, fn)
		case *WideBlock:
			WalkBlocks(node.Blocks, fn)
		case *ColumnsBlock:
			for _, col := range node.Columns {
				WalkBlocks(col, fn)
			}
		}
	}
}

// WalkDocumentBlocks visits every Block in d.Blocks and all Sidenote definitions in pre-order.
func WalkDocumentBlocks(d *SourceDocument, fn func(Block)) {
	if d == nil || fn == nil {
		return
	}
	WalkBlocks(d.Blocks, fn)
	for _, sn := range d.Sidenotes {
		WalkBlocks(sn.Blocks, fn)
	}
}

// WalkInlines traverses inline spans in pre-order, descending into EmphasisInline and LinkInline children.
func WalkInlines(inlines []Inline, fn func(Inline)) {
	if fn == nil {
		return
	}
	for _, in := range inlines {
		if in == nil {
			continue
		}
		fn(in)
		switch node := in.(type) {
		case *EmphasisInline:
			WalkInlines(node.Children, fn)
		case *LinkInline:
			WalkInlines(node.Children, fn)
		}
	}
}

// ResolveAssetPath resolves a local figure or image source path relative to docDir,
// returning "" for empty paths and remote or data URLs.
func ResolveAssetPath(docDir, src string) string {
	trimmed := strings.TrimSpace(src)
	lower := strings.ToLower(trimmed)
	if trimmed == "" ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	if docDir == "" {
		return filepath.Clean(filepath.FromSlash(trimmed))
	}
	return filepath.Join(docDir, filepath.FromSlash(trimmed))
}

// ReferencedAssets returns all deduplicated local figure and inline image assets
// referenced across d.Blocks, d.Sidenotes, and d.Slides in encounter order.
func (d *SourceDocument) ReferencedAssets() []AssetRef {
	if d == nil {
		return nil
	}
	var out []AssetRef
	seen := make(map[string]struct{})
	add := func(src, resolved string) {
		if resolved == "" {
			return
		}
		if _, ok := seen[resolved]; ok {
			return
		}
		seen[resolved] = struct{}{}
		out = append(out, AssetRef{Src: src, ResolvedPath: resolved})
	}
	visitInlines := func(inlines []Inline) {
		WalkInlines(inlines, func(in Inline) {
			if img, ok := in.(*ImageInline); ok {
				add(img.Src, ResolveAssetPath(d.Dir, img.Src))
			}
		})
	}
	visitBlocks := func(blocks []Block) {
		WalkBlocks(blocks, func(blk Block) {
			switch node := blk.(type) {
			case *HeadingBlock:
				visitInlines(node.Inlines)
			case *ParagraphBlock:
				visitInlines(node.Inlines)
			case *TableBlock:
				for _, cell := range node.Headers {
					visitInlines(cell.Inlines)
				}
				for _, row := range node.Rows {
					for _, cell := range row {
						visitInlines(cell.Inlines)
					}
				}
			case *FigureBlock:
				if node.ResolvedPath != "" {
					add(node.Src, node.ResolvedPath)
				} else if p := ResolveAssetPath(d.Dir, node.Src); p != "" {
					add(node.Src, p)
				}
			}
		})
	}

	visitBlocks(d.Blocks)
	for _, sn := range d.Sidenotes {
		visitBlocks(sn.Blocks)
		visitInlines(sn.Inlines)
	}
	for _, sl := range d.Slides {
		visitBlocks(sl.Blocks)
	}
	return out
}
