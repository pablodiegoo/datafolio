package doc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// ParseFile reads and parses a Markdown Source Document from disk.
func ParseFile(path string) (*SourceDocument, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	return Parse(absPath, raw)
}

// Parse parses raw Markdown bytes with optional YAML frontmatter into a SourceDocument.
func Parse(path string, src []byte) (*SourceDocument, error) {
	fmBytes, body, lineOffset, err := SplitFrontmatter(src)
	if err != nil {
		return nil, err
	}

	var meta Metadata
	if len(fmBytes) > 0 {
		if err := yaml.Unmarshal(fmBytes, &meta); err != nil {
			return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
		}
	}

	absPath := path
	if path != "" && !filepath.IsAbs(path) {
		if resolved, absErr := filepath.Abs(path); absErr == nil {
			absPath = resolved
		}
	}
	docDir := filepath.Dir(absPath)

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	reader := text.NewReader(body)
	root := md.Parser().Parse(reader)

	blocks := convertBlocks(root, body, docDir, lineOffset)
	blocks = attachCaptionsAndSources(blocks)

	return &SourceDocument{
		Path:           absPath,
		Dir:            docDir,
		Meta:           meta,
		RawFrontmatter: fmBytes,
		BodyLineOffset: lineOffset,
		Blocks:         blocks,
	}, nil
}

// SplitFrontmatter extracts an optional leading YAML frontmatter block (`--- ... ---`)
// and returns the raw YAML bytes, the remaining Markdown body, and the 1-based starting line of the body.
func SplitFrontmatter(src []byte) ([]byte, []byte, int, error) {
	normalized := bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	normalized = bytes.TrimPrefix(normalized, []byte("\xEF\xBB\xBF"))

	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, normalized, 1, nil
	}

	rest := normalized[len("---\n"):]
	if bytes.HasPrefix(rest, []byte("---\n")) {
		return nil, rest[len("---\n"):], 3, nil
	}
	if bytes.Equal(rest, []byte("---")) {
		return nil, nil, 3, nil
	}

	lines := bytes.Split(rest, []byte("\n"))
	for i, line := range lines {
		if bytes.Equal(bytes.TrimRight(line, " \t"), []byte("---")) {
			fm := bytes.Join(lines[:i], []byte("\n"))
			var body []byte
			if i+1 < len(lines) {
				body = bytes.Join(lines[i+1:], []byte("\n"))
			}
			return fm, body, i + 3, nil
		}
	}

	return nil, normalized, 1, nil
}

func convertBlocks(parent ast.Node, src []byte, docDir string, lineOffset int) []Block {
	var blocks []Block
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		line := lineNumber(n, src, lineOffset)
		switch node := n.(type) {
		case *ast.Heading:
			id := ""
			if val, ok := node.AttributeString("id"); ok {
				if b, isBytes := val.([]byte); isBytes {
					id = string(b)
				} else if s, isStr := val.(string); isStr {
					id = s
				}
			}
			blocks = append(blocks, &HeadingBlock{
				Level:   node.Level,
				ID:      id,
				Inlines: convertInlines(node, src),
				Line:    line,
			})
		case *ast.Paragraph:
			inlines := convertInlines(node, src)
			if fig, ok := tryFigureFromInlines(inlines, docDir, line); ok {
				blocks = append(blocks, fig)
				continue
			}
			blocks = append(blocks, &ParagraphBlock{
				Inlines: inlines,
				Line:    line,
			})
		case *ast.TextBlock:
			blocks = append(blocks, &ParagraphBlock{
				Inlines: convertInlines(node, src),
				Line:    line,
			})
		case *extast.Table:
			blocks = append(blocks, convertTable(node, src, line))
		case *ast.List:
			items := make([][]Block, 0, node.ChildCount())
			for item := node.FirstChild(); item != nil; item = item.NextSibling() {
				items = append(items, convertBlocks(item, src, docDir, lineOffset))
			}
			blocks = append(blocks, &ListBlock{
				Ordered: node.IsOrdered(),
				Items:   items,
				Line:    line,
			})
		case *ast.Blockquote:
			blocks = append(blocks, &BlockquoteBlock{
				Blocks: convertBlocks(node, src, docDir, lineOffset),
				Line:   line,
			})
		case *ast.FencedCodeBlock:
			blocks = append(blocks, &CodeBlock{
				Language: string(node.Language(src)),
				Code:     extractLinesText(node, src),
				Line:     line,
			})
		case *ast.CodeBlock:
			blocks = append(blocks, &CodeBlock{
				Code: extractLinesText(node, src),
				Line: line,
			})
		case *ast.ThematicBreak:
			blocks = append(blocks, &ThematicBreakBlock{Line: line})
		}
	}
	return blocks
}

