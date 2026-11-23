package tokens

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/pablodiegoo/datafolio/internal/school"
)

// AllTargets lists the five supported export targets in canonical order.
var AllTargets = []string{"css", "tailwind", "scss", "typst", "matplotlib"}

// ExportOptions configures token export generation.
type ExportOptions struct {
	Base    school.SchoolTokens
	Targets []string
}

// Artifact holds a single generated export file name and its rendered content.
type Artifact struct {
	Filename string
	Content  []byte
}

// ParseTargets parses a comma-separated list of export targets ("all" expands to all five).
func ParseTargets(spec string) ([]string, error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" || strings.EqualFold(trimmed, "all") {
		out := make([]string, len(AllTargets))
		copy(out, AllTargets)
		return out, nil
	}

	rawParts := strings.Split(trimmed, ",")
	seen := make(map[string]bool)
	var out []string
	for _, part := range rawParts {
		t := strings.ToLower(strings.TrimSpace(part))
		if t == "" {
			continue
		}
		if t == "all" {
			for _, all := range AllTargets {
				if !seen[all] {
					seen[all] = true
					out = append(out, all)
				}
			}
			continue
		}
		switch t {
		case "css", "tailwind", "scss", "typst", "matplotlib":
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		default:
			return nil, fmt.Errorf("unknown export target %q (supported: %s)", part, strings.Join(AllTargets, ", "))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no export targets specified (supported: %s)", strings.Join(AllTargets, ", "))
	}
	return out, nil
}

type semanticPair struct {
	key   string
	value string
}

type exportModel struct {
	contract    *Contract
	active      school.SchoolTokens
	darkColors  []semanticPair
	serifRaw    string
	monoRaw     string
	sansRaw     string
	themeName   string
	contractKey []string
}

// Export renders synchronized artifacts for the requested targets without mutating inputs.
func Export(c *Contract, opts ExportOptions) ([]Artifact, error) {
	if c == nil {
		c = &Contract{Tokens: map[string]Token{}}
	}
	targets := opts.Targets
	if len(targets) == 0 {
		targets = AllTargets
	}

	model := buildExportModel(c, opts.Base)

	var artifacts []Artifact
	for _, target := range targets {
		switch strings.ToLower(strings.TrimSpace(target)) {
		case "css":
			artifacts = append(artifacts, Artifact{
				Filename: "tokens.css",
				Content:  []byte(renderCSS(model)),
			})
		case "tailwind":
			artifacts = append(artifacts, Artifact{
				Filename: "tokens.tailwind.css",
				Content:  []byte(renderTailwind(model)),
			})
		case "scss":
			artifacts = append(artifacts, Artifact{
				Filename: "_tokens.scss",
				Content:  []byte(renderSCSS(model)),
			})
		case "typst":
			artifacts = append(artifacts, Artifact{
				Filename: "tokens.typ",
				Content:  []byte(renderTypst(model)),
			})
		case "matplotlib":
			jsonBytes, err := renderTokensJSON(model)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts,
				Artifact{
					Filename: "datafolio.mplstyle",
					Content:  []byte(renderMatplotlibStyle(model)),
				},
				Artifact{
					Filename: "tokens.json",
					Content:  jsonBytes,
				},
			)
		default:
			return nil, fmt.Errorf("unknown export target %q (supported: %s)", target, strings.Join(AllTargets, ", "))
		}
	}
	return artifacts, nil
}

