package math

import (
	"strings"
	"unicode"
)

// SymbolKind classifies a mathematical symbol or operator for downstream compilers.
type SymbolKind int

const (
	SymbolOrd SymbolKind = iota
	SymbolOp
	SymbolRel
	SymbolBigOp
	SymbolDelim
	SymbolGreek
	SymbolPunct
)

// Node represents a node in the parsed LaTeX math AST.
type Node interface {
	isMathNode()
}

// SequenceNode represents an ordered sequence of math nodes.
type SequenceNode struct {
	Items []Node
}

func (*SequenceNode) isMathNode() {}

// NumberNode represents a numeric literal.
type NumberNode struct {
	Value string
}

func (*NumberNode) isMathNode() {}

// IdentNode represents a mathematical identifier or variable.
type IdentNode struct {
	Name string
}

func (*IdentNode) isMathNode() {}

// SymbolNode represents an operator, relation, Greek letter, or named symbol.
type SymbolNode struct {
	Command string
	Kind    SymbolKind
}

func (*SymbolNode) isMathNode() {}

// TextNode represents literal text inside `\text{...}` or `\mbox{...}`.
type TextNode struct {
	Text string
}

func (*TextNode) isMathNode() {}

// GroupNode represents an explicit `{...}` brace group.
type GroupNode struct {
	Body Node
}

func (*GroupNode) isMathNode() {}

// SubSupNode represents a base expression with an optional subscript and/or superscript.
type SubSupNode struct {
	Base Node
	Sub  Node
	Sup  Node
}

func (*SubSupNode) isMathNode() {}

// FracNode represents `\frac`, `\dfrac`, `\tfrac`, or `\binom`.
type FracNode struct {
	Num     Node
	Den     Node
	NoBar   bool
	Variant string
}

func (*FracNode) isMathNode() {}

// SqrtNode represents `\sqrt{...}` or `\sqrt[index]{...}`.
type SqrtNode struct {
	Index    Node
	Radicand Node
}

func (*SqrtNode) isMathNode() {}

// AccentNode represents an accent or bar over/under a base node.
type AccentNode struct {
	Command string
	Base    Node
}

func (*AccentNode) isMathNode() {}

// StyleNode represents a font/style command such as `\text`, `\mathbf`, `\mathbb`, `\mathcal`, etc.
type StyleNode struct {
	Command string
	Body    Node
}

func (*StyleNode) isMathNode() {}

// DelimNode represents a `\left<L> ... \right<R>` delimited expression.
type DelimNode struct {
	Left  string
	Right string
	Body  Node
}

func (*DelimNode) isMathNode() {}

// OverUnderNode represents `\overset`, `\underset`, or `\stackrel`.
type OverUnderNode struct {
	Command string
	Label   Node
	Base    Node
}

func (*OverUnderNode) isMathNode() {}

// EnvNode represents a LaTeX environment such as `aligned`, `cases`, `pmatrix`, `bmatrix`, or `array`.
type EnvNode struct {
	Name string
	Spec string
	Rows [][]Node
}

func (*EnvNode) isMathNode() {}

// SpaceNode represents explicit math spacing (`\,`, `\:`, `\;`, `\!`, `\quad`, `\qquad`, `\ `).
type SpaceNode struct {
	Command string
}

func (*SpaceNode) isMathNode() {}

// ParseLaTeX parses a LaTeX math expression into a semantic SequenceNode AST.
func ParseLaTeX(src string) (*SequenceNode, error) {
	p := &latexParser{src: src}
	seq := p.parseSequence("", "")
	return seq, nil
}

type latexParser struct {
	src string
	pos int
}

func (p *latexParser) eof() bool {
	return p.pos >= len(p.src)
}

