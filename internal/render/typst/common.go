package typst

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/pablodiegoo/datafolio/internal/config"
	"github.com/pablodiegoo/datafolio/internal/doc"
	"github.com/pablodiegoo/datafolio/internal/math"
	"github.com/pablodiegoo/datafolio/internal/school"
	"github.com/pablodiegoo/datafolio/internal/tokens"
)

// RenderOptions configures path resolution and staging for Typst compilation.
type RenderOptions struct {
	RootDir     string
	AssetMapper func(absPath string) (string, error)
}

// RenderContext carries shared state while rendering SourceDocument blocks and inlines to Typst.
type RenderContext struct {
	Doc                 *doc.SourceDocument
	Cfg                 config.Config
	Tok                 school.SchoolTokens
	Labels              school.Labels
	BibEntries          map[string]doc.BibEntry
	Opts                RenderOptions
	TopLevel            bool
	InMargin            bool
	RenderedSidenotes   map[string]bool
	SectionLevel        int
	MinBodyHeadingLevel int
	FullWidthColumns    bool
	HasBibliography     bool
}

// PrepareWorkspace validates local figure, bibliography, and CSL files, computes the
// tightest common Typst --root directory covering tmpDir and all referenced files, and
// returns RenderOptions whose AssetMapper maps files to Typst root-relative `/...` paths.
func PrepareWorkspace(d *doc.SourceDocument, cfg config.Config, tmpDir string) (string, RenderOptions, error) {
	if d == nil {
		return "", RenderOptions{}, fmt.Errorf("cannot prepare workspace for nil SourceDocument")
	}
	absTmp, err := filepath.Abs(tmpDir)
	if err != nil {
		return "", RenderOptions{}, fmt.Errorf("resolve temp dir: %w", err)
	}
	absTmp = filepath.Clean(absTmp)

	var localFiles []string
	var dirs []string
	dirs = append(dirs, absTmp)
	if d.Dir != "" {
		if absDocDir, err := filepath.Abs(d.Dir); err == nil {
			dirs = append(dirs, filepath.Clean(absDocDir))
		}
	}

	if err := collectAndValidateAssets(d, &localFiles); err != nil {
		return "", RenderOptions{}, err
	}

	if bibPath := ResolveBibPath(cfg.Bibliography, d.Dir); bibPath != "" {
		if _, err := os.Stat(bibPath); err != nil {
			return "", RenderOptions{}, fmt.Errorf("bibliography %q: %w", cfg.Bibliography, err)
		}
		if _, err := doc.ParseBibFile(bibPath); err != nil {
			return "", RenderOptions{}, fmt.Errorf("bibliography %q: %w", cfg.Bibliography, err)
		}
		localFiles = append(localFiles, bibPath)
	}

	cslArg, isFile, err := ResolveCSL(cfg.CSL, d.Dir)
	if err != nil {
		return "", RenderOptions{}, err
	}
	if isFile {
		localFiles = append(localFiles, cslArg)
	}

	for _, f := range localFiles {
		absF, err := filepath.Abs(f)
		if err == nil {
			dirs = append(dirs, filepath.Dir(filepath.Clean(absF)))
		}
	}

	rootDir := absTmp
	staged := make(map[string]string)
	canShareRoot := true
	for _, dir := range dirs[1:] {
		common, ok := commonAncestorDir(rootDir, dir)
		if !ok {
			canShareRoot = false
			break
		}
		rootDir = common
	}

	if !canShareRoot {
		// Cross-drive fallback (e.g., Windows): stage referenced files inside absTmp.
		rootDir = absTmp
		stageDir := filepath.Join(absTmp, "_staged")
		if err := os.MkdirAll(stageDir, 0o755); err != nil {
			return "", RenderOptions{}, fmt.Errorf("create staging dir: %w", err)
		}
		for i, f := range localFiles {
			absF, err := filepath.Abs(f)
			if err != nil {
				return "", RenderOptions{}, err
			}
			absF = filepath.Clean(absF)
			if _, ok := staged[absF]; ok {
				continue
			}
			dstName := fmt.Sprintf("%03d_%s", i, filepath.Base(absF))
			dstPath := filepath.Join(stageDir, dstName)
			if err := copyFile(absF, dstPath); err != nil {
				return "", RenderOptions{}, fmt.Errorf("stage asset %s: %w", f, err)
			}
			staged[absF] = "/_staged/" + dstName
		}
	}

	mapper := func(path string) (string, error) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		absPath = filepath.Clean(absPath)
		if mapped, ok := staged[absPath]; ok {
			return mapped, nil
		}
		rel, err := filepath.Rel(rootDir, absPath)
		if err != nil {
			return "", fmt.Errorf("map %q relative to root %q: %w", absPath, rootDir, err)
		}
		return "/" + filepath.ToSlash(rel), nil
	}

	return rootDir, RenderOptions{RootDir: rootDir, AssetMapper: mapper}, nil
}

func collectAndValidateAssets(d *doc.SourceDocument, out *[]string) error {
	var visitInlines func([]doc.Inline) error
	visitInlines = func(inlines []doc.Inline) error {
		for _, in := range inlines {
			switch node := in.(type) {
			case *doc.EmphasisInline:
				if err := visitInlines(node.Children); err != nil {
					return err
				}
			case *doc.LinkInline:
				if err := visitInlines(node.Children); err != nil {
					return err
				}
			case *doc.ImageInline:
				resolved := resolveLocalAsset(d.Dir, node.Src)
				if resolved != "" {
					if _, err := os.Stat(resolved); err != nil {
						return fmt.Errorf("figure %q: %w", node.Src, err)
					}
					*out = append(*out, resolved)
				}
			}
		}
		return nil
	}

	var visitBlocks func([]doc.Block) error
	visitBlocks = func(blocks []doc.Block) error {
		for _, blk := range blocks {
			switch node := blk.(type) {
			case *doc.HeadingBlock:
				if err := visitInlines(node.Inlines); err != nil {
					return err
				}
			case *doc.ParagraphBlock:
				if err := visitInlines(node.Inlines); err != nil {
					return err
				}
			case *doc.ListBlock:
				for _, item := range node.Items {
					if err := visitBlocks(item); err != nil {
						return err
					}
				}
			case *doc.BlockquoteBlock:
				if err := visitBlocks(node.Blocks); err != nil {
					return err
				}
			case *doc.MarginNoteBlock:
				if err := visitBlocks(node.Blocks); err != nil {
					return err
				}
			case *doc.WideBlock:
				if err := visitBlocks(node.Blocks); err != nil {
					return err
				}
			case *doc.ColumnsBlock:
				for _, col := range node.Columns {
					if err := visitBlocks(col); err != nil {
						return err
					}
				}
			case *doc.TableBlock:
				for _, cell := range node.Headers {
					if err := visitInlines(cell.Inlines); err != nil {
						return err
					}
				}
				for _, row := range node.Rows {
					for _, cell := range row {
						if err := visitInlines(cell.Inlines); err != nil {
							return err
						}
					}
				}
			case *doc.FigureBlock:
				if isLocalFigure(node.Src) {
					target := node.ResolvedPath
					if target == "" {
						target = resolveLocalAsset(d.Dir, node.Src)
					}
					if target != "" {
						if _, err := os.Stat(target); err != nil {
							return fmt.Errorf("figure %q: %w", node.Src, err)
						}
						*out = append(*out, target)
					}
				}
			}
		}
		return nil
	}

	if err := visitBlocks(d.Blocks); err != nil {
		return err
	}
	for _, sn := range d.Sidenotes {
		if err := visitBlocks(sn.Blocks); err != nil {
			return err
		}
	}
	return nil
}