func buildExportModel(c *Contract, base school.SchoolTokens) exportModel {
	active := Merge(base, c)
	themeName := strings.TrimSpace(active.Theme)
	if themeName == "" {
		themeName = "paper"
	}

	pickRaw := func(fallback string, paths ...string) string {
		for _, p := range paths {
			if tok, ok := c.Tokens[p]; ok && strings.TrimSpace(tok.Value) != "" {
				return strings.TrimSpace(tok.Value)
			}
		}
		return fallback
	}

	serifRaw := pickRaw(active.SerifFont, "font.serif", "typography.fontFamily.serif", "typography.family.heading", "typography.family.serif")
	monoRaw := pickRaw(active.MonoFont, "font.mono", "typography.fontFamily.mono", "typography.family.mono")
	sansRaw := pickRaw(active.SansFont, "font.sans", "typography.fontFamily.sans", "typography.family.body", "typography.family.sans")

	var darkColors []semanticPair
	if !strings.EqualFold(themeName, "night") && !strings.EqualFold(themeName, "dark") {
		darkColors = resolveDarkPairs(c)
	}

	keys := make([]string, 0, len(c.Tokens))
	for k := range c.Tokens {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return exportModel{
		contract:    c,
		active:      active,
		darkColors:  darkColors,
		serifRaw:    serifRaw,
		monoRaw:     monoRaw,
		sansRaw:     sansRaw,
		themeName:   themeName,
		contractKey: keys,
	}
}

func resolveDarkPairs(c *Contract) []semanticPair {
	pick := func(paths ...string) string {
		for _, p := range paths {
			if tok, ok := c.Tokens[p]; ok && strings.TrimSpace(tok.Value) != "" {
				return strings.TrimSpace(tok.Value)
			}
		}
		return ""
	}
	candidates := []struct {
		key   string
		paths []string
	}{
		{"paper", []string{"color.dark.paper", "color.dark.surface_canvas", "colors.night_theme.paper", "color.night_theme.paper", "color.night.paper"}},
		{"paper_subtle", []string{"color.dark.paper_subtle", "color.dark.surface_card", "colors.night_theme.paper_subtle", "color.night_theme.paper_subtle", "color.night.paper_subtle"}},
		{"paper_alt", []string{"color.dark.paper_alt", "color.dark.surface_subtle", "colors.night_theme.paper_alt", "color.night_theme.paper_alt", "color.night.paper_alt"}},
		{"ink", []string{"color.dark.ink", "color.dark.text_primary", "colors.night_theme.ink", "color.night_theme.ink", "color.night.ink"}},
		{"ink_soft", []string{"color.dark.ink_soft", "color.dark.text_secondary", "colors.night_theme.ink_soft", "color.night_theme.ink_soft", "color.night.ink_soft"}},
		{"muted", []string{"color.dark.muted", "color.dark.text_muted", "colors.night_theme.muted", "color.night_theme.muted", "color.night.muted"}},
		{"accent", []string{"color.dark.accent", "colors.night_theme.accent", "color.night_theme.accent", "color.night.accent", "color.brand.primary_dark"}},
		{"ochre", []string{"color.dark.ochre", "colors.night_theme.ochre", "color.night_theme.ochre", "color.night.ochre"}},
		{"forest", []string{"color.dark.forest", "colors.night_theme.forest", "color.night_theme.forest", "color.night.forest", "color.status.public_dark_text"}},
		{"rule", []string{"color.dark.rule", "color.dark.border_subtle", "colors.night_theme.rule", "color.night_theme.rule", "color.night.rule"}},
		{"rule_strong", []string{"color.dark.rule_strong", "color.dark.border_strong", "colors.night_theme.rule_strong", "color.night_theme.rule_strong", "color.night.rule_strong"}},
	}

	var out []semanticPair
	for _, cand := range candidates {
		if val := pick(cand.paths...); val != "" {
			out = append(out, semanticPair{key: cand.key, value: val})
		}
	}
	return out
}

func semanticDFVars(m exportModel) []semanticPair {
	return []semanticPair{
		{"paper", m.active.Paper},
		{"paper-subtle", m.active.PaperSubtle},
		{"paper-alt", m.active.PaperAlt},
		{"ink", m.active.Ink},
		{"ink-soft", m.active.InkSoft},
		{"muted", m.active.Muted},
		{"accent", m.active.Accent},
		{"ochre", m.active.Ochre},
		{"forest", m.active.Forest},
		{"rule", m.active.Rule},
		{"rule-strong", m.active.RuleStrong},
		{"font-serif", formatCSSFont(m.serifRaw)},
		{"font-mono", formatCSSFont(m.monoRaw)},
		{"font-sans", formatCSSFont(m.sansRaw)},
		{"main-width", m.active.MainWidth},
		{"margin-width", m.active.MarginWidth},
		{"column-gap", m.active.ColumnGap},
		{"radius", m.active.BorderRadius},
		{"shadow", m.active.BoxShadow},
		{"booktabs-top", m.active.BooktabsTopRule},
		{"booktabs-mid", m.active.BooktabsMidRule},
		{"booktabs-row", m.active.BooktabsRowRule},
	}
}

func renderCSS(m exportModel) string {
	var b strings.Builder
	b.WriteString("/* Generated by datafolio tokens export */\n")
	b.WriteString(":root {\n")

	seen := make(map[string]bool)
	for _, p := range semanticDFVars(m) {
		if p.value == "" {
			continue
		}
		name := "--df-" + p.key
		seen[name] = true
		fmt.Fprintf(&b, "  %s: %s;\n", name, p.value)
	}

	for _, k := range m.contractKey {
		tok := m.contract.Tokens[k]
		name := "--" + kebabIdent(k)
		if seen[name] {
			continue
		}
		val := formatCSSValue(tok)
		if val == "" {
			continue
		}
		seen[name] = true
		fmt.Fprintf(&b, "  %s: %s;\n", name, val)
	}
	b.WriteString("}\n")

	if len(m.darkColors) > 0 {
		b.WriteString("\n[data-theme=\"night\"],\n[data-theme=\"dark\"] {\n")
		for _, p := range m.darkColors {
			fmt.Fprintf(&b, "  --df-%s: %s;\n", kebabIdent(p.key), p.value)
		}
		b.WriteString("}\n")
	}

	return b.String()
}

func renderTailwind(m exportModel) string {
	var b strings.Builder
	b.WriteString("/* Generated by datafolio tokens export (Tailwind v4) */\n")
	b.WriteString("@theme {\n")

	seen := make(map[string]bool)
	emit := func(name, val string) {
		if val == "" || seen[name] {
			return
		}
		seen[name] = true
		fmt.Fprintf(&b, "  %s: %s;\n", name, val)
	}

	emit("--color-paper", m.active.Paper)
	emit("--color-paper-subtle", m.active.PaperSubtle)
	emit("--color-paper-alt", m.active.PaperAlt)
	emit("--color-ink", m.active.Ink)
	emit("--color-ink-soft", m.active.InkSoft)
	emit("--color-muted", m.active.Muted)
	emit("--color-accent", m.active.Accent)
	emit("--color-ochre", m.active.Ochre)
	emit("--color-forest", m.active.Forest)
	emit("--color-rule", m.active.Rule)
	emit("--color-rule-strong", m.active.RuleStrong)
	emit("--font-serif", formatCSSFont(m.serifRaw))
	emit("--font-mono", formatCSSFont(m.monoRaw))
	emit("--font-sans", formatCSSFont(m.sansRaw))
	emit("--radius-default", m.active.BorderRadius)
	emit("--shadow-default", m.active.BoxShadow)

	for _, k := range m.contractKey {
		tok := m.contract.Tokens[k]
		twName := tailwindVarName(tok)
		if twName == "" {
			continue
		}
		emit(twName, formatCSSValue(tok))
	}
	b.WriteString("}\n")

	if len(m.darkColors) > 0 {
		b.WriteString("\n[data-theme=\"night\"],\n[data-theme=\"dark\"] {\n")
		for _, p := range m.darkColors {
			fmt.Fprintf(&b, "  --color-%s: %s;\n", kebabIdent(p.key), p.value)
		}
		b.WriteString("}\n")
	}

	return b.String()
}

func tailwindVarName(tok Token) string {
	lower := strings.ToLower(tok.Path)
	kebab := kebabIdent(tok.Path)
	switch {
	case tok.Type == "color" || strings.HasPrefix(lower, "color.") || strings.HasPrefix(lower, "colors."):
		kebab = strings.TrimPrefix(kebab, "colors-")
		kebab = strings.TrimPrefix(kebab, "color-")
		return "--color-" + kebab
	case tok.Type == "fontFamily" || strings.HasPrefix(lower, "font.") || strings.HasPrefix(lower, "typography.family.") || strings.HasPrefix(lower, "typography.fontfamily."):
		kebab = strings.TrimPrefix(kebab, "typography-font-family-")
		kebab = strings.TrimPrefix(kebab, "typography-family-")
		kebab = strings.TrimPrefix(kebab, "font-")
		return "--font-" + kebab
	case strings.HasPrefix(lower, "radius.") || strings.HasPrefix(lower, "shape.borderradius."):
		kebab = strings.TrimPrefix(kebab, "shape-border-radius-")
		kebab = strings.TrimPrefix(kebab, "radius-")
		return "--radius-" + kebab
	case strings.HasPrefix(lower, "spacing."):
		kebab = strings.TrimPrefix(kebab, "spacing-")
		return "--spacing-" + kebab
	case strings.HasPrefix(lower, "shadow.") || strings.Contains(lower, "boxshadow"):
		kebab = strings.TrimPrefix(kebab, "shadow-")
		return "--shadow-" + kebab
	default:
		return ""
	}
}

func renderSCSS(m exportModel) string {
	var b strings.Builder
	b.WriteString("// Generated by datafolio tokens export\n")

	seen := make(map[string]bool)
	emit := func(name, val string) {
		if val == "" || seen[name] {
			return
		}
		seen[name] = true
		fmt.Fprintf(&b, "$%s: %s;\n", name, val)
	}

	for _, p := range semanticDFVars(m) {
		emit("df-"+p.key, p.value)
	}
	for _, p := range m.darkColors {
		emit("df-night-"+kebabIdent(p.key), p.value)
	}
	for _, k := range m.contractKey {
		tok := m.contract.Tokens[k]
		val := formatSCSSValue(tok)
		if val == "" {
			continue
		}
		emit(kebabIdent(k), val)
	}
	return b.String()
}

func renderTypst(m exportModel) string {
	var b strings.Builder
	b.WriteString("// Generated by datafolio tokens export\n")

	seen := make(map[string]bool)
	emit := func(name, expr string) {
		if expr == "" || seen[name] {
			return
		}
		seen[name] = true
		fmt.Fprintf(&b, "#let %s = %s\n", name, expr)
	}

	emitColor := func(name, val string) {
		if val == "" {
			return
		}
		emit(name, typstColorExpr(val))
	}
	emitDim := func(name, val string) {
		if val == "" {
			return
		}
		emit(name, typstDimExpr(val))
	}

	emitColor("df-paper", m.active.Paper)
	emitColor("df-paper-subtle", m.active.PaperSubtle)
	emitColor("df-paper-alt", m.active.PaperAlt)
	emitColor("df-ink", m.active.Ink)
	emitColor("df-ink-soft", m.active.InkSoft)
	emitColor("df-muted", m.active.Muted)
	emitColor("df-accent", m.active.Accent)
	emitColor("df-ochre", m.active.Ochre)
	emitColor("df-forest", m.active.Forest)
	emitColor("df-rule", m.active.Rule)
	emitColor("df-rule-strong", m.active.RuleStrong)

	emit("df-font-serif", typstFontExpr(m.serifRaw))
	emit("df-font-mono", typstFontExpr(m.monoRaw))
	emit("df-font-sans", typstFontExpr(m.sansRaw))

	emitDim("df-main-width", m.active.MainWidth)
	emitDim("df-margin-width", m.active.MarginWidth)
	emitDim("df-column-gap", m.active.ColumnGap)
	emitDim("df-radius", m.active.BorderRadius)
	emit("df-shadow", typstString(m.active.BoxShadow))
	emitDim("df-booktabs-top", m.active.BooktabsTopRule)
	emitDim("df-booktabs-mid", m.active.BooktabsMidRule)
	emitDim("df-booktabs-row", m.active.BooktabsRowRule)

	for _, p := range m.darkColors {
		emitColor("df-night-"+kebabIdent(p.key), p.value)
	}

	for _, k := range m.contractKey {
		tok := m.contract.Tokens[k]
		name := kebabIdent(k)
		if name == "" || unicode.IsDigit(rune(name[0])) {
			name = "t-" + name
		}
		if seen[name] {
			continue
		}
		emit(name, typstTokenExpr(tok))
	}
	return b.String()
}

func renderMatplotlibStyle(m exportModel) string {
	var b strings.Builder
	b.WriteString("# Generated by datafolio tokens export\n")
	fmt.Fprintf(&b, "figure.facecolor: %s\n", mplColor(m.active.Paper))
	fmt.Fprintf(&b, "axes.facecolor: %s\n", mplColor(m.active.Paper))
	fmt.Fprintf(&b, "savefig.facecolor: %s\n", mplColor(m.active.Paper))
	fmt.Fprintf(&b, "text.color: %s\n", mplColor(m.active.Ink))
	fmt.Fprintf(&b, "axes.labelcolor: %s\n", mplColor(m.active.InkSoft))
	fmt.Fprintf(&b, "xtick.color: %s\n", mplColor(m.active.Muted))
	fmt.Fprintf(&b, "ytick.color: %s\n", mplColor(m.active.Muted))
	fmt.Fprintf(&b, "axes.edgecolor: %s\n", mplColor(m.active.RuleStrong))
	b.WriteString("axes.linewidth: 0.8\n")
	b.WriteString("axes.spines.top: False\n")
	b.WriteString("axes.spines.right: False\n")
	b.WriteString("axes.grid: True\n")
	b.WriteString("axes.axisbelow: True\n")
	fmt.Fprintf(&b, "grid.color: %s\n", mplColor(m.active.Rule))
	b.WriteString("grid.linewidth: 0.5\n")
	b.WriteString("grid.linestyle: :\n")
	b.WriteString("font.family: serif\n")
	fmt.Fprintf(&b, "font.serif: %s\n", mplFontList(m.active.SerifFont, "Georgia", "DejaVu Serif"))
	fmt.Fprintf(&b, "font.sans-serif: %s\n", mplFontList(m.active.SansFont, "DejaVu Sans"))
	fmt.Fprintf(&b, "font.monospace: %s\n", mplFontList(m.active.MonoFont, "DejaVu Sans Mono"))

	palette := buildPalette(m.active)
	quoted := make([]string, 0, len(palette))
	for _, c := range palette {
		quoted = append(quoted, "'"+mplColor(c)+"'")
	}
	fmt.Fprintf(&b, "axes.prop_cycle: cycler('color', [%s])\n", strings.Join(quoted, ", "))
	return b.String()
}

type tokensJSONDoc struct {
	Name   string                    `json:"name,omitempty"`
	Theme  string                    `json:"theme"`
	Colors tokensJSONColors          `json:"colors"`
	Dark   *tokensJSONDarkColors     `json:"dark,omitempty"`
	Fonts  tokensJSONFonts           `json:"fonts"`
	Layout tokensJSONLayout          `json:"layout"`
	Tokens map[string]tokensJSONItem `json:"tokens"`
}

type tokensJSONColors struct {
	Paper       string   `json:"paper"`
	PaperSubtle string   `json:"paper_subtle"`
	PaperAlt    string   `json:"paper_alt"`
	Ink         string   `json:"ink"`
	InkSoft     string   `json:"ink_soft"`
	Muted       string   `json:"muted"`
	Accent      string   `json:"accent"`
	Ochre       string   `json:"ochre"`
	Forest      string   `json:"forest"`
	Rule        string   `json:"rule"`
	RuleStrong  string   `json:"rule_strong"`
	Palette     []string `json:"palette"`
}

type tokensJSONDarkColors struct {
	Paper       string `json:"paper,omitempty"`
	PaperSubtle string `json:"paper_subtle,omitempty"`
	PaperAlt    string `json:"paper_alt,omitempty"`
	Ink         string `json:"ink,omitempty"`
	InkSoft     string `json:"ink_soft,omitempty"`
	Muted       string `json:"muted,omitempty"`
	Accent      string `json:"accent,omitempty"`
	Ochre       string `json:"ochre,omitempty"`
	Forest      string `json:"forest,omitempty"`
	Rule        string `json:"rule,omitempty"`
	RuleStrong  string `json:"rule_strong,omitempty"`
}

type tokensJSONFonts struct {
	Serif string `json:"serif"`
	Mono  string `json:"mono"`
	Sans  string `json:"sans"`
}

type tokensJSONLayout struct {
	MainWidth    string `json:"main_width"`
	MarginWidth  string `json:"margin_width"`
	ColumnGap    string `json:"column_gap"`
	BorderRadius string `json:"border_radius"`
	BoxShadow    string `json:"box_shadow"`
	BooktabsTop  string `json:"booktabs_top"`
	BooktabsMid  string `json:"booktabs_mid"`
	BooktabsRow  string `json:"booktabs_row"`
}

type tokensJSONItem struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func renderTokensJSON(m exportModel) ([]byte, error) {
	doc := tokensJSONDoc{
		Name:  m.contract.Name,
		Theme: m.themeName,
		Colors: tokensJSONColors{
			Paper:       m.active.Paper,
			PaperSubtle: m.active.PaperSubtle,
			PaperAlt:    m.active.PaperAlt,
			Ink:         m.active.Ink,
			InkSoft:     m.active.InkSoft,
			Muted:       m.active.Muted,
			Accent:      m.active.Accent,
			Ochre:       m.active.Ochre,
			Forest:      m.active.Forest,
			Rule:        m.active.Rule,
			RuleStrong:  m.active.RuleStrong,
			Palette:     buildPalette(m.active),
		},
		Fonts: tokensJSONFonts{
			Serif: m.active.SerifFont,
			Mono:  m.active.MonoFont,
			Sans:  m.active.SansFont,
		},
		Layout: tokensJSONLayout{
			MainWidth:    m.active.MainWidth,
			MarginWidth:  m.active.MarginWidth,
			ColumnGap:    m.active.ColumnGap,
			BorderRadius: m.active.BorderRadius,
			BoxShadow:    m.active.BoxShadow,
			BooktabsTop:  m.active.BooktabsTopRule,
			BooktabsMid:  m.active.BooktabsMidRule,
			BooktabsRow:  m.active.BooktabsRowRule,
		},
		Tokens: make(map[string]tokensJSONItem, len(m.contract.Tokens)),
	}

	if len(m.darkColors) > 0 {
		dark := &tokensJSONDarkColors{}
		for _, p := range m.darkColors {
			switch p.key {
			case "paper":
				dark.Paper = p.value
			case "paper_subtle":
				dark.PaperSubtle = p.value
			case "paper_alt":
				dark.PaperAlt = p.value
			case "ink":
				dark.Ink = p.value
			case "ink_soft":
				dark.InkSoft = p.value
			case "muted":
				dark.Muted = p.value
			case "accent":
				dark.Accent = p.value
			case "ochre":
				dark.Ochre = p.value
			case "forest":
				dark.Forest = p.value
			case "rule":
				dark.Rule = p.value
			case "rule_strong":
				dark.RuleStrong = p.value
			}
		}
		doc.Dark = dark
	}

	for _, k := range m.contractKey {
		tok := m.contract.Tokens[k]
		doc.Tokens[k] = tokensJSONItem{
			Type:  tok.Type,
			Value: tok.Value,
		}
	}

	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling tokens.json: %w", err)
	}
	return append(encoded, '\n'), nil
}

