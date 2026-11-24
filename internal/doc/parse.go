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
	"github.com/yuin/goldmark/util"
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
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithBlockParsers(
				util.Prioritized(&sidenoteDefBlockParser{lineOffset: lineOffset}, 650),
				util.Prioritized(&mathBlockParser{lineOffset: lineOffset}, 670),
				util.Prioritized(&fencedContainerBlockParser{lineOffset: lineOffset}, 680),
			),
			parser.WithInlineParsers(
				util.Prioritized(&mathInlineParser{}, 140),
				util.Prioritized(&sidenoteRefInlineParser{lineOffset: lineOffset}, 150),
				util.Prioritized(&citationInlineParser{}, 160),
			),
		),
	)
	reader := text.NewReader(body)
	root := md.Parser().Parse(reader)

	var rawSidenotes []*sidenoteDefNode
	blocks := convertBlocks(root, body, docDir, lineOffset, &rawSidenotes)
	cleanBlocks, slides := buildSlides(blocks, lineOffset)
	cleanBlocks = attachCaptionsAndSources(cleanBlocks)
	for i := range slides {
		slides[i].Blocks = attachCaptionsAndSources(slides[i].Blocks)
		if slides[i].Title == "" {
			slides[i].Title = firstSlideTitle(slides[i].Blocks)
		}
	}

	doc := &SourceDocument{
		Path:           absPath,
		Dir:            docDir,
		Meta:           meta,
		RawFrontmatter: fmBytes,
		BodyLineOffset: lineOffset,
		Blocks:         cleanBlocks,
		Slides:         slides,
	}
	resolveSidenotes(doc, rawSidenotes, body, docDir, lineOffset)
	resolveCitations(doc)

	return doc, nil
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

func convertBlocks(parent ast.Node, src []byte, docDir string, lineOffset int, rawSidenotes *[]*sidenoteDefNode) []Block {
	var blocks []Block
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		line := lineNumber(n, src, lineOffset)
		switch node := n.(type) {
		case *sidenoteDefNode:
			if rawSidenotes != nil {
				*rawSidenotes = append(*rawSidenotes, node)
			}
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
				items = append(items, convertBlocks(item, src, docDir, lineOffset, rawSidenotes))
			}
			blocks = append(blocks, &ListBlock{
				Ordered: node.IsOrdered(),
				Items:   items,
				Line:    line,
			})
		case *ast.Blockquote:
			bqBlocks := convertBlocks(node, src, docDir, lineOffset, rawSidenotes)
			if mn, ok := tryMarginNoteFromBlockquote(bqBlocks, line); ok {
				blocks = append(blocks, mn)
				continue
			}
			blocks = append(blocks, &BlockquoteBlock{
				Blocks: bqBlocks,
				Line:   line,
			})
		case *fencedContainerNode:
			switch node.ContainerKind {
			case "note":
				childBlocks := convertBlocks(node, src, docDir, lineOffset, rawSidenotes)
				childBlocks = attachCaptionsAndSources(childBlocks)
				blocks = append(blocks, &MarginNoteBlock{
					Title:  node.Arg,
					Blocks: childBlocks,
					Line:   node.Line,
				})
			case "wide":
				childBlocks := convertBlocks(node, src, docDir, lineOffset, rawSidenotes)
				childBlocks = attachCaptionsAndSources(childBlocks)
				blocks = append(blocks, &WideBlock{
					Blocks: childBlocks,
					Line:   node.Line,
				})
			case "kpi":
				childBlocks := convertBlocks(node, src, docDir, lineOffset, rawSidenotes)
				childBlocks = attachCaptionsAndSources(childBlocks)
				blocks = append(blocks, &KPIBlock{
					Items: parseKPIItems(childBlocks),
					Line:  node.Line,
				})
			case "columns":
				cols := convertColumns(node, src, docDir, lineOffset, rawSidenotes)
				blocks = append(blocks, &ColumnsBlock{
					Columns: cols,
					Line:    node.Line,
				})
			default:
				childBlocks := convertBlocks(node, src, docDir, lineOffset, rawSidenotes)
				childBlocks = attachCaptionsAndSources(childBlocks)
				blocks = append(blocks, childBlocks...)
			}
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
		case *ast.HTMLBlock:
			raw := extractLinesText(node, src)
			if dirs := parseSlideDirectives(raw, line); len(dirs) > 0 {
				for _, d := range dirs {
					blocks = append(blocks, d)
				}
			}
		case *mathBlockNode:
			blocks = append(blocks, &MathBlock{
				LaTeX: strings.TrimSpace(node.Body.String()),
				Line:  node.Line,
			})
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
		ResolvedPath: ResolveAssetPath(docDir, img.Src),
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

// InlinesPlainText flattens a slice of Inline nodes into plain text.
func InlinesPlainText(inlines []Inline) string {
	return inlinesPlainText(inlines)
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
		case *MathInline:
			b.WriteString(n.LaTeX)
		case *CitationInline:
			b.WriteString(n.Raw)
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
		case *sidenoteRefNode:
			out = append(out, &SidenoteRefInline{
				ID:   node.ID,
				Line: node.Line,
			})
		case *mathInlineNode:
			out = append(out, &MathInline{
				LaTeX:   node.LaTeX,
				Display: node.Display,
			})
		case *citationInlineNode:
			out = append(out, &CitationInline{
				Items: node.Items,
				Raw:   node.Raw,
			})
		default:
			children := convertInlines(node, src)
			out = append(out, children...)
		}
	}
	return out
}