func isLocalFigure(src string) bool {
	trimmed := strings.TrimSpace(src)
	lower := strings.ToLower(trimmed)
	return trimmed != "" &&
		!strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "data:")
}

func resolveLocalAsset(docDir, src string) string {
	if !isLocalFigure(src) {
		return ""
	}
	trimmed := strings.TrimSpace(src)
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	if docDir == "" {
		return filepath.Clean(filepath.FromSlash(trimmed))
	}
	return filepath.Join(docDir, filepath.FromSlash(trimmed))
}

func commonAncestorDir(a, b string) (string, bool) {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if filepath.VolumeName(a) != filepath.VolumeName(b) {
		return "", false
	}
	sep := string(filepath.Separator)
	vol := filepath.VolumeName(a)
	aRest := strings.TrimPrefix(a, vol)
	bRest := strings.TrimPrefix(b, vol)

	aParts := splitPathParts(aRest)
	bParts := splitPathParts(bRest)
	limit := len(aParts)
	if len(bParts) < limit {
		limit = len(bParts)
	}
	var common []string
	for i := 0; i < limit; i++ {
		if aParts[i] != bParts[i] {
			break
		}
		common = append(common, aParts[i])
	}
	if strings.HasPrefix(aRest, sep) {
		return vol + sep + filepath.Join(common...), true
	}
	if len(common) == 0 {
		return ".", true
	}
	return vol + filepath.Join(common...), true
}

func splitPathParts(p string) []string {
	var parts []string
	for _, seg := range strings.Split(p, string(filepath.Separator)) {
		if seg != "" && seg != "." {
			parts = append(parts, seg)
		}
	}
	return parts
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// ResolveBibPath resolves cfg.Bibliography relative to docDir when non-empty and relative.
func ResolveBibPath(rawBib, docDir string) string {
	trimmed := strings.TrimSpace(rawBib)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) || docDir == "" {
		return filepath.Clean(trimmed)
	}
	return filepath.Join(docDir, filepath.FromSlash(trimmed))
}

// ResolveCSL resolves cfg.CSL to either a local CSL file path (isFile=true) or a built-in Typst CSL style name (isFile=false).
func ResolveCSL(rawCSL, docDir string) (style string, isFile bool, err error) {
	trimmed := strings.TrimSpace(rawCSL)
	if trimmed == "" {
		return "", false, nil
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) && docDir != "" {
		if _, statErr := os.Stat(candidate); statErr != nil {
			candidate = filepath.Join(docDir, filepath.FromSlash(trimmed))
		}
	}
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
		return filepath.Clean(candidate), true, nil
	}

	base := filepath.Base(trimmed)
	ext := strings.ToLower(filepath.Ext(base))
	if ext == ".csl" || ext == ".xml" {
		return "", false, fmt.Errorf("csl %q: file not found", rawCSL)
	}
	// When config.Resolve joins a bare built-in CSL name (e.g. "apa" or "abnt") onto BaseDir,
	// allow resolving the bare slug as a Typst built-in citation style.
	if base != "" && !strings.Contains(base, ".") {
		return mapBuiltinCSLAlias(base), false, nil
	}
	return "", false, fmt.Errorf("csl %q: file not found", rawCSL)
}

func mapBuiltinCSLAlias(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "abnt":
		return "associacao-brasileira-de-normas-tecnicas"
	case "chicago":
		return "chicago-author-date"
	default:
		return strings.TrimSpace(name)
	}
}

// ResolveTitle returns the editorial title from metadata, falling back to SourceDocument filename or a neutral title.
func ResolveTitle(d *doc.SourceDocument) string {
	if d == nil {
		return "DataFolio Document"
	}
	title := strings.TrimSpace(d.Meta.Title)
	if title == "" && d.Path != "" {
		base := filepath.Base(d.Path)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if title == "" {
		return "DataFolio Document"
	}
	return title
}

// TypstLangRegion maps canonical DataFolio language tags ("en", "pt-BR") to Typst lang and region codes.
func TypstLangRegion(lang string) (string, string) {
	norm, err := school.NormalizeLang(lang)
	if err == nil && norm == "pt-BR" {
		return "pt", "BR"
	}
	return "en", ""
}

// TypstColor formats a CSS/DTCG color value into a Typst `rgb(...)` expression.
func TypstColor(raw string) string {
	rgba, err := tokens.ParseColor(raw)
	if err != nil {
		return TypstString(strings.TrimSpace(raw))
	}
	return fmt.Sprintf("rgb(%q)", rgba.Hex())
}

// TypstFont formats a font family or comma-separated font stack as a Typst font value.
func TypstFont(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return `""`
	}
	if !strings.ContainsRune(s, ',') {
		name := strings.Trim(s, `"'`)
		return TypstString(name)
	}
	parts := strings.Split(s, ",")
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.Trim(strings.TrimSpace(p), `"'`)
		if name != "" {
			quoted = append(quoted, TypstString(name))
		}
	}
	if len(quoted) == 0 {
		return `""`
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return "(" + strings.Join(quoted, ", ") + ")"
}

// TypstDim converts CSS/DTCG dimensions (`px`, `rem`, `pt`, `mm`, etc.) into Typst lengths.
func TypstDim(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "0pt"
	}
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "px") {
		numStr := strings.TrimSpace(s[:len(s)-2])
		if v, err := strconv.ParseFloat(numStr, 64); err == nil {
			if v == 0 {
				return "0pt"
			}
			return strconv.FormatFloat(v*0.75, 'f', -1, 64) + "pt"
		}
	}
	if strings.HasSuffix(lower, "rem") {
		numStr := strings.TrimSpace(s[:len(s)-3])
		if v, err := strconv.ParseFloat(numStr, 64); err == nil {
			return strconv.FormatFloat(v, 'f', -1, 64) + "em"
		}
	}
	return s
}

