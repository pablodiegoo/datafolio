package math

import (
	"fmt"
	"strconv"
	"strings"
)

// ToTypst parses a LaTeX math expression and compiles it to Typst math markup (ADR-0003).
func ToTypst(latex string, display bool) (string, error) {
	astNode, err := ParseLaTeX(latex)
	if err != nil {
		return "", err
	}
	return RenderTypst(astNode, display)
}

// RenderTypst compiles a parsed LaTeX math AST node into Typst math syntax (`$...$` or `$ ... $`).
func RenderTypst(node Node, display bool) (string, error) {
	body := strings.TrimSpace(renderTypstNode(node, false))
	if body == "" {
		if display {
			return "$ $", nil
		}
		return "$$", nil
	}
	if display {
		return "$ " + body + " $", nil
	}
	return "$" + body + "$", nil
}

// RenderTypstBody compiles a parsed LaTeX math AST node into its inner Typst math expression without `$` delimiters.
func RenderTypstBody(node Node) string {
	return strings.TrimSpace(renderTypstNode(node, false))
}

func renderTypstNode(node Node, inArg bool) string {
	if node == nil {
		return ""
	}
	switch n := node.(type) {
	case *SequenceNode:
		return renderTypstSequence(n.Items, inArg)
	case *GroupNode:
		return renderTypstNode(n.Body, inArg)
	case *NumberNode:
		return n.Value
	case *IdentNode:
		return n.Name
	case *TextNode:
		return strconv.Quote(n.Text)
	case *SpaceNode:
		return renderTypstSpace(n.Command)
	case *SymbolNode:
		return renderTypstSymbol(n, inArg)
	case *SubSupNode:
		return renderTypstSubSup(n)
	case *FracNode:
		num := renderTypstNode(n.Num, true)
		den := renderTypstNode(n.Den, true)
		if n.NoBar {
			return fmt.Sprintf("binom(%s, %s)", num, den)
		}
		return fmt.Sprintf("frac(%s, %s)", num, den)
	case *SqrtNode:
		rad := renderTypstNode(n.Radicand, true)
		if n.Index != nil {
			idx := renderTypstNode(n.Index, true)
			return fmt.Sprintf("root(%s, %s)", idx, rad)
		}
		return fmt.Sprintf("sqrt(%s)", rad)
	case *AccentNode:
		return renderTypstAccent(n)
	case *StyleNode:
		return renderTypstStyle(n, inArg)
	case *DelimNode:
		return renderTypstDelim(n)
	case *OverUnderNode:
		return renderTypstOverUnder(n)
	case *EnvNode:
		return renderTypstEnv(n)
	default:
		return ""
	}
}

func renderTypstSequence(items []Node, inArg bool) string {
	var b strings.Builder
	var prev Node
	var prevText string

	for _, item := range items {
		if item == nil {
			continue
		}
		cur := renderTypstNode(item, inArg)
		if cur == "" {
			continue
		}
		if b.Len() > 0 && needsTypstMathSpace(prev, prevText, item, cur) {
			b.WriteByte(' ')
		}
		b.WriteString(cur)
		prev = item
		prevText = cur
	}
	return b.String()
}

func needsTypstMathSpace(prev Node, prevText string, cur Node, curText string) bool {
	if prevText == "" || curText == "" {
		return false
	}
	if isNoSpaceAfter(prev, prevText) {
		return false
	}
	if isNoSpaceBefore(cur, curText) {
		return false
	}
	return true
}

func isNoSpaceAfter(n Node, text string) bool {
	if sym, ok := n.(*SymbolNode); ok {
		switch sym.Command {
		case "(", "[", `\{`:
			return true
		}
	}
	return strings.HasSuffix(text, "(") || strings.HasSuffix(text, "[")
}

func isNoSpaceBefore(n Node, text string) bool {
	if sym, ok := n.(*SymbolNode); ok {
		switch sym.Command {
		case "(", ")", "[", "]", ",", ".", ";", ":", "!", "?", "'", "''", "'''", `\}`, `\rangle`, `\rfloor`, `\rceil`:
			return true
		}
	}
	return text == ")" || text == "]" || text == "," || text == `","` || text == "." || text == ";" || text == ":" || strings.HasPrefix(text, "'")
}

