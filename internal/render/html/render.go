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
	"github.com/pablodiegoo/datafolio/internal/math"
	"github.com/pablodiegoo/datafolio/internal/school"
)

type renderState struct {
	doc                *doc.SourceDocument
	labels             school.Labels
	mathMode           string
	bibEntries         map[string]doc.BibEntry
	tableNum           int
	figureNum          int
	renderedSidenotes  map[string]bool
	renderedRefAnchors map[string]bool
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
	renderChromeBand(&b, cfg.Header, true, d, title)
	b.WriteString("<article class=\"df-article\">\n")

	renderHeader(&b, d.Meta)

	mathMode := strings.ToLower(strings.TrimSpace(cfg.Math))
	if mathMode == "" {
		mathMode = "mathml"
	}

	var bibEntries map[string]doc.BibEntry
	if bibPath := strings.TrimSpace(cfg.Bibliography); bibPath != "" {
		if !filepath.IsAbs(bibPath) && d.Dir != "" {
			bibPath = filepath.Join(d.Dir, bibPath)
		}
		var err error
		bibEntries, err = doc.ParseBibFile(bibPath)
		if err != nil {
			return "", fmt.Errorf("bibliography %q: %w", cfg.Bibliography, err)
		}
	}

	state := &renderState{
		doc:                d,
		labels:             labels,
		mathMode:           mathMode,
		bibEntries:         bibEntries,
		renderedSidenotes:  make(map[string]bool),
		renderedRefAnchors: make(map[string]bool),
	}
	for _, blk := range d.Blocks {
		if err := renderBlock(&b, blk, state, true); err != nil {
			return "", err
		}
	}

	renderReferences(&b, d, state)

	b.WriteString("</article>\n")
	renderChromeBand(&b, cfg.Footer, false, d, title)
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

func renderChromeBand(b *strings.Builder, spec config.ChromeSpec, isHeader bool, d *doc.SourceDocument, fallbackTitle string) {
	if !spec.Enabled {
		return
	}
	left := interpolateChromeSlot(spec.Left, d, fallbackTitle)
	center := interpolateChromeSlot(spec.Center, d, fallbackTitle)
	right := interpolateChromeSlot(spec.Right, d, fallbackTitle)

	if left == "" && center == "" && right == "" {
		if isHeader {
			left = d.Meta.Kicker
			if left == "" {
				left = fallbackTitle
			}
			right = d.Meta.Date
		} else {
			left = d.Meta.Author
			right = d.Meta.Date
			if left == "" && right == "" {
				left = fallbackTitle
			}
		}
	}
	if left == "" && center == "" && right == "" {
		return
	}

	tag := "footer"
	kindCls := "df-chrome-footer"
	if isHeader {
		tag = "header"
		kindCls = "df-chrome-header"
	}
	ruleCls := ""
	if spec.Rule {
		ruleCls = " df-chrome-rule"
	}
	b.WriteString(fmt.Sprintf("<%s class=\"df-chrome %s%s\">\n", tag, kindCls, ruleCls))
	b.WriteString("<span class=\"df-chrome-left\">" + stdhtml.EscapeString(left) + "</span>\n")
	b.WriteString("<span class=\"df-chrome-center\">" + stdhtml.EscapeString(center) + "</span>\n")
	b.WriteString("<span class=\"df-chrome-right\">" + stdhtml.EscapeString(right) + "</span>\n")
	b.WriteString(fmt.Sprintf("</%s>\n", tag))
}

func interpolateChromeSlot(tmpl string, d *doc.SourceDocument, fallbackTitle string) string {
	if tmpl == "" {
		return ""
	}
	title := strings.TrimSpace(d.Meta.Title)
	if title == "" {
		title = fallbackTitle
	}
	replacer := strings.NewReplacer(
		"{title}", title,
		"{subtitle}", d.Meta.Subtitle,
		"{kicker}", d.Meta.Kicker,
		"{author}", d.Meta.Author,
		"{date}", d.Meta.Date,
		"{section}", firstHeadingPlain(d.Blocks),
		"{page}", "",
		"{pages}", "",
	)
	return strings.TrimSpace(replacer.Replace(tmpl))
}

func firstHeadingPlain(blocks []doc.Block) string {
	for _, blk := range blocks {
		if h, ok := blk.(*doc.HeadingBlock); ok {
			return strings.TrimSpace(doc.InlinesPlainText(h.Inlines))
		}
	}
	return ""
}

func renderBlock(b *strings.Builder, blk doc.Block, state *renderState, topLevel bool) error {
	mainClass := ""
	if topLevel {
		mainClass = "df-main"
	}
	switch node := blk.(type) {
	case *doc.HeadingBlock:
		level := node.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		classAttr := ""
		if mainClass != "" {
			classAttr = fmt.Sprintf(" class=%q", mainClass)
		}
		idAttr := ""
		if node.ID != "" {
			idAttr = fmt.Sprintf(" id=%q", node.ID)
		}
		b.WriteString(fmt.Sprintf("<h%d%s%s>", level, classAttr, idAttr))
		renderInlines(b, node.Inlines, state)
		b.WriteString(fmt.Sprintf("</h%d>\n", level))
	case *doc.ParagraphBlock:
		if mainClass != "" {
			b.WriteString("<p class=\"df-main\">")
		} else {
			b.WriteString("<p>")
		}
		renderInlines(b, node.Inlines, state)
		b.WriteString("</p>\n")
	case *doc.ListBlock:
		tag := "ul"
		if node.Ordered {
			tag = "ol"
		}
		if mainClass != "" {
			b.WriteString(fmt.Sprintf("<%s class=\"df-main\">\n", tag))
		} else {
			b.WriteString(fmt.Sprintf("<%s>\n", tag))
		}
		for _, item := range node.Items {
			b.WriteString("<li>")
			if len(item) == 1 {
				if p, ok := item[0].(*doc.ParagraphBlock); ok {
					renderInlines(b, p.Inlines, state)
					b.WriteString("</li>\n")
					continue
				}
			}
			for _, sub := range item {
				if err := renderBlock(b, sub, state, false); err != nil {
					return err
				}
			}
			b.WriteString("</li>\n")
		}
		b.WriteString(fmt.Sprintf("</%s>\n", tag))
	case *doc.BlockquoteBlock:
		b.WriteString("<blockquote class=\"" + strings.TrimSpace(mainClass+" df-blockquote") + "\">\n")
		for _, sub := range node.Blocks {
			if err := renderBlock(b, sub, state, false); err != nil {
				return err
			}
		}
		b.WriteString("</blockquote>\n")
	case *doc.CodeBlock:
		classAttr := ""
		if node.Language != "" {
			classAttr = fmt.Sprintf(" class=%q", "language-"+node.Language)
		}
		b.WriteString("<pre class=\"" + strings.TrimSpace(mainClass+" df-code") + "\"><code" + classAttr + ">")
		b.WriteString(stdhtml.EscapeString(node.Code))
		b.WriteString("</code></pre>\n")
	case *doc.ThematicBreakBlock:
		b.WriteString("<hr class=\"" + strings.TrimSpace(mainClass+" df-rule") + "\">\n")
	case *doc.TableBlock:
		renderTable(b, node, state, topLevel)
	case *doc.FigureBlock:
		if err := renderFigure(b, node, state, topLevel); err != nil {
			return err
		}
	case *doc.MarginNoteBlock:
		if err := renderMarginNote(b, node, state, topLevel); err != nil {
			return err
		}
	case *doc.WideBlock:
		b.WriteString("<section class=\"df-wide\">\n")
		for _, sub := range node.Blocks {
			if err := renderBlock(b, sub, state, false); err != nil {
				return err
			}
		}
		b.WriteString("</section>\n")
	case *doc.KPIBlock:
		renderKPI(b, node, topLevel)
	case *doc.ColumnsBlock:
		if err := renderColumns(b, node, state, topLevel); err != nil {
			return err
		}
	case *doc.MathBlock:
		if err := renderMathBlock(b, node, state, topLevel); err != nil {
			return err
		}
	}

	if topLevel {
		sidenotes := collectBlockSidenotes(blk, state)
		if err := renderMarginSidenotes(b, sidenotes, state); err != nil {
			return err
		}
	}
	return nil
}

func renderTable(b *strings.Builder, node *doc.TableBlock, state *renderState, topLevel bool) {
	state.tableNum++
	cls := "df-table-wrap"
	if topLevel {
		cls = "df-main df-table-wrap"
	}
	b.WriteString("<figure class=\"" + cls + "\">\n")
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
			renderInlines(b, cell.Inlines, state)
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
				renderInlines(b, cell.Inlines, state)
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

func renderFigure(b *strings.Builder, node *doc.FigureBlock, state *renderState, topLevel bool) error {
	state.figureNum++
	mediaHTML, err := resolveFigureMediaHTML(node)
	if err != nil {
		return err
	}

	cls := "df-figure"
	if topLevel {
		cls = "df-main df-figure"
	}
	b.WriteString("<figure class=\"" + cls + "\">\n")
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
			b.WriteString(" &middot; ")
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

func renderInlines(b *strings.Builder, inlines []doc.Inline, state *renderState) {
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
			renderInlines(b, node.Children, state)
			b.WriteString("</" + tag + ">")
		case *doc.CodeInline:
			b.WriteString("<code>" + stdhtml.EscapeString(node.Code) + "</code>")
		case *doc.LinkInline:
			titleAttr := ""
			if node.Title != "" {
				titleAttr = fmt.Sprintf(" title=%q", node.Title)
			}
			b.WriteString(fmt.Sprintf("<a href=%q%s>", sanitizeURL(node.URL), titleAttr))
			renderInlines(b, node.Children, state)
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
		case *doc.SidenoteRefInline:
			if node.Defined {
				idAttr := ""
				if state != nil && !state.renderedRefAnchors[node.ID] {
					state.renderedRefAnchors[node.ID] = true
					idAttr = fmt.Sprintf(` id="snref-%d"`, node.Number)
				}
				b.WriteString(fmt.Sprintf(`<sup class="df-sidenote-ref"><a%s href="#sn-%d">%d</a></sup>`, idAttr, node.Number, node.Number))
			} else {
				b.WriteString(fmt.Sprintf(`<sup class="df-sidenote-ref df-sidenote-orphan">[%s]</sup>`, stdhtml.EscapeString(node.ID)))
			}
		case *doc.MathInline:
			renderMathInline(b, node, state)
		case *doc.CitationInline:
			renderCitationInline(b, node, state)
		}
	}
}

func renderCitationInline(b *strings.Builder, node *doc.CitationInline, state *renderState) {
	b.WriteByte('(')
	for i, item := range node.Items {
		if i > 0 {
			b.WriteString("; ")
		}
		if item.Prefix != "" {
			b.WriteString(stdhtml.EscapeString(item.Prefix) + " ")
		}
		if state != nil && state.bibEntries != nil {
			if entry, ok := state.bibEntries[item.Key]; ok {
				label := formatInlineCiteLabel(entry)
				b.WriteString(fmt.Sprintf(`<a class="df-cite" href="#ref-%s">%s</a>`, stdhtml.EscapeString(item.Key), stdhtml.EscapeString(label)))
			} else {
				b.WriteString(fmt.Sprintf(`<span class="df-cite df-cite-missing">%s</span>`, stdhtml.EscapeString(item.Key)))
			}
		} else {
			b.WriteString(fmt.Sprintf(`<span class="df-cite">%s</span>`, stdhtml.EscapeString(item.Key)))
		}
		if item.Locator != "" {
			b.WriteString(", " + stdhtml.EscapeString(item.Locator))
		}
	}
	b.WriteByte(')')
}

func formatInlineCiteLabel(entry doc.BibEntry) string {
	var who string
	switch len(entry.Authors) {
	case 0:
		who = entry.Key
	case 1:
		who = doc.AuthorSurname(entry.Authors[0])
	case 2:
		who = doc.AuthorSurname(entry.Authors[0]) + " & " + doc.AuthorSurname(entry.Authors[1])
	default:
		who = doc.AuthorSurname(entry.Authors[0]) + " et al."
	}
	if who == "" {
		who = entry.Key
	}
	if entry.Year != "" {
		return who + ", " + entry.Year
	}
	return who
}

func renderReferences(b *strings.Builder, d *doc.SourceDocument, state *renderState) {
	if len(state.bibEntries) == 0 || len(d.Citations) == 0 {
		return
	}
	var entries []doc.BibEntry
	for _, key := range d.Citations {
		if entry, ok := state.bibEntries[key]; ok {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return
	}

	b.WriteString("<section class=\"df-main df-references\">\n")
	heading := state.labels.References
	if heading == "" {
		heading = "References"
	}
	b.WriteString("<h2 class=\"df-references-title\">" + stdhtml.EscapeString(heading) + "</h2>\n")
	b.WriteString("<ol class=\"df-reference-list\">\n")
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("<li class=\"df-reference-item\" id=\"ref-%s\">", stdhtml.EscapeString(e.Key)))
		renderBibReferenceHTML(b, e)
		b.WriteString("</li>\n")
	}
	b.WriteString("</ol>\n")
	b.WriteString("</section>\n")
}