// TypstString encodes a Go string into a safe Typst string literal preserving UTF-8 characters.
func TypstString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u{%x}`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// EscapeMarkup escapes prose text for Typst markup mode so literal characters cannot trigger Typst syntax.
func EscapeMarkup(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	atLineStart := true
	inLeadingDigits := false
	for i, r := range s {
		switch r {
		case '\\', '#', '$', '*', '_', '`', '<', '>', '@', '[', ']', '~', '/':
			b.WriteByte('\\')
			b.WriteRune(r)
			atLineStart = false
			inLeadingDigits = false
		case '-', '+', '=':
			if atLineStart {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
			if r != '=' {
				atLineStart = false
			}
			inLeadingDigits = false
		case '.':
			if inLeadingDigits {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
			atLineStart = false
			inLeadingDigits = false
		case '\n':
			b.WriteByte('\n')
			atLineStart = true
			inLeadingDigits = false
		default:
			b.WriteRune(r)
			if r >= '0' && r <= '9' && (atLineStart || inLeadingDigits) {
				inLeadingDigits = true
				atLineStart = false
			} else if unicode.IsSpace(r) && atLineStart {
				// Stay at logical line start across leading indentation.
				_ = i
			} else {
				atLineStart = false
				inLeadingDigits = false
			}
		}
	}
	return b.String()
}

// ParsePercentRatio parses percentage strings like "65%" into a 0..1 fraction.
func ParsePercentRatio(raw string, fallback float64) float64 {
	trimmed := strings.TrimSpace(raw)
	if strings.HasSuffix(trimmed, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(trimmed, "%")), 64); err == nil && v > 0 {
			return v / 100.0
		}
	}
	return fallback
}

// EmitSchoolPreamble writes canonical DataFolio School, Theme, font, booktabs, and localized label bindings.
func EmitSchoolPreamble(b *strings.Builder, tok school.SchoolTokens, labels school.Labels) {
	fmt.Fprintf(b, "#let df-paper = %s\n", TypstColor(tok.Paper))
	fmt.Fprintf(b, "#let df-paper-subtle = %s\n", TypstColor(tok.PaperSubtle))
	fmt.Fprintf(b, "#let df-paper-alt = %s\n", TypstColor(tok.PaperAlt))
	fmt.Fprintf(b, "#let df-ink = %s\n", TypstColor(tok.Ink))
	fmt.Fprintf(b, "#let df-ink-soft = %s\n", TypstColor(tok.InkSoft))
	fmt.Fprintf(b, "#let df-muted = %s\n", TypstColor(tok.Muted))
	fmt.Fprintf(b, "#let df-accent = %s\n", TypstColor(tok.Accent))
	fmt.Fprintf(b, "#let df-ochre = %s\n", TypstColor(tok.Ochre))
	fmt.Fprintf(b, "#let df-forest = %s\n", TypstColor(tok.Forest))
	fmt.Fprintf(b, "#let df-rule = %s\n", TypstColor(tok.Rule))
	fmt.Fprintf(b, "#let df-rule-strong = %s\n", TypstColor(tok.RuleStrong))
	fmt.Fprintf(b, "#let df-font-serif = %s\n", TypstFont(tok.SerifFont))
	fmt.Fprintf(b, "#let df-font-mono = %s\n", TypstFont(tok.MonoFont))
	fmt.Fprintf(b, "#let df-font-sans = %s\n", TypstFont(tok.SansFont))
	fmt.Fprintf(b, "#let df-booktabs-top = %s\n", TypstDim(tok.BooktabsTopRule))
	fmt.Fprintf(b, "#let df-booktabs-mid = %s\n", TypstDim(tok.BooktabsMidRule))
	fmt.Fprintf(b, "#let df-booktabs-row = %s\n", TypstDim(tok.BooktabsRowRule))
	fmt.Fprintf(b, "#let df-label-figure = %s\n", TypstString(fallbackStr(labels.Figure, "Figure")))
	fmt.Fprintf(b, "#let df-label-table = %s\n", TypstString(fallbackStr(labels.Table, "Table")))
	fmt.Fprintf(b, "#let df-label-source = %s\n", TypstString(fallbackStr(labels.Source, "Source")))
	fmt.Fprintf(b, "#let df-label-margin-note = %s\n", TypstString(fallbackStr(labels.MarginNote, "Margin Note")))
	fmt.Fprintf(b, "#let df-label-references = %s\n", TypstString(fallbackStr(labels.References, "References")))
	fmt.Fprintf(b, "#let df-label-contents = %s\n", TypstString(fallbackStr(labels.Contents, "Contents")))
}

// NormalizedColumnRatios resolves normalized main, gap, and margin column ratios from SchoolTokens.
func NormalizedColumnRatios(tok school.SchoolTokens) (mainR, gapR, marginR float64) {
	mainR = ParsePercentRatio(tok.MainWidth, 0.65)
	gapR = ParsePercentRatio(tok.ColumnGap, 0.05)
	marginR = ParsePercentRatio(tok.MarginWidth, 0.30)
	total := mainR + gapR + marginR
	if total <= 0 {
		return 0.65, 0.05, 0.30
	}
	return mainR / total, gapR / total, marginR / total
}

// ResolveBibliographyAssets parses bibliography entries and resolves/maps bibliography and CSL paths for Typst.
func ResolveBibliographyAssets(d *doc.SourceDocument, cfg config.Config, opts RenderOptions) (bibEntries map[string]doc.BibEntry, mappedBibPath string, hasBib bool, cslArg string, err error) {
	if bibPath := ResolveBibPath(cfg.Bibliography, d.Dir); bibPath != "" {
		entries, parseErr := doc.ParseBibFile(bibPath)
		if parseErr != nil {
			return nil, "", false, "", fmt.Errorf("bibliography %q: %w", cfg.Bibliography, parseErr)
		}
		bibEntries = entries
		hasBib = true
		if opts.AssetMapper != nil {
			mappedBibPath, err = opts.AssetMapper(bibPath)
			if err != nil {
				return nil, "", false, "", fmt.Errorf("bibliography %q: %w", cfg.Bibliography, err)
			}
		} else {
			mappedBibPath = bibPath
		}
	}

	var cslIsFile bool
	cslArg, cslIsFile, err = ResolveCSL(cfg.CSL, d.Dir)
	if err != nil {
		return nil, "", false, "", err
	}
	if cslIsFile && opts.AssetMapper != nil {
		cslArg, err = opts.AssetMapper(cslArg)
		if err != nil {
			return nil, "", false, "", fmt.Errorf("csl %q: %w", cfg.CSL, err)
		}
	}
	return bibEntries, mappedBibPath, hasBib, cslArg, nil
}