func renderTypstSpace(cmd string) string {
	switch cmd {
	case `\,`, `\thinspace`:
		return "thin"
	case `\:`:
		return "med"
	case `\;`:
		return "thick"
	case `\!`:
		return "#h(-0.167em)"
	case `\ `, `~`, `\enspace`:
		return "#h(0.5em)"
	case `\quad`:
		return "quad"
	case `\qquad`:
		return "wide"
	default:
		return "thin"
	}
}

func renderTypstSubSup(n *SubSupNode) string {
	base := renderTypstBase(n.Base)
	var b strings.Builder
	b.WriteString(base)
	if n.Sub != nil {
		b.WriteByte('_')
		b.WriteString(wrapTypstScript(n.Sub))
	}
	if n.Sup != nil {
		if sym, ok := n.Sup.(*SymbolNode); ok && (sym.Command == "'" || sym.Command == "''" || sym.Command == "'''") {
			b.WriteString(sym.Command)
		} else {
			b.WriteByte('^')
			b.WriteString(wrapTypstScript(n.Sup))
		}
	}
	return b.String()
}

func renderTypstBase(base Node) string {
	switch b := base.(type) {
	case *GroupNode:
		inner := renderTypstNode(b.Body, false)
		if isSimpleScriptAtom(b.Body) {
			return inner
		}
		return "(" + inner + ")"
	case *SequenceNode:
		inner := renderTypstNode(b, false)
		if isSimpleScriptAtom(b) {
			return inner
		}
		return "(" + inner + ")"
	default:
		return renderTypstNode(base, false)
	}
}

func wrapTypstScript(script Node) string {
	inner := renderTypstNode(script, false)
	if inner == "" {
		return "()"
	}
	if isSimpleScriptAtom(script) {
		return inner
	}
	return "(" + inner + ")"
}

func isSimpleScriptAtom(n Node) bool {
	switch node := n.(type) {
	case *IdentNode:
		return true
	case *NumberNode:
		return !strings.ContainsRune(node.Value, '.')
	case *SymbolNode:
		if _, ok := typstGreekMap[node.Command]; ok {
			return true
		}
		return node.Command == `\infty`
	case *GroupNode:
		return isSimpleScriptAtom(node.Body)
	case *SequenceNode:
		if len(node.Items) == 1 {
			return isSimpleScriptAtom(node.Items[0])
		}
		return false
	default:
		return false
	}
}

func renderTypstAccent(n *AccentNode) string {
	inner := renderTypstNode(n.Base, true)
	fn := "hat"
	switch n.Command {
	case `\hat`, `\widehat`:
		fn = "hat"
	case `\bar`:
		fn = "macron"
	case `\overline`:
		fn = "overline"
	case `\tilde`, `\widetilde`:
		fn = "tilde"
	case `\vec`:
		fn = "arrow"
	case `\dot`:
		fn = "dot"
	case `\ddot`:
		fn = "dot.double"
	case `\underline`:
		fn = "underline"
	}
	return fmt.Sprintf("%s(%s)", fn, inner)
}

func renderTypstStyle(n *StyleNode, inArg bool) string {
	switch n.Command {
	case `\text`, `\mbox`, `\hbox`:
		if txt, ok := n.Body.(*TextNode); ok {
			return strconv.Quote(txt.Text)
		}
		return renderTypstNode(n.Body, inArg)
	case `\operatorname`:
		if txt, ok := n.Body.(*TextNode); ok {
			return fmt.Sprintf("op(%s)", strconv.Quote(txt.Text))
		}
		return fmt.Sprintf("op(%s)", renderTypstNode(n.Body, true))
	case `\mathbb`:
		return renderStyledFunc("bb", n.Body, false)
	case `\mathcal`:
		return renderStyledFunc("cal", n.Body, false)
	case `\mathbf`, `\boldsymbol`:
		return renderStyledFunc("bold", n.Body, false)
	case `\mathrm`:
		return renderStyledFunc("upright", n.Body, false)
	case `\mathit`:
		return renderStyledFunc("italic", n.Body, false)
	case `\mathsf`:
		return renderStyledFunc("sans", n.Body, false)
	case `\mathtt`:
		return renderStyledFunc("mono", n.Body, true)
	default:
		return renderTypstNode(n.Body, inArg)
	}
}

