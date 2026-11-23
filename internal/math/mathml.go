package math

import (
	"fmt"
	stdhtml "html"
	"strings"
)

// ToMathML parses a LaTeX math expression and compiles it to standard browser-native MathML.
func ToMathML(latex string, display bool) (string, error) {
	astNode, err := ParseLaTeX(latex)
	if err != nil {
		return "", err
	}
	return RenderMathML(astNode, display)
}

// RenderMathML compiles a parsed LaTeX AST node into a `<math xmlns="http://www.w3.org/1998/Math/MathML" ...>` element.
func RenderMathML(node Node, display bool) (string, error) {
	mode := "inline"
	if display {
		mode = "block"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<math xmlns="http://www.w3.org/1998/Math/MathML" display=%q>`, mode))
	renderNode(&b, node, display, false)
	b.WriteString(`</math>`)
	return b.String(), nil
}

func renderNode(b *strings.Builder, node Node, display bool, wrapMulti bool) {
	if node == nil {
		b.WriteString(`<mrow></mrow>`)
		return
	}
	switch n := node.(type) {
	case *SequenceNode:
		nonEmpty := 0
		for _, item := range n.Items {
			if item != nil {
				nonEmpty++
			}
		}
		if nonEmpty == 0 {
			if wrapMulti {
				b.WriteString(`<mrow></mrow>`)
			}
			return
		}
		if wrapMulti && nonEmpty > 1 {
			b.WriteString(`<mrow>`)
			for _, item := range n.Items {
				renderNode(b, item, display, false)
			}
			b.WriteString(`</mrow>`)
			return
		}
		for _, item := range n.Items {
			renderNode(b, item, display, false)
		}
	case *GroupNode:
		renderNode(b, n.Body, display, wrapMulti)
	case *NumberNode:
		b.WriteString(`<mn>` + stdhtml.EscapeString(n.Value) + `</mn>`)
	case *IdentNode:
		b.WriteString(`<mi>` + stdhtml.EscapeString(n.Name) + `</mi>`)
	case *TextNode:
		b.WriteString(`<mtext>` + stdhtml.EscapeString(n.Text) + `</mtext>`)
	case *SpaceNode:
		renderSpace(b, n.Command)
	case *SymbolNode:
		renderSymbol(b, n)
	case *SubSupNode:
		renderSubSup(b, n, display)
	case *FracNode:
		if n.NoBar {
			b.WriteString(`<mrow><mo>(</mo><mfrac linethickness="0">`)
			renderNode(b, n.Num, display, true)
			renderNode(b, n.Den, display, true)
			b.WriteString(`</mfrac><mo>)</mo></mrow>`)
		} else {
			b.WriteString(`<mfrac>`)
			renderNode(b, n.Num, display, true)
			renderNode(b, n.Den, display, true)
			b.WriteString(`</mfrac>`)
		}
	case *SqrtNode:
		if n.Index != nil {
			b.WriteString(`<mroot>`)
			renderNode(b, n.Radicand, display, true)
			renderNode(b, n.Index, display, true)
			b.WriteString(`</mroot>`)
		} else {
			b.WriteString(`<msqrt>`)
			renderNode(b, n.Radicand, display, false)
			b.WriteString(`</msqrt>`)
		}
	case *AccentNode:
		renderAccent(b, n, display)
	case *StyleNode:
		renderStyle(b, n, display)
	case *DelimNode:
		b.WriteString(`<mrow>`)
		renderFenceDelim(b, n.Left)
		renderNode(b, n.Body, display, false)
		renderFenceDelim(b, n.Right)
		b.WriteString(`</mrow>`)
	case *OverUnderNode:
		if n.Command == `\underset` {
			b.WriteString(`<munder>`)
			renderNode(b, n.Base, display, true)
			renderNode(b, n.Label, display, true)
			b.WriteString(`</munder>`)
		} else {
			b.WriteString(`<mover>`)
			renderNode(b, n.Base, display, true)
			renderNode(b, n.Label, display, true)
			b.WriteString(`</mover>`)
		}
	case *EnvNode:
		renderEnv(b, n, display)
	}
}

func renderSpace(b *strings.Builder, cmd string) {
	switch cmd {
	case `\,`, `\thinspace`:
		b.WriteString(`<mspace width="0.167em"/>`)
	case `\:`:
		b.WriteString(`<mspace width="0.222em"/>`)
	case `\;`:
		b.WriteString(`<mspace width="0.278em"/>`)
	case `\!`:
		b.WriteString(`<mspace width="-0.167em"/>`)
	case `\ `, `~`, `\enspace`:
		b.WriteString(`<mspace width="0.5em"/>`)
	case `\quad`:
		b.WriteString(`<mspace width="1em"/>`)
	case `\qquad`:
		b.WriteString(`<mspace width="2em"/>`)
	default:
		b.WriteString(`<mspace width="0.25em"/>`)
	}
}

func renderSymbol(b *strings.Builder, n *SymbolNode) {
	if g, ok := greekMap[n.Command]; ok {
		b.WriteString(`<mi>` + stdhtml.EscapeString(g) + `</mi>`)
		return
	}
	if op, ok := bigOpMap[n.Command]; ok {
		if op.movableLimits {
			b.WriteString(`<mo movablelimits="true">` + stdhtml.EscapeString(op.glyph) + `</mo>`)
		} else {
			b.WriteString(`<mo>` + stdhtml.EscapeString(op.glyph) + `</mo>`)
		}
		return
	}
	if r, ok := relMap[n.Command]; ok {
		b.WriteString(`<mo>` + stdhtml.EscapeString(r) + `</mo>`)
		return
	}
	if o, ok := opMap[n.Command]; ok {
		b.WriteString(`<mo>` + stdhtml.EscapeString(o) + `</mo>`)
		return
	}
	if d, ok := delimMap[n.Command]; ok {
		b.WriteString(`<mo>` + stdhtml.EscapeString(d) + `</mo>`)
		return
	}
	if m, ok := miscMap[n.Command]; ok {
		if m.tag == "mi" {
			b.WriteString(`<mi>` + stdhtml.EscapeString(m.glyph) + `</mi>`)
		} else {
			b.WriteString(`<mo>` + stdhtml.EscapeString(m.glyph) + `</mo>`)
		}
		return
	}
	switch n.Command {
	case "+", "*", "/", "=", "<", ">", "(", ")", "[", "]", "|", ",", ":", ";", "!", "?", ":=":
		b.WriteString(`<mo>` + stdhtml.EscapeString(n.Command) + `</mo>`)
	case "-":
		b.WriteString(`<mo>−</mo>`)
	case ".":
		b.WriteString(`<mo>.</mo>`)
	case `\$`, `\%`, `\&`, `\#`, `\_`:
		b.WriteString(`<mo>` + stdhtml.EscapeString(strings.TrimPrefix(n.Command, `\`)) + `</mo>`)
	case "'", "''", "'''":
		b.WriteString(`<mo>` + strings.Repeat("′", len(n.Command)) + `</mo>`)
	default:
		if strings.HasPrefix(n.Command, `\`) {
			b.WriteString(`<mi>` + stdhtml.EscapeString(strings.TrimPrefix(n.Command, `\`)) + `</mi>`)
		} else {
			b.WriteString(`<mo>` + stdhtml.EscapeString(n.Command) + `</mo>`)
		}
	}
}

func renderSubSup(b *strings.Builder, n *SubSupNode, display bool) {
	useOverUnder := false
	if display {
		if sym, ok := n.Base.(*SymbolNode); ok {
			if op, exists := bigOpMap[sym.Command]; exists && op.underOverInDisplay {
				useOverUnder = true
			}
		}
	}

	switch {
	case n.Sub != nil && n.Sup != nil:
		tag := "msubsup"
		if useOverUnder {
			tag = "munderover"
		}
		b.WriteString("<" + tag + ">")
		renderNode(b, n.Base, display, true)
		renderNode(b, n.Sub, display, true)
		renderNode(b, n.Sup, display, true)
		b.WriteString("</" + tag + ">")
	case n.Sub != nil:
		tag := "msub"
		if useOverUnder {
			tag = "munder"
		}
		b.WriteString("<" + tag + ">")
		renderNode(b, n.Base, display, true)
		renderNode(b, n.Sub, display, true)
		b.WriteString("</" + tag + ">")
	case n.Sup != nil:
		tag := "msup"
		if useOverUnder {
			tag = "mover"
		}
		b.WriteString("<" + tag + ">")
		renderNode(b, n.Base, display, true)
		renderNode(b, n.Sup, display, true)
		b.WriteString("</" + tag + ">")
	default:
		renderNode(b, n.Base, display, false)
	}
}

func renderAccent(b *strings.Builder, n *AccentNode, display bool) {
	switch n.Command {
	case `\underline`:
		b.WriteString(`<munder accentunder="true">`)
		renderNode(b, n.Base, display, true)
		b.WriteString(`<mo>_</mo></munder>`)
		return
	}
	glyph := "^"
	switch n.Command {
	case `\hat`, `\widehat`:
		glyph = "^"
	case `\bar`, `\overline`:
		glyph = "¯"
	case `\tilde`, `\widetilde`:
		glyph = "~"
	case `\vec`:
		glyph = "→"
	case `\dot`:
		glyph = "˙"
	case `\ddot`:
		glyph = "¨"
	}
	b.WriteString(`<mover accent="true">`)
	renderNode(b, n.Base, display, true)
	b.WriteString(`<mo>` + stdhtml.EscapeString(glyph) + `</mo></mover>`)
}

func renderStyle(b *strings.Builder, n *StyleNode, display bool) {
	switch n.Command {
	case `\text`, `\mbox`, `\hbox`:
		if txt, ok := n.Body.(*TextNode); ok {
			b.WriteString(`<mtext>` + stdhtml.EscapeString(txt.Text) + `</mtext>`)
			return
		}
		renderNode(b, n.Body, display, false)
	case `\operatorname`:
		if txt, ok := n.Body.(*TextNode); ok {
			b.WriteString(`<mo>` + stdhtml.EscapeString(txt.Text) + `</mo>`)
			return
		}
		renderNode(b, n.Body, display, false)
	case `\mathbb`:
		b.WriteString(renderMappedStyle(n.Body, doubleStruckMap, "double-struck", display))
	case `\mathcal`:
		b.WriteString(renderMappedStyle(n.Body, scriptMap, "script", display))
	case `\mathbf`, `\boldsymbol`:
		b.WriteString(`<mstyle mathvariant="bold">`)
		renderStyledContent(b, n.Body, "bold", display)
		b.WriteString(`</mstyle>`)
	case `\mathrm`:
		b.WriteString(`<mstyle mathvariant="normal">`)
		renderStyledContent(b, n.Body, "normal", display)
		b.WriteString(`</mstyle>`)
	case `\mathsf`:
		b.WriteString(`<mstyle mathvariant="sans-serif">`)
		renderStyledContent(b, n.Body, "sans-serif", display)
		b.WriteString(`</mstyle>`)
	case `\mathtt`:
		b.WriteString(`<mstyle mathvariant="monospace">`)
		renderStyledContent(b, n.Body, "monospace", display)
		b.WriteString(`</mstyle>`)
	default:
		renderNode(b, n.Body, display, false)
	}
}

func renderStyledContent(b *strings.Builder, node Node, variant string, display bool) {
	switch inner := node.(type) {
	case *IdentNode:
		b.WriteString(fmt.Sprintf(`<mi mathvariant=%q>%s</mi>`, variant, stdhtml.EscapeString(inner.Name)))
	case *GroupNode:
		renderStyledContent(b, inner.Body, variant, display)
	case *SequenceNode:
		for _, item := range inner.Items {
			renderStyledContent(b, item, variant, display)
		}
	default:
		renderNode(b, node, display, false)
	}
}

func renderMappedStyle(body Node, table map[rune]string, fallbackVariant string, display bool) string {
	var textBuf strings.Builder
	if collectStyleChars(body, &textBuf) {
		var mapped strings.Builder
		allMapped := true
		for _, r := range textBuf.String() {
			if rep, ok := table[r]; ok {
				mapped.WriteString(rep)
			} else {
				allMapped = false
				mapped.WriteRune(r)
			}
		}
		if allMapped && mapped.Len() > 0 {
			return `<mi>` + stdhtml.EscapeString(mapped.String()) + `</mi>`
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<mstyle mathvariant=%q>`, fallbackVariant))
	renderNode(&b, body, display, false)
	b.WriteString(`</mstyle>`)
	return b.String()
}

func collectStyleChars(node Node, out *strings.Builder) bool {
	switch n := node.(type) {
	case *IdentNode:
		out.WriteString(n.Name)
		return true
	case *NumberNode:
		out.WriteString(n.Value)
		return true
	case *GroupNode:
		return collectStyleChars(n.Body, out)
	case *SequenceNode:
		for _, item := range n.Items {
			if !collectStyleChars(item, out) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func renderFenceDelim(b *strings.Builder, d string) {
	if d == "." || d == "" {
		return
	}
	if mapped, ok := delimMap[d]; ok {
		b.WriteString(`<mo>` + stdhtml.EscapeString(mapped) + `</mo>`)
		return
	}
	b.WriteString(`<mo>` + stdhtml.EscapeString(d) + `</mo>`)
}

func renderEnv(b *strings.Builder, n *EnvNode, display bool) {
	var leftDelim, rightDelim string
	isAligned := false
	isCases := false
	switch n.Name {
	case "pmatrix":
		leftDelim, rightDelim = "(", ")"
	case "bmatrix":
		leftDelim, rightDelim = "[", "]"
	case "Bmatrix":
		leftDelim, rightDelim = "{", "}"
	case "vmatrix":
		leftDelim, rightDelim = "|", "|"
	case "Vmatrix":
		leftDelim, rightDelim = "‖", "‖"
	case "cases":
		leftDelim, rightDelim = "{", ""
		isCases = true
	case "aligned", "align", "align*", "split", "eqnarray":
		isAligned = true
	}

	if leftDelim != "" || rightDelim != "" {
		b.WriteString(`<mrow>`)
		if leftDelim != "" {
			b.WriteString(`<mo>` + stdhtml.EscapeString(leftDelim) + `</mo>`)
		}
	}

	b.WriteString(`<mtable>`)
	for _, row := range n.Rows {
		b.WriteString(`<mtr>`)
		for colIdx, cell := range row {
			attr := ""
			if isCases {
				attr = ` style="text-align: left;"`
			} else if isAligned {
				if colIdx%2 == 0 {
					attr = ` style="text-align: right;"`
				} else {
					attr = ` style="text-align: left;"`
				}
			}
			b.WriteString(`<mtd` + attr + `>`)
			renderNode(b, cell, display, false)
			b.WriteString(`</mtd>`)
		}
		b.WriteString(`</mtr>`)
	}
	b.WriteString(`</mtable>`)

	if leftDelim != "" || rightDelim != "" {
		if rightDelim != "" {
			b.WriteString(`<mo>` + stdhtml.EscapeString(rightDelim) + `</mo>`)
		}
		b.WriteString(`</mrow>`)
	}
}

type bigOpSpec struct {
	glyph              string
	movableLimits      bool
	underOverInDisplay bool
}

type miscSpec struct {
	glyph string
	tag   string
}

var greekMap = map[string]string{
	`\alpha`: "α", `\beta`: "β", `\gamma`: "γ", `\delta`: "δ",
	`\epsilon`: "ϵ", `\varepsilon`: "ε", `\zeta`: "ζ", `\eta`: "η",
	`\theta`: "θ", `\vartheta`: "ϑ", `\iota`: "ι", `\kappa`: "κ",
	`\lambda`: "λ", `\mu`: "μ", `\nu`: "ν", `\xi`: "ξ",
	`\omicron`: "ο", `\pi`: "π", `\varpi`: "ϖ", `\rho`: "ρ",
	`\varrho`: "ϱ", `\sigma`: "σ", `\varsigma`: "ς", `\tau`: "τ",
	`\upsilon`: "υ", `\phi`: "ϕ", `\varphi`: "φ", `\chi`: "χ",
	`\psi`: "ψ", `\omega`: "ω",
	`\Alpha`: "Α", `\Beta`: "Β", `\Gamma`: "Γ", `\Delta`: "Δ",
	`\Epsilon`: "Ε", `\Zeta`: "Ζ", `\Eta`: "Η", `\Theta`: "Θ",
	`\Iota`: "Ι", `\Kappa`: "Κ", `\Lambda`: "Λ", `\Mu`: "Μ",
	`\Nu`: "Ν", `\Xi`: "Ξ", `\Omicron`: "Ο", `\Pi`: "Π",
	`\Rho`: "Ρ", `\Sigma`: "Σ", `\Tau`: "Τ", `\Upsilon`: "Υ",
	`\Phi`: "Φ", `\Chi`: "Χ", `\Psi`: "Ψ", `\Omega`: "Ω",
}

var bigOpMap = map[string]bigOpSpec{
	`\sum`:       {glyph: "∑", movableLimits: true, underOverInDisplay: true},
	`\prod`:      {glyph: "∏", movableLimits: true, underOverInDisplay: true},
	`\coprod`:    {glyph: "∐", movableLimits: true, underOverInDisplay: true},
	`\bigcup`:    {glyph: "⋃", movableLimits: true, underOverInDisplay: true},
	`\bigcap`:    {glyph: "⋂", movableLimits: true, underOverInDisplay: true},
	`\bigoplus`:  {glyph: "⨁", movableLimits: true, underOverInDisplay: true},
	`\bigotimes`: {glyph: "⨂", movableLimits: true, underOverInDisplay: true},
	`\lim`:       {glyph: "lim", movableLimits: true, underOverInDisplay: true},
	`\limsup`:    {glyph: "lim sup", movableLimits: true, underOverInDisplay: true},
	`\liminf`:    {glyph: "lim inf", movableLimits: true, underOverInDisplay: true},
	`\sup`:       {glyph: "sup", movableLimits: true, underOverInDisplay: true},
	`\inf`:       {glyph: "inf", movableLimits: true, underOverInDisplay: true},
	`\max`:       {glyph: "max", movableLimits: true, underOverInDisplay: true},
	`\min`:       {glyph: "min", movableLimits: true, underOverInDisplay: true},
	`\Pr`:        {glyph: "Pr", movableLimits: true, underOverInDisplay: true},
	`\int`:       {glyph: "∫", movableLimits: false, underOverInDisplay: false},
	`\iint`:      {glyph: "∬", movableLimits: false, underOverInDisplay: false},
	`\iiint`:     {glyph: "∭", movableLimits: false, underOverInDisplay: false},
	`\oint`:      {glyph: "∮", movableLimits: false, underOverInDisplay: false},
	`\arg`:       {glyph: "arg", movableLimits: false, underOverInDisplay: false},
	`\det`:       {glyph: "det", movableLimits: false, underOverInDisplay: false},
	`\dim`:       {glyph: "dim", movableLimits: false, underOverInDisplay: false},
	`\gcd`:       {glyph: "gcd", movableLimits: false, underOverInDisplay: false},
	`\log`:       {glyph: "log", movableLimits: false, underOverInDisplay: false},
	`\ln`:        {glyph: "ln", movableLimits: false, underOverInDisplay: false},
	`\exp`:       {glyph: "exp", movableLimits: false, underOverInDisplay: false},
	`\sin`:       {glyph: "sin", movableLimits: false, underOverInDisplay: false},
	`\cos`:       {glyph: "cos", movableLimits: false, underOverInDisplay: false},
	`\tan`:       {glyph: "tan", movableLimits: false, underOverInDisplay: false},
	`\cot`:       {glyph: "cot", movableLimits: false, underOverInDisplay: false},
	`\sec`:       {glyph: "sec", movableLimits: false, underOverInDisplay: false},
	`\csc`:       {glyph: "csc", movableLimits: false, underOverInDisplay: false},
	`\arcsin`:    {glyph: "arcsin", movableLimits: false, underOverInDisplay: false},
	`\arccos`:    {glyph: "arccos", movableLimits: false, underOverInDisplay: false},
	`\arctan`:    {glyph: "arctan", movableLimits: false, underOverInDisplay: false},
	`\sinh`:      {glyph: "sinh", movableLimits: false, underOverInDisplay: false},
	`\cosh`:      {glyph: "cosh", movableLimits: false, underOverInDisplay: false},
	`\tanh`:      {glyph: "tanh", movableLimits: false, underOverInDisplay: false},
}

var relMap = map[string]string{
	`\le`:             "≤",
	`\leq`:            "≤",
	`\ge`:             "≥",
	`\geq`:            "≥",
	`\neq`:            "≠",
	`\ne`:             "≠",
	`\approx`:         "≈",
	`\equiv`:          "≡",
	`\sim`:            "∼",
	`\simeq`:          "≃",
	`\cong`:           "≅",
	`\propto`:         "∝",
	`\ll`:             "≪",
	`\gg`:             "≫",
	`\subset`:         "⊂",
	`\subseteq`:       "⊆",
	`\supset`:         "⊃",
	`\supseteq`:       "⊇",
	`\in`:             "∈",
	`\notin`:          "∉",
	`\ni`:             "∋",
	`\to`:             "→",
	`\rightarrow`:     "→",
	`\leftarrow`:      "←",
	`\leftrightarrow`: "↔",
	`\Rightarrow`:     "⇒",
	`\Leftarrow`:      "⇐",
	`\Leftrightarrow`: "⇔",
	`\mapsto`:         "↦",
	`\implies`:        "⇒",
	`\iff`:            "⇔",
	`\perp`:           "⊥",
	`\parallel`:       "∥",
	`\mid`:            "∣",
	`\coloneqq`:       "≔",
}

var opMap = map[string]string{
	`\pm`:       "±",
	`\mp`:       "∓",
	`\times`:    "×",
	`\div`:      "÷",
	`\cdot`:     "⋅",
	`\ast`:      "∗",
	`\star`:     "⋆",
	`\circ`:     "∘",
	`\bullet`:   "∙",
	`\oplus`:    "⊕",
	`\otimes`:   "⊗",
	`\wedge`:    "∧",
	`\vee`:      "∨",
	`\cup`:      "∪",
	`\cap`:      "∩",
	`\setminus`: "∖",
	`\nabla`:    "∇",
	`\partial`:  "∂",
}

var delimMap = map[string]string{
	`\{`:      "{",
	`\}`:      "}",
	`\|`:      "‖",
	`\langle`: "⟨",
	`\rangle`: "⟩",
	`\lfloor`: "⌊",
	`\rfloor`: "⌋",
	`\lceil`:  "⌈",
	`\rceil`:  "⌉",
}

var miscMap = map[string]miscSpec{
	`\infty`:      {glyph: "∞", tag: "mi"},
	`\forall`:     {glyph: "∀", tag: "mo"},
	`\exists`:     {glyph: "∃", tag: "mo"},
	`\nexists`:    {glyph: "∄", tag: "mo"},
	`\emptyset`:   {glyph: "∅", tag: "mi"},
	`\varnothing`: {glyph: "∅", tag: "mi"},
	`\neg`:        {glyph: "¬", tag: "mo"},
	`\lnot`:       {glyph: "¬", tag: "mo"},
	`\top`:        {glyph: "⊤", tag: "mo"},
	`\bot`:        {glyph: "⊥", tag: "mo"},
	`\dots`:       {glyph: "…", tag: "mo"},
	`\ldots`:      {glyph: "…", tag: "mo"},
	`\cdots`:      {glyph: "⋯", tag: "mo"},
	`\vdots`:      {glyph: "⋮", tag: "mo"},
	`\ddots`:      {glyph: "⋱", tag: "mo"},
	`\prime`:      {glyph: "′", tag: "mo"},
	`\hbar`:       {glyph: "ℏ", tag: "mi"},
	`\ell`:        {glyph: "ℓ", tag: "mi"},
	`\Re`:         {glyph: "ℜ", tag: "mi"},
	`\Im`:         {glyph: "ℑ", tag: "mi"},
	`\wp`:         {glyph: "℘", tag: "mi"},
	`\aleph`:      {glyph: "ℵ", tag: "mi"},
	`\degree`:     {glyph: "°", tag: "mo"},
	`\angle`:      {glyph: "∠", tag: "mo"},
	`\triangle`:   {glyph: "△", tag: "mo"},
	`\square`:     {glyph: "□", tag: "mo"},
}

var doubleStruckMap = map[rune]string{
	'A': "𝔸", 'B': "𝔹", 'C': "ℂ", 'D': "𝔻", 'E': "𝔼", 'F': "𝔽",
	'G': "𝔾", 'H': "ℍ", 'I': "𝕀", 'J': "𝕁", 'K': "𝕂", 'L': "𝕃",
	'M': "𝕄", 'N': "ℕ", 'O': "𝕆", 'P': "ℙ", 'Q': "ℚ", 'R': "ℝ",
	'S': "𝕊", 'T': "𝕋", 'U': "𝕌", 'V': "𝕍", 'W': "𝕎", 'X': "𝕏",
	'Y': "𝕐", 'Z': "ℤ", '0': "𝟘", '1': "𝟙",
}

var scriptMap = map[rune]string{
	'A': "𝒜", 'B': "ℬ", 'C': "𝒞", 'D': "𝒟", 'E': "ℰ", 'F': "ℱ",
	'G': "𝒢", 'H': "ℋ", 'I': "ℐ", 'J': "𝒥", 'K': "𝒦", 'L': "ℒ",
	'M': "ℳ", 'N': "𝒩", 'O': "𝒪", 'P': "𝒫", 'Q': "𝒬", 'R': "ℛ",
	'S': "𝒮", 'T': "𝒯", 'U': "𝒰", 'V': "𝒱", 'W': "𝒲", 'X': "𝒳",
	'Y': "𝒴", 'Z': "𝒵",
}