func renderBibReferenceHTML(b *strings.Builder, e doc.BibEntry) {
	var parts []string
	if e.Author != "" {
		parts = append(parts, stdhtml.EscapeString(e.Author))
	}
	if e.Year != "" {
		parts = append(parts, "("+stdhtml.EscapeString(e.Year)+")")
	}
	if e.Title != "" {
		if e.Journal != "" || e.Booktitle != "" {
			parts = append(parts, stdhtml.EscapeString(e.Title))
		} else {
			parts = append(parts, "<em>"+stdhtml.EscapeString(e.Title)+"</em>")
		}
	}
	if e.Journal != "" {
		j := "<em>" + stdhtml.EscapeString(e.Journal) + "</em>"
		if e.Volume != "" {
			j += ", " + stdhtml.EscapeString(e.Volume)
			if e.Number != "" {
				j += "(" + stdhtml.EscapeString(e.Number) + ")"
			}
		}
		if e.Pages != "" {
			j += ", " + stdhtml.EscapeString(e.Pages)
		}
		parts = append(parts, j)
	} else if e.Booktitle != "" {
		bt := "In <em>" + stdhtml.EscapeString(e.Booktitle) + "</em>"
		if e.Pages != "" {
			bt += ", pp. " + stdhtml.EscapeString(e.Pages)
		}
		parts = append(parts, bt)
	} else if e.Publisher != "" {
		parts = append(parts, stdhtml.EscapeString(e.Publisher))
	}
	if e.DOI != "" {
		parts = append(parts, "DOI: "+stdhtml.EscapeString(e.DOI))
	} else if e.URL != "" {
		parts = append(parts, fmt.Sprintf(`<a href=%q>%s</a>`, sanitizeURL(e.URL), stdhtml.EscapeString(e.URL)))
	}
	if len(parts) == 0 {
		b.WriteString(stdhtml.EscapeString(e.Key))
		return
	}
	b.WriteString(strings.Join(parts, ". ") + ".")
}