var (
	kindSidenoteDef     = ast.NewNodeKind("DataFolioSidenoteDef")
	kindSidenoteRef     = ast.NewNodeKind("DataFolioSidenoteRef")
	kindFencedContainer = ast.NewNodeKind("DataFolioFencedContainer")
	kindMathBlock       = ast.NewNodeKind("DataFolioMathBlock")
	kindMathInline      = ast.NewNodeKind("DataFolioMathInline")
	kindCitationInline  = ast.NewNodeKind("DataFolioCitationInline")
)

type citationInlineNode struct {
	ast.BaseInline
	Items []CitationItem
	Raw   string
}

func (n *citationInlineNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Raw": n.Raw}, nil)
}

func (*citationInlineNode) Kind() ast.NodeKind {
	return kindCitationInline
}

type citationInlineParser struct{}

func (*citationInlineParser) Trigger() []byte {
	return []byte{'['}
}

func (*citationInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 4 || line[0] != '[' {
		return nil
	}
	if line[1] == '^' {
		return nil
	}
	closeIdx := -1
	for i := 1; i < len(line); i++ {
		c := line[i]
		if c == '[' || c == '\n' || c == '\r' {
			return nil
		}
		if c == ']' {
			closeIdx = i
			break
		}
	}
	if closeIdx <= 1 {
		return nil
	}
	if closeIdx+1 < len(line) && (line[closeIdx+1] == '(' || line[closeIdx+1] == '[') {
		return nil
	}
	inner := string(line[1:closeIdx])
	if !strings.Contains(inner, "@") {
		return nil
	}
	items, ok := parseCitationItems(inner)
	if !ok {
		return nil
	}
	raw := string(line[:closeIdx+1])
	block.Advance(closeIdx + 1)
	return &citationInlineNode{
		Items: items,
		Raw:   raw,
	}
}

func parseCitationItems(inner string) ([]CitationItem, bool) {
	parts := strings.Split(inner, ";")
	items := make([]CitationItem, 0, len(parts))
	for _, part := range parts {
		seg := strings.TrimSpace(part)
		if seg == "" {
			return nil, false
		}
		atIdx := strings.IndexByte(seg, '@')
		if atIdx < 0 {
			return nil, false
		}
		if atIdx > 0 && seg[atIdx-1] != ' ' && seg[atIdx-1] != '\t' && seg[atIdx-1] != '-' {
			return nil, false
		}
		prefix := strings.TrimSpace(seg[:atIdx])
		rest := seg[atIdx+1:]
		if rest == "" || !isBibKeyStart(rest[0]) {
			return nil, false
		}
		kEnd := 1
		for kEnd < len(rest) && isBibKeyChar(rest[kEnd]) {
			kEnd++
		}
		key := strings.TrimRight(rest[:kEnd], ".,;:")
		if key == "" {
			return nil, false
		}
		locator := strings.TrimSpace(rest[len(key):])
		locator = strings.TrimSpace(strings.TrimPrefix(locator, ","))
		items = append(items, CitationItem{
			Key:     key,
			Prefix:  prefix,
			Locator: locator,
		})
	}
	return items, len(items) > 0
}

func isBibKeyStart(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_'
}

func isBibKeyChar(c byte) bool {
	return isBibKeyStart(c) || c == '-' || c == ':' || c == '.' || c == '/'
}

func resolveCitations(d *SourceDocument) {
	seen := make(map[string]bool)
	var visit func([]Inline)
	visit = func(inlines []Inline) {
		for _, in := range inlines {
			switch node := in.(type) {
			case *EmphasisInline:
				visit(node.Children)
			case *LinkInline:
				visit(node.Children)
			case *CitationInline:
				for _, item := range node.Items {
					if !seen[item.Key] {
						seen[item.Key] = true
						d.Citations = append(d.Citations, item.Key)
					}
				}
			}
		}
	}
	walkDocInlines(d.Blocks, visit)
	for _, sn := range d.Sidenotes {
		if sn.Referenced {
			walkDocInlines(sn.Blocks, visit)
		}
	}
}

type slideDirectiveBlock struct {
	Kind  string
	Value string
	Line  int
}

func (*slideDirectiveBlock) isBlock() {}

func parseSlideDirectives(raw string, line int) []*slideDirectiveBlock {
	var dirs []*slideDirectiveBlock
	rest := raw
	for {
		start := strings.Index(rest, "<!--")
		if start < 0 {
			break
		}
		after := rest[start+4:]
		end := strings.Index(after, "-->")
		if end < 0 {
			break
		}
		comment := strings.TrimSpace(after[:end])
		rest = after[end+3:]
		colon := strings.IndexByte(comment, ':')
		if colon <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(comment[:colon]))
		val := strings.TrimSpace(comment[colon+1:])
		switch key {
		case "kicker", "header", "footer":
			dirs = append(dirs, &slideDirectiveBlock{
				Kind:  key,
				Value: val,
				Line:  line,
			})
		}
	}
	return dirs
}

