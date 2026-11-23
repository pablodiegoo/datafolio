package math_test

import (
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/math"
)

func TestToTypstBasicMathGreekSubSupFracRoots(t *testing.T) {
	cases := []struct {
		name    string
		latex   string
		display bool
		wants   []string
		forbid  []string
	}{
		{
			name:    "inline mode omits outer spaces and translates greek",
			latex:   `\alpha + \beta + \gamma + \varepsilon + \phi + \varphi + \Gamma + \Omega`,
			display: false,
			wants:   []string{"$", "alpha", "beta", "gamma", "epsilon", "phi.alt", "phi", "Gamma", "Omega"},
		},
		{
			name:    "display mode wraps with outer spaces",
			latex:   `a^2 + b^2 = c^2`,
			display: true,
			wants:   []string{"$ a^2 + b^2 = c^2 $"},
		},
		{
			name:    "subscripts superscripts and primes",
			latex:   `x_i + x_{i+1} + e^{-x^2} + x_i^2 + f'(x)`,
			display: false,
			wants:   []string{"x_i", "x_(i + 1)", "e^(- x^2)", "x_i^2", "f'(x)"},
		},
		{
			name:    "fractions and binomials",
			latex:   `\frac{a+b}{c} + \dfrac{1}{2} + \tfrac{3}{4} + \binom{n}{k}`,
			display: false,
			wants:   []string{"frac(a + b, c)", "frac(1, 2)", "frac(3, 4)", "binom(n, k)"},
		},
		{
			name:    "square and nth roots",
			latex:   `\sqrt{a^2 + b^2} + \sqrt[3]{x^2 + 1}`,
			display: false,
			wants:   []string{"sqrt(a^2 + b^2)", "root(3, x^2 + 1)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := math.ToTypst(tc.latex, tc.display)
			if err != nil {
				t.Fatalf("ToTypst(%q) error: %v", tc.latex, err)
			}
			if tc.display {
				if !strings.HasPrefix(got, "$ ") || !strings.HasSuffix(got, " $") {
					t.Errorf("display ToTypst(%q) = %q, expected outer `$ ... $` spaces", tc.latex, got)
				}
			} else {
				if !strings.HasPrefix(got, "$") || !strings.HasSuffix(got, "$") || strings.HasPrefix(got, "$ ") || strings.HasSuffix(got, " $") {
					t.Errorf("inline ToTypst(%q) = %q, expected tight `$...$` without outer spaces", tc.latex, got)
				}
			}
			for _, w := range tc.wants {
				if !strings.Contains(got, w) {
					t.Errorf("ToTypst(%q) missing %q\ngot: %s", tc.latex, w, got)
				}
			}
			for _, f := range tc.forbid {
				if strings.Contains(got, f) {
					t.Errorf("ToTypst(%q) must not contain %q\ngot: %s", tc.latex, f, got)
				}
			}
		})
	}
}

