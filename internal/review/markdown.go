package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pablodiegoo/datafolio/internal/config"
	"github.com/pablodiegoo/datafolio/internal/doc"
	typstrender "github.com/pablodiegoo/datafolio/internal/render/typst"
	"github.com/pablodiegoo/datafolio/internal/school"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

const (
	RuleSidenoteOrphanRef    = "sidenote-orphan-ref"
	RuleSidenoteOrphanDef    = "sidenote-orphan-def"
	RuleEmojiDisallowed      = "emoji-disallowed"
	RuleProseDeletionTest    = "prose-deletion-test"
	RuleTableMissingCaption  = "table-missing-caption"
	RuleTableMissingSource   = "table-missing-source"
	RuleFigureMissingCaption = "figure-missing-caption"
	RuleFigureMissingSource  = "figure-missing-source"
	RuleSlideOverflow        = "slide-overflow"
)

func reviewMarkdownFile(ctx context.Context, absPath, displayPath string, opts Options, explicitTokensPath string) ([]Finding, error) {
	projectOverride, _, err := config.DiscoverUpward(filepath.Dir(absPath))
	if err != nil {
		return nil, usageErrorf("%v", err)
	}

	rawBytes, err := os.ReadFile(absPath)
	if err != nil {
		return nil, usageErrorf("%s: %v", displayPath, err)
	}

	parsedDoc, err := doc.Parse(absPath, rawBytes)
	if err != nil {
		return nil, usageErrorf("%s: %v", displayPath, err)
	}

	fmOverride, err := config.ParseYAML(parsedDoc.RawFrontmatter, parsedDoc.Dir)
	if err != nil {
		return nil, usageErrorf("%s: %v", displayPath, err)
	}

	cliOverride := config.Override{BaseDir: opts.WorkDir}
	if strings.TrimSpace(opts.School) != "" {
		s := opts.School
		cliOverride.School = &s
	}
	if explicitTokensPath != "" {
		t := explicitTokensPath
		cliOverride.Tokens = &t
	}

	cfg, s, tok, err := config.Resolve(projectOverride, fmOverride, cliOverride)
	if err != nil {
		return nil, usageErrorf("%s: %v", displayPath, err)
	}

	policy := s.Policy()
	rawLines := splitSourceLines(rawBytes)
	tableLines := resolveTableLines(parsedDoc, rawLines)

	var findings []Finding
	findings = append(findings, checkSidenoteOrphans(parsedDoc, displayPath)...)
	if policy.DisallowEmojis {
		findings = append(findings, checkDisallowedEmojis(parsedDoc, rawLines, tableLines, displayPath, s.Name())...)
	}
	findings = append(findings, checkDeletionTest(parsedDoc, rawLines, displayPath)...)
	findings = append(findings, checkTableAndFigureAttributions(parsedDoc, tableLines, policy, displayPath)...)

	if cfg.Format == "slides" {
		slideFindings, err := checkSlideOverflow(ctx, parsedDoc, cfg, s, tok, rawLines, displayPath, opts)
		if err != nil {
			return nil, err
		}
		findings = append(findings, slideFindings...)
	}

	return findings, nil
}

func splitSourceLines(raw []byte) []string {
	text := strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(text, "\n")
}

func checkSidenoteOrphans(d *doc.SourceDocument, displayPath string) []Finding {
	var findings []Finding
	for _, ref := range d.SidenoteRefs {
		if ref.Defined {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   RuleSidenoteOrphanRef,
			File:     displayPath,
			Line:     maxLine(ref.Line),
			Severity: SeverityError,
			Message:  fmt.Sprintf("orphan Sidenote reference [^%s] has no matching definition", ref.ID),
			Hint:     fmt.Sprintf("Add a Sidenote definition [^%s]: ... or remove the unresolved reference.", ref.ID),
		})
	}
	for _, def := range d.Sidenotes {
		if def.Referenced {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   RuleSidenoteOrphanDef,
			File:     displayPath,
			Line:     maxLine(def.Line),
			Severity: SeverityError,
			Message:  fmt.Sprintf("orphan Sidenote definition [^%s] is never referenced", def.ID),
			Hint:     fmt.Sprintf("Reference [^%s] in the Source Document or remove the unused Sidenote definition.", def.ID),
		})
	}
	return findings
}