func renderMathInline(b *strings.Builder, node *doc.MathInline, state *renderState) {
	if state != nil && state.mathMode == "katex" {
		if node.Display {
			b.WriteString(`<span class="math display katex">\[` + stdhtml.EscapeString(node.LaTeX) + `\]</span>`)
			return
		}
		b.WriteString(`<span class="math inline katex">\(` + stdhtml.EscapeString(node.LaTeX) + `\)</span>`)
		return
	}
	mml, err := math.ToMathML(node.LaTeX, node.Display)
	if err != nil {
		b.WriteString(`<code>` + stdhtml.EscapeString(node.LaTeX) + `</code>`)
		return
	}
	b.WriteString(mml)
}

func renderMathBlock(b *strings.Builder, node *doc.MathBlock, state *renderState, topLevel bool) error {
	cls := "df-math-display"
	if topLevel {
		cls = "df-main df-math-display"
	}
	if state != nil && state.mathMode == "katex" {
		b.WriteString(`<div class="` + cls + ` math display katex">\[` + stdhtml.EscapeString(node.LaTeX) + `\]</div>` + "\n")
		return nil
	}
	mml, err := math.ToMathML(node.LaTeX, true)
	if err != nil {
		return err
	}
	b.WriteString(`<div class="` + cls + `">` + mml + "</div>\n")
	return nil
}