// HasCitedEntries reports whether at least one cited key exists in bibEntries.
func HasCitedEntries(citations []string, bibEntries map[string]doc.BibEntry) bool {
	for _, key := range citations {
		if _, ok := bibEntries[key]; ok {
			return true
		}
	}
	return false
}

// EmitLanguageSetText writes the Typst base `#set text(...)` rule with language/region and tabular numerals.
func EmitLanguageSetText(b *strings.Builder, lang, sizeExpr string) {
	typLang, typRegion := TypstLangRegion(lang)
	if typRegion != "" {
		fmt.Fprintf(b, "#set text(font: df-font-serif, fill: df-ink, size: %s, lang: %s, region: %s, number-width: \"tabular\")\n",
			sizeExpr, TypstString(typLang), TypstString(typRegion))
		return
	}
	fmt.Fprintf(b, "#set text(font: df-font-serif, fill: df-ink, size: %s, lang: %s, number-width: \"tabular\")\n",
		sizeExpr, TypstString(typLang))
}

// EmitTypstHelpers writes shared Typst helper functions for section lookup, margin placement, sidenotes, and semantic containers.
func EmitTypstHelpers(b *strings.Builder) {
	b.WriteString(`#let df-section() = {
  let pg = here().page()
  let hs = query(heading.where(level: df-sec-level, outlined: true))
  if hs.len() == 0 {
    hs = query(heading.where(outlined: true))
  }
  let current = ""
  for h in hs {
    let hpg = h.location().page()
    if hpg < pg {
      current = h.body
    } else if hpg == pg {
      current = h.body
      break
    }
  }
  current
}

#let df-place-margin(body, align-offset: 0pt, page-origin: false) = context {
  let pos = here().position()
  let pg = pos.page
  let anchor-y = pos.y
  let prevs = query(selector(<df-margin-mark>).before(here()))
  let min-y = df-top-margin.to-absolute()
  for m in prevs {
    if m.value.page == pg and m.value.bottom > min-y {
      min-y = m.value.bottom
    }
  }
  let note-box = block(width: df-margin-w, breakable: false)[
    #set text(font: df-font-sans, size: 8.2pt, fill: df-ink-soft)
    #set par(leading: 0.52em, justify: false)
    #body
  ]
  let sz = measure(note-box)
  let max-y = page.height - df-bottom-margin.to-absolute() - sz.height
  let desired-y = anchor-y - align-offset.to-absolute()
  let target-y = calc.max(calc.min(desired-y, max-y), min-y)
  let bottom-y = target-y + sz.height + 5pt
  let dx = if page-origin { df-main-w + df-gap-w } else { (df-outer-x + df-main-w + df-gap-w).to-absolute() - pos.x }
  let dy = if page-origin { target-y - df-top-margin.to-absolute() } else { target-y - anchor-y }
  [#metadata((page: pg, bottom: bottom-y)) <df-margin-mark>]
  place(top + left, dx: dx, dy: dy, note-box)
}

#let df-sidenote(num, body) = {
  super[#text(fill: df-accent, weight: "semibold")[#num]]
  box(width: 0pt, height: 0pt)[
    #df-place-margin(align-offset: 0.7em)[
      #text(fill: df-accent, weight: "semibold")[#num. ]#body
    ]
  ]
}

#let df-margin-note(body) = {
  df-place-margin(align-offset: 0pt, page-origin: true, body)
}

#let df-inline-note(body) = block(
  width: 100%,
  inset: (left: 10pt, y: 6pt, right: 8pt),
  fill: df-paper-subtle,
  stroke: (left: 2pt + df-rule-strong),
)[
  #set text(font: df-font-sans, size: 8.8pt, fill: df-ink-soft)
  #body
]

#let df-wide(body) = {
  block(width: df-printable-w, breakable: true)[#body]
  context [#metadata((page: here().page(), bottom: here().position().y + 5pt)) <df-margin-mark>]
}

#let df-kpi(body) = block(
  width: 100%,
  inset: (y: 6pt),
  stroke: (top: df-booktabs-top + df-rule-strong, bottom: df-booktabs-top + df-rule-strong),
)[#body]

#let df-blockquote(body) = block(
  width: 100%,
  inset: (left: 10pt, y: 2pt),
  stroke: (left: 2pt + df-accent),
)[
  #set text(fill: df-ink-soft, style: "italic")
  #body
]

#let df-code-block(body) = block(
  width: 100%,
  inset: 8pt,
  fill: df-paper-subtle,
  stroke: 0.5pt + df-rule,
)[
  #set text(font: df-font-mono, size: 8.5pt, fill: df-ink)
  #body
]

`)
}

// EmitSharedShowRules writes Typst show rules common to both `document` and `slides` Formats.
func EmitSharedShowRules(b *strings.Builder, secLevel int, figGap string) {
	b.WriteString("#show raw.where(block: false): it => box(fill: df-paper-subtle, inset: (x: 2.5pt, y: 0pt), outset: (y: 1.5pt), stroke: 0.4pt + df-rule)[#text(font: df-font-mono, size: 8.8pt)[#it]]\n")
	b.WriteString("#show link: set text(fill: df-accent)\n")
	fmt.Fprintf(b, "#show figure: set block(above: %s, below: %s)\n", figGap, figGap)
	b.WriteString("#show figure.where(kind: table): set block(breakable: true)\n")
	b.WriteString("#show figure.where(kind: table): set figure.caption(position: top)\n")
	b.WriteString("#show figure.caption: set text(font: df-font-sans, size: 8.5pt, fill: df-ink-soft)\n")
	fmt.Fprintf(b, "#show bibliography: set heading(level: %d)\n\n", secLevel)
}