func renderStyledFunc(fn string, body Node, alwaysQuoteMulti bool) string {
	var textBuf strings.Builder
	if collectStyleChars(body, &textBuf) {
		txt := textBuf.String()
		if txt != "" && (len(txt) > 1 || alwaysQuoteMulti) {
			return fmt.Sprintf("%s(%s)", fn, strconv.Quote(txt))
		}
	}
	return fmt.Sprintf("%s(%s)", fn, renderTypstNode(body, true))
}

func renderTypstDelim(n *DelimNode) string {
	left := mapTypstDelim(n.Left)
	right := mapTypstDelim(n.Right)
	body := renderTypstNode(n.Body, false)

	var b strings.Builder
	b.WriteString("lr(")
	if left != "" {
		b.WriteString(left)
		if body != "" && needsDelimInnerSpace(left, true) {
			b.WriteByte(' ')
		}
	}
	b.WriteString(body)
	if right != "" {
		if body != "" && needsDelimInnerSpace(right, false) {
			b.WriteByte(' ')
		}
		b.WriteString(right)
	}
	b.WriteByte(')')
	return b.String()
}

func mapTypstDelim(d string) string {
	switch d {
	case "", ".":
		return ""
	case `\{`:
		return `\{`
	case `\}`:
		return `\}`
	case `\|`:
		return "‖"
	case `\langle`:
		return "chevron.l"
	case `\rangle`:
		return "chevron.r"
	case `\lfloor`:
		return "floor.l"
	case `\rfloor`:
		return "floor.r"
	case `\lceil`:
		return "ceil.l"
	case `\rceil`:
		return "ceil.r"
	default:
		return d
	}
}

func needsDelimInnerSpace(delim string, _ bool) bool {
	switch delim {
	case "(", ")", "[", "]", `\{`, `\}`:
		return false
	default:
		return true
	}
}

func renderTypstOverUnder(n *OverUnderNode) string {
	base := renderTypstNode(n.Base, true)
	label := renderTypstNode(n.Label, false)
	if n.Command == `\underset` {
		return fmt.Sprintf("limits(%s)_(%s)", base, label)
	}
	return fmt.Sprintf("limits(%s)^(%s)", base, label)
}

func renderTypstEnv(n *EnvNode) string {
	switch n.Name {
	case "aligned", "align", "align*", "split", "eqnarray":
		var rows []string
		for _, row := range n.Rows {
			var cells []string
			for _, cell := range row {
				cells = append(cells, strings.TrimSpace(renderTypstNode(cell, false)))
			}
			rows = append(rows, strings.Join(cells, " & "))
		}
		return strings.Join(rows, ` \ `)
	case "cases":
		var rows []string
		for _, row := range n.Rows {
			var cells []string
			for _, cell := range row {
				cells = append(cells, strings.TrimSpace(renderTypstNode(cell, true)))
			}
			rows = append(rows, strings.Join(cells, " & "))
		}
		return "cases(" + strings.Join(rows, ", ") + ")"
	default:
		delim := "#none"
		switch n.Name {
		case "pmatrix":
			delim = `"("`
		case "bmatrix":
			delim = `"["`
		case "Bmatrix":
			delim = `"{"`
		case "vmatrix":
			delim = `"|"`
		case "Vmatrix":
			delim = `"‖"`
		}
		var rows []string
		for _, row := range n.Rows {
			var cells []string
			for _, cell := range row {
				cells = append(cells, strings.TrimSpace(renderTypstNode(cell, true)))
			}
			rows = append(rows, strings.Join(cells, ", "))
		}
		return fmt.Sprintf("mat(delim: %s, %s)", delim, strings.Join(rows, "; "))
	}
}