func collectBlockSidenotes(blk doc.Block, state *renderState) []doc.SidenoteDef {
	if state == nil || state.doc == nil {
		return nil
	}
	var defs []doc.SidenoteDef
	var visitInlines func([]doc.Inline)
	visitInlines = func(inlines []doc.Inline) {
		for _, in := range inlines {
			switch node := in.(type) {
			case *doc.EmphasisInline:
				visitInlines(node.Children)
			case *doc.LinkInline:
				visitInlines(node.Children)
			case *doc.SidenoteRefInline:
				if !node.Defined || state.renderedSidenotes[node.ID] {
					continue
				}
				if def, ok := state.doc.SidenoteByID(node.ID); ok {
					state.renderedSidenotes[node.ID] = true
					defs = append(defs, def)
				}
			}
		}
	}
	var visitBlock func(doc.Block)
	visitBlock = func(b doc.Block) {
		switch node := b.(type) {
		case *doc.HeadingBlock:
			visitInlines(node.Inlines)
		case *doc.ParagraphBlock:
			visitInlines(node.Inlines)
		case *doc.ListBlock:
			for _, item := range node.Items {
				for _, sub := range item {
					visitBlock(sub)
				}
			}
		case *doc.BlockquoteBlock:
			for _, sub := range node.Blocks {
				visitBlock(sub)
			}
		case *doc.TableBlock:
			for _, cell := range node.Headers {
				visitInlines(cell.Inlines)
			}
			for _, row := range node.Rows {
				for _, cell := range row {
					visitInlines(cell.Inlines)
				}
			}
		case *doc.WideBlock:
			for _, sub := range node.Blocks {
				visitBlock(sub)
			}
		case *doc.ColumnsBlock:
			for _, col := range node.Columns {
				for _, sub := range col {
					visitBlock(sub)
				}
			}
		}
	}
	visitBlock(blk)
	return defs
}