func tryFigureFromInlines(inlines []Inline, docDir string, line int) (*FigureBlock, bool) {
	if len(inlines) == 0 {
		return nil, false
	}
	img, ok := inlines[0].(*ImageInline)
	if !ok {
		return nil, false
	}

	var source string
	if len(inlines) > 1 {
		rest := trimLeadingBreaks(inlines[1:])
		if len(rest) == 0 {
			// trailing whitespace/breaks only
		} else if srcVal, ok := parseSourceText(inlinesPlainText(rest)); ok {
			source = srcVal
		} else {
			return nil, false
		}
	}

	caption := strings.TrimSpace(img.Alt)
	if source == "" && img.Title != "" {
		if srcVal, ok := parseSourceText(img.Title); ok {
			source = srcVal
		} else if caption == "" {
			caption = strings.TrimSpace(img.Title)
		}
	}

	return &FigureBlock{
		Src:          img.Src,
		ResolvedPath: resolveAssetPath(docDir, img.Src),
		Caption:      caption,
		Source:       source,
		Line:         line,
	}, true
}

func trimLeadingBreaks(inlines []Inline) []Inline {
	i := 0
	for i < len(inlines) {
		switch in := inlines[i].(type) {
		case *SoftBreakInline, *HardBreakInline:
			i++
		case *TextInline:
			if strings.TrimSpace(in.Text) == "" {
				i++
			} else {
				return inlines[i:]
			}
		default:
			return inlines[i:]
		}
	}
	return nil
}

func convertTable(tbl *extast.Table, src []byte, line int) *TableBlock {
	aligns := make([]ColumnAlign, len(tbl.Alignments))
	for i, a := range tbl.Alignments {
		switch a {
		case extast.AlignLeft:
			aligns[i] = AlignLeft
		case extast.AlignCenter:
			aligns[i] = AlignCenter
		case extast.AlignRight:
			aligns[i] = AlignRight
		default:
			aligns[i] = AlignDefault
		}
	}

	var headers []TableCell
	var rows [][]TableCell

	for child := tbl.FirstChild(); child != nil; child = child.NextSibling() {
		switch rowNode := child.(type) {
		case *extast.TableHeader:
			headers = convertTableCells(rowNode, src)
		case *extast.TableRow:
			rows = append(rows, convertTableCells(rowNode, src))
		}
	}

	colCount := len(headers)
	for _, r := range rows {
		if len(r) > colCount {
			colCount = len(r)
		}
	}
	for len(aligns) < colCount {
		aligns = append(aligns, AlignDefault)
	}

	numericCols := make([]bool, colCount)
	for col := 0; col < colCount; col++ {
		if isNumericColumn(rows, col) {
			numericCols[col] = true
			if aligns[col] == AlignDefault {
				aligns[col] = AlignRight
			}
		} else if aligns[col] == AlignRight && len(rows) > 0 && hasAnyNumericCell(rows, col) {
			numericCols[col] = true
		}
	}

	return &TableBlock{
		Headers:     headers,
		Alignments:  aligns,
		NumericCols: numericCols,
		Rows:        rows,
		Line:        line,
	}
}

func convertTableCells(row ast.Node, src []byte) []TableCell {
	var cells []TableCell
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*extast.TableCell); ok {
			inlines := convertInlines(cell, src)
			cells = append(cells, TableCell{
				Inlines: inlines,
				Plain:   strings.TrimSpace(inlinesPlainText(inlines)),
			})
		}
	}
	return cells
}

func isNumericColumn(rows [][]TableCell, col int) bool {
	if len(rows) == 0 {
		return false
	}
	sawNumeric := false
	for _, row := range rows {
		if col >= len(row) {
			continue
		}
		val := strings.TrimSpace(row[col].Plain)
		if val == "" || val == "-" || val == "—" || val == "–" || strings.EqualFold(val, "n/a") {
			continue
		}
		if !isNumericValue(val) {
			return false
		}
		sawNumeric = true
	}
	return sawNumeric
}

func hasAnyNumericCell(rows [][]TableCell, col int) bool {
	for _, row := range rows {
		if col < len(row) && isNumericValue(row[col].Plain) {
			return true
		}
	}
	return false
}

func isNumericValue(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") && len(s) > 2 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	for _, prefix := range []string{"R$", "US$", "$", "€", "£", "¥"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
			break
		}
	}
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "−")
	for _, suffix := range []string{"%", "pp", "bps", "x"} {
		if strings.HasSuffix(s, suffix) {
			s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
			break
		}
	}
	if s == "" {
		return false
	}

	digits := 0
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.' || r == ',' || r == '_' || r == ' ':
			// valid grouping/decimal separator
		case (r == 'e' || r == 'E') && i > 0 && digits > 0:
			// scientific notation allowed if digits follow
		case (r == '+' || r == '-') && i > 0 && (s[i-1] == 'e' || s[i-1] == 'E'):
			// exponent sign
		default:
			if !unicode.IsDigit(r) {
				return false
			}
			digits++
		}
	}
	return digits > 0
}