func buildSlides(blocks []Block, lineOffset int) ([]Block, []Slide) {
	cleanBlocks := make([]Block, 0, len(blocks))
	slides := []Slide{{Index: 1, Line: lineOffset}}

	for _, blk := range blocks {
		switch node := blk.(type) {
		case *slideDirectiveBlock:
			cur := &slides[len(slides)-1]
			switch node.Kind {
			case "kicker":
				cur.Kicker = node.Value
			case "header":
				if ov, err := ParseSlideChromeOverride(node.Value); err == nil {
					cur.Header = ov
				}
			case "footer":
				if ov, err := ParseSlideChromeOverride(node.Value); err == nil {
					cur.Footer = ov
				}
			}
		case *ThematicBreakBlock:
			cleanBlocks = append(cleanBlocks, node)
			slides = append(slides, Slide{
				Index: len(slides) + 1,
				Line:  node.Line,
			})
		default:
			cleanBlocks = append(cleanBlocks, blk)
			cur := &slides[len(slides)-1]
			cur.Blocks = append(cur.Blocks, blk)
		}
	}
	return cleanBlocks, slides
}

// ParseSlideChromeOverride parses Running Chrome shorthand ("false", "true", "left | center | right",
// "left: ... | rule: true", or JSON/YAML object "{...}") into a SlideChromeOverride.
func ParseSlideChromeOverride(raw string) (SlideChromeOverride, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return SlideChromeOverride{}, fmt.Errorf("empty running chrome specification")
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "false", "off", "none":
		disabled := false
		return SlideChromeOverride{Set: true, Enabled: &disabled, Raw: trimmed}, nil
	case "true", "on":
		enabled := true
		return SlideChromeOverride{Set: true, Enabled: &enabled, Raw: trimmed}, nil
	}

	if strings.HasPrefix(trimmed, "{") {
		var obj struct {
			Enabled   *bool   `yaml:"enabled"`
			Left      *string `yaml:"left"`
			Center    *string `yaml:"center"`
			Right     *string `yaml:"right"`
			SkipFirst *bool   `yaml:"skip_first"`
			Rule      *bool   `yaml:"rule"`
		}
		if err := yaml.Unmarshal([]byte(trimmed), &obj); err != nil {
			return SlideChromeOverride{}, fmt.Errorf("invalid running chrome mapping: %w", err)
		}
		enabled := true
		if obj.Enabled != nil {
			enabled = *obj.Enabled
		}
		ov := SlideChromeOverride{
			Set:       true,
			Enabled:   &enabled,
			SkipFirst: obj.SkipFirst,
			Rule:      obj.Rule,
			Raw:       trimmed,
		}
		if obj.Left != nil {
			ov.Left = *obj.Left
			ov.HasLeft = true
		}
		if obj.Center != nil {
			ov.Center = *obj.Center
			ov.HasCenter = true
		}
		if obj.Right != nil {
			ov.Right = *obj.Right
			ov.HasRight = true
		}
		return ov, nil
	}

	enabled := true
	ov := SlideChromeOverride{
		Set:     true,
		Enabled: &enabled,
		Raw:     trimmed,
	}
	if strings.Contains(trimmed, "|") {
		parts := strings.Split(trimmed, "|")
		if hasNamedChromePipePart(parts) {
			for _, part := range parts {
				p := strings.TrimSpace(part)
				lp := strings.ToLower(p)
				switch {
				case strings.HasPrefix(lp, "left:"):
					ov.Left = strings.TrimSpace(p[len("left:"):])
					ov.HasLeft = true
				case strings.HasPrefix(lp, "center:"):
					ov.Center = strings.TrimSpace(p[len("center:"):])
					ov.HasCenter = true
				case strings.HasPrefix(lp, "right:"):
					ov.Right = strings.TrimSpace(p[len("right:"):])
					ov.HasRight = true
				case strings.HasPrefix(lp, "rule:"):
					if b, ok := parseChromeBool(p[len("rule:"):]); ok {
						ov.Rule = &b
					}
				case strings.HasPrefix(lp, "skip_first:"):
					if b, ok := parseChromeBool(p[len("skip_first:"):]); ok {
						ov.SkipFirst = &b
					}
				case strings.HasPrefix(lp, "enabled:"):
					if b, ok := parseChromeBool(p[len("enabled:"):]); ok {
						ov.Enabled = &b
					}
				}
			}
			return ov, nil
		}
		ov.ReplaceAllSlots = true
		ov.HasLeft = true
		ov.HasCenter = true
		ov.HasRight = true
		if len(parts) > 0 {
			ov.Left = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			ov.Center = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			ov.Right = strings.TrimSpace(strings.Join(parts[2:], "|"))
		}
		return ov, nil
	}
	ov.Center = trimmed
	ov.HasCenter = true
	ov.ReplaceAllSlots = true
	return ov, nil
}