func buildPalette(tok school.SchoolTokens) []string {
	candidates := []string{tok.Accent, tok.Ink, tok.Ochre, tok.Forest, tok.Muted}
	var out []string
	seen := make(map[string]bool)
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func mplColor(raw string) string {
	rgba, err := ParseColor(raw)
	if err != nil {
		return strings.TrimPrefix(strings.TrimSpace(raw), "#")
	}
	return strings.TrimPrefix(rgba.Hex(), "#")
}

func mplFontList(primary string, fallbacks ...string) string {
	var list []string
	seen := make(map[string]bool)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[strings.ToLower(name)] {
			return
		}
		seen[strings.ToLower(name)] = true
		list = append(list, name)
	}
	add(primary)
	for _, fb := range fallbacks {
		add(fb)
	}
	return strings.Join(list, ", ")
}

func formatCSSFont(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.ContainsRune(s, ',') || strings.HasPrefix(s, `"`) || strings.HasPrefix(s, `'`) {
		return s
	}
	if strings.ContainsRune(s, ' ') {
		return `"` + s + `"`
	}
	return `"` + s + `"`
}

func formatCSSValue(tok Token) string {
	v := strings.TrimSpace(tok.Value)
	if v == "" || strings.ContainsRune(v, '\n') {
		return ""
	}
	if tok.Type == "fontFamily" {
		return formatCSSFont(v)
	}
	return v
}

