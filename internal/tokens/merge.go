package tokens

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/school"
)

// Merge overlays matching tokens from Contract onto a copy of base SchoolTokens
// without mutating base or any external file.
func Merge(base school.SchoolTokens, c *Contract) school.SchoolTokens {
	if c == nil || len(c.Tokens) == 0 {
		return base
	}
	out := base
	theme := strings.ToLower(strings.TrimSpace(base.Theme))
	isDark := theme == "night" || theme == "dark"
	isPrint := theme == "print"

	pick := func(paths ...string) (string, bool) {
		for _, p := range paths {
			if tok, ok := c.Tokens[p]; ok && strings.TrimSpace(tok.Value) != "" {
				return strings.TrimSpace(tok.Value), true
			}
		}
		return "", false
	}

	apply := func(dst *string, paths ...string) {
		if v, ok := pick(paths...); ok {
			*dst = v
		}
	}

	switch {
	case isDark:
		apply(&out.Paper,
			"color.dark.paper", "color.dark.surface_canvas", "colors.night_theme.paper", "color.night_theme.paper", "color.night.paper",
			"color.paper", "color.surface_canvas", "color.canvas", "color.background",
		)
		apply(&out.PaperSubtle,
			"color.dark.paper_subtle", "color.dark.surface_card", "colors.night_theme.paper_subtle", "color.night_theme.paper_subtle", "color.night.paper_subtle",
			"color.paper_subtle", "color.surface_card",
		)
		apply(&out.PaperAlt,
			"color.dark.paper_alt", "color.dark.surface_subtle", "colors.night_theme.paper_alt", "color.night_theme.paper_alt", "color.night.paper_alt",
			"color.paper_alt", "color.surface_subtle",
		)
		apply(&out.Ink,
			"color.dark.ink", "color.dark.text_primary", "colors.night_theme.ink", "color.night_theme.ink", "color.night.ink",
			"color.ink", "color.text_primary",
		)
		apply(&out.InkSoft,
			"color.dark.ink_soft", "color.dark.text_secondary", "colors.night_theme.ink_soft", "color.night_theme.ink_soft", "color.night.ink_soft",
			"color.ink_soft", "color.text_secondary",
		)
		apply(&out.Muted,
			"color.dark.muted", "color.dark.text_muted", "colors.night_theme.muted", "color.night_theme.muted", "color.night.muted",
			"color.muted", "color.text_muted",
		)
		apply(&out.Accent,
			"color.dark.accent", "colors.night_theme.accent", "color.night_theme.accent", "color.night.accent",
			"color.brand.primary_dark", "color.accent", "color.brand.accent", "color.brand.primary",
		)
		apply(&out.Ochre,
			"color.dark.ochre", "colors.night_theme.ochre", "color.night_theme.ochre", "color.night.ochre",
			"color.ochre",
		)
		apply(&out.Forest,
			"color.dark.forest", "colors.night_theme.forest", "color.night_theme.forest", "color.night.forest",
			"color.status.public_dark_text", "color.forest",
		)
		apply(&out.Rule,
			"color.dark.rule", "color.dark.border_subtle", "colors.night_theme.rule", "color.night_theme.rule", "color.night.rule",
			"color.rule", "color.border_subtle",
		)
		apply(&out.RuleStrong,
			"color.dark.rule_strong", "color.dark.border_strong", "colors.night_theme.rule_strong", "color.night_theme.rule_strong", "color.night.rule_strong",
			"color.rule_strong", "color.border_strong",
		)
	default:
		var printPrefix []string
		if isPrint {
			printPrefix = []string{"color.print.", "colors.print_theme."}
		}
		applyWithPrint := func(dst *string, lightPaths ...string) {
			if len(printPrefix) > 0 {
				var candidates []string
				for _, lp := range lightPaths {
					if strings.HasPrefix(lp, "color.light.") {
						suffix := strings.TrimPrefix(lp, "color.light.")
						candidates = append(candidates, "color.print."+suffix, "colors.print_theme."+suffix)
					}
				}
				candidates = append(candidates, lightPaths...)
				apply(dst, candidates...)
				return
			}
			apply(dst, lightPaths...)
		}

		applyWithPrint(&out.Paper,
			"color.light.paper", "color.light.surface_canvas", "colors.paper_theme.paper", "color.paper_theme.paper",
			"color.paper", "color.surface_canvas", "color.canvas", "color.background",
		)
		applyWithPrint(&out.PaperSubtle,
			"color.light.paper_subtle", "color.light.surface_card", "colors.paper_theme.paper_subtle", "color.paper_theme.paper_subtle",
			"color.paper_subtle", "color.surface_card",
		)
		applyWithPrint(&out.PaperAlt,
			"color.light.paper_alt", "color.light.surface_subtle", "colors.paper_theme.paper_alt", "color.paper_theme.paper_alt",
			"color.paper_alt", "color.surface_subtle",
		)
		applyWithPrint(&out.Ink,
			"color.light.ink", "color.light.text_primary", "colors.paper_theme.ink", "color.paper_theme.ink",
			"color.ink", "color.text_primary",
		)
		applyWithPrint(&out.InkSoft,
			"color.light.ink_soft", "color.light.text_secondary", "colors.paper_theme.ink_soft", "color.paper_theme.ink_soft",
			"color.ink_soft", "color.text_secondary",
		)
		applyWithPrint(&out.Muted,
			"color.light.muted", "color.light.text_muted", "colors.paper_theme.muted", "color.paper_theme.muted",
			"color.muted", "color.text_muted",
		)
		applyWithPrint(&out.Accent,
			"color.light.accent", "colors.paper_theme.accent", "color.paper_theme.accent",
			"color.accent", "color.brand.accent", "color.brand.primary",
		)
		applyWithPrint(&out.Ochre,
			"color.light.ochre", "colors.paper_theme.ochre", "color.paper_theme.ochre", "color.ochre",
		)
		applyWithPrint(&out.Forest,
			"color.light.forest", "colors.paper_theme.forest", "color.paper_theme.forest",
			"color.forest", "color.status.public_text",
		)
		applyWithPrint(&out.Rule,
			"color.light.rule", "color.light.border_subtle", "colors.paper_theme.rule", "color.paper_theme.rule",
			"color.rule", "color.border_subtle",
		)
		applyWithPrint(&out.RuleStrong,
			"color.light.rule_strong", "color.light.border_strong", "colors.paper_theme.rule_strong", "color.paper_theme.rule_strong",
			"color.rule_strong", "color.border_strong",
		)
	}

	if v, ok := pick("font.serif", "typography.fontFamily.serif", "typography.family.heading", "typography.family.serif"); ok {
		if pf := primaryFontFamily(v); pf != "" {
			out.SerifFont = pf
		}
	}
	if v, ok := pick("font.mono", "typography.fontFamily.mono", "typography.family.mono"); ok {
		if pf := primaryFontFamily(v); pf != "" {
			out.MonoFont = pf
		}
	}
	if v, ok := pick("font.sans", "typography.fontFamily.sans", "typography.family.body", "typography.family.sans"); ok {
		if pf := primaryFontFamily(v); pf != "" {
			out.SansFont = pf
		}
	}

	applyWidth := func(dst *string, paths ...string) {
		if v, ok := pick(paths...); ok {
			if w := extractLeadingWidth(v); w != "" {
				*dst = w
			}
		}
	}

	apply(&out.MainWidth, "layout.main_width", "layout.mainWidth")
	apply(&out.MarginWidth, "layout.margin_width", "layout.marginWidth")
	apply(&out.ColumnGap, "layout.column_gap", "layout.columnGap", "layout.asymmetricGrid.columnGap")
	apply(&out.BorderRadius, "radius.default", "shape.borderRadius.default", "radius.base", "layout.border_radius")
	apply(&out.BoxShadow, "shadow.default", "elevation.boxShadow", "layout.box_shadow")
	applyWidth(&out.BooktabsTopRule, "rules.booktabs_top", "shape.rules.booktabsTop", "dividers.booktabs_outer.width")
	applyWidth(&out.BooktabsMidRule, "rules.booktabs_mid", "shape.rules.booktabsMid", "dividers.booktabs_inner.width")
	applyWidth(&out.BooktabsRowRule, "rules.booktabs_row", "shape.rules.hairline", "dividers.rule_standard.width")

	return out
}