func hasNamedChromePipePart(parts []string) bool {
	for _, part := range parts {
		lp := strings.ToLower(strings.TrimSpace(part))
		for _, key := range []string{"left:", "center:", "right:", "rule:", "skip_first:", "enabled:"} {
			if strings.HasPrefix(lp, key) {
				return true
			}
		}
	}
	return false
}

func parseChromeBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "on", "yes", "1":
		return true, true
	case "false", "off", "no", "0":
		return false, true
	default:
		return false, false
	}
}

func firstSlideTitle(blocks []Block) string {
	for _, blk := range blocks {
		if h, ok := blk.(*HeadingBlock); ok {
			return strings.TrimSpace(inlinesPlainText(h.Inlines))
		}
	}
	return ""
}

type mathBlockNode struct {
	ast.BaseBlock
	Body   strings.Builder
	Closed bool
	Line   int
}

func (n *mathBlockNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"LaTeX": n.Body.String()}, nil)
}

func (*mathBlockNode) Kind() ast.NodeKind {
	return kindMathBlock
}

func (*mathBlockNode) IsRaw() bool {
	return true
}

type mathInlineNode struct {
	ast.BaseInline
	LaTeX   string
	Display bool
}

func (n *mathInlineNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"LaTeX": n.LaTeX}, nil)
}

func (*mathInlineNode) Kind() ast.NodeKind {
	return kindMathInline
}

type mathBlockParser struct {
	lineOffset int
}

func (*mathBlockParser) Trigger() []byte {
	return []byte{'$'}
}

func (b *mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos+1 >= len(line) || line[pos] != '$' || line[pos+1] != '$' {
		return nil, parser.NoChildren
	}

	rest := line[pos+2:]
	lineNum, _ := reader.Position()
	absLine := b.lineOffset + lineNum

	if idx := findUnescapedDoubleDollar(rest); idx >= 0 {
		if !util.IsBlank(rest[idx+2:]) {
			return nil, parser.NoChildren
		}
		node := &mathBlockNode{Closed: true, Line: absLine}
		node.Body.Write(bytes.TrimSpace(rest[:idx]))
		advanceLineEnd(reader, line, segment)
		return node, parser.NoChildren
	}

	node := &mathBlockNode{Closed: false, Line: absLine}
	firstLine := strings.TrimSpace(string(rest))
	if firstLine != "" {
		node.Body.WriteString(firstLine)
	}
	advanceLineEnd(reader, line, segment)
	return node, parser.NoChildren
}

func (*mathBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	mb := node.(*mathBlockNode)
	if mb.Closed {
		return parser.Close
	}
	line, segment := reader.PeekLine()
	trimmed := bytes.TrimRight(line, " \t\r\n")
	if bytes.HasSuffix(trimmed, []byte("$$")) && !isEscapedOffset(trimmed, len(trimmed)-2) {
		before := strings.TrimSpace(string(trimmed[:len(trimmed)-2]))
		if before != "" {
			if mb.Body.Len() > 0 {
				mb.Body.WriteByte('\n')
			}
			mb.Body.WriteString(before)
		}
		advanceLineEnd(reader, line, segment)
		mb.Closed = true
		return parser.Close
	}
	content := strings.TrimRight(string(line), "\r\n")
	if mb.Body.Len() > 0 {
		mb.Body.WriteByte('\n')
	}
	mb.Body.WriteString(content)
	advanceLineEnd(reader, line, segment)
	return parser.Continue | parser.NoChildren
}