// EmitCoverBlock writes the editorial cover typography block (kicker, title, subtitle, heavy rule, author/date).
func EmitCoverBlock(b *strings.Builder, meta doc.Metadata, fallbackTitle, topSpace string, useWide bool) {
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = fallbackTitle
	}
	if useWide {
		b.WriteString("#df-wide[\n")
	} else {
		b.WriteString("#block(width: df-printable-w)[\n")
	}
	fmt.Fprintf(b, "  #v(%s)\n", topSpace)
	if meta.Kicker != "" {
		fmt.Fprintf(b, "  #text(font: df-font-sans, size: 9pt, weight: \"semibold\", fill: df-accent, tracking: 0.12em)[#upper(%s)]\n  #v(0.6em)\n",
			TypstString(meta.Kicker))
	}
	fmt.Fprintf(b, "  #text(size: 28pt, weight: \"medium\", fill: df-ink)[%s]\n", EscapeMarkup(title))
	if meta.Subtitle != "" {
		fmt.Fprintf(b, "  #v(0.6em)\n  #text(size: 14pt, style: \"italic\", fill: df-ink-soft)[%s]\n", EscapeMarkup(meta.Subtitle))
	}
	b.WriteString("  #v(1.4em)\n  #line(length: 100%, stroke: 1.5pt + df-rule-strong)\n  #v(0.9em)\n")
	if meta.Author != "" || meta.Date != "" {
		b.WriteString("  #text(font: df-font-sans, size: 10pt, fill: df-muted)[")
		if meta.Author != "" {
			b.WriteString(EscapeMarkup(meta.Author))
		}
		if meta.Author != "" && meta.Date != "" {
			b.WriteString("  ·  ")
		}
		if meta.Date != "" {
			b.WriteString(EscapeMarkup(meta.Date))
		}
		b.WriteString("]\n")
	}
	b.WriteString("]\n")
}

// EmitBibliography writes a Typst `#bibliography(...)` block with optional CSL style.
func EmitBibliography(b *strings.Builder, mappedBibPath, cslArg string, includeTitle bool) {
	titleOpt := ""
	if includeTitle {
		titleOpt = ", title: [#df-label-references]"
	} else {
		titleOpt = ", title: none"
	}
	if cslArg != "" {
		fmt.Fprintf(b, "#bibliography(%s%s, style: %s)\n",
			TypstString(mappedBibPath), titleOpt, TypstString(cslArg))
		return
	}
	fmt.Fprintf(b, "#bibliography(%s%s)\n", TypstString(mappedBibPath), titleOpt)
}

func fallbackStr(val, fb string) string {
	if strings.TrimSpace(val) == "" {
		return fb
	}
	return val
}

// EffectiveChromeSpec resolves discreet document defaults when a ChromeSpec is enabled with empty slots.
func EffectiveChromeSpec(spec config.ChromeSpec, isHeader bool, d *doc.SourceDocument, fallbackTitle string) config.ChromeSpec {
	if !spec.Enabled {
		return spec
	}
	if spec.Left != "" || spec.Center != "" || spec.Right != "" {
		return spec
	}
	title := ResolveTitle(d)
	if title == "" {
		title = fallbackTitle
	}
	out := spec
	if isHeader {
		out.Left = strings.TrimSpace(d.Meta.Kicker)
		if out.Left == "" {
			out.Left = title
		}
		if hasAnyHeading(d.Blocks) {
			out.Right = "{section}"
		} else {
			out.Right = strings.TrimSpace(d.Meta.Date)
		}
	} else {
		out.Left = strings.TrimSpace(d.Meta.Author)
		if out.Left == "" {
			out.Left = strings.TrimSpace(d.Meta.Date)
		}
		if out.Left == "" {
			out.Left = title
		}
		out.Right = "{page} / {pages}"
	}
	return out
}

func hasAnyHeading(blocks []doc.Block) bool {
	for _, blk := range blocks {
		if _, ok := blk.(*doc.HeadingBlock); ok {
			return true
		}
	}
	return false
}

// BuildChromeExpr produces a Typst expression for page header or footer Running Chrome.
func BuildChromeExpr(spec config.ChromeSpec, isHeader bool, d *doc.SourceDocument, fallbackTitle string, widthExpr string) string {
	eff := EffectiveChromeSpec(spec, isHeader, d, fallbackTitle)
	if !eff.Enabled || (eff.Left == "" && eff.Center == "" && eff.Right == "") {
		return "none"
	}

	leftExpr := BuildChromeSlotExpr(eff.Left, d, fallbackTitle)
	centerExpr := BuildChromeSlotExpr(eff.Center, d, fallbackTitle)
	rightExpr := BuildChromeSlotExpr(eff.Right, d, fallbackTitle)

	var b strings.Builder
	b.WriteString("context {\n")
	if eff.SkipFirst {
		b.WriteString("    if here().page() == 1 { return }\n")
	}
	fmt.Fprintf(&b, "    block(width: %s)[\n", widthExpr)
	b.WriteString("      #set text(font: df-font-sans, size: 7.8pt, fill: df-muted, tracking: 0.04em)\n")
	if !isHeader && eff.Rule {
		b.WriteString("      #line(length: 100%, stroke: 0.5pt + df-rule)\n")
		b.WriteString("      #v(4pt)\n")
	}
	b.WriteString("      #grid(\n")
	b.WriteString("        columns: (1fr, auto, 1fr),\n")
	b.WriteString("        align: (left, center, right),\n")
	fmt.Fprintf(&b, "        %s,\n        %s,\n        %s,\n", leftExpr, centerExpr, rightExpr)
	b.WriteString("      )\n")
	if isHeader && eff.Rule {
		b.WriteString("      #v(4pt)\n")
		b.WriteString("      #line(length: 100%, stroke: 0.5pt + df-rule)\n")
	}
	b.WriteString("    ]\n")
	b.WriteString("  }")
	return b.String()
}

// BuildChromeSlotExpr interpolates static document metadata and dynamic Typst page/section queries for one slot.
func BuildChromeSlotExpr(tmpl string, d *doc.SourceDocument, fallbackTitle string) string {
	if tmpl == "" {
		return "[]"
	}
	title := ResolveTitle(d)
	if title == "" {
		title = fallbackTitle
	}
	var metaSubtitle, metaKicker, metaAuthor, metaDate string
	if d != nil {
		metaSubtitle = d.Meta.Subtitle
		metaKicker = d.Meta.Kicker
		metaAuthor = d.Meta.Author
		metaDate = d.Meta.Date
	}
	staticReplaced := strings.NewReplacer(
		"{title}", title,
		"{subtitle}", metaSubtitle,
		"{kicker}", metaKicker,
		"{author}", metaAuthor,
		"{date}", metaDate,
	).Replace(tmpl)

	tokensList := []string{"{section}", "{page}", "{pages}"}
	var parts []string
	rest := staticReplaced
	for rest != "" {
		nextIdx := -1
		nextTok := ""
		for _, tok := range tokensList {
			if idx := strings.Index(rest, tok); idx >= 0 && (nextIdx < 0 || idx < nextIdx) {
				nextIdx = idx
				nextTok = tok
			}
		}
		if nextIdx < 0 {
			parts = append(parts, "#("+TypstString(rest)+")")
			break
		}
		if nextIdx > 0 {
			parts = append(parts, "#("+TypstString(rest[:nextIdx])+")")
		}
		switch nextTok {
		case "{section}":
			parts = append(parts, "#df-section()")
		case "{page}":
			parts = append(parts, "#str(counter(page).get().first())")
		case "{pages}":
			parts = append(parts, "#str(counter(page).final().first())")
		}
		rest = rest[nextIdx+len(nextTok):]
	}
	if len(parts) == 0 {
		return "[]"
	}
	return "[" + strings.Join(parts, "") + "]"
}

