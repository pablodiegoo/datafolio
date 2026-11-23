package html

import (
	"encoding/base64"
	"fmt"
	stdhtml "html"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/config"
	"github.com/pablodiegoo/datafolio/internal/doc"
	"github.com/pablodiegoo/datafolio/internal/school"
)

type renderState struct {
	labels    school.Labels
	tableNum  int
	figureNum int
}

// Render compiles a SourceDocument into a standalone, offline HTML5 web monograph.
func Render(d *doc.SourceDocument, cfg config.Config, tok school.SchoolTokens, labels school.Labels) (string, error) {
	if d == nil {
		return "", fmt.Errorf("cannot render nil SourceDocument")
	}

	title := strings.TrimSpace(d.Meta.Title)
	if title == "" && d.Path != "" {
		base := filepath.Base(d.Path)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if title == "" {
		title = "DataFolio Monograph"
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString(fmt.Sprintf("<html lang=%q data-theme=%q>\n", cfg.Lang, tok.Theme))
	b.WriteString("<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>" + stdhtml.EscapeString(title) + "</title>\n")
	b.WriteString("<style>\n")
	b.WriteString(buildStylesheet(tok))
	b.WriteString("\n</style>\n")
	b.WriteString("</head>\n")
	b.WriteString("<body>\n")
	b.WriteString("<main class=\"df-page\">\n")
	b.WriteString("<article class=\"df-article\">\n")

	renderHeader(&b, d.Meta)

	state := &renderState{labels: labels}
	for _, blk := range d.Blocks {
		if err := renderBlock(&b, blk, state); err != nil {
			return "", err
		}
	}

	b.WriteString("</article>\n")
	b.WriteString("</main>\n")
	b.WriteString("</body>\n")
	b.WriteString("</html>\n")

	return b.String(), nil
}

func renderHeader(b *strings.Builder, m doc.Metadata) {
	hasHeader := m.Kicker != "" || m.Title != "" || m.Subtitle != "" || m.Author != "" || m.Date != ""
	if !hasHeader {
		return
	}
	b.WriteString("<header class=\"df-header df-main\">\n")
	if m.Kicker != "" {
		b.WriteString("<p class=\"df-kicker\">" + stdhtml.EscapeString(m.Kicker) + "</p>\n")
	}
	if m.Title != "" {
		b.WriteString("<h1 class=\"df-title\">" + stdhtml.EscapeString(m.Title) + "</h1>\n")
	}
	if m.Subtitle != "" {
		b.WriteString("<p class=\"df-subtitle\">" + stdhtml.EscapeString(m.Subtitle) + "</p>\n")
	}
	if m.Author != "" || m.Date != "" {
		b.WriteString("<p class=\"df-byline\">")
		if m.Author != "" {
			b.WriteString("<span class=\"df-author\">" + stdhtml.EscapeString(m.Author) + "</span>")
		}
		if m.Author != "" && m.Date != "" {
			b.WriteString(" <span class=\"df-sep\" aria-hidden=\"true\">&middot;</span> ")
		}
		if m.Date != "" {
			b.WriteString("<span class=\"df-date\">" + stdhtml.EscapeString(m.Date) + "</span>")
		}
		b.WriteString("</p>\n")
	}
	b.WriteString("</header>\n")
}

func renderBlock(b *strings.Builder, blk doc.Block, state *renderState) error {
	switch node := blk.(type) {
	case *doc.HeadingBlock:
		level := node.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		idAttr := ""
		if node.ID != "" {
			idAttr = fmt.Sprintf(" id=%q", node.ID)
		}
		b.WriteString(fmt.Sprintf("<h%d class=\"df-main\"%s>", level, idAttr))
		renderInlines(b, node.Inlines)
		b.WriteString(fmt.Sprintf("</h%d>\n", level))
	case *doc.ParagraphBlock:
		b.WriteString("<p class=\"df-main\">")
		renderInlines(b, node.Inlines)
		b.WriteString("</p>\n")
	case *doc.ListBlock:
		tag := "ul"
		if node.Ordered {
			tag = "ol"
		}
		b.WriteString(fmt.Sprintf("<%s class=\"df-main\">\n", tag))
		for _, item := range node.Items {
			b.WriteString("<li>")
			if len(item) == 1 {
				if p, ok := item[0].(*doc.ParagraphBlock); ok {
					renderInlines(b, p.Inlines)
					b.WriteString("</li>\n")
					continue
				}
			}
			for _, sub := range item {
				if err := renderBlock(b, sub, state); err != nil {
					return err
				}
			}
			b.WriteString("</li>\n")
		}
		b.WriteString(fmt.Sprintf("</%s>\n", tag))
	case *doc.BlockquoteBlock:
		b.WriteString("<blockquote class=\"df-main df-blockquote\">\n")
		for _, sub := range node.Blocks {
			if err := renderBlock(b, sub, state); err != nil {
				return err
			}
		}
		b.WriteString("</blockquote>\n")
	case *doc.CodeBlock:
		classAttr := ""
		if node.Language != "" {
			classAttr = fmt.Sprintf(" class=%q", "language-"+node.Language)
		}
		b.WriteString("<pre class=\"df-main df-code\"><code" + classAttr + ">")
		b.WriteString(stdhtml.EscapeString(node.Code))
		b.WriteString("</code></pre>\n")
	case *doc.ThematicBreakBlock:
		b.WriteString("<hr class=\"df-main df-rule\">\n")
	case *doc.TableBlock:
		renderTable(b, node, state)
	case *doc.FigureBlock:
		if err := renderFigure(b, node, state); err != nil {
			return err
		}
	}
	return nil
}

func renderTable(b *strings.Builder, node *doc.TableBlock, state *renderState) {
	state.tableNum++
	b.WriteString("<figure class=\"df-main df-table-wrap\">\n")
	if node.Caption != "" {
		b.WriteString(fmt.Sprintf(
			"<figcaption class=\"df-caption\"><span class=\"df-label\">%s %d:</span> %s</figcaption>\n",
			stdhtml.EscapeString(state.labels.Table),
			state.tableNum,
			stdhtml.EscapeString(node.Caption),
		))
	}
	b.WriteString("<table class=\"df-booktabs\">\n")
	if len(node.Headers) > 0 {
		b.WriteString("<thead><tr>")
		for col, cell := range node.Headers {
			b.WriteString("<th" + tableCellAttr(node.Alignments, node.NumericCols, col) + ">")
			renderInlines(b, cell.Inlines)
			b.WriteString("</th>")
		}
		b.WriteString("</tr></thead>\n")
	}
	if len(node.Rows) > 0 {
		b.WriteString("<tbody>\n")
		for _, row := range node.Rows {
			b.WriteString("<tr>")
			for col, cell := range row {
				b.WriteString("<td" + tableCellAttr(node.Alignments, node.NumericCols, col) + ">")
				renderInlines(b, cell.Inlines)
				b.WriteString("</td>")
			}
			b.WriteString("</tr>\n")
		}
		b.WriteString("</tbody>\n")
	}
	b.WriteString("</table>\n")
	if node.Source != "" {
		b.WriteString(fmt.Sprintf(
			"<p class=\"df-source\"><span class=\"df-source-label\">%s:</span> %s</p>\n",
			stdhtml.EscapeString(state.labels.Source),
			stdhtml.EscapeString(node.Source),
		))
	}
	b.WriteString("</figure>\n")
}

func tableCellAttr(aligns []doc.ColumnAlign, numeric []bool, col int) string {
	isNum := col < len(numeric) && numeric[col]
	align := doc.AlignDefault
	if col < len(aligns) {
		align = aligns[col]
	}

	var attrs strings.Builder
	if isNum {
		attrs.WriteString(` class="num"`)
	}
	switch align {
	case doc.AlignCenter:
		attrs.WriteString(` style="text-align: center"`)
	case doc.AlignRight:
		if !isNum {
			attrs.WriteString(` style="text-align: right"`)
		}
	case doc.AlignLeft:
		if isNum {
			attrs.WriteString(` style="text-align: left"`)
		}
	}
	return attrs.String()
}

func renderFigure(b *strings.Builder, node *doc.FigureBlock, state *renderState) error {
	state.figureNum++
	mediaHTML, err := resolveFigureMediaHTML(node)
	if err != nil {
		return err
	}

	b.WriteString("<figure class=\"df-main df-figure\">\n")
	b.WriteString("<div class=\"df-figure-media\">\n")
	b.WriteString(mediaHTML)
	b.WriteString("\n</div>\n")

	if node.Caption != "" || node.Source != "" {
		b.WriteString("<figcaption class=\"df-caption\">")
		if node.Caption != "" {
			b.WriteString(fmt.Sprintf(
				"<span class=\"df-label\">%s %d:</span> %s",
				stdhtml.EscapeString(state.labels.Figure),
				state.figureNum,
				stdhtml.EscapeString(node.Caption),
			))
		}
		if node.Caption != "" && node.Source != "" {
			b.WriteByte(' ')
		}
		if node.Source != "" {
			b.WriteString(fmt.Sprintf(
				"<span class=\"df-source\"><span class=\"df-source-label\">%s:</span> %s</span>",
				stdhtml.EscapeString(state.labels.Source),
				stdhtml.EscapeString(node.Source),
			))
		}
		b.WriteString("</figcaption>\n")
	}

	b.WriteString("</figure>\n")
	return nil
}

func resolveFigureMediaHTML(node *doc.FigureBlock) (string, error) {
	if node.ResolvedPath == "" {
		return fmt.Sprintf("<img src=%q alt=%q>", sanitizeURL(node.Src), node.Caption), nil
	}

	raw, err := os.ReadFile(node.ResolvedPath)
	if err != nil {
		return "", fmt.Errorf("figure %q: %w", node.Src, err)
	}

	ext := strings.ToLower(filepath.Ext(node.ResolvedPath))
	switch ext {
	case ".svg":
		return sanitizeInlineSVG(raw, node.Src)
	case ".png":
		return fmt.Sprintf("<img src=\"data:image/png;base64,%s\" alt=%q>", base64.StdEncoding.EncodeToString(raw), node.Caption), nil
	case ".jpg", ".jpeg":
		return fmt.Sprintf("<img src=\"data:image/jpeg;base64,%s\" alt=%q>", base64.StdEncoding.EncodeToString(raw), node.Caption), nil
	case ".webp":
		return fmt.Sprintf("<img src=\"data:image/webp;base64,%s\" alt=%q>", base64.StdEncoding.EncodeToString(raw), node.Caption), nil
	case ".gif":
		return fmt.Sprintf("<img src=\"data:image/gif;base64,%s\" alt=%q>", base64.StdEncoding.EncodeToString(raw), node.Caption), nil
	default:
		return "", fmt.Errorf("figure %q: unsupported image extension %q", node.Src, ext)
	}
}

func sanitizeInlineSVG(raw []byte, src string) (string, error) {
	s := string(raw)
	idx := strings.Index(strings.ToLower(s), "<svg")
	if idx < 0 {
		return "", fmt.Errorf("figure %q: invalid SVG file (missing <svg> element)", src)
	}
	return strings.TrimSpace(s[idx:]), nil
}

func renderInlines(b *strings.Builder, inlines []doc.Inline) {
	for _, in := range inlines {
		switch node := in.(type) {
		case *doc.TextInline:
			b.WriteString(stdhtml.EscapeString(node.Text))
		case *doc.EmphasisInline:
			tag := "em"
			if node.Strong {
				tag = "strong"
			}
			b.WriteString("<" + tag + ">")
			renderInlines(b, node.Children)
			b.WriteString("</" + tag + ">")
		case *doc.CodeInline:
			b.WriteString("<code>" + stdhtml.EscapeString(node.Code) + "</code>")
		case *doc.LinkInline:
			titleAttr := ""
			if node.Title != "" {
				titleAttr = fmt.Sprintf(" title=%q", node.Title)
			}
			b.WriteString(fmt.Sprintf("<a href=%q%s>", sanitizeURL(node.URL), titleAttr))
			renderInlines(b, node.Children)
			b.WriteString("</a>")
		case *doc.ImageInline:
			titleAttr := ""
			if node.Title != "" {
				titleAttr = fmt.Sprintf(" title=%q", node.Title)
			}
			b.WriteString(fmt.Sprintf("<img src=%q alt=%q%s>", sanitizeURL(node.Src), node.Alt, titleAttr))
		case *doc.SoftBreakInline:
			b.WriteByte('\n')
		case *doc.HardBreakInline:
			b.WriteString("<br>\n")
		}
	}
}

func sanitizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "vbscript:") {
		return "#"
	}
	return trimmed
}