func (p *latexParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *latexParser) skipSpaces() {
	for !p.eof() {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
			continue
		}
		if c == '%' {
			for !p.eof() && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *latexParser) parseSequence(stopChar string, stopEnv string) *SequenceNode {
	var items []Node
	for {
		p.skipSpaces()
		if p.eof() {
			break
		}
		if stopChar != "" && strings.HasPrefix(p.src[p.pos:], stopChar) {
			break
		}
		if stopEnv != "" && strings.HasPrefix(p.src[p.pos:], `\end`) {
			saved := p.pos
			p.pos += len(`\end`)
			p.skipSpaces()
			if name, ok := p.tryReadBracedRaw(); ok && name == stopEnv {
				p.pos = saved
				break
			}
			p.pos = saved
		}
		if stopEnv == "" && strings.HasPrefix(p.src[p.pos:], `\right`) {
			// Check if followed by delimiter (caller handles \right when inside DelimNode).
			if stopChar == `\right` {
				break
			}
		}

		atom := p.parseAtom(stopChar, stopEnv)
		if atom == nil {
			break
		}
		atom = p.parseScripts(atom)
		items = append(items, atom)
	}
	return &SequenceNode{Items: items}
}

func (p *latexParser) parseScripts(base Node) Node {
	var sub, sup Node
	for {
		p.skipSpaces()
		if p.eof() {
			break
		}
		c := p.peek()
		if c == '_' && sub == nil {
			p.pos++
			sub = p.parseArg()
			continue
		}
		if c == '^' && sup == nil {
			p.pos++
			sup = p.parseArg()
			continue
		}
		if (c == '_' || c == '^') && (sub != nil || sup != nil) {
			// Nested script like x_i_j
			base = &SubSupNode{Base: base, Sub: sub, Sup: sup}
			sub, sup = nil, nil
			continue
		}
		if c == '\'' {
			primes := 0
			for !p.eof() && p.peek() == '\'' {
				p.pos++
				primes++
			}
			primeSym := &SymbolNode{Command: strings.Repeat("'", primes), Kind: SymbolOrd}
			if sup == nil {
				sup = primeSym
			} else {
				sup = &SequenceNode{Items: []Node{sup, primeSym}}
			}
			continue
		}
		break
	}
	if sub != nil || sup != nil {
		return &SubSupNode{Base: base, Sub: sub, Sup: sup}
	}
	return base
}

func (p *latexParser) parseArg() Node {
	p.skipSpaces()
	if p.eof() {
		return &SequenceNode{}
	}
	if p.peek() == '{' {
		p.pos++
		seq := p.parseSequence("}", "")
		if !p.eof() && p.peek() == '}' {
			p.pos++
		}
		if len(seq.Items) == 1 {
			return seq.Items[0]
		}
		return seq
	}
	atom := p.parseAtom("", "")
	if atom == nil {
		return &SequenceNode{}
	}
	return atom
}

func (p *latexParser) parseAtom(stopChar string, stopEnv string) Node {
	p.skipSpaces()
	if p.eof() {
		return nil
	}
	c := p.peek()
	if stopChar != "" && strings.HasPrefix(p.src[p.pos:], stopChar) {
		return nil
	}
	switch {
	case c == '{':
		p.pos++
		seq := p.parseSequence("}", "")
		if !p.eof() && p.peek() == '}' {
			p.pos++
		}
		return &GroupNode{Body: seq}
	case c == '}':
		// Unmatched closing brace: consume to avoid infinite loop.
		p.pos++
		return nil
	case c == '\\':
		return p.parseCommand(stopEnv)
	case c >= '0' && c <= '9':
		start := p.pos
		p.pos++
		for !p.eof() && (p.peek() >= '0' && p.peek() <= '9') {
			p.pos++
		}
		if !p.eof() && p.peek() == '.' && p.pos+1 < len(p.src) && p.src[p.pos+1] >= '0' && p.src[p.pos+1] <= '9' {
			p.pos++
			for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
				p.pos++
			}
		}
		return &NumberNode{Value: p.src[start:p.pos]}
	case isASCIIAlpha(c):
		p.pos++
		return &IdentNode{Name: string(c)}
	case c == '~':
		p.pos++
		return &SpaceNode{Command: "~"}
	case c == '+' || c == '-' || c == '*' || c == '/' || c == '=' || c == '<' || c == '>':
		p.pos++
		kind := SymbolOp
		if c == '=' || c == '<' || c == '>' {
			kind = SymbolRel
		}
		return &SymbolNode{Command: string(c), Kind: kind}
	case c == '(' || c == ')' || c == '[' || c == ']' || c == '|':
		p.pos++
		return &SymbolNode{Command: string(c), Kind: SymbolDelim}
	case c == ',' || c == '.' || c == ':' || c == ';' || c == '!' || c == '?':
		if c == ':' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '=' {
			p.pos += 2
			return &SymbolNode{Command: ":=", Kind: SymbolRel}
		}
		p.pos++
		return &SymbolNode{Command: string(c), Kind: SymbolPunct}
	case c == '&':
		p.pos++
		return &SymbolNode{Command: "&", Kind: SymbolPunct}
	default:
		// Handle UTF-8 rune (e.g., direct Unicode math characters)
		r, size := decodeRune(p.src[p.pos:])
		p.pos += size
		if unicode.IsLetter(r) {
			return &IdentNode{Name: string(r)}
		}
		return &SymbolNode{Command: string(r), Kind: SymbolOrd}
	}
}