// DetectMinHeadingLevel returns the smallest heading level present in blocks (defaulting to 2).
func DetectMinHeadingLevel(blocks []doc.Block) int {
	minLvl := 0
	for _, blk := range blocks {
		if h, ok := blk.(*doc.HeadingBlock); ok {
			lvl := h.Level
			if lvl < 1 {
				lvl = 1
			}
			if minLvl == 0 || lvl < minLvl {
				minLvl = lvl
			}
		}
	}
	if minLvl == 0 {
		return 2
	}
	return minLvl
}

// RenderBlocks renders a slice of semantic blocks into Typst markup.
func RenderBlocks(b *strings.Builder, blocks []doc.Block, ctx *RenderContext) error {
	for i, blk := range blocks {
		if i > 0 {
			b.WriteByte('\n')
		}
		if err := RenderBlock(b, blk, ctx); err != nil {
			return err
		}
	}
	return nil
}

// RenderBlock renders a single semantic block into Typst markup.
func RenderBlock(b *strings.Builder, blk doc.Block, ctx *RenderContext) error {
	switch node := blk.(type) {
	case *doc.HeadingBlock:
		lvl := node.Level
		if ctx != nil && ctx.MinBodyHeadingLevel > 0 && lvl < ctx.MinBodyHeadingLevel {
			lvl = ctx.MinBodyHeadingLevel
		}
		if lvl < 1 {
			lvl = 1
		}
		if lvl > 6 {
			lvl = 6
		}
		if node.ID != "" {
			b.WriteString(fmt.Sprintf("#heading(level: %d)[", lvl))
			if err := RenderInlines(b, node.Inlines, ctx); err != nil {
				return err
			}
			fmt.Fprintf(b, "]#label(%s)\n", TypstString("heading-"+node.ID))
		} else {
			b.WriteString(fmt.Sprintf("#heading(level: %d)[", lvl))
			if err := RenderInlines(b, node.Inlines, ctx); err != nil {
				return err
			}
			b.WriteString("]\n")
		}
	case *doc.ParagraphBlock:
		if err := RenderInlines(b, node.Inlines, ctx); err != nil {
			return err
		}
		b.WriteByte('\n')
	case *doc.ListBlock:
		fn := "list"
		if node.Ordered {
			fn = "enum"
		}
		fmt.Fprintf(b, "#%s(\n", fn)
		for _, item := range node.Items {
			b.WriteString("  [\n")
			nested := *ctx
			nested.TopLevel = false
			for _, sub := range item {
				if p, ok := sub.(*doc.ParagraphBlock); ok {
					b.WriteString("    ")
					if err := RenderInlines(b, p.Inlines, &nested); err != nil {
						return err
					}
					b.WriteString("\n\n")
					continue
				}
				if err := RenderBlock(b, sub, &nested); err != nil {
					return err
				}
			}
			b.WriteString("  ],\n")
		}
		b.WriteString(")\n")
	case *doc.BlockquoteBlock:
		b.WriteString("#df-blockquote[\n")
		nested := *ctx
		nested.TopLevel = false
		if err := RenderBlocks(b, node.Blocks, &nested); err != nil {
			return err
		}
		b.WriteString("]\n")
	case *doc.CodeBlock:
		if strings.TrimSpace(node.Language) != "" {
			fmt.Fprintf(b, "#df-code-block[#raw(block: true, lang: %s, %s)]\n", TypstString(node.Language), TypstString(strings.TrimSuffix(node.Code, "\n")))
		} else {
			fmt.Fprintf(b, "#df-code-block[#raw(block: true, %s)]\n", TypstString(strings.TrimSuffix(node.Code, "\n")))
		}
	case *doc.ThematicBreakBlock:
		b.WriteString("#v(0.6em)\n#line(length: 100%, stroke: 0.5pt + df-rule)\n#v(0.6em)\n")
	case *doc.TableBlock:
		if err := renderTable(b, node, ctx); err != nil {
			return err
		}
	case *doc.FigureBlock:
		if err := renderFigure(b, node, ctx); err != nil {
			return err
		}
	case *doc.MarginNoteBlock:
		if err := renderMarginNote(b, node, ctx); err != nil {
			return err
		}
	case *doc.WideBlock:
		b.WriteString("#df-wide[\n")
		nested := *ctx
		nested.TopLevel = false
		if err := RenderBlocks(b, node.Blocks, &nested); err != nil {
			return err
		}
		b.WriteString("]\n")
	case *doc.KPIBlock:
		renderKPI(b, node)
	case *doc.ColumnsBlock:
		if err := renderColumns(b, node, ctx); err != nil {
			return err
		}
	case *doc.MathBlock:
		typMath, err := math.ToTypst(node.LaTeX, true)
		if err != nil {
			return err
		}
		b.WriteString("#v(0.25em)\n")
		b.WriteString(typMath)
		b.WriteString("\n#v(0.25em)\n")
	}
	return nil
}