func (*mathBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (*mathBlockParser) CanInterruptParagraph() bool {
	return true
}

func (*mathBlockParser) CanAcceptIndentedLine() bool {
	return false
}

func advanceLineEnd(reader text.Reader, line []byte, segment text.Segment) {
	newline := 0
	if len(line) > 0 && line[len(line)-1] == '\n' {
		newline = 1
	}
	reader.Advance(segment.Stop - segment.Start - newline + segment.Padding)
}

func isEscapedOffset(b []byte, idx int) bool {
	backslashes := 0
	for i := idx - 1; i >= 0 && b[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func findUnescapedDoubleDollar(b []byte) int {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '$' && b[i+1] == '$' && !isEscapedOffset(b, i) {
			return i
		}
	}
	return -1
}

type mathInlineParser struct{}

func (*mathInlineParser) Trigger() []byte {
	return []byte{'$'}
}

func (*mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 2 || line[0] != '$' {
		return nil
	}
	if line[1] == '$' {
		return parseInlineDoubleDollar(block)
	}
	// Pandoc currency-safe rule 1: no whitespace immediately after opening '$'.
	if isMathSpace(line[1]) {
		return nil
	}

	var buf strings.Builder
	j := 1
	curLine := line
	for {
		escaped := false
		for ; j < len(curLine); j++ {
			c := curLine[j]
			if escaped {
				buf.WriteByte('\\')
				buf.WriteByte(c)
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '`' {
				if hasInlineCodeSpan(block, curLine, j) {
					return nil
				}
			}
			if c == '$' {
				// Pandoc currency-safe rule 2: no whitespace immediately before closing '$'.
				if j == 0 || isMathSpace(curLine[j-1]) {
					return nil
				}
				// Pandoc currency-safe rule 3: closing '$' must not be followed by a digit.
				if j+1 < len(curLine) && curLine[j+1] >= '0' && curLine[j+1] <= '9' {
					return nil
				}
				latex := buf.String()
				if latex == "" {
					return nil
				}
				block.Advance(j + 1)
				return &mathInlineNode{LaTeX: latex, Display: false}
			}
			buf.WriteByte(c)
		}
		if escaped {
			buf.WriteByte('\\')
		}
		block.AdvanceLine()
		nextLine, _ := block.PeekLine()
		if nextLine == nil {
			return nil
		}
		curLine = nextLine
		j = 0
	}
}

func parseInlineDoubleDollar(block text.Reader) ast.Node {
	line, _ := block.PeekLine()
	var buf strings.Builder
	j := 2
	curLine := line
	for {
		escaped := false
		for ; j < len(curLine); j++ {
			c := curLine[j]
			if escaped {
				buf.WriteByte('\\')
				buf.WriteByte(c)
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '$' && j+1 < len(curLine) && curLine[j+1] == '$' {
				latex := strings.TrimSpace(buf.String())
				if latex == "" {
					return nil
				}
				block.Advance(j + 2)
				return &mathInlineNode{LaTeX: latex, Display: true}
			}
			buf.WriteByte(c)
		}
		if escaped {
			buf.WriteByte('\\')
		}
		block.AdvanceLine()
		nextLine, _ := block.PeekLine()
		if nextLine == nil {
			return nil
		}
		curLine = nextLine
		j = 0
	}
}

func isMathSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func hasInlineCodeSpan(block text.Reader, curLine []byte, j int) bool {
	opener := 0
	for k := j; k < len(curLine) && curLine[k] == '`'; k++ {
		opener++
	}
	if opener == 0 {
		return false
	}
	savedLine, savedPos := block.Position()
	defer block.SetPosition(savedLine, savedPos)
	k := j + opener
	line := curLine
	for {
		for k < len(line) {
			if line[k] == '`' {
				run := 0
				for k+run < len(line) && line[k+run] == '`' {
					run++
				}
				if run == opener {
					return true
				}
				k += run
				continue
			}
			k++
		}
		block.AdvanceLine()
		nextLine, _ := block.PeekLine()
		if nextLine == nil {
			return false
		}
		line = nextLine
		k = 0
	}
}

type fencedContainerNode struct {
	ast.BaseBlock
	ContainerKind string
	Arg           string
	FenceLen      int
	Line          int
}

func (n *fencedContainerNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Kind": n.ContainerKind,
		"Arg":  n.Arg,
	}, nil)
}

func (*fencedContainerNode) Kind() ast.NodeKind {
	return kindFencedContainer
}

type fencedContainerBlockParser struct {
	lineOffset int
}

func (*fencedContainerBlockParser) Trigger() []byte {
	return []byte{':'}
}

func (b *fencedContainerBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos >= len(line) || line[pos] != ':' {
		return nil, parser.NoChildren
	}
	i := pos
	for i < len(line) && line[i] == ':' {
		i++
	}
	fenceLen := i - pos
	if fenceLen < 3 {
		return nil, parser.NoChildren
	}
	rest := strings.TrimSpace(string(line[i:]))
	if rest == "" {
		return nil, parser.NoChildren
	}

	kind := rest
	arg := ""
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		kind = rest[:idx]
		arg = strings.TrimSpace(rest[idx+1:])
	}
	kind = strings.ToLower(kind)
	switch kind {
	case "note", "wide", "kpi", "columns", "column":
	default:
		return nil, parser.NoChildren
	}

	lineNum, _ := reader.Position()
	newline := 0
	if len(line) > 0 && line[len(line)-1] == '\n' {
		newline = 1
	}
	reader.Advance(segment.Stop - segment.Start - newline + segment.Padding)

	return &fencedContainerNode{
		ContainerKind: kind,
		Arg:           arg,
		FenceLen:      fenceLen,
		Line:          b.lineOffset + lineNum,
	}, parser.HasChildren
}

func (*fencedContainerBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	fcNode := node.(*fencedContainerNode)

	opened := pc.OpenedBlocks()
	afterSelf := false
	for _, ob := range opened {
		if ob.Node == node {
			afterSelf = true
			continue
		}
		if afterSelf {
			if _, isFC := ob.Node.(*fencedContainerNode); isFC || ob.Node.IsRaw() {
				return parser.Continue | parser.HasChildren
			}
		}
	}

	line, segment := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w < 4 && pos >= 0 && pos < len(line) && line[pos] == ':' {
		i := pos
		for i < len(line) && line[i] == ':' {
			i++
		}
		if i-pos >= fcNode.FenceLen && util.IsBlank(line[i:]) {
			newline := 0
			if len(line) > 0 && line[len(line)-1] == '\n' {
				newline = 1
			}
			reader.Advance(segment.Stop - segment.Start - newline + segment.Padding)
			return parser.Close
		}
	}
	return parser.Continue | parser.HasChildren
}

func (*fencedContainerBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (*fencedContainerBlockParser) CanInterruptParagraph() bool {
	return true
}

func (*fencedContainerBlockParser) CanAcceptIndentedLine() bool {
	return false
}

func tryMarginNoteFromBlockquote(blocks []Block, line int) (*MarginNoteBlock, bool) {
	if len(blocks) == 0 {
		return nil, false
	}
	firstPara, ok := blocks[0].(*ParagraphBlock)
	if !ok || len(firstPara.Inlines) == 0 {
		return nil, false
	}
	emp, ok := firstPara.Inlines[0].(*EmphasisInline)
	if !ok || !emp.Strong {
		return nil, false
	}
	rawLabel := strings.TrimSpace(inlinesPlainText(emp.Children))
	cleanLabel := strings.TrimSpace(strings.TrimSuffix(rawLabel, ":"))
	if !strings.EqualFold(cleanLabel, "margin note") && !strings.EqualFold(cleanLabel, "nota de margem") {
		return nil, false
	}

	rest := firstPara.Inlines[1:]
	if len(rest) > 0 {
		if t, ok := rest[0].(*TextInline); ok && strings.HasPrefix(t.Text, ":") {
			rest[0] = &TextInline{Text: strings.TrimPrefix(t.Text, ":")}
		}
	}
	rest = trimLeadingInlineSpace(rest)

	outBlocks := make([]Block, 0, len(blocks))
	if len(rest) > 0 {
		outBlocks = append(outBlocks, &ParagraphBlock{
			Inlines: rest,
			Line:    firstPara.Line,
		})
	}
	outBlocks = append(outBlocks, blocks[1:]...)

	return &MarginNoteBlock{
		Blocks: outBlocks,
		Line:   line,
	}, true
}

func convertColumns(node *fencedContainerNode, src []byte, docDir string, lineOffset int, rawSidenotes *[]*sidenoteDefNode) [][]Block {
	var cols [][]Block
	hasExplicitCols := false
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if fc, ok := child.(*fencedContainerNode); ok && fc.ContainerKind == "column" {
			hasExplicitCols = true
			colBlocks := convertBlocks(fc, src, docDir, lineOffset, rawSidenotes)
			colBlocks = attachCaptionsAndSources(colBlocks)
			cols = append(cols, colBlocks)
		}
	}
	if hasExplicitCols {
		return cols
	}

	childBlocks := convertBlocks(node, src, docDir, lineOffset, rawSidenotes)
	childBlocks = attachCaptionsAndSources(childBlocks)
	var current []Block
	for _, blk := range childBlocks {
		if _, isBreak := blk.(*ThematicBreakBlock); isBreak {
			cols = append(cols, current)
			current = nil
			continue
		}
		current = append(current, blk)
	}
	if len(current) > 0 || len(cols) > 0 {
		cols = append(cols, current)
	}
	return cols
}

func parseKPIItems(blocks []Block) []KPIItem {
	var items []KPIItem
	for _, blk := range blocks {
		switch node := blk.(type) {
		case *TableBlock:
			items = append(items, kpiItemsFromTable(node)...)
		case *ListBlock:
			for _, itemBlocks := range node.Items {
				items = append(items, parseKPIItems(itemBlocks)...)
			}
		case *ParagraphBlock:
			for _, line := range splitInlineLines(node.Inlines) {
				if item, ok := parseKPILine(line); ok {
					items = append(items, item)
				}
			}
		}
	}
	return items
}

func kpiItemsFromTable(tbl *TableBlock) []KPIItem {
	if len(tbl.Rows) == 0 {
		return nil
	}
	if len(tbl.Rows) == 1 && len(tbl.Headers) >= 2 && !isRowOrientedKPIHeader(tbl.Headers) {
		items := make([]KPIItem, 0, len(tbl.Headers))
		for i, hdr := range tbl.Headers {
			val := ""
			if i < len(tbl.Rows[0]) {
				val = strings.TrimSpace(tbl.Rows[0][i].Plain)
			}
			label := strings.TrimSpace(hdr.Plain)
			if label != "" || val != "" {
				items = append(items, KPIItem{Label: label, Value: val})
			}
		}
		return items
	}

	items := make([]KPIItem, 0, len(tbl.Rows))
	for _, row := range tbl.Rows {
		if len(row) == 0 {
			continue
		}
		label := strings.TrimSpace(row[0].Plain)
		value := ""
		note := ""
		if len(row) > 1 {
			value = strings.TrimSpace(row[1].Plain)
		}
		if len(row) > 2 {
			note = strings.TrimSpace(row[2].Plain)
		}
		if label != "" || value != "" {
			items = append(items, KPIItem{Label: label, Value: value, Note: note})
		}
	}
	return items
}

func isRowOrientedKPIHeader(headers []TableCell) bool {
	if len(headers) < 2 {
		return false
	}
	h0 := strings.ToLower(strings.TrimSpace(headers[0].Plain))
	h1 := strings.ToLower(strings.TrimSpace(headers[1].Plain))
	switch h0 {
	case "metric", "kpi", "label", "indicator", "indicador", "métrica", "metrica", "item", "name":
		return true
	}
	switch h1 {
	case "value", "valor":
		return true
	}
	return false
}

func splitInlineLines(inlines []Inline) []string {
	var lines []string
	var cur strings.Builder
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			lines = append(lines, s)
		}
		cur.Reset()
	}
	for _, in := range inlines {
		switch node := in.(type) {
		case *SoftBreakInline, *HardBreakInline:
			flush()
		default:
			text := inlinesPlainText([]Inline{node})
			if strings.Contains(text, "\n") {
				parts := strings.Split(text, "\n")
				for i, p := range parts {
					if i > 0 {
						flush()
					}
					cur.WriteString(p)
				}
			} else {
				cur.WriteString(text)
			}
		}
	}
	flush()
	return lines
}

