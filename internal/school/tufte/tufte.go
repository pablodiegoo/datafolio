package tufte

import (
	"fmt"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/school"
)

func init() {
	school.Register(New())
}

// Tufte implements the built-in v1 Archival Carmine Tufte School.
type Tufte struct{}

// New returns an instance of the Tufte School.
func New() *Tufte {
	return &Tufte{}
}

// Name returns "tufte".
func (t *Tufte) Name() string {
	return "tufte"
}

// Tokens returns the Archival Carmine SchoolTokens for "paper", "night", or "print".
func (t *Tufte) Tokens(theme string) (school.SchoolTokens, error) {
	base := school.SchoolTokens{
		SerifFont:       "EB Garamond",
		MonoFont:        "JetBrains Mono",
		SansFont:        "Source Sans 3",
		MainWidth:       "65%",
		MarginWidth:     "30%",
		ColumnGap:       "5%",
		BorderRadius:    "0px",
		BoxShadow:       "none",
		BooktabsTopRule: "1.5px",
		BooktabsMidRule: "1px",
		BooktabsRowRule: "0.5px",
	}

	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "", "paper":
		base.Theme = "paper"
		base.Paper = "#fffff8"
		base.PaperSubtle = "#fbf9f2"
		base.PaperAlt = "#f6f3ea"
		base.Ink = "#111111"
		base.InkSoft = "#2b2926"
		base.Muted = "#6b6559"
		base.Accent = "#8b1e1e"
		base.Ochre = "#854d0e"
		base.Forest = "#166534"
		base.Rule = "#ded8ca"
		base.RuleStrong = "#3d3932"
		return base, nil
	case "night":
		base.Theme = "night"
		base.Paper = "#121316"
		base.PaperSubtle = "#16181d"
		base.PaperAlt = "#1a1c21"
		base.Ink = "#ece8de"
		base.InkSoft = "#d2cdc0"
		base.Muted = "#989284"
		base.Accent = "#e06c60"
		base.Ochre = "#d9a04e"
		base.Forest = "#65c494"
		base.Rule = "#2c3038"
		base.RuleStrong = "#78808f"
		return base, nil
	case "print":
		base.Theme = "print"
		base.Paper = "#ffffff"
		base.PaperSubtle = "#fafafa"
		base.PaperAlt = "#f4f4f4"
		base.Ink = "#111111"
		base.InkSoft = "#2b2926"
		base.Muted = "#595959"
		base.Accent = "#8b1e1e"
		base.Ochre = "#854d0e"
		base.Forest = "#166534"
		base.Rule = "#d4d4d4"
		base.RuleStrong = "#3d3932"
		return base, nil
	default:
		return school.SchoolTokens{}, fmt.Errorf("tufte: unsupported theme %q (supported: paper, night, print)", theme)
	}
}

// Labels returns localized structural labels for "en" or "pt-BR".
func (t *Tufte) Labels(lang string) school.Labels {
	norm, err := school.NormalizeLang(lang)
	if err == nil && norm == "pt-BR" {
		return school.Labels{
			Figure:     "Figura",
			Table:      "Tabela",
			Source:     "Fonte",
			MarginNote: "Nota de Margem",
			References: "Referências",
			Contents:   "Sumário",
		}
	}
	return school.Labels{
		Figure:     "Figure",
		Table:      "Table",
		Source:     "Source",
		MarginNote: "Margin Note",
		References: "References",
		Contents:   "Contents",
	}
}

// Policy returns the editorial and visual ReviewPolicy for the Tufte School.
func (t *Tufte) Policy() school.ReviewPolicy {
	return school.ReviewPolicy{
		DisallowEmojis:    true,
		MaxBorderRadius:   "0px",
		AllowBoxShadow:    false,
		RequireCaptions:   true,
		RequireSources:    true,
		MinContrastNormal: 4.5,
		MinContrastLarge:  3.0,
	}
}