func checkDisallowedEmojis(d *doc.SourceDocument, rawLines []string, tableLines map[*doc.TableBlock]int, displayPath, schoolName string) []Finding {
	var findings []Finding
	emit := func(line int, emoji string) {
		findings = append(findings, Finding{
			RuleID:   RuleEmojiDisallowed,
			File:     displayPath,
			Line:     maxLine(line),
			Severity: SeverityError,
			Message:  fmt.Sprintf("emoji %s is disallowed under the %s School", emoji, schoolName),
			Hint:     fmt.Sprintf("Remove %s and replace it with plain typography or domain prose.", emoji),
		})
	}

	scanString := func(text string, line int) {
		for _, emoji := range extractEmojis(text) {
			resolvedLine := refineLineInSource(rawLines, line, emoji)
			emit(resolvedLine, emoji)
		}
	}

	scanInlines := func(inlines []doc.Inline, startLine int) {
		for _, seg := range flattenInlinesWithLines(inlines, startLine, false) {
			for _, emoji := range extractEmojis(seg.text) {
				resolvedLine := refineLineInSource(rawLines, seg.line, emoji)
				emit(resolvedLine, emoji)
			}
		}
	}

	// Scan editorial frontmatter fields.
	if d.BodyLineOffset > 1 {
		for _, field := range []string{d.Meta.Title, d.Meta.Subtitle, d.Meta.Kicker} {
			if strings.TrimSpace(field) != "" {
				for _, emoji := range extractEmojis(field) {
					line := findLineInRange(rawLines, 1, d.BodyLineOffset-1, emoji, 1)
					emit(line, emoji)
				}
			}
		}
	}

	// Scan slide kickers parsed from HTML comments.
	for _, sl := range d.Slides {
		if strings.TrimSpace(sl.Kicker) != "" {
			for _, emoji := range extractEmojis(sl.Kicker) {
				start := maxLine(sl.Line)
				end := minInt(len(rawLines), start+5)
				line := findLineInRange(rawLines, start, end, emoji, start)
				emit(line, emoji)
			}
		}
	}

	scanBlocks := func(blocks []doc.Block) {
		doc.WalkBlocks(blocks, func(blk doc.Block) {
			switch node := blk.(type) {
			case *doc.HeadingBlock:
				scanInlines(node.Inlines, node.Line)
			case *doc.ParagraphBlock:
				scanInlines(node.Inlines, node.Line)
			case *doc.MarginNoteBlock:
				if strings.TrimSpace(node.Title) != "" {
					scanString(node.Title, node.Line)
				}
			case *doc.KPIBlock:
				for _, item := range node.Items {
					for _, val := range []string{item.Label, item.Value, item.Note} {
						if val == "" {
							continue
						}
						for _, emoji := range extractEmojis(val) {
							line := findLineInRange(rawLines, maxLine(node.Line), minInt(len(rawLines), maxLine(node.Line)+len(node.Items)+4), emoji, maxLine(node.Line))
							emit(line, emoji)
						}
					}
				}
			case *doc.TableBlock:
				tLine := tableLineOf(node, tableLines)
				if strings.TrimSpace(node.Caption) != "" {
					capLine := findTableCaptionLine(rawLines, tLine)
					scanString(node.Caption, capLine)
				}
				for _, cell := range node.Headers {
					scanInlines(cell.Inlines, tLine)
				}
				for rowIdx, row := range node.Rows {
					rowLine := tLine + 2 + rowIdx
					for _, cell := range row {
						scanInlines(cell.Inlines, rowLine)
					}
				}
				if strings.TrimSpace(node.Source) != "" {
					srcLine := findTableSourceLine(rawLines, tLine+2+len(node.Rows))
					scanString(node.Source, srcLine)
				}
			case *doc.FigureBlock:
				if strings.TrimSpace(node.Caption) != "" {
					scanString(node.Caption, node.Line)
				}
				if strings.TrimSpace(node.Source) != "" {
					srcLine := findFigureSourceLine(rawLines, node.Line)
					scanString(node.Source, srcLine)
				}
			}
		})
	}

	scanBlocks(d.Blocks)
	for _, sn := range d.Sidenotes {
		if strings.TrimSpace(sn.Title) != "" {
			scanString(sn.Title, sn.Line)
		}
		scanBlocks(sn.Blocks)
	}

	return findings
}