func TestToTypstOperatorsSetsStylesAccentsDelimsEnvironments(t *testing.T) {
	cases := []struct {
		name    string
		latex   string
		display bool
		wants   []string
		forbid  []string
	}{
		{
			name:    "big operators with limits and integrals",
			latex:   `\lim_{n \to \infty} \sum_{i=1}^n \frac{x_i}{n} \approx \int_0^\infty f(t)\,dt + \prod_{j=1}^k p_j`,
			display: true,
			wants:   []string{"lim_(n -> infinity)", "sum_(i = 1)^n", "frac(x_i, n)", "approx", "integral_0^infinity", "thin", "d t", "product_(j = 1)^k"},
		},
		{
			name:    "relations sets arrows and operators",
			latex:   `\alpha \le \beta \ge \gamma \neq \delta \equiv \eta \in \mathbb{R} \setminus \{0\} \subset \subseteq \cup \cap \Rightarrow \Leftrightarrow \iff \mapsto \pm \times \cdot \oplus \otimes`,
			display: false,
			wants:   []string{"<=", ">=", "!=", "equiv", "in", "bb(R)", "without", "subset", "subset.eq", "union", "inter", "=>", "<=>", "<==>", "|->", "plus.minus", "times", "dot.op", "plus.o", "times.o"},
			forbid:  []string{"sect", "plus.circle", "times.circle"},
		},
		{
			name:    "accents and styles including multi-char words",
			latex:   `\hat{\beta} + \bar{x} + \overline{x+y} + \tilde{y} + \vec{v} + \dot{x} + \ddot{x} + \underline{z} + \mathbf{X} + \mathcal{L} + \mathsf{A} + \mathtt{code} + \operatorname{argmax} + \text{otherwise}`,
			display: false,
			wants:   []string{"hat(beta)", "macron(x)", "overline(x + y)", "tilde(y)", "arrow(v)", "dot(x)", "dot.double(x)", "underline(z)", "bold(X)", "cal(L)", "sans(A)", `mono("code")`, `op("argmax")`, `"otherwise"`},
		},
		{
			name:    "delimiters left right and overset underset",
			latex:   `\left. \binom{n}{k} \right|_{k=1} + \left[ 0, 1 \right) + \left\langle a, b \right\rangle + \left\| v \right\| + \overset{\star}{=} + \underset{i \in I}{\sum}`,
			display: true,
			wants:   []string{"lr(binom(n, k) |)_(k = 1)", "lr([0, 1))", "lr(chevron.l a, b chevron.r)", "lr(‖ v ‖)", "limits(=)^(star)", "limits(sum)_(i in I)"},
		},
		{
			name:    "aligned and cases with commas",
			latex:   `\begin{aligned} a &\neq b \pm c \\ f(x) &\Rightarrow \begin{cases} x^2, & x \ge 0 \\ 0, & \text{otherwise} \end{cases} \end{aligned}`,
			display: true,
			wants:   []string{"a & != b plus.minus c", `\`, "f(x) & => cases(", `x^2"," & x >= 0`, `0"," & "otherwise"`},
		},
		{
			name:    "matrices pmatrix bmatrix Bmatrix vmatrix Vmatrix array",
			latex:   `\begin{pmatrix} 1 & 2 \\ 3 & 4 \end{pmatrix} + \begin{bmatrix} a & b \\ c & d \end{bmatrix} + \begin{Bmatrix} 1 & 0 \\ 0 & 1 \end{Bmatrix} + \begin{vmatrix} 1 & 2 \\ 3 & 4 \end{vmatrix} + \begin{Vmatrix} 1 & 2 \\ 3 & 4 \end{Vmatrix} + \begin{array}{cc} 1 & 2 \\ 3 & 4 \end{array}`,
			display: true,
			wants:   []string{`mat(delim: "(", 1, 2; 3, 4)`, `mat(delim: "[", a, b; c, d)`, `mat(delim: "{", 1, 0; 0, 1)`, `mat(delim: "|", 1, 2; 3, 4)`, `mat(delim: "‖", 1, 2; 3, 4)`, `mat(delim: #none, 1, 2; 3, 4)`},
		},
		{
			name:    "adjacent identifiers and reserved Typst keywords stay separated",
			latex:   `ab + in + \unknowncmd + \text{say "hi" #1 $x}`,
			display: false,
			wants:   []string{"a b", "i n", `"unknowncmd"`, `"say \"hi\" #1 $x"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := math.ToTypst(tc.latex, tc.display)
			if err != nil {
				t.Fatalf("ToTypst(%q) error: %v", tc.latex, err)
			}
			for _, w := range tc.wants {
				if !strings.Contains(got, w) {
					t.Errorf("ToTypst(%q) missing %q\ngot: %s", tc.latex, w, got)
				}
			}
			for _, f := range tc.forbid {
				if strings.Contains(got, f) {
					t.Errorf("ToTypst(%q) must not contain %q\ngot: %s", tc.latex, f, got)
				}
			}
		})
	}
}