func parseKPILine(line string) (KPIItem, bool) {
	trimmed := strings.TrimSpace(line)
	for _, bullet := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(trimmed, bullet) {
			trimmed = strings.TrimSpace(trimmed[len(bullet):])
			break
		}
	}
	idx := strings.IndexByte(trimmed, ':')
	if idx <= 0 {
		return KPIItem{}, false
	}
	label := strings.TrimSpace(strings.Trim(trimmed[:idx], "*_"))
	rest := strings.TrimSpace(trimmed[idx+1:])
	if label == "" || rest == "" {
		return KPIItem{}, false
	}
	value := rest
	note := ""
	if pipeIdx := strings.IndexByte(rest, '|'); pipeIdx >= 0 {
		value = strings.TrimSpace(rest[:pipeIdx])
		note = strings.TrimSpace(rest[pipeIdx+1:])
	}
	return KPIItem{Label: label, Value: value, Note: note}, true
}

type sidenoteDefNode struct {
	ast.BaseBlock
	ID   string
	Line int
}

func (n *sidenoteDefNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"ID": n.ID}, nil)
}

func (*sidenoteDefNode) Kind() ast.NodeKind {
	return kindSidenoteDef
}

type sidenoteRefNode struct {
	ast.BaseInline
	ID   string
	Line int
}