func attachCaptionsAndSources(blocks []Block) []Block {
	out := make([]Block, 0, len(blocks))
	for i := 0; i < len(blocks); i++ {
		blk := blocks[i]
		switch node := blk.(type) {
		case *TableBlock:
			if len(out) > 0 {
				if prev, ok := out[len(out)-1].(*ParagraphBlock); ok {
					if capText, ok := parseTableCaptionText(inlinesPlainText(prev.Inlines)); ok {
						node.Caption = capText
						out = out[:len(out)-1]
					}
				}
			}
			if i+1 < len(blocks) {
				if next, ok := blocks[i+1].(*ParagraphBlock); ok {
					if srcText, ok := parseSourceText(inlinesPlainText(next.Inlines)); ok {
						node.Source = srcText
						i++
					}
				}
			}
			out = append(out, node)
		case *FigureBlock:
			if node.Source == "" && i+1 < len(blocks) {
				if next, ok := blocks[i+1].(*ParagraphBlock); ok {
					if srcText, ok := parseSourceText(inlinesPlainText(next.Inlines)); ok {
						node.Source = srcText
						i++
					}
				}
			}
			out = append(out, node)
		default:
			out = append(out, blk)
		}
	}
	return out
}

func parseTableCaptionText(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"table:", "tabela:", "caption:", "legenda:", ":"} {
		if strings.HasPrefix(lower, prefix) {
			val := strings.TrimSpace(trimmed[len(prefix):])
			if val != "" {
				return val, true
			}
		}
	}
	return "", false
}

func parseSourceText(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"source:", "fonte:"} {
		if strings.HasPrefix(lower, prefix) {
			val := strings.TrimSpace(trimmed[len(prefix):])
			if val != "" {
				return val, true
			}
		}
	}
	return "", false
}

func resolveAssetPath(docDir, src string) string {
	trimmed := strings.TrimSpace(src)
	lower := strings.ToLower(trimmed)
	if trimmed == "" || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:") {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	return filepath.Join(docDir, filepath.FromSlash(trimmed))
}

func inlinesPlainText(inlines []Inline) string {
	var b strings.Builder
	for _, in := range inlines {
		switch n := in.(type) {
		case *TextInline:
			b.WriteString(n.Text)
		case *EmphasisInline:
			b.WriteString(inlinesPlainText(n.Children))
		case *CodeInline:
			b.WriteString(n.Code)
		case *LinkInline:
			b.WriteString(inlinesPlainText(n.Children))
		case *SoftBreakInline, *HardBreakInline:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func convertInlines(parent ast.Node, src []byte) []Inline {
	var out []Inline
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch node := n.(type) {
		case *ast.Text:
			txt := string(node.Segment.Value(src))
			if txt != "" {
				appendText(&out, txt)
			}
			if node.HardLineBreak() {
				out = append(out, &HardBreakInline{})
			} else if node.SoftLineBreak() {
				out = append(out, &SoftBreakInline{})
			}
		case *ast.String:
			txt := string(node.Value)
			if txt != "" {
				appendText(&out, txt)
			}
		case *ast.Emphasis:
			out = append(out, &EmphasisInline{
				Strong:   node.Level >= 2,
				Children: convertInlines(node, src),
			})
		case *ast.CodeSpan:
			out = append(out, &CodeInline{
				Code: collectPlainText(node, src),
			})
		case *ast.Link:
			out = append(out, &LinkInline{
				URL:      string(node.Destination),
				Title:    string(node.Title),
				Children: convertInlines(node, src),
			})
		case *ast.AutoLink:
			u := string(node.URL(src))
			label := string(node.Label(src))
			out = append(out, &LinkInline{
				URL:      u,
				Children: []Inline{&TextInline{Text: label}},
			})
		case *ast.Image:
			out = append(out, &ImageInline{
				Src:   string(node.Destination),
				Alt:   collectPlainText(node, src),
				Title: string(node.Title),
			})
		default:
			children := convertInlines(node, src)
			out = append(out, children...)
		}
	}
	return out
}

func appendText(out *[]Inline, s string) {
	if len(*out) > 0 {
		if prev, ok := (*out)[len(*out)-1].(*TextInline); ok {
			prev.Text += s
			return
		}
	}
	*out = append(*out, &TextInline{Text: s})
}

func collectPlainText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch cn := c.(type) {
		case *ast.Text:
			b.Write(cn.Segment.Value(src))
			if cn.SoftLineBreak() || cn.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(cn.Value)
		default:
			b.WriteString(collectPlainText(cn, src))
		}
	}
	return b.String()
}

func extractLinesText(n ast.Node, src []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(src))
	}
	return b.String()
}

func lineNumber(n ast.Node, src []byte, lineOffset int) int {
	if n.Type() == ast.TypeBlock && n.Lines() != nil && n.Lines().Len() > 0 {
		start := n.Lines().At(0).Start
		return lineOffset + bytes.Count(src[:start], []byte("\n"))
	}
	return lineOffset
}