func renderTypstSymbol(n *SymbolNode, inArg bool) string {
	if g, ok := typstGreekMap[n.Command]; ok {
		return g
	}
	if op, ok := typstBigOpMap[n.Command]; ok {
		return op
	}
	if rel, ok := typstRelMap[n.Command]; ok {
		return rel
	}
	if op, ok := typstOpMap[n.Command]; ok {
		return op
	}
	if d, ok := typstDelimSymMap[n.Command]; ok {
		return d
	}
	if m, ok := typstMiscMap[n.Command]; ok {
		return m
	}
	if inArg && n.Command == "," {
		return `","`
	}
	switch n.Command {
	case "+", "-", "*", "/", "=", "<", ">", "(", ")", "[", "]", "|", ",", ".", ":", ";", "!", "?", ":=", "&":
		return n.Command
	case "'", "''", "'''":
		return n.Command
	case `\$`, `\%`, `\&`, `\#`, `\_`:
		return `\` + strings.TrimPrefix(n.Command, `\`)
	case `\\`:
		return `\`
	default:
		if strings.HasPrefix(n.Command, `\`) {
			name := strings.TrimPrefix(n.Command, `\`)
			return strconv.Quote(name)
		}
		return n.Command
	}
}

var typstGreekMap = map[string]string{
	`\alpha`: "alpha", `\beta`: "beta", `\gamma`: "gamma", `\delta`: "delta",
	`\epsilon`: "epsilon.alt", `\varepsilon`: "epsilon", `\zeta`: "zeta", `\eta`: "eta",
	`\theta`: "theta", `\vartheta`: "theta.alt", `\iota`: "iota", `\kappa`: "kappa",
	`\lambda`: "lambda", `\mu`: "mu", `\nu`: "nu", `\xi`: "xi",
	`\omicron`: "omicron", `\pi`: "pi", `\varpi`: "pi.alt", `\rho`: "rho",
	`\varrho`: "rho.alt", `\sigma`: "sigma", `\varsigma`: "sigma.alt", `\tau`: "tau",
	`\upsilon`: "upsilon", `\phi`: "phi.alt", `\varphi`: "phi", `\chi`: "chi",
	`\psi`: "psi", `\omega`: "omega",
	`\Alpha`: "Alpha", `\Beta`: "Beta", `\Gamma`: "Gamma", `\Delta`: "Delta",
	`\Epsilon`: "Epsilon", `\Zeta`: "Zeta", `\Eta`: "Eta", `\Theta`: "Theta",
	`\Iota`: "Iota", `\Kappa`: "Kappa", `\Lambda`: "Lambda", `\Mu`: "Mu",
	`\Nu`: "Nu", `\Xi`: "Xi", `\Omicron`: "Omicron", `\Pi`: "Pi",
	`\Rho`: "Rho", `\Sigma`: "Sigma", `\Tau`: "Tau", `\Upsilon`: "Upsilon",
	`\Phi`: "Phi", `\Chi`: "Chi", `\Psi`: "Psi", `\Omega`: "Omega",
}

var typstBigOpMap = map[string]string{
	`\sum`:       "sum",
	`\prod`:      "product",
	`\coprod`:    "product.co",
	`\bigcup`:    "union.big",
	`\bigcap`:    "inter.big",
	`\bigoplus`:  "plus.o.big",
	`\bigotimes`: "times.o.big",
	`\lim`:       "lim",
	`\limsup`:    `limits(op("lim sup"))`,
	`\liminf`:    `limits(op("lim inf"))`,
	`\sup`:       "sup",
	`\inf`:       "inf",
	`\max`:       "max",
	`\min`:       "min",
	`\Pr`:        "Pr",
	`\int`:       "integral",
	`\iint`:      "integral.double",
	`\iiint`:     "integral.triple",
	`\oint`:      "integral.cont",
	`\arg`:       "arg",
	`\det`:       "det",
	`\dim`:       "dim",
	`\gcd`:       "gcd",
	`\log`:       "log",
	`\ln`:        "ln",
	`\exp`:       "exp",
	`\sin`:       "sin",
	`\cos`:       "cos",
	`\tan`:       "tan",
	`\cot`:       "cot",
	`\sec`:       "sec",
	`\csc`:       "csc",
	`\arcsin`:    "arcsin",
	`\arccos`:    "arccos",
	`\arctan`:    "arctan",
	`\sinh`:      "sinh",
	`\cosh`:      "cosh",
	`\tanh`:      "tanh",
}

var typstRelMap = map[string]string{
	`\le`:             "<=",
	`\leq`:            "<=",
	`\ge`:             ">=",
	`\geq`:            ">=",
	`\neq`:            "!=",
	`\ne`:             "!=",
	`\approx`:         "approx",
	`\equiv`:          "equiv",
	`\sim`:            "tilde.op",
	`\simeq`:          "tilde.eq",
	`\cong`:           "tilde.equiv",
	`\propto`:         "prop",
	`\ll`:             "lt.double",
	`\gg`:             "gt.double",
	`\subset`:         "subset",
	`\subseteq`:       "subset.eq",
	`\supset`:         "supset",
	`\supseteq`:       "supset.eq",
	`\in`:             "in",
	`\notin`:          "in.not",
	`\ni`:             "in.rev",
	`\to`:             "->",
	`\rightarrow`:     "->",
	`\leftarrow`:      "<-",
	`\leftrightarrow`: "<->",
	`\Rightarrow`:     "=>",
	`\Leftarrow`:      "arrow.l.double",
	`\Leftrightarrow`: "<=>",
	`\mapsto`:         "|->",
	`\implies`:        "==>",
	`\iff`:            "<==>",
	`\perp`:           "perp",
	`\parallel`:       "parallel",
	`\mid`:            "mid",
	`\coloneqq`:       ":=",
}

var typstOpMap = map[string]string{
	`\pm`:       "plus.minus",
	`\mp`:       "minus.plus",
	`\times`:    "times",
	`\div`:      "div",
	`\cdot`:     "dot.op",
	`\ast`:      "ast.op",
	`\star`:     "star",
	`\circ`:     "compose",
	`\bullet`:   "bullet",
	`\oplus`:    "plus.o",
	`\otimes`:   "times.o",
	`\wedge`:    "and",
	`\vee`:      "or",
	`\cup`:      "union",
	`\cap`:      "inter",
	`\setminus`: "without",
	`\nabla`:    "nabla",
	`\partial`:  "partial",
}

var typstDelimSymMap = map[string]string{
	`\{`:      `\{`,
	`\}`:      `\}`,
	`\|`:      "‖",
	`\langle`: "chevron.l",
	`\rangle`: "chevron.r",
	`\lfloor`: "floor.l",
	`\rfloor`: "floor.r",
	`\lceil`:  "ceil.l",
	`\rceil`:  "ceil.r",
}

var typstMiscMap = map[string]string{
	`\infty`:      "infinity",
	`\forall`:     "forall",
	`\exists`:     "exists",
	`\nexists`:    "exists.not",
	`\emptyset`:   "nothing",
	`\varnothing`: "nothing",
	`\neg`:        "not",
	`\lnot`:       "not",
	`\top`:        "top",
	`\bot`:        "bot",
	`\dots`:       "dots",
	`\ldots`:      "dots.h",
	`\cdots`:      "dots.c",
	`\vdots`:      "dots.v",
	`\ddots`:      "dots.down",
	`\prime`:      "prime",
	`\hbar`:       "planck",
	`\ell`:        "ell",
	`\Re`:         "Re",
	`\Im`:         "Im",
	`\wp`:         "℘",
	`\aleph`:      "aleph",
	`\degree`:     "degree",
	`\angle`:      "angle",
	`\triangle`:   "triangle",
	`\square`:     "square",
}