func (n *sidenoteRefNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"ID": n.ID}, nil)
}

func (*sidenoteRefNode) Kind() ast.NodeKind {
	return kindSidenoteRef
}

type sidenoteDefBlockParser struct {
	lineOffset int
}

func (*sidenoteDefBlockParser) Trigger() []byte {
	return []byte{'['}
}

func (b *sidenoteDefBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos+2 >= len(line) || line[pos] != '[' || line[pos+1] != '^' {
		return nil, parser.NoChildren
	}
	closeRel := bytes.IndexByte(line[pos+2:], ']')
	if closeRel <= 0 {
		return nil, parser.NoChildren
	}
	idBytes := line[pos+2 : pos+2+closeRel]
	if bytes.ContainsAny(idBytes, " \t\r\n[") {
		return nil, parser.NoChildren
	}
	colonPos := pos + 2 + closeRel + 1
	if colonPos >= len(line) || line[colonPos] != ':' {
		return nil, parser.NoChildren
	}

	lineNum, _ := reader.Position()
	node := &sidenoteDefNode{
		ID:   string(idBytes),
		Line: b.lineOffset + lineNum,
	}

	padding := segment.Padding
	advance := colonPos + 1 - padding
	if advance >= len(line) {
		reader.Advance(advance)
		return node, parser.NoChildren
	}
	reader.AdvanceAndSetPadding(advance, padding)
	return node, parser.HasChildren
}

func (*sidenoteDefBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if util.IsBlank(line) {
		return parser.Continue | parser.HasChildren
	}
	childpos, padding := util.IndentPosition(line, reader.LineOffset(), 4)
	if childpos < 0 {
		return parser.Close
	}
	reader.AdvanceAndSetPadding(childpos, padding)
	return parser.Continue | parser.HasChildren
}

func (*sidenoteDefBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (*sidenoteDefBlockParser) CanInterruptParagraph() bool {
	return true
}

func (*sidenoteDefBlockParser) CanAcceptIndentedLine() bool {
	return false
}

type sidenoteRefInlineParser struct {
	lineOffset int
}

func (*sidenoteRefInlineParser) Trigger() []byte {
	return []byte{'['}
}

func (s *sidenoteRefInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, seg := block.PeekLine()
	if len(line) < 4 || line[0] != '[' || line[1] != '^' {
		return nil
	}
	closeIdx := -1
	for i := 2; i < len(line); i++ {
		c := line[i]
		if c == ']' {
			closeIdx = i
			break
		}
		if c == '[' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			return nil
		}
	}
	if closeIdx <= 2 {
		return nil
	}
	if closeIdx+1 < len(line) && line[closeIdx+1] == '(' {
		return nil
	}
	src := block.Source()
	absLine := s.lineOffset
	if seg.Start >= 0 && seg.Start <= len(src) {
		absLine += bytes.Count(src[:seg.Start], []byte("\n"))
	}
	block.Advance(closeIdx + 1)
	return &sidenoteRefNode{
		ID:   string(line[2:closeIdx]),
		Line: absLine,
	}
}