type inlineLineSeg struct {
	line int
	text string
}

// flattenInlinesWithLines walks inline AST nodes, advancing line counts on soft/hard breaks and
// skipping code/math spans. When markSkippedWithBoundary is true, skipped spans emit a NUL
// byte so adjacent prose words never concatenate into false phrase matches.
func flattenInlinesWithLines(inlines []doc.Inline, startLine int, markSkippedWithBoundary bool) []inlineLineSeg {
	curLine := maxLine(startLine)
	var out []inlineLineSeg
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			out = append(out, inlineLineSeg{line: curLine, text: cur.String()})
			cur.Reset()
		}
	}

	doc.WalkInlines(inlines, func(in doc.Inline) {
		switch node := in.(type) {
		case *doc.TextInline:
			cur.WriteString(node.Text)
		case *doc.ImageInline:
			if node.Alt != "" {
				cur.WriteString(node.Alt)
			}
			if node.Title != "" {
				cur.WriteByte(' ')
				cur.WriteString(node.Title)
			}
		case *doc.SoftBreakInline, *doc.HardBreakInline:
			flush()
			curLine++
		case *doc.CodeInline, *doc.MathInline:
			if markSkippedWithBoundary {
				cur.WriteByte('\x00')
			}
		}
	})

	flush()
	return out
}

func refineLineInSource(rawLines []string, approxLine int, needle string) int {
	return refineMatchingLine(rawLines, approxLine, func(line string) bool {
		return strings.Contains(line, needle)
	})
}

func refineMatchingLine(rawLines []string, approxLine int, match func(string) bool) int {
	if approxLine <= 0 || approxLine > len(rawLines) {
		return maxLine(approxLine)
	}
	if match(rawLines[approxLine-1]) {
		return approxLine
	}
	// Handle cases where trimLeadingInlineSpace removed a SoftBreakInline right after a title.
	for offset := 1; offset <= 3; offset++ {
		if approxLine-1+offset < len(rawLines) && match(rawLines[approxLine-1+offset]) {
			return approxLine + offset
		}
	}
	return approxLine
}

func findLineInRange(rawLines []string, startLine, endLine int, needle string, fallback int) int {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(rawLines) {
		endLine = len(rawLines)
	}
	for l := startLine; l <= endLine; l++ {
		if strings.Contains(rawLines[l-1], needle) {
			return l
		}
	}
	return maxLine(fallback)
}

func findTableCaptionLine(rawLines []string, tableLine int) int {
	start := tableLine - 1
	if start > len(rawLines) {
		start = len(rawLines)
	}
	for l := start; l >= 1 && l >= tableLine-4; l-- {
		lower := strings.ToLower(strings.TrimSpace(rawLines[l-1]))
		if lower == "" {
			continue
		}
		for _, prefix := range []string{"table:", "tabela:", "caption:", "legenda:", ":"} {
			if strings.HasPrefix(lower, prefix) {
				return l
			}
		}
		break
	}
	return maxLine(tableLine)
}

func findTableSourceLine(rawLines []string, afterLine int) int {
	return findSourceAttrLine(rawLines, afterLine, 4, afterLine)
}

func findFigureSourceLine(rawLines []string, figLine int) int {
	return findSourceAttrLine(rawLines, figLine, 4, figLine)
}