func renderMarginSidenotes(b *strings.Builder, defs []doc.SidenoteDef, state *renderState) error {
	if len(defs) == 0 {
		return nil
	}
	if len(defs) == 1 {
		def := defs[0]
		b.WriteString(fmt.Sprintf("<aside class=\"df-margin tufte-margin df-sidenote\" id=\"sn-%d\">\n", def.Number))
		if err := renderSidenoteContent(b, def, state); err != nil {
			return err
		}
		b.WriteString("</aside>\n")
		return nil
	}
	b.WriteString("<div class=\"df-margin tufte-margin df-sidenote-group\">\n")
	for _, def := range defs {
		b.WriteString(fmt.Sprintf("<aside class=\"df-sidenote\" id=\"sn-%d\">\n", def.Number))
		if err := renderSidenoteContent(b, def, state); err != nil {
			return err
		}
		b.WriteString("</aside>\n")
	}
	b.WriteString("</div>\n")
	return nil
}

func renderKPI(b *strings.Builder, node *doc.KPIBlock, topLevel bool) {
	cls := "df-kpi-strip"
	if topLevel {
		cls = "df-main df-kpi-strip"
	}
	b.WriteString("<section class=\"" + cls + "\">\n")
	for _, item := range node.Items {
		b.WriteString("<div class=\"df-kpi-item\">\n")
		if item.Label != "" {
			b.WriteString("<div class=\"df-kpi-label\">" + stdhtml.EscapeString(item.Label) + "</div>\n")
		}
		if item.Value != "" {
			b.WriteString("<div class=\"df-kpi-value\">" + stdhtml.EscapeString(item.Value) + "</div>\n")
		}
		if item.Note != "" {
			b.WriteString("<div class=\"df-kpi-note\">" + stdhtml.EscapeString(item.Note) + "</div>\n")
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</section>\n")
}

func renderColumns(b *strings.Builder, node *doc.ColumnsBlock, state *renderState, topLevel bool) error {
	cls := "df-columns"
	if topLevel {
		cls = "df-main df-columns"
	}
	colCount := len(node.Columns)
	if colCount < 1 {
		colCount = 2
	}
	b.WriteString(fmt.Sprintf("<div class=\"%s\" style=\"--df-cols: %d;\">\n", cls, colCount))
	for _, col := range node.Columns {
		b.WriteString("<div class=\"df-column\">\n")
		for _, sub := range col {
			if err := renderBlock(b, sub, state, false); err != nil {
				return err
			}
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</div>\n")
	return nil
}

func renderMarginNote(b *strings.Builder, node *doc.MarginNoteBlock, state *renderState, topLevel bool) error {
	cls := "df-margin-note"
	if topLevel {
		cls = "df-margin tufte-margin df-margin-note"
	}
	b.WriteString("<aside class=\"" + cls + "\">\n")
	label := strings.TrimSpace(node.Title)
	if label == "" {
		label = state.labels.MarginNote
	}
	if label != "" {
		b.WriteString("<span class=\"df-margin-note-label\">" + stdhtml.EscapeString(label) + ":</span> ")
	}
	if err := renderMarginBlocks(b, node.Blocks, state); err != nil {
		return err
	}
	b.WriteString("</aside>\n")
	return nil
}

func renderMarginBlocks(b *strings.Builder, blocks []doc.Block, state *renderState) error {
	for i, blk := range blocks {
		if i == 0 {
			if p, ok := blk.(*doc.ParagraphBlock); ok {
				renderInlines(b, p.Inlines, state)
				b.WriteByte('\n')
				continue
			}
		}
		if err := renderBlock(b, blk, state, false); err != nil {
			return err
		}
	}
	return nil
}

func renderSidenoteContent(b *strings.Builder, def doc.SidenoteDef, state *renderState) error {
	b.WriteString(fmt.Sprintf("<span class=\"df-sidenote-number\">%d.</span> ", def.Number))
	if def.Title != "" {
		b.WriteString("<strong class=\"df-sidenote-title\">" + stdhtml.EscapeString(def.Title) + "</strong> ")
	}
	return renderMarginBlocks(b, def.Blocks, state)
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
}

.df-margin-note,
.df-sidenote,
.df-kpi-strip,
.df-blockquote,
blockquote,
.df-code,
pre,
.df-figure img,
.df-figure svg {
  border-radius: var(--df-radius);
  box-shadow: var(--df-shadow);
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
  width: 100%%;
}

.df-kpi-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  border-top: var(--df-booktabs-top) solid var(--df-rule-strong);
  border-bottom: var(--df-booktabs-top) solid var(--df-rule-strong);
  margin: 1.5rem 0;
  padding: 0.75rem 0;
  font-variant-numeric: tabular-nums;
}

.df-kpi-item {
  padding: 0.25rem 1rem;
  border-right: 1px solid var(--df-rule);
}

.df-kpi-item:first-child {
  padding-left: 0;
}

.df-kpi-item:last-child {
  border-right: none;
  padding-right: 0;
}

.df-kpi-label {
  font-family: var(--df-font-sans);
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--df-muted);
  margin-bottom: 0.2rem;
}

.df-kpi-value {
  font-family: var(--df-font-serif);
  font-size: 1.55rem;
  font-weight: 600;
  line-height: 1.2;
  color: var(--df-ink);
  font-variant-numeric: tabular-nums;
}

.df-kpi-note {
  font-family: var(--df-font-sans);
  font-size: 0.78rem;
  color: var(--df-ink-soft);
  margin-top: 0.15rem;
}

.df-columns {
  display: grid;
  grid-template-columns: repeat(var(--df-cols, 2), minmax(0, 1fr));
  gap: 1.75rem;
  align-items: start;
  margin: 1.25rem 0;
}

.df-column > :first-child {
  margin-top: 0;
}

.df-column > :last-child {
  margin-bottom: 0;
}

.df-margin, .tufte-margin {
  grid-column: 2;
  font-family: var(--df-font-sans);
  font-size: 0.85rem;
  line-height: 1.5;
  color: var(--df-ink-soft);
}

.df-sidenote-ref {
  font-family: var(--df-font-sans);
  font-size: 0.72em;
  line-height: 0;
  margin-left: 0.1em;
}

.df-sidenote-ref a {
  text-decoration: none;
  font-weight: 600;
}

.df-sidenote-group {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.df-sidenote {
  margin-bottom: 0.75rem;
}

.df-sidenote-group .df-sidenote {
  margin-bottom: 0;
}

.df-sidenote-number {
  color: var(--df-accent);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.df-sidenote-title {
  color: var(--df-ink);
  font-weight: 600;
}

.df-sidenote p:last-child,
.df-margin-note p:last-child {
  margin-bottom: 0;
}

.df-margin-note {
  margin-bottom: 0.75rem;
}

.df-margin-note-label {
  font-weight: 600;
  color: var(--df-ink);
  letter-spacing: 0.02em;
}

.df-header {
  padding-bottom: 1.5rem;
  margin-bottom: 2rem;
  border-bottom: 1.5px solid var(--df-rule-strong);
}

.df-chrome {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: baseline;
  gap: 1rem;
  font-family: var(--df-font-sans);
  font-size: 0.78rem;
  letter-spacing: 0.04em;
  color: var(--df-muted);
}

.df-chrome-header {
  margin-bottom: 2rem;
  padding-bottom: 0.55rem;
}

.df-chrome-footer {
  margin-top: 3rem;
  padding-top: 0.65rem;
}

.df-chrome-header.df-chrome-rule {
  border-bottom: 1px solid var(--df-rule);
}

.df-chrome-footer.df-chrome-rule {
  border-top: 1px solid var(--df-rule);
}

.df-chrome-left {
  text-align: left;
}

.df-chrome-center {
  text-align: center;
}

.df-chrome-right {
  text-align: right;
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

.df-math-display {
  margin: 1.35rem 0;
  overflow-x: auto;
  text-align: center;
}

.df-cite {
  text-decoration: none;
}

.df-cite:hover {
  text-decoration: underline;
}

.df-references {
  margin-top: 2.5rem;
  padding-top: 1rem;
  border-top: 1px solid var(--df-rule-strong);
}

.df-references-title {
  margin-top: 0;
}

.df-reference-list {
  padding-left: 1.4rem;
  font-size: 0.94rem;
  line-height: 1.55;
}

.df-reference-item {
  margin-bottom: 0.55rem;
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
  .df-main, .df-wide, .df-margin, .tufte-margin {
    grid-column: 1;
  }
  .df-columns {
    grid-template-columns: minmax(0, 1fr);
  }
  .df-margin, .tufte-margin {
    border-left: 2px solid var(--df-rule-strong);
    background-color: var(--df-paper-subtle);
    padding: 0.55rem 0.85rem;
    margin: 0.5rem 0 1.25rem;
  }
}

@media print {
  @page {
    size: auto;
    margin: 20mm 18mm;
  }
  body {
    background: #ffffff;
    color: #111111;
    font-size: 11pt;
  }
  .df-page {
    max-width: none;
    padding: 0;
  }
  .df-chrome {
    color: #555555;
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