func resolveSidenotes(d *SourceDocument, rawDefs []*sidenoteDefNode, src []byte, docDir string, lineOffset int) {
	defs := make([]SidenoteDef, 0, len(rawDefs))
	defIndexByID := make(map[string]int, len(rawDefs))

	for _, raw := range rawDefs {
		var nested []*sidenoteDefNode
		blocks := convertBlocks(raw, src, docDir, lineOffset, &nested)
		blocks = attachCaptionsAndSources(blocks)
		title, bodyBlocks := extractSidenoteTitle(blocks)
		var firstInlines []Inline
		if len(bodyBlocks) > 0 {
			if p, ok := bodyBlocks[0].(*ParagraphBlock); ok {
				firstInlines = p.Inlines
			}
		}
		defIndexByID[raw.ID] = len(defs)
		defs = append(defs, SidenoteDef{
			ID:      raw.ID,
			Title:   title,
			Blocks:  bodyBlocks,
			Inlines: firstInlines,
			Line:    raw.Line,
		})
	}

	nextNum := 1
	var visitInlines func([]Inline)
	visitInlines = func(inlines []Inline) {
		for _, in := range inlines {
			switch node := in.(type) {
			case *EmphasisInline:
				visitInlines(node.Children)
			case *LinkInline:
				visitInlines(node.Children)
			case *SidenoteRefInline:
				idx, exists := defIndexByID[node.ID]
				if exists {
					if !defs[idx].Referenced {
						defs[idx].Referenced = true
						defs[idx].Number = nextNum
						nextNum++
					}
					node.Number = defs[idx].Number
					node.Defined = true
				}
				d.SidenoteRefs = append(d.SidenoteRefs, SidenoteRef{
					ID:      node.ID,
					Number:  node.Number,
					Line:    node.Line,
					Defined: node.Defined,
				})
			}
		}
	}

	walkDocInlines(d.Blocks, visitInlines)
	// Also walk referenced sidenote definitions in case a sidenote cites another sidenote.
	for i := 0; i < len(defs); i++ {
		if defs[i].Referenced {
			walkDocInlines(defs[i].Blocks, visitInlines)
		}
	}

	d.Sidenotes = defs
}

func extractSidenoteTitle(blocks []Block) (string, []Block) {
	if len(blocks) == 0 {
		return "", blocks
	}
	firstPara, ok := blocks[0].(*ParagraphBlock)
	if !ok || len(firstPara.Inlines) == 0 {
		return "", blocks
	}
	emp, ok := firstPara.Inlines[0].(*EmphasisInline)
	if !ok || !emp.Strong {
		return "", blocks
	}
	rest := trimLeadingInlineSpace(firstPara.Inlines[1:])
	if len(rest) == 0 && len(blocks) == 1 {
		return "", blocks
	}
	title := strings.TrimSpace(inlinesPlainText(emp.Children))
	if title == "" {
		return "", blocks
	}
	if len(rest) == 0 {
		return title, blocks[1:]
	}
	out := make([]Block, len(blocks))
	copy(out, blocks)
	out[0] = &ParagraphBlock{
		Inlines: rest,
		Line:    firstPara.Line,
	}
	return title, out
}

func trimLeadingInlineSpace(inlines []Inline) []Inline {
	out := make([]Inline, 0, len(inlines))
	trimmed := false
	for _, in := range inlines {
		if trimmed {
			out = append(out, in)
			continue
		}
		switch node := in.(type) {
		case *SoftBreakInline, *HardBreakInline:
			continue
		case *TextInline:
			s := strings.TrimLeft(node.Text, " \t\n\r")
			if s == "" {
				continue
			}
			trimmed = true
			out = append(out, &TextInline{Text: s})
		default:
			trimmed = true
			out = append(out, in)
		}
	}
	return out
}

func walkDocInlines(blocks []Block, fn func([]Inline)) {
	for _, blk := range blocks {
		switch node := blk.(type) {
		case *HeadingBlock:
			fn(node.Inlines)
		case *ParagraphBlock:
			fn(node.Inlines)
		case *ListBlock:
			for _, item := range node.Items {
				walkDocInlines(item, fn)
			}
		case *BlockquoteBlock:
			walkDocInlines(node.Blocks, fn)
		case *MarginNoteBlock:
			walkDocInlines(node.Blocks, fn)
		case *WideBlock:
			walkDocInlines(node.Blocks, fn)
		case *ColumnsBlock:
			for _, col := range node.Columns {
				walkDocInlines(col, fn)
			}
		case *TableBlock:
			for _, cell := range node.Headers {
				fn(cell.Inlines)
			}
			for _, row := range node.Rows {
				for _, cell := range row {
					fn(cell.Inlines)
				}
			}
		}
	}
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