func renderTable(b *strings.Builder, node *doc.TableBlock, ctx *RenderContext) error {
	colCount := len(node.Headers)
	for _, r := range node.Rows {
		if len(r) > colCount {
			colCount = len(r)
		}
	}
	if colCount == 0 {
		return nil
	}

	colSpecs := make([]string, colCount)
	alignSpecs := make([]string, colCount)
	for i := 0; i < colCount; i++ {
		colSpecs[i] = "1fr"
		alignSpecs[i] = typstTableAlign(node.Alignments, node.NumericCols, i)
	}

	b.WriteString("#{\n")
	hasFigureWrap := node.Caption != ""
	if hasFigureWrap {
		b.WriteString("  figure(\n")
		b.WriteString("    kind: table,\n")
		b.WriteString("    supplement: df-label-table,\n")
		fmt.Fprintf(b, "    caption: [%s],\n", EscapeMarkup(node.Caption))
		b.WriteString("    [\n")
	}

	b.WriteString("      #table(\n")
	fmt.Fprintf(b, "        columns: (%s),\n", strings.Join(colSpecs, ", "))
	fmt.Fprintf(b, "        align: (%s),\n", strings.Join(alignSpecs, ", "))
	b.WriteString("        inset: (x: 6pt, y: 5pt),\n")
	b.WriteString("        stroke: none,\n")

	if len(node.Headers) > 0 {
		b.WriteString("        table.header(\n")
		b.WriteString("          table.hline(stroke: df-booktabs-top + df-rule-strong),\n")
		for col := 0; col < colCount; col++ {
			b.WriteString("          table.cell[")
			if col < len(node.Headers) {
				b.WriteString("#text(font: df-font-sans, size: 8.8pt, weight: \"semibold\")[")
				if err := RenderInlines(b, node.Headers[col].Inlines, ctx); err != nil {
					return err
				}
				b.WriteByte(']')
			}
			b.WriteString("],\n")
		}
		b.WriteString("          table.hline(stroke: df-booktabs-mid + df-rule-strong),\n")
		b.WriteString("          repeat: true,\n")
		b.WriteString("        ),\n")
	} else {
		b.WriteString("        table.hline(stroke: df-booktabs-top + df-rule-strong),\n")
	}

	for rIdx, row := range node.Rows {
		for col := 0; col < colCount; col++ {
			b.WriteString("        [")
			if col < len(row) {
				if err := RenderInlines(b, row[col].Inlines, ctx); err != nil {
					return err
				}
			}
			b.WriteString("],\n")
		}
		if rIdx+1 < len(node.Rows) {
			b.WriteString("        table.hline(stroke: df-booktabs-row + df-rule),\n")
		}
	}
	b.WriteString("        table.hline(stroke: df-booktabs-top + df-rule-strong),\n")
	b.WriteString("      )\n")

	if node.Source != "" {
		fmt.Fprintf(b, "      #v(0.2em)\n      #align(left)[#text(font: df-font-sans, size: 8pt, fill: df-muted)[*%s:* %s]]\n",
			EscapeMarkup(fallbackStr(ctx.Labels.Source, "Source")), EscapeMarkup(node.Source))
	}

	if hasFigureWrap {
		b.WriteString("    ]\n")
		b.WriteString("  )\n")
	}
	b.WriteString("}\n")
	return nil
}

func typstTableAlign(aligns []doc.ColumnAlign, numeric []bool, col int) string {
	isNum := col < len(numeric) && numeric[col]
	align := doc.AlignDefault
	if col < len(aligns) {
		align = aligns[col]
	}
	switch align {
	case doc.AlignCenter:
		return "center"
	case doc.AlignRight:
		return "right"
	case doc.AlignLeft:
		return "left"
	default:
		if isNum {
			return "right"
		}
		return "left"
	}
}

func renderFigure(b *strings.Builder, node *doc.FigureBlock, ctx *RenderContext) error {
	imgPath, err := mapFigurePath(node.Src, node.ResolvedPath, ctx)
	if err != nil {
		return err
	}

	b.WriteString("#figure(\n")
	b.WriteString("  kind: image,\n")
	b.WriteString("  supplement: df-label-figure,\n")
	if node.Caption != "" || node.Source != "" {
		b.WriteString("  caption: [")
		if node.Caption != "" {
			b.WriteString(EscapeMarkup(node.Caption))
		}
		if node.Caption != "" && node.Source != "" {
			b.WriteString(" ")
		}
		if node.Source != "" {
			fmt.Fprintf(b, "#text(fill: df-muted)[*%s:* %s]",
				EscapeMarkup(fallbackStr(ctx.Labels.Source, "Source")),
				EscapeMarkup(node.Source))
		}
		b.WriteString("],\n")
	}
	fmt.Fprintf(b, "  image(%s, width: 100%%),\n", TypstString(imgPath))
	b.WriteString(")\n")
	return nil
}

func mapFigurePath(rawSrc, resolvedPath string, ctx *RenderContext) (string, error) {
	if !isLocalFigure(rawSrc) {
		return strings.TrimSpace(rawSrc), nil
	}
	target := resolvedPath
	if target == "" && ctx != nil && ctx.Doc != nil {
		target = resolveLocalAsset(ctx.Doc.Dir, rawSrc)
	}
	if target == "" {
		return "", fmt.Errorf("figure %q: empty path", rawSrc)
	}
	if _, err := os.Stat(target); err != nil {
		return "", fmt.Errorf("figure %q: %w", rawSrc, err)
	}
	if ctx != nil && ctx.Opts.AssetMapper != nil {
		return ctx.Opts.AssetMapper(target)
	}
	return filepath.ToSlash(target), nil
}

func renderMarginNote(b *strings.Builder, node *doc.MarginNoteBlock, ctx *RenderContext) error {
	label := strings.TrimSpace(node.Title)
	if label == "" {
		label = fallbackStr(ctx.Labels.MarginNote, "Margin Note")
	}
	nested := *ctx
	nested.TopLevel = false
	nested.InMargin = true

	if ctx.TopLevel && !ctx.InMargin {
		b.WriteString("#df-margin-note[\n")
	} else {
		b.WriteString("#df-inline-note[\n")
	}
	if label != "" {
		fmt.Fprintf(b, "  #text(weight: \"semibold\", fill: df-ink)[%s:] ", EscapeMarkup(label))
	}
	if err := renderCompactBlocks(b, node.Blocks, &nested); err != nil {
		return err
	}
	b.WriteString("]\n")
	return nil
}

func renderCompactBlocks(b *strings.Builder, blocks []doc.Block, ctx *RenderContext) error {
	for i, blk := range blocks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if p, ok := blk.(*doc.ParagraphBlock); ok {
			if err := RenderInlines(b, p.Inlines, ctx); err != nil {
				return err
			}
			continue
		}
		if err := RenderBlock(b, blk, ctx); err != nil {
			return err
		}
	}
	return nil
}