func buildStylesheet(tok school.SchoolTokens) string {
	return fmt.Sprintf(`:root {
  --df-paper: %s;
  --df-paper-subtle: %s;
  --df-paper-alt: %s;
  --df-ink: %s;
  --df-ink-soft: %s;
  --df-muted: %s;
  --df-accent: %s;
  --df-ochre: %s;
  --df-forest: %s;
  --df-rule: %s;
  --df-rule-strong: %s;
  --df-font-serif: %q, "Palatino Linotype", Palatino, Georgia, serif;
  --df-font-mono: %q, ui-monospace, SFMono-Regular, Menlo, monospace;
  --df-font-sans: %q, system-ui, -apple-system, sans-serif;
  --df-radius: %s;
  --df-shadow: %s;
  --df-booktabs-top: %s;
  --df-booktabs-mid: %s;
  --df-booktabs-row: %s;
}

*, *::before, *::after {
  box-sizing: border-box;
  border-radius: 0px;
  box-shadow: none;
}

html, body {
  margin: 0;
  padding: 0;
  background-color: var(--df-paper);
  color: var(--df-ink);
  font-family: var(--df-font-serif);
  font-size: 18px;
  line-height: 1.65;
  -webkit-font-smoothing: antialiased;
}

.df-page {
  max-width: 1180px;
  margin: 0 auto;
  padding: 3.5rem 2.5rem 5rem;
}

.df-article {
  display: grid;
  grid-template-columns: minmax(0, %s) minmax(230px, %s);
  column-gap: %s;
  align-items: start;
}

.df-main {
  grid-column: 1;
}

.df-wide {
  grid-column: 1 / -1;
}

.df-margin {
  grid-column: 2;
  font-family: var(--df-font-sans);
  font-size: 0.85rem;
  line-height: 1.5;
  color: var(--df-ink-soft);
}

.df-header {
  padding-bottom: 1.5rem;
  margin-bottom: 2rem;
  border-bottom: 1.5px solid var(--df-rule-strong);
}

.df-kicker {
  margin: 0 0 0.5rem;
  font-family: var(--df-font-sans);
  font-size: 0.78rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--df-accent);
  font-weight: 600;
}

.df-title {
  margin: 0;
  font-size: 2.35rem;
  line-height: 1.15;
  font-weight: 500;
  color: var(--df-ink);
}

.df-subtitle {
  margin: 0.6rem 0 0;
  font-size: 1.25rem;
  line-height: 1.35;
  font-style: italic;
  color: var(--df-ink-soft);
}

.df-byline {
  margin: 1rem 0 0;
  font-family: var(--df-font-sans);
  font-size: 0.88rem;
  color: var(--df-muted);
}

h2, h3, h4 {
  font-weight: 500;
  line-height: 1.25;
  color: var(--df-ink);
  margin: 2.1rem 0 0.75rem;
}

h2 {
  font-size: 1.5rem;
  border-bottom: 1px solid var(--df-rule);
  padding-bottom: 0.25rem;
}

h3 {
  font-size: 1.2rem;
  font-style: italic;
}

p {
  margin: 0 0 1.1rem;
  color: var(--df-ink);
}

a {
  color: var(--df-accent);
  text-decoration: underline;
  text-underline-offset: 2px;
}

code, pre {
  font-family: var(--df-font-mono);
  font-size: 0.86em;
  background-color: var(--df-paper-subtle);
}

code {
  padding: 0.1em 0.3em;
  border: 1px solid var(--df-rule);
}

.df-code {
  padding: 0.9rem 1.1rem;
  border: 1px solid var(--df-rule);
  overflow-x: auto;
  margin: 1.25rem 0;
}

.df-code code {
  border: none;
  padding: 0;
  background: transparent;
}

.df-blockquote {
  margin: 1.25rem 0;
  padding: 0.25rem 0 0.25rem 1.1rem;
  border-left: 2px solid var(--df-accent);
  color: var(--df-ink-soft);
  font-style: italic;
}

.df-rule {
  border: none;
  border-top: 1px solid var(--df-rule);
  margin: 2rem 0;
}

ul, ol {
  margin: 0 0 1.1rem;
  padding-left: 1.4rem;
}

li {
  margin-bottom: 0.35rem;
}

.df-table-wrap,
.df-figure {
  margin: 1.75rem 0;
}

.df-caption {
  font-family: var(--df-font-sans);
  font-size: 0.86rem;
  line-height: 1.45;
  color: var(--df-ink-soft);
  margin: 0.55rem 0;
}

.df-label {
  font-weight: 600;
  color: var(--df-ink);
}

.df-source {
  font-family: var(--df-font-sans);
  font-size: 0.8rem;
  color: var(--df-muted);
  margin: 0.4rem 0 0;
}

.df-source-label {
  font-weight: 600;
}

.df-figure-media img,
.df-figure-media svg {
  display: block;
  max-width: 100%%;
  height: auto;
}

.df-booktabs {
  width: 100%%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
  border-top: var(--df-booktabs-top) solid var(--df-rule-strong);
  border-bottom: var(--df-booktabs-top) solid var(--df-rule-strong);
  border-left: none;
  border-right: none;
}

.df-booktabs th,
.df-booktabs td {
  padding: 0.48rem 0.75rem;
  text-align: left;
  border-left: none;
  border-right: none;
  font-variant-numeric: tabular-nums;
}

.df-booktabs thead th {
  border-bottom: var(--df-booktabs-mid) solid var(--df-rule-strong);
  font-family: var(--df-font-sans);
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--df-ink);
}

.df-booktabs tbody tr {
  border-bottom: var(--df-booktabs-row) solid var(--df-rule);
}

.df-booktabs tbody tr:last-child {
  border-bottom: none;
}

.df-booktabs th.num,
.df-booktabs td.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 900px) {
  .df-article {
    grid-template-columns: minmax(0, 1fr);
    column-gap: 0;
  }
  .df-main, .df-wide, .df-margin {
    grid-column: 1;
  }
}

@media print {
  body {
    background: #ffffff;
    color: #111111;
    font-size: 11pt;
  }
  .df-page {
    max-width: none;
    padding: 0;
  }
}`,
		tok.Paper,
		tok.PaperSubtle,
		tok.PaperAlt,
		tok.Ink,
		tok.InkSoft,
		tok.Muted,
		tok.Accent,
		tok.Ochre,
		tok.Forest,
		tok.Rule,
		tok.RuleStrong,
		tok.SerifFont,
		tok.MonoFont,
		tok.SansFont,
		tok.BorderRadius,
		tok.BoxShadow,
		tok.BooktabsTopRule,
		tok.BooktabsMidRule,
		tok.BooktabsRowRule,
		tok.MainWidth,
		tok.MarginWidth,
		tok.ColumnGap,
	)
}