func findSourceAttrLine(rawLines []string, startLine, maxScan, fallback int) int {
	if startLine < 1 {
		startLine = 1
	}
	end := minInt(len(rawLines), startLine+maxScan)
	for l := startLine; l <= end; l++ {
		trimmed := strings.TrimSpace(rawLines[l-1])
		stripped := strings.Trim(trimmed, "*_")
		lower := strings.ToLower(stripped)
		if strings.HasPrefix(lower, "source:") || strings.HasPrefix(lower, "fonte:") {
			return l
		}
	}
	return maxLine(fallback)
}

func extractEmojis(s string) []string {
	if s == "" {
		return nil
	}
	runes := []rune(s)
	var out []string

	for i := 0; i < len(runes); {
		r := runes[i]

		// Keycap emoji sequence: [0-9#*] + optional U+FE0F + U+20E3
		if isKeycapBase(r) {
			if i+1 < len(runes) && runes[i+1] == 0x20E3 {
				out = append(out, string(runes[i:i+2]))
				i += 2
				continue
			}
			if i+2 < len(runes) && runes[i+1] == 0xFE0F && runes[i+2] == 0x20E3 {
				out = append(out, string(runes[i:i+3]))
				i += 3
				continue
			}
			i++
			continue
		}

		// Regional indicator flag pairs (e.g., 🇧🇷)
		if isRegionalIndicator(r) {
			if i+1 < len(runes) && isRegionalIndicator(runes[i+1]) {
				out = append(out, string(runes[i:i+2]))
				i += 2
				continue
			}
			out = append(out, string(runes[i:i+1]))
			i++
			continue
		}

		if !startsEmojiCluster(runes, i) {
			i++
			continue
		}

		end := consumeEmojiCluster(runes, i)
		out = append(out, string(runes[i:end]))
		i = end
	}
	return out
}