func renderKPI(b *strings.Builder, node *doc.KPIBlock) {
	if len(node.Items) == 0 {
		return
	}
	cols := make([]string, len(node.Items))
	for i := range cols {
		cols[i] = "1fr"
	}
	b.WriteString("#df-kpi[\n")
	fmt.Fprintf(b, "  #grid(columns: (%s), column-gutter: 12pt,\n", strings.Join(cols, ", "))
	for i, item := range node.Items {
		b.WriteString("    box(width: 100%, inset: (y: 4pt)")
		if i+1 < len(node.Items) {
			b.WriteString(", stroke: (right: 0.5pt + df-rule)")
		}
		b.WriteString(")[\n")
		if item.Label != "" {
			fmt.Fprintf(b, "      #text(font: df-font-sans, size: 7.5pt, fill: df-muted, tracking: 0.06em)[#upper(%s)]#linebreak()\n", TypstString(item.Label))
		}
		if item.Value != "" {
			fmt.Fprintf(b, "      #text(font: df-font-serif, size: 15pt, weight: \"semibold\", fill: df-ink)[%s]\n", EscapeMarkup(item.Value))
		}
		if item.Note != "" {
			fmt.Fprintf(b, "      #linebreak()#text(font: df-font-sans, size: 7.8pt, fill: df-ink-soft)[%s]\n", EscapeMarkup(item.Note))
		}
		b.WriteString("    ],\n")
	}
	b.WriteString("  )\n")
	b.WriteString("]\n")
}

func renderColumns(b *strings.Builder, node *doc.ColumnsBlock, ctx *RenderContext) error {
	if len(node.Columns) == 0 {
		return nil
	}
	cols := make([]string, len(node.Columns))
	for i := range cols {
		cols[i] = "1fr"
	}
	wrapWide := ctx != nil && ctx.FullWidthColumns && ctx.TopLevel
	nested := *ctx
	nested.TopLevel = false
	if wrapWide {
		b.WriteString("#df-wide[\n")
	}
	fmt.Fprintf(b, "#grid(columns: (%s), column-gutter: 16pt,\n", strings.Join(cols, ", "))
	for _, col := range node.Columns {
		b.WriteString("  [\n")
		if err := RenderBlocks(b, col, &nested); err != nil {
			return err
		}
		b.WriteString("  ],\n")
	}
	b.WriteString(")\n")
	if wrapWide {
		b.WriteString("]\n")
	}
	return nil
}

// RenderInlines renders a slice of inline nodes into Typst markup.
func RenderInlines(b *strings.Builder, inlines []doc.Inline, ctx *RenderContext) error {
	for _, in := range inlines {
		switch node := in.(type) {
		case *doc.TextInline:
			b.WriteString(EscapeMarkup(node.Text))
		case *doc.EmphasisInline:
			if node.Strong {
				b.WriteString("#strong[")
			} else {
				b.WriteString("#emph[")
			}
			if err := RenderInlines(b, node.Children, ctx); err != nil {
				return err
			}
			b.WriteByte(']')
		case *doc.CodeInline:
			fmt.Fprintf(b, "#raw(%s)", TypstString(node.Code))
		case *doc.LinkInline:
			if err := renderLinkInline(b, node, ctx); err != nil {
				return err
			}
		case *doc.ImageInline:
			imgPath, err := mapFigurePath(node.Src, "", ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "#image(%s, width: 100%%)", TypstString(imgPath))
		case *doc.SoftBreakInline:
			b.WriteByte(' ')
		case *doc.HardBreakInline:
			b.WriteString("#linebreak()")
		case *doc.SidenoteRefInline:
			if err := renderSidenoteRefInline(b, node, ctx); err != nil {
				return err
			}
		case *doc.MathInline:
			typMath, err := math.ToTypst(node.LaTeX, node.Display)
			if err != nil {
				return err
			}
			b.WriteString(typMath)
		case *doc.CitationInline:
			renderCitationInline(b, node, ctx)
		}
	}
	return nil
}

func renderLinkInline(b *strings.Builder, node *doc.LinkInline, ctx *RenderContext) error {
	trimmedURL := strings.TrimSpace(node.URL)
	if strings.HasPrefix(trimmedURL, "#") && len(trimmedURL) > 1 {
		targetID := strings.TrimPrefix(trimmedURL, "#")
		fmt.Fprintf(b, "#link(label(%s))[", TypstString("heading-"+targetID))
	} else {
		fmt.Fprintf(b, "#link(%s)[", TypstString(sanitizeLinkURL(trimmedURL)))
	}
	if err := RenderInlines(b, node.Children, ctx); err != nil {
		return err
	}
	b.WriteByte(']')
	return nil
}

func sanitizeLinkURL(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "vbscript:") {
		return "#"
	}
	return strings.TrimSpace(raw)
}

func renderSidenoteRefInline(b *strings.Builder, node *doc.SidenoteRefInline, ctx *RenderContext) error {
	if !node.Defined || ctx == nil || ctx.Doc == nil {
		fmt.Fprintf(b, "#super[#text(fill: df-accent)[\\[%s\\]]]", EscapeMarkup(node.ID))
		return nil
	}
	if ctx.InMargin {
		fmt.Fprintf(b, "#super[#text(fill: df-accent, weight: \"semibold\")[%d]]", node.Number)
		return nil
	}
	if ctx.RenderedSidenotes[node.ID] {
		fmt.Fprintf(b, "#super[#text(fill: df-accent, weight: \"semibold\")[%d]]", node.Number)
		return nil
	}
	def, ok := ctx.Doc.SidenoteByID(node.ID)
	if !ok {
		fmt.Fprintf(b, "#super[#text(fill: df-accent)[\\[%s\\]]]", EscapeMarkup(node.ID))
		return nil
	}
	ctx.RenderedSidenotes[node.ID] = true

	nested := *ctx
	nested.InMargin = true
	nested.TopLevel = false

	fmt.Fprintf(b, "#df-sidenote(%d)[", def.Number)
	if def.Title != "" {
		fmt.Fprintf(b, "#text(weight: \"semibold\", fill: df-ink)[%s] ", EscapeMarkup(def.Title))
	}
	if err := renderCompactBlocks(b, def.Blocks, &nested); err != nil {
		return err
	}
	b.WriteByte(']')
	return nil
}

func renderCitationInline(b *strings.Builder, node *doc.CitationInline, ctx *RenderContext) {
	if ctx == nil || !ctx.HasBibliography || len(ctx.BibEntries) == 0 {
		b.WriteString(EscapeMarkup(node.Raw))
		return
	}
	for i, item := range node.Items {
		if i > 0 {
			b.WriteByte(' ')
		}
		if item.Prefix != "" {
			b.WriteString(EscapeMarkup(item.Prefix) + " ")
		}
		if _, exists := ctx.BibEntries[item.Key]; exists {
			if item.Locator != "" {
				fmt.Fprintf(b, "#cite(label(%s), supplement: [%s])", TypstString(item.Key), EscapeMarkup(item.Locator))
			} else {
				fmt.Fprintf(b, "#cite(label(%s))", TypstString(item.Key))
			}
		} else {
			b.WriteString(EscapeMarkup("[@" + item.Key + "]"))
		}
	}
}
