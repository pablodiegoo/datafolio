package math_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/math"
)

func TestToMathMLCoreConstructsAndValidXML(t *testing.T) {
	cases := []struct {
		name    string
		latex   string
		display bool
		wants   []string
	}{
		{
			name:    "greek and relations",
			latex:   `\alpha + \beta \le \gamma \neq \delta \approx \epsilon`,
			display: false,
			wants:   []string{`display="inline"`, `<mi>α</mi>`, `<mi>β</mi>`, `<mo>≤</mo>`, `<mo>≠</mo>`, `<mo>≈</mo>`},
		},
		{
			name:    "fraction and nested sqrt",
			latex:   `\frac{\sqrt{a^2 + b^2}}{\sqrt[n]{c}}`,
			display: true,
			wants:   []string{`display="block"`, `<mfrac>`, `<msqrt>`, `<mroot>`, `<mi>c</mi><mi>n</mi></mroot>`},
		},
		{
			name:    "display limits on sum and lim",
			latex:   `\lim_{x \to 0} \sum_{i=1}^{n} x_i`,
			display: true,
			wants:   []string{`<munder>`, `<munderover>`, `<mo movablelimits="true">lim</mo>`, `<mo movablelimits="true">∑</mo>`},
		},
		{
			name:    "accents and styles",
			latex:   `\hat{\beta} + \bar{x} + \tilde{y} + \vec{v} + \mathbf{X} + \mathbb{R}`,
			display: false,
			wants:   []string{`<mover accent="true">`, `^`, `¯`, `~`, `→`, `mathvariant="bold"`, `ℝ`},
		},
		{
			name:    "left right invisible dot and binomial",
			latex:   `\left. \binom{n}{k} \right|_{k=1}`,
			display: true,
			wants:   []string{`linethickness="0"`, `<mo>|</mo>`},
		},
		{
			name:    "cases and matrices",
			latex:   `\begin{cases} 1 & x > 0 \\ 0 & \text{else} \end{cases} + \begin{pmatrix} a & b \\ c & d \end{pmatrix} + \begin{bmatrix} 1 & 0 \\ 0 & 1 \end{bmatrix}`,
			display: true,
			wants:   []string{`<mtable>`, `<mtr>`, `<mtd>`, `<mtext>else</mtext>`, `<mo>{</mo>`, `<mo>(</mo>`, `<mo>[</mo>`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := math.ToMathML(tc.latex, tc.display)
			if err != nil {
				t.Fatalf("ToMathML(%q) error: %v", tc.latex, err)
			}
			for _, w := range tc.wants {
				if !strings.Contains(out, w) {
					t.Errorf("ToMathML(%q) missing %q\ngot: %s", tc.latex, w, out)
				}
			}
			var v any
			if err := xml.Unmarshal([]byte(out), &v); err != nil {
				t.Errorf("ToMathML(%q) produced invalid XML: %v\n%s", tc.latex, err, out)
			}
		})
	}
}