func (p *latexParser) parseCommand(stopEnv string) Node {
	p.pos++ // consume '\'
	if p.eof() {
		return &SymbolNode{Command: `\`, Kind: SymbolOrd}
	}
	c := p.peek()
	if !isASCIIAlpha(c) {
		p.pos++
		cmd := `\` + string(c)
		switch cmd {
		case `\,`, `\:`, `\;`, `\!`, `\ `:
			return &SpaceNode{Command: cmd}
		case `\\`:
			return &SymbolNode{Command: `\\`, Kind: SymbolPunct}
		case `\{`, `\}`, `\|`:
			return &SymbolNode{Command: cmd, Kind: SymbolDelim}
		default:
			return &SymbolNode{Command: cmd, Kind: SymbolOrd}
		}
	}

	start := p.pos
	for !p.eof() && isASCIIAlpha(p.peek()) {
		p.pos++
	}
	name := p.src[start:p.pos]
	cmd := `\` + name

	switch name {
	case "quad", "qquad", "enspace", "thinspace":
		return &SpaceNode{Command: cmd}
	case "limits", "nolimits":
		// Formatting hint; skip cleanly.
		return p.parseAtom("", stopEnv)
	case "frac", "dfrac", "tfrac":
		num := p.parseArg()
		den := p.parseArg()
		variant := ""
		if name != "frac" {
			variant = name
		}
		return &FracNode{Num: num, Den: den, Variant: variant}
	case "binom", "dbinom", "tbinom":
		num := p.parseArg()
		den := p.parseArg()
		return &FracNode{Num: num, Den: den, NoBar: true, Variant: "binom"}
	case "sqrt":
		p.skipSpaces()
		var idx Node
		if !p.eof() && p.peek() == '[' {
			p.pos++
			idx = p.parseSequence("]", "")
			if !p.eof() && p.peek() == ']' {
				p.pos++
			}
		}
		rad := p.parseArg()
		return &SqrtNode{Index: idx, Radicand: rad}
	case "hat", "widehat", "bar", "overline", "tilde", "widetilde", "vec", "dot", "ddot", "underline":
		base := p.parseArg()
		return &AccentNode{Command: cmd, Base: base}
	case "text", "mbox", "hbox":
		txt := p.readBracedText()
		return &StyleNode{Command: cmd, Body: &TextNode{Text: txt}}
	case "operatorname":
		txt := p.readBracedText()
		return &StyleNode{Command: cmd, Body: &TextNode{Text: txt}}
	case "mathrm", "mathbf", "boldsymbol", "mathit", "mathcal", "mathbb", "mathsf", "mathtt":
		body := p.parseArg()
		return &StyleNode{Command: cmd, Body: body}
	case "overset", "underset", "stackrel":
		label := p.parseArg()
		base := p.parseArg()
		return &OverUnderNode{Command: cmd, Label: label, Base: base}
	case "left":
		leftDelim := p.readDelimiter()
		body := p.parseSequence(`\right`, stopEnv)
		rightDelim := "."
		if strings.HasPrefix(p.src[p.pos:], `\right`) {
			p.pos += len(`\right`)
			rightDelim = p.readDelimiter()
		}
		return &DelimNode{Left: leftDelim, Right: rightDelim, Body: body}
	case "begin":
		envName, _ := p.tryReadBracedRaw()
		spec := ""
		if envName == "array" {
			p.skipSpaces()
			if s, ok := p.tryReadBracedRaw(); ok {
				spec = s
			}
		}
		rows := p.parseEnvRows(envName)
		return &EnvNode{Name: envName, Spec: spec, Rows: rows}
	}

	kind := classifyCommand(cmd)
	return &SymbolNode{Command: cmd, Kind: kind}
}

func (p *latexParser) parseEnvRows(envName string) [][]Node {
	var rows [][]Node
	var curRow []Node
	var curCell []Node

	flushCell := func() {
		if len(curCell) == 1 {
			curRow = append(curRow, curCell[0])
		} else {
			curRow = append(curRow, &SequenceNode{Items: curCell})
		}
		curCell = nil
	}
	flushRow := func() {
		if len(curCell) > 0 || len(curRow) > 0 {
			flushCell()
			rows = append(rows, curRow)
			curRow = nil
		}
	}

	for {
		p.skipSpaces()
		if p.eof() {
			break
		}
		if strings.HasPrefix(p.src[p.pos:], `\end`) {
			saved := p.pos
			p.pos += len(`\end`)
			p.skipSpaces()
			if name, ok := p.tryReadBracedRaw(); ok && name == envName {
				break
			}
			p.pos = saved
		}
		if p.peek() == '&' {
			p.pos++
			flushCell()
			continue
		}
		if strings.HasPrefix(p.src[p.pos:], `\\`) {
			p.pos += 2
			// Optional spacing argument like \\[4pt]
			p.skipSpaces()
			if !p.eof() && p.peek() == '[' {
				for !p.eof() && p.peek() != ']' {
					p.pos++
				}
				if !p.eof() && p.peek() == ']' {
					p.pos++
				}
			}
			flushRow()
			continue
		}
		atom := p.parseAtom("", envName)
		if atom == nil {
			break
		}
		atom = p.parseScripts(atom)
		curCell = append(curCell, atom)
	}
	flushRow()
	return rows
}

func (p *latexParser) tryReadBracedRaw() (string, bool) {
	p.skipSpaces()
	if p.eof() || p.peek() != '{' {
		return "", false
	}
	p.pos++
	start := p.pos
	depth := 1
	for !p.eof() && depth > 0 {
		switch p.src[p.pos] {
		case '\\':
			p.pos += 2
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				res := p.src[start:p.pos]
				p.pos++
				return res, true
			}
		}
		p.pos++
	}
	return p.src[start:p.pos], true
}

func (p *latexParser) readBracedText() string {
	p.skipSpaces()
	if p.eof() {
		return ""
	}
	if p.peek() != '{' {
		atom := p.parseAtom("", "")
		if id, ok := atom.(*IdentNode); ok {
			return id.Name
		}
		return ""
	}
	p.pos++
	var b strings.Builder
	depth := 1
	for !p.eof() && depth > 0 {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			next := p.src[p.pos+1]
			switch next {
			case '$', '%', '&', '#', '_', '{', '}':
				b.WriteByte(next)
				p.pos += 2
				continue
			case '\\':
				b.WriteByte('\\')
				p.pos += 2
				continue
			}
		}
		if c == '{' {
			depth++
			p.pos++
			continue
		}
		if c == '}' {
			depth--
			p.pos++
			continue
		}
		b.WriteByte(c)
		p.pos++
	}
	return b.String()
}

func (p *latexParser) readDelimiter() string {
	p.skipSpaces()
	if p.eof() {
		return "."
	}
	c := p.peek()
	if c == '\\' {
		p.pos++
		if p.eof() {
			return "."
		}
		next := p.peek()
		if !isASCIIAlpha(next) {
			p.pos++
			return `\` + string(next)
		}
		start := p.pos
		for !p.eof() && isASCIIAlpha(p.peek()) {
			p.pos++
		}
		return `\` + p.src[start:p.pos]
	}
	p.pos++
	return string(c)
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func decodeRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 1
}

func classifyCommand(cmd string) SymbolKind {
	if _, ok := greekMap[cmd]; ok {
		return SymbolGreek
	}
	if _, ok := bigOpMap[cmd]; ok {
		return SymbolBigOp
	}
	if _, ok := relMap[cmd]; ok {
		return SymbolRel
	}
	if _, ok := opMap[cmd]; ok {
		return SymbolOp
	}
	if _, ok := delimMap[cmd]; ok {
		return SymbolDelim
	}
	return SymbolOrd
}
