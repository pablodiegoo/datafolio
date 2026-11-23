package doc

// Metadata holds editorial metadata decoded from a Source Document's YAML frontmatter.
type Metadata struct {
	Title    string `yaml:"title"`
	Subtitle string `yaml:"subtitle"`
	Kicker   string `yaml:"kicker"`
	Author   string `yaml:"author"`
	Date     string `yaml:"date"`
}

// SourceDocument is the School-agnostic Semantic Intermediate Representation of a Markdown file.
type SourceDocument struct {
	Path           string
	Dir            string
	Meta           Metadata
	RawFrontmatter []byte
	BodyLineOffset int
	Blocks         []Block
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
