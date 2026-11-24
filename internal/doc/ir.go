package doc

// Metadata holds editorial metadata decoded from a Source Document's YAML frontmatter.
type Metadata struct {
	Title    string `yaml:"title"`
	Subtitle string `yaml:"subtitle"`
	Kicker   string `yaml:"kicker"`
	Author   string `yaml:"author"`
	Date     string `yaml:"date"`
}

// SidenoteDef holds a parsed numbered Sidenote definition (`[^id]: ...`).
type SidenoteDef struct {
	ID         string
	Number     int
	Title      string
	Blocks     []Block
	Inlines    []Inline
	Line       int
	Referenced bool
}

// SidenoteRef records a single inline Sidenote reference (`[^id]`) and its resolution status.
type SidenoteRef struct {
	ID      string
	Number  int
	Line    int
	Defined bool
}

// SlideChromeOverride captures per-slide Running Chrome overrides parsed from HTML comments or CLI flags.
type SlideChromeOverride struct {
	Set             bool
	Enabled         *bool
	Left            string
	Center          string
	Right           string
	HasLeft         bool
	HasCenter       bool
	HasRight        bool
	ReplaceAllSlots bool
	SkipFirst       *bool
	Rule            *bool
	Raw             string
}

// Slide represents a single logical slide split at top-level `---` thematic breaks.
type Slide struct {
	Index  int
	Kicker string
	Title  string
	Header SlideChromeOverride
	Footer SlideChromeOverride
	Blocks []Block
	Line   int
}

// SourceDocument is the School-agnostic Semantic Intermediate Representation of a Markdown file.
type SourceDocument struct {
	Path           string
	Dir            string
	Meta           Metadata
	RawFrontmatter []byte
	BodyLineOffset int
	Blocks         []Block
	Sidenotes      []SidenoteDef
	SidenoteRefs   []SidenoteRef
	Citations      []string
	Slides         []Slide
}

// SidenoteByID looks up a parsed Sidenote definition by its raw Sidenote identifier.
func (d *SourceDocument) SidenoteByID(id string) (SidenoteDef, bool) {
	if d == nil {
		return SidenoteDef{}, false
	}
	for _, sn := range d.Sidenotes {
		if sn.ID == id {
			return sn, true
		}
	}
	return SidenoteDef{}, false
}

// Block represents a top-level semantic block inside a SourceDocument.
type Block interface {
	isBlock()
}

// Inline represents an inline semantic span inside prose, headings, or captions.
type Inline interface {
	isInline()
}

// HeadingBlock represents a section heading (h1-h6).
type HeadingBlock struct {
	Level   int
	ID      string
	Inlines []Inline
	Line    int
}

func (*HeadingBlock) isBlock() {}

// ParagraphBlock represents a paragraph of inline content.
type ParagraphBlock struct {
	Inlines []Inline
	Line    int
}

func (*ParagraphBlock) isBlock() {}

// ListBlock represents an ordered or unordered list.
type ListBlock struct {
	Ordered bool
	Items   [][]Block
	Line    int
}

func (*ListBlock) isBlock() {}

// BlockquoteBlock represents a quotation block.
type BlockquoteBlock struct {
	Blocks []Block
	Line   int
}

func (*BlockquoteBlock) isBlock() {}

// CodeBlock represents a fenced or indented code listing.
type CodeBlock struct {
	Language string
	Code     string
	Line     int
}

func (*CodeBlock) isBlock() {}

// ThematicBreakBlock represents a horizontal rule.
type ThematicBreakBlock struct {
	Line int
}

func (*ThematicBreakBlock) isBlock() {}

// ColumnAlign specifies horizontal alignment for a table column.
type ColumnAlign int

const (
	AlignDefault ColumnAlign = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// TableCell holds parsed inline spans and plain text for a single table cell.
type TableCell struct {
	Inlines []Inline
	Plain   string
}

// TableBlock represents a GFM booktabs table with optional caption and source attribution.
type TableBlock struct {
	Caption     string
	Source      string
	Headers     []TableCell
	Alignments  []ColumnAlign
	NumericCols []bool
	Rows        [][]TableCell
	Line        int
}

func (*TableBlock) isBlock() {}

// FigureBlock represents a standalone local or remote figure with caption and source attribution.
type FigureBlock struct {
	Src          string
	ResolvedPath string
	Caption      string
	Source       string
	Line         int
}

func (*FigureBlock) isBlock() {}

// MarginNoteBlock represents an unnumbered contextual Margin Note (`:::note` or blockquote form).
type MarginNoteBlock struct {
	Title  string
	Blocks []Block
	Line   int
}

func (*MarginNoteBlock) isBlock() {}

// WideBlock represents a full-width structural container (`:::wide`) spanning across the margin column.
type WideBlock struct {
	Blocks []Block
	Line   int
}

func (*WideBlock) isBlock() {}

// KPIItem holds a single key performance metric inside a KPIBlock.
type KPIItem struct {
	Label string
	Value string
	Note  string
}

// KPIBlock represents a booktabs KPI strip (`:::kpi`).
type KPIBlock struct {
	Items []KPIItem
	Line  int
}

func (*KPIBlock) isBlock() {}

// ColumnsBlock represents a multi-column side-by-side structural block (`:::columns`).
type ColumnsBlock struct {
	Columns [][]Block
	Line    int
}

func (*ColumnsBlock) isBlock() {}

// MathBlock represents a display-mode LaTeX math block (`$$...$$`).
type MathBlock struct {
	LaTeX string
	Line  int
}

func (*MathBlock) isBlock() {}

// TextInline holds plain text.
type TextInline struct {
	Text string
}

func (*TextInline) isInline() {}

// EmphasisInline holds italicized (Strong=false) or bold (Strong=true) inline spans.
type EmphasisInline struct {
	Strong   bool
	Children []Inline
}

func (*EmphasisInline) isInline() {}

// CodeInline holds an inline code span.
type CodeInline struct {
	Code string
}

func (*CodeInline) isInline() {}

// LinkInline holds a hyperlink.
type LinkInline struct {
	URL      string
	Title    string
	Children []Inline
}

func (*LinkInline) isInline() {}

// ImageInline holds an inline image reference.
type ImageInline struct {
	Src   string
	Alt   string
	Title string
}

func (*ImageInline) isInline() {}

// SoftBreakInline represents a single line break within a paragraph.
type SoftBreakInline struct{}

func (*SoftBreakInline) isInline() {}

// HardBreakInline represents an explicit line break.
type HardBreakInline struct{}

func (*HardBreakInline) isInline() {}

// SidenoteRefInline represents an inline numbered Sidenote anchor (`[^id]`).
type SidenoteRefInline struct {
	ID      string
	Number  int
	Defined bool
	Line    int
}

func (*SidenoteRefInline) isInline() {}

// MathInline represents an inline LaTeX math span (`$...$`).
type MathInline struct {
	LaTeX   string
	Display bool
}

func (*MathInline) isInline() {}

// CitationItem represents one bibliographic key inside a CitationInline.
type CitationItem struct {
	Key     string
	Prefix  string
	Locator string
}

// CitationInline represents a bibliographic citation span (`[@key]` or `[@key1; @key2]`).
type CitationInline struct {
	Items []CitationItem
	Raw   string
}

func (*CitationInline) isInline() {}