func formatSCSSValue(tok Token) string {
	v := formatCSSValue(tok)
	if v == "" {
		return ""
	}
	if strings.ContainsRune(v, ':') && !strings.HasPrefix(v, `"`) && !strings.HasPrefix(v, `'`) {
		return strconv.Quote(v)
	}
	return v
}

func typstColorExpr(raw string) string {
	rgba, err := ParseColor(raw)
	if err != nil {
		return typstString(raw)
	}
	return fmt.Sprintf("rgb(%q)", rgba.Hex())
}

func typstFontExpr(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return `""`
	}
	if !strings.ContainsRune(s, ',') {
		return typstString(primaryFontFamily(s))
	}
	parts := strings.Split(s, ",")
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.Trim(strings.TrimSpace(p), `"'`)
		if name != "" {
			quoted = append(quoted, strconv.Quote(name))
		}
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return "(" + strings.Join(quoted, ", ") + ")"
}

func typstDimExpr(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return `""`
	}
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "px") {
		numStr := strings.TrimSpace(s[:len(s)-2])
		if v, err := strconv.ParseFloat(numStr, 64); err == nil {
			if v == 0 {
				return "0pt"
			}
			pt := v * 0.75
			return strconv.FormatFloat(pt, 'f', -1, 64) + "pt"
		}
	}
	if strings.HasSuffix(lower, "rem") {
		numStr := strings.TrimSpace(s[:len(s)-3])
		if v, err := strconv.ParseFloat(numStr, 64); err == nil {
			return strconv.FormatFloat(v, 'f', -1, 64) + "em"
		}
	}
	for _, unit := range []string{"pt", "mm", "cm", "in", "em", "%", "fr"} {
		if strings.HasSuffix(lower, unit) {
			numStr := strings.TrimSpace(s[:len(s)-len(unit)])
			if _, err := strconv.ParseFloat(numStr, 64); err == nil {
				return s
			}
		}
	}
	return typstString(s)
}

func typstTokenExpr(tok Token) string {
	v := strings.TrimSpace(tok.Value)
	if tok.Type == "color" || looksLikeColor(v) {
		return typstColorExpr(v)
	}
	if tok.Type == "fontFamily" {
		return typstFontExpr(v)
	}
	if tok.Type == "dimension" {
		return typstDimExpr(v)
	}
	return typstString(v)
}

func typstString(s string) string {
	return strconv.Quote(s)
}

func kebabIdent(path string) string {
	var b strings.Builder
	prevDash := false
	for i, r := range path {
		switch {
		case r == '.' || r == '_' || r == '-' || unicode.IsSpace(r):
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		case unicode.IsUpper(r):
			if i > 0 && !prevDash {
				prev := rune(path[i-1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					b.WriteByte('-')
				}
			}
			b.WriteRune(unicode.ToLower(r))
			prevDash = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevDash = false
		}
	}
	return strings.Trim(b.String(), "-")
}