func isKeycapBase(r rune) bool {
	return (r >= '0' && r <= '9') || r == '#' || r == '*'
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

func startsEmojiCluster(runes []rune, idx int) bool {
	r := runes[idx]
	if isEmojiPresentation(r) {
		return true
	}
	if isTextDefaultEmoji(r) && idx+1 < len(runes) && runes[idx+1] == 0xFE0F {
		return true
	}
	return false
}

func consumeEmojiCluster(runes []rune, start int) int {
	i := start + 1
	for i < len(runes) {
		r := runes[i]
		switch {
		case r == 0xFE0F || r == 0xFE0E:
			i++
		case r >= 0x1F3FB && r <= 0x1F3FF:
			i++
		case r >= 0xE0020 && r <= 0xE007F:
			i++
		case r == 0x200D && i+1 < len(runes) && (startsEmojiCluster(runes, i+1) || isRegionalIndicator(runes[i+1])):
			if isRegionalIndicator(runes[i+1]) && i+2 < len(runes) && isRegionalIndicator(runes[i+2]) {
				i += 3
			} else {
				i = consumeEmojiCluster(runes, i+1)
				return i
			}
		default:
			return i
		}
	}
	return i
}

// isEmojiPresentation covers Unicode Emoji_Presentation=Yes codepoints plus
// explicit warning/heart pictographs (U+26A0, U+2764), while excluding ASCII,
// Latin/PT-BR/ES letters, general punctuation, and mathematical operators.
func isEmojiPresentation(r rune) bool {
	switch {
	case r == 0x231A || r == 0x231B:
		return true
	case r >= 0x23E9 && r <= 0x23EC:
		return true
	case r == 0x23F0 || r == 0x23F3:
		return true
	case r == 0x25FD || r == 0x25FE:
		return true
	case r == 0x2614 || r == 0x2615:
		return true
	case r >= 0x2648 && r <= 0x2653:
		return true
	case r == 0x267F || r == 0x2693 || r == 0x26A0 || r == 0x26A1:
		return true
	case r == 0x26AA || r == 0x26AB:
		return true
	case r == 0x26BD || r == 0x26BE || r == 0x26C4 || r == 0x26C5 || r == 0x26CE || r == 0x26D4:
		return true
	case r == 0x26EA || r == 0x26F2 || r == 0x26F3 || r == 0x26F5 || r == 0x26FA || r == 0x26FD:
		return true
	case r == 0x2705 || r == 0x270A || r == 0x270B || r == 0x2728:
		return true
	case r == 0x274C || r == 0x274E || (r >= 0x2753 && r <= 0x2755) || r == 0x2757 || r == 0x2764:
		return true
	case (r >= 0x2795 && r <= 0x2797) || r == 0x27B0 || r == 0x27BF:
		return true
	case r == 0x2B1B || r == 0x2B1C || r == 0x2B50 || r == 0x2B55:
		return true
	case r == 0x1F004 || r == 0x1F0CF || r == 0x1F18E || (r >= 0x1F191 && r <= 0x1F19A):
		return true
	case r == 0x1F201 || r == 0x1F21A || r == 0x1F22F || (r >= 0x1F232 && r <= 0x1F236) || (r >= 0x1F238 && r <= 0x1F23A) || r == 0x1F250 || r == 0x1F251:
		return true
	case r >= 0x1F300 && r <= 0x1F6FF:
		return true
	case r >= 0x1F7E0 && r <= 0x1F7F0:
		return true
	case r >= 0x1F900 && r <= 0x1FAFF:
		return true
	default:
		return false
	}
}

// isTextDefaultEmoji matches Unicode Emoji=Yes characters whose emoji presentation
// requires an explicit U+FE0F variation selector (e.g., ☀️, ❤️, ©️, ✈️, ➡️, ☑️).
func isTextDefaultEmoji(r rune) bool {
	switch {
	case r == 0x00A9 || r == 0x00AE:
		return true
	case r == 0x203C || r == 0x2049 || r == 0x2122 || r == 0x2139:
		return true
	case (r >= 0x2194 && r <= 0x2199) || r == 0x21A9 || r == 0x21AA:
		return true
	case r == 0x2328 || r == 0x23CF || (r >= 0x23ED && r <= 0x23F2) || (r >= 0x23F8 && r <= 0x23FA):
		return true
	case r == 0x24C2 || r == 0x25AA || r == 0x25AB || r == 0x25B6 || r == 0x25C0 || r == 0x25FB || r == 0x25FC:
		return true
	case r >= 0x2600 && r <= 0x26FF:
		return true
	case r >= 0x2700 && r <= 0x27BF:
		return true
	case r == 0x2934 || r == 0x2935 || (r >= 0x2B05 && r <= 0x2B07):
		return true
	case r == 0x3030 || r == 0x303D || r == 0x3297 || r == 0x3299:
		return true
	case r >= 0x1F000 && r <= 0x1F2FF:
		return true
	default:
		return false
	}
}

func maxLine(line int) int {
	if line <= 0 {
		return 1
	}
	return line
}

func resolveTableLines(d *doc.SourceDocument, rawLines []string) map[*doc.TableBlock]int {
	out := make(map[*doc.TableBlock]int)
	if d == nil || len(rawLines) == 0 {
		return out
	}

	walk := func(blocks []doc.Block, minLine *int) {
		doc.WalkBlocks(blocks, func(blk doc.Block) {
			switch node := blk.(type) {
			case *doc.HeadingBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.ParagraphBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.FigureBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.CodeBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.MathBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.ThematicBreakBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.BlockquoteBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.MarginNoteBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.WideBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.ColumnsBlock:
				if node.Line > *minLine {
					*minLine = node.Line
				}
			case *doc.TableBlock:
				tLine := findTableHeaderLine(rawLines, *minLine, node)
				out[node] = tLine
				*minLine = tLine + 2 + len(node.Rows)
			}
		})
	}

	cursor := maxLine(d.BodyLineOffset)
	walk(d.Blocks, &cursor)

	for _, sn := range d.Sidenotes {
		snCursor := maxLine(sn.Line)
		walk(sn.Blocks, &snCursor)
	}
	return out
}

func tableLineOf(tbl *doc.TableBlock, tableLines map[*doc.TableBlock]int) int {
	if l, ok := tableLines[tbl]; ok && l > 0 {
		return l
	}
	return maxLine(tbl.Line)
}

func findTableHeaderLine(rawLines []string, startLine int, tbl *doc.TableBlock) int {
	if startLine < 1 {
		startLine = 1
	}
	replacer := strings.NewReplacer("*", "", "_", "", "`", "")
	for l := startLine; l < len(rawLines); l++ {
		headerRaw := rawLines[l-1]
		delimRaw := rawLines[l]
		if !strings.Contains(headerRaw, "|") || !isGFMDelimiterLine(delimRaw) {
			continue
		}
		headerNorm := replacer.Replace(headerRaw)
		matched := true
		for _, h := range tbl.Headers {
			p := strings.TrimSpace(h.Plain)
			if p != "" && !strings.Contains(headerNorm, p) {
				matched = false
				break
			}
		}
		if matched {
			return l
		}
	}
	return maxLine(tbl.Line)
}

func isGFMDelimiterLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	if !strings.Contains(trimmed, "-") {
		return false
	}
	if !strings.Contains(trimmed, "|") {
		return false
	}
	for _, r := range trimmed {
		switch r {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

func checkTableAndFigureAttributions(d *doc.SourceDocument, tableLines map[*doc.TableBlock]int, policy school.ReviewPolicy, displayPath string) []Finding {
	if !policy.RequireCaptions && !policy.RequireSources {
		return nil
	}
	var findings []Finding

	doc.WalkDocumentBlocks(d, func(blk doc.Block) {
		switch node := blk.(type) {
		case *doc.TableBlock:
			line := tableLineOf(node, tableLines)
			if policy.RequireCaptions && strings.TrimSpace(node.Caption) == "" {
				findings = append(findings, Finding{
					RuleID:   RuleTableMissingCaption,
					File:     displayPath,
					Line:     line,
					Severity: SeverityWarn,
					Message:  "table is missing a caption",
					Hint:     `Add a caption directly above the table (e.g., "Table: Summary of metrics").`,
				})
			}
			if policy.RequireSources && strings.TrimSpace(node.Source) == "" {
				findings = append(findings, Finding{
					RuleID:   RuleTableMissingSource,
					File:     displayPath,
					Line:     line,
					Severity: SeverityWarn,
					Message:  "table is missing a source attribution",
					Hint:     `Add a source line directly below the table (e.g., "*Source: Internal telemetry*").`,
				})
			}
		case *doc.FigureBlock:
			line := maxLine(node.Line)
			if policy.RequireCaptions && strings.TrimSpace(node.Caption) == "" {
				findings = append(findings, Finding{
					RuleID:   RuleFigureMissingCaption,
					File:     displayPath,
					Line:     line,
					Severity: SeverityWarn,
					Message:  "figure is missing a caption",
					Hint:     "Provide descriptive alt text as the figure caption: ![Descriptive caption](path/to/figure.svg).",
				})
			}
			if policy.RequireSources && strings.TrimSpace(node.Source) == "" {
				findings = append(findings, Finding{
					RuleID:   RuleFigureMissingSource,
					File:     displayPath,
					Line:     line,
					Severity: SeverityWarn,
					Message:  "figure is missing a source attribution",
					Hint:     `Add a source line below the figure (e.g., "*Source: Author calculations*").`,
				})
			}
		}
	})
	return findings
}

type deletionPhrase struct {
	text  string
	runes []rune
}

var deletionTestPhrases = buildDeletionPhrases([]string{
	// English
	"it's important to note that",
	"it is important to note that",
	"based on the data provided",
	"interestingly",
	"it is worth mentioning",
	"one can observe that",
	"the data suggests that",
	"in conclusion",
	"as we can see",
	// Portuguese
	"é importante notar que",
	"é importante destacar que",
	"com base nos dados fornecidos",
	"interessantemente",
	"vale mencionar que",
	"vale destacar que",
	"pode-se observar que",
	"os dados sugerem que",
	"em conclusão",
	"como podemos ver",
	// Spanish
	"es importante notar que",
	"es importante señalar que",
	"con base en los datos proporcionados",
	"interesantemente",
	"vale la pena mencionar que",
	"cabe mencionar que",
	"se puede observar que",
	"los datos sugieren que",
	"en conclusión",
	"como podemos ver",
})

func buildDeletionPhrases(raw []string) []deletionPhrase {
	seen := make(map[string]bool, len(raw))
	out := make([]deletionPhrase, 0, len(raw))
	for _, p := range raw {
		norm := strings.ToLower(p)
		if seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, deletionPhrase{
			text:  p,
			runes: []rune(norm),
		})
	}
	return out
}

func checkDeletionTest(d *doc.SourceDocument, rawLines []string, displayPath string) []Finding {
	var findings []Finding

	doc.WalkDocumentBlocks(d, func(blk doc.Block) {
		p, ok := blk.(*doc.ParagraphBlock)
		if !ok {
			return
		}
		segs := flattenInlinesWithLines(p.Inlines, p.Line, true)
		if len(segs) == 0 {
			return
		}
		normRunes, runeLines := normalizeProseRunes(segs)
		if len(normRunes) == 0 {
			return
		}
		for _, phrase := range deletionTestPhrases {
			for _, idx := range findPhraseIndices(normRunes, phrase.runes) {
				line := refinePhraseLine(rawLines, runeLines[idx], phrase.text)
				findings = append(findings, Finding{
					RuleID:   RuleProseDeletionTest,
					File:     displayPath,
					Line:     line,
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("throat-clearing phrase %q fails the editorial Deletion Test", phrase.text),
					Hint:     fmt.Sprintf("Delete %q and start directly with the empirical claim or observation.", phrase.text),
				})
			}
		}
	})
	return findings
}

func normalizeProseRunes(segs []inlineLineSeg) ([]rune, []int) {
	var runes []rune
	var lines []int
	lastSpace := false

	appendSpace := func(line int) {
		if len(runes) == 0 || lastSpace {
			return
		}
		runes = append(runes, ' ')
		lines = append(lines, line)
		lastSpace = true
	}

	for i, seg := range segs {
		if i > 0 {
			appendSpace(seg.line)
		}
		for _, r := range seg.text {
			if r == '\x00' {
				runes = append(runes, '\x00')
				lines = append(lines, seg.line)
				lastSpace = false
				continue
			}
			if r == '’' {
				r = '\''
			}
			if unicode.IsSpace(r) {
				appendSpace(seg.line)
				continue
			}
			runes = append(runes, unicode.ToLower(r))
			lines = append(lines, seg.line)
			lastSpace = false
		}
	}
	return runes, lines
}

func findPhraseIndices(haystack, needle []rune) []int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return nil
	}
	var matches []int
	limit := len(haystack) - len(needle)
	for i := 0; i <= limit; i++ {
		if i > 0 && isProseWordRune(haystack[i-1]) {
			continue
		}
		end := i + len(needle)
		if end < len(haystack) && isProseWordRune(haystack[end]) {
			continue
		}
		matched := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, i)
		}
	}
	return matches
}

func isProseWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func refinePhraseLine(rawLines []string, approxLine int, phrase string) int {
	words := strings.Fields(phrase)
	if len(words) == 0 {
		return maxLine(approxLine)
	}
	firstWord := strings.ToLower(words[0])
	return refineMatchingLine(rawLines, approxLine, func(line string) bool {
		return strings.Contains(strings.ToLower(strings.ReplaceAll(line, "’", "'")), firstWord)
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func checkSlideOverflow(
	ctx context.Context,
	parsedDoc *doc.SourceDocument,
	cfg config.Config,
	s school.School,
	tok school.SchoolTokens,
	rawLines []string,
	displayPath string,
	opts Options,
) ([]Finding, error) {
	res := toolchain.ResolveTypst(opts.LookPath, opts.CacheDir, opts.GOOS, opts.GOARCH)
	if !res.Found {
		return nil, fmt.Errorf("typst not found on PATH or in %s (run \"datafolio doctor --install-typst\" to install %s)",
			toolchain.CachedTypstPath(opts.CacheDir, opts.GOOS), toolchain.PinnedTypstVersion)
	}

	tmpDir, err := os.MkdirTemp("", "datafolio-review-*")
	if err != nil {
		return nil, fmt.Errorf("create temp workspace: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cacheDir := opts.CacheDir
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir = filepath.Join(tmpDir, "cache")
	}
	fontPath, err := toolchain.EnsureFonts(cacheDir)
	if err != nil {
		return nil, err
	}

	rootDir, renderOpts, err := typstrender.PrepareWorkspace(parsedDoc, cfg, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", displayPath, err)
	}

	typSource, err := typstrender.RenderSlides(parsedDoc, cfg, tok, s.Labels(cfg.Lang), renderOpts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", displayPath, err)
	}

	typPath := filepath.Join(tmpDir, "slides.typ")
	pdfPath := filepath.Join(tmpDir, "slides.pdf")
	if err := os.WriteFile(typPath, []byte(typSource), 0o644); err != nil {
		return nil, fmt.Errorf("write typst source: %w", err)
	}

	if err := opts.TypstRunner.Compile(ctx, toolchain.CompileRequest{
		TypstBin:   res.Path,
		InputPath:  typPath,
		OutputPath: pdfPath,
		RootDir:    rootDir,
		FontPath:   fontPath,
	}); err != nil {
		return nil, err
	}

	overflows, err := typstrender.CheckSlideOverflow(ctx, opts.TypstRunner, toolchain.QueryRequest{
		TypstBin:  res.Path,
		InputPath: typPath,
		Selector:  typstrender.SlideMarkerSelector,
		RootDir:   rootDir,
		FontPath:  fontPath,
	})
	if err != nil {
		return nil, err
	}

	slideLines := mapLogicalSlideLines(parsedDoc)
	var findings []Finding
	for _, ov := range overflows {
		line := slideLines[ov.Index]
		if line <= 0 {
			line = maxLine(len(rawLines))
		}
		findings = append(findings, Finding{
			RuleID:   RuleSlideOverflow,
			File:     displayPath,
			Line:     line,
			Severity: SeverityError,
			Message:  ov.Summary(),
			Hint:     "Reduce content density on this slide, move secondary details into a Sidenote, or split across a --- slide break.",
		})
	}
	return findings, nil
}

func mapLogicalSlideLines(d *doc.SourceDocument) map[int]int {
	lines := make(map[int]int)
	if d == nil {
		return lines
	}
	bodySlides := d.Slides
	hasCover := strings.TrimSpace(d.Meta.Title) != "" ||
		strings.TrimSpace(d.Meta.Subtitle) != "" ||
		strings.TrimSpace(d.Meta.Kicker) != "" ||
		strings.TrimSpace(d.Meta.Author) != "" ||
		strings.TrimSpace(d.Meta.Date) != ""

	if hasCover {
		coverLine := 1
		if len(bodySlides) > 0 && isSlideCoverIntro(bodySlides[0]) {
			coverLine = maxLine(bodySlides[0].Line)
			bodySlides = bodySlides[1:]
		}
		lines[1] = coverLine
	}

	for _, s := range bodySlides {
		idx := len(lines) + 1
		line := maxLine(s.Line)
		if len(s.Blocks) > 0 {
			if h, ok := s.Blocks[0].(*doc.HeadingBlock); ok && h.Line > 0 {
				line = h.Line
			}
		}
		lines[idx] = line
	}
	return lines
}

func isSlideCoverIntro(s doc.Slide) bool {
	for _, blk := range s.Blocks {
		switch blk.(type) {
		case *doc.ParagraphBlock, *doc.MarginNoteBlock, *doc.BlockquoteBlock, *doc.ListBlock:
			continue
		default:
			return false
		}
	}
	return true
}