func extractLeadingWidth(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// primaryFontFamily extracts the leading unquoted font name from a CSS font stack.
func primaryFontFamily(stack string) string {
	first := stack
	if idx := strings.IndexByte(stack, ','); idx >= 0 {
		first = stack[:idx]
	}
	first = strings.TrimSpace(first)
	first = strings.Trim(first, `"'`)
	return strings.TrimSpace(first)
}

// FromSchoolTokens constructs a canonical Contract from a SchoolTokens snapshot.
func FromSchoolTokens(tok school.SchoolTokens) *Contract {
	c := &Contract{
		Name:   fmt.Sprintf("datafolio-%s", tok.Theme),
		Tokens: make(map[string]Token),
	}
	add := func(path, val, typ string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		c.Tokens[path] = Token{
			Path:     path,
			RawValue: val,
			Value:    val,
			Type:     typ,
		}
		c.Order = append(c.Order, path)
	}

	add("color.paper", tok.Paper, "color")
	add("color.paper_subtle", tok.PaperSubtle, "color")
	add("color.paper_alt", tok.PaperAlt, "color")
	add("color.ink", tok.Ink, "color")
	add("color.ink_soft", tok.InkSoft, "color")
	add("color.muted", tok.Muted, "color")
	add("color.accent", tok.Accent, "color")
	add("color.ochre", tok.Ochre, "color")
	add("color.forest", tok.Forest, "color")
	add("color.rule", tok.Rule, "color")
	add("color.rule_strong", tok.RuleStrong, "color")

	add("font.serif", tok.SerifFont, "fontFamily")
	add("font.mono", tok.MonoFont, "fontFamily")
	add("font.sans", tok.SansFont, "fontFamily")

	add("layout.main_width", tok.MainWidth, "dimension")
	add("layout.margin_width", tok.MarginWidth, "dimension")
	add("layout.column_gap", tok.ColumnGap, "dimension")
	add("radius.default", tok.BorderRadius, "dimension")
	add("shadow.default", tok.BoxShadow, "shadow")
	add("rules.booktabs_top", tok.BooktabsTopRule, "dimension")
	add("rules.booktabs_mid", tok.BooktabsMidRule, "dimension")
	add("rules.booktabs_row", tok.BooktabsRowRule, "dimension")

	sort.Strings(c.Order)
	return c
}
