package tokens

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// RGBA represents an sRGB color with an alpha channel in [0, 1].
type RGBA struct {
	R uint8
	G uint8
	B uint8
	A float64
}

// Hex returns the normalized lowercase #rrggbb (or #rrggbbaa when translucent) string.
func (c RGBA) Hex() string {
	if c.A < 1.0-1e-6 {
		a := uint8(math.Round(clamp01(c.A) * 255.0))
		return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, a)
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// ContrastResult holds the WCAG 2.1 contrast evaluation for a single foreground/background pair.
type ContrastResult struct {
	Polarity        string
	Level           string
	ForegroundToken string
	ForegroundColor string
	BackgroundToken string
	BackgroundColor string
	Ratio           float64
	Required        float64
	Pass            bool
}

// Format returns a human-readable diagnostic summary for the ContrastResult.
func (r ContrastResult) Format() string {
	status := "PASS"
	op := ">="
	if !r.Pass {
		status = "FAIL"
		op = "<"
	}
	return fmt.Sprintf(
		"%s [%s] %s (%s) on %s (%s): %.2f:1 %s %.1f:1 (%s)",
		status,
		r.Polarity,
		r.ForegroundToken,
		r.ForegroundColor,
		r.BackgroundToken,
		r.BackgroundColor,
		r.Ratio,
		op,
		r.Required,
		r.Level,
	)
}

// ParseColor parses #RGB, #RGBA, #RRGGBB, #RRGGBBAA, rgb(...), and rgba(...) color literals.
func ParseColor(raw string) (RGBA, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return RGBA{}, fmt.Errorf("empty color value")
	}
	if strings.HasPrefix(s, "#") {
		return parseHexColor(s[1:])
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "rgba(") && strings.HasSuffix(lower, ")") {
		return parseRGBFunc(s[5:len(s)-1], true)
	}
	if strings.HasPrefix(lower, "rgb(") && strings.HasSuffix(lower, ")") {
		return parseRGBFunc(s[4:len(s)-1], false)
	}
	return RGBA{}, fmt.Errorf("unsupported color format %q", raw)
}

func parseHexColor(hex string) (RGBA, error) {
	switch len(hex) {
	case 3, 4:
		r, err := parseHexNibble(hex[0])
		if err != nil {
			return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
		}
		g, err := parseHexNibble(hex[1])
		if err != nil {
			return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
		}
		b, err := parseHexNibble(hex[2])
		if err != nil {
			return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
		}
		a := 1.0
		if len(hex) == 4 {
			av, err := parseHexNibble(hex[3])
			if err != nil {
				return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
			}
			a = float64(av) / 255.0
		}
		return RGBA{R: r, G: g, B: b, A: a}, nil
	case 6, 8:
		r, err := strconv.ParseUint(hex[0:2], 16, 8)
		if err != nil {
			return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
		}
		g, err := strconv.ParseUint(hex[2:4], 16, 8)
		if err != nil {
			return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
		}
		b, err := strconv.ParseUint(hex[4:6], 16, 8)
		if err != nil {
			return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
		}
		a := 1.0
		if len(hex) == 8 {
			av, err := strconv.ParseUint(hex[6:8], 16, 8)
			if err != nil {
				return RGBA{}, fmt.Errorf("invalid hex color #%s", hex)
			}
			a = float64(av) / 255.0
		}
		return RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: a}, nil
	default:
		return RGBA{}, fmt.Errorf("invalid hex color length #%s", hex)
	}
}

func parseHexNibble(ch byte) (uint8, error) {
	v, err := strconv.ParseUint(string(ch), 16, 8)
	if err != nil {
		return 0, err
	}
	return uint8(v*16 + v), nil
}

func parseRGBFunc(inner string, hasAlphaHint bool) (RGBA, error) {
	var parts []string
	var alphaStr string

	if slashIdx := strings.IndexByte(inner, '/'); slashIdx >= 0 {
		alphaStr = strings.TrimSpace(inner[slashIdx+1:])
		rgbPart := strings.TrimSpace(inner[:slashIdx])
		parts = splitRGBChannels(rgbPart)
	} else {
		parts = splitRGBChannels(inner)
		if len(parts) == 4 {
			alphaStr = parts[3]
			parts = parts[:3]
		}
	}

	if len(parts) != 3 {
		return RGBA{}, fmt.Errorf("expected 3 RGB channels in %q", inner)
	}
	if hasAlphaHint && alphaStr == "" && len(parts) != 3 {
		return RGBA{}, fmt.Errorf("expected alpha channel in rgba(%s)", inner)
	}

	r, err := parseRGBChannel(parts[0])
	if err != nil {
		return RGBA{}, err
	}
	g, err := parseRGBChannel(parts[1])
	if err != nil {
		return RGBA{}, err
	}
	b, err := parseRGBChannel(parts[2])
	if err != nil {
		return RGBA{}, err
	}

	a := 1.0
	if alphaStr != "" {
		a, err = parseAlpha(alphaStr)
		if err != nil {
			return RGBA{}, err
		}
	}
	return RGBA{R: r, G: g, B: b, A: a}, nil
}

func splitRGBChannels(s string) []string {
	if strings.ContainsRune(s, ',') {
		raw := strings.Split(s, ",")
		out := make([]string, 0, len(raw))
		for _, p := range raw {
			out = append(out, strings.TrimSpace(p))
		}
		return out
	}
	return strings.Fields(s)
}

func parseRGBChannel(s string) (uint8, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid RGB percentage %q", s)
		}
		return uint8(math.Round(clamp01(pct/100.0) * 255.0)), nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid RGB channel %q", s)
	}
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(math.Round(v)), nil
}

func parseAlpha(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid alpha percentage %q", s)
		}
		return clamp01(pct / 100.0), nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid alpha value %q", s)
	}
	return clamp01(v), nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// CompositeOver alpha-composites fg over bg using sRGB source-over blending.
func CompositeOver(fg, bg RGBA) RGBA {
	if fg.A >= 1.0-1e-6 {
		return RGBA{R: fg.R, G: fg.G, B: fg.B, A: 1.0}
	}
	outA := fg.A + bg.A*(1.0-fg.A)
	if outA <= 1e-6 {
		return RGBA{}
	}
	blend := func(fc, bc uint8) uint8 {
		num := float64(fc)*fg.A + float64(bc)*bg.A*(1.0-fg.A)
		return uint8(math.Round(num / outA))
	}
	return RGBA{
		R: blend(fg.R, bg.R),
		G: blend(fg.G, bg.G),
		B: blend(fg.B, bg.B),
		A: clamp01(outA),
	}
}

// RelativeLuminance calculates the WCAG 2.1 relative luminance in [0, 1].
func RelativeLuminance(c RGBA) float64 {
	r := linearizeChannel(float64(c.R) / 255.0)
	g := linearizeChannel(float64(c.G) / 255.0)
	b := linearizeChannel(float64(c.B) / 255.0)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

func linearizeChannel(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// ContrastRatio computes the WCAG 2.1 contrast ratio between fgStr and bgStr.
func ContrastRatio(fgStr, bgStr string) (float64, error) {
	fg, err := ParseColor(fgStr)
	if err != nil {
		return 0, fmt.Errorf("foreground %w", err)
	}
	bg, err := ParseColor(bgStr)
	if err != nil {
		return 0, fmt.Errorf("background %w", err)
	}
	if bg.A < 1.0-1e-6 {
		bg = CompositeOver(bg, RGBA{R: 255, G: 255, B: 255, A: 1.0})
	}
	if fg.A < 1.0-1e-6 {
		fg = CompositeOver(fg, bg)
	}
	l1 := RelativeLuminance(fg)
	l2 := RelativeLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05), nil
}

type colorEntry struct {
	token Token
	rgba  RGBA
	leaf  string
	group string
}

type polarityBucket struct {
	name             string
	fallbackCanvas   RGBA
	entries          []colorEntry
	primaryCanvas    *colorEntry
	secondarySurface []colorEntry
}

// CheckContrast evaluates WCAG 2.1 contrast pairs across light and dark polarities in a Contract.
func CheckContrast(c *Contract, minNormal, minLarge float64) ([]ContrastResult, error) {
	if minNormal <= 0 {
		minNormal = 4.5
	}
	if minLarge <= 0 {
		minLarge = 3.0
	}
	if c == nil || len(c.Tokens) == 0 {
		return nil, nil
	}

	lightBucket := &polarityBucket{name: "light", fallbackCanvas: RGBA{R: 255, G: 255, B: 255, A: 1.0}}
	darkBucket := &polarityBucket{name: "dark", fallbackCanvas: RGBA{R: 18, G: 19, B: 22, A: 1.0}}
	subGroups := make(map[string]*polarityBucket)
	var shared []colorEntry
	statusGroups := make(map[string]map[string]colorEntry)

	for _, path := range c.Order {
		tok := c.Tokens[path]
		if !isColorCheckTarget(tok) {
			continue
		}
		rgba, err := ParseColor(tok.Value)
		if err != nil {
			return nil, fmt.Errorf("token %q (%q): %w", tok.Path, tok.Value, err)
		}

		group, leaf := splitGroupLeaf(tok.Path)
		entry := colorEntry{token: tok, rgba: rgba, leaf: strings.ToLower(leaf), group: group}

		if strings.Contains(strings.ToLower(group), "status") {
			if statusGroups[group] == nil {
				statusGroups[group] = make(map[string]colorEntry)
			}
			statusGroups[group][entry.leaf] = entry
			continue
		}

		if pol := classifyPolarity(tok.Path, entry.leaf); pol != "" {
			if pol == "dark" {
				darkBucket.entries = append(darkBucket.entries, entry)
			} else {
				lightBucket.entries = append(lightBucket.entries, entry)
			}
			continue
		}

		if isIsolatedColorGroup(group) {
			b := subGroups[group]
			if b == nil {
				b = &polarityBucket{name: group, fallbackCanvas: RGBA{R: 255, G: 255, B: 255, A: 1.0}}
				subGroups[group] = b
			}
			b.entries = append(b.entries, entry)
			continue
		}

		shared = append(shared, entry)
	}

	if len(shared) > 0 {
		hasExplicitLight := len(lightBucket.entries) > 0
		hasExplicitDark := len(darkBucket.entries) > 0
		if !hasExplicitLight && !hasExplicitDark {
			if sfc := findPrimaryCanvas(shared); sfc != nil && RelativeLuminance(sfc.rgba) < 0.179 {
				darkBucket.entries = append(darkBucket.entries, shared...)
			} else {
				lightBucket.entries = append(lightBucket.entries, shared...)
			}
		} else {
			lightBucket.entries = append(lightBucket.entries, shared...)
		}
	}

	var results []ContrastResult
	seen := make(map[string]bool)

	evaluateBucket := func(b *polarityBucket) {
		if len(b.entries) == 0 {
			return
		}
		b.primaryCanvas = findPrimaryCanvas(b.entries)
		b.secondarySurface = findSecondarySurfaces(b.entries)

		bgPrimary := b.fallbackCanvas
		bgPrimaryTok := "(default-" + b.name + "-canvas)"
		bgPrimaryStr := bgPrimary.Hex()
		if b.primaryCanvas != nil {
			bgPrimary = b.primaryCanvas.rgba
			if bgPrimary.A < 1.0-1e-6 {
				bgPrimary = CompositeOver(bgPrimary, b.fallbackCanvas)
			}
			bgPrimaryTok = b.primaryCanvas.token.Path
			bgPrimaryStr = b.primaryCanvas.token.Value
		}

		bucketPol := b.name
		if b.primaryCanvas != nil && b.name != "light" && b.name != "dark" {
			if RelativeLuminance(bgPrimary) < 0.179 {
				bucketPol = "dark"
			} else {
				bucketPol = "light"
			}
		}

		for _, fg := range b.entries {
			level, req, checkSecondary := classifyContrastRole(fg.leaf)
			if level == "" {
				continue
			}
			threshold := minNormal
			if level == "large" {
				threshold = minLarge
			} else if req > 0 {
				threshold = req
			}

			addPair := func(bgRGBA RGBA, bgTok, bgVal string) {
				key := fg.token.Path + "|" + bgTok + "|" + level
				if seen[key] {
					return
				}
				seen[key] = true

				effFG := fg.rgba
				if effFG.A < 1.0-1e-6 {
					effFG = CompositeOver(effFG, bgRGBA)
				}
				ratio := contrastLuminanceRatio(effFG, bgRGBA)
				results = append(results, ContrastResult{
					Polarity:        bucketPol,
					Level:           level,
					ForegroundToken: fg.token.Path,
					ForegroundColor: fg.token.Value,
					BackgroundToken: bgTok,
					BackgroundColor: bgVal,
					Ratio:           ratio,
					Required:        threshold,
					Pass:            ratio >= threshold,
				})
			}

			addPair(bgPrimary, bgPrimaryTok, bgPrimaryStr)

			if checkSecondary {
				for _, sec := range b.secondarySurface {
					secBG := sec.rgba
					if secBG.A < 1.0-1e-6 {
						secBG = CompositeOver(secBG, bgPrimary)
					}
					addPair(secBG, sec.token.Path, sec.token.Value)
				}
			}
		}
	}

	evaluateBucket(lightBucket)
	evaluateBucket(darkBucket)

	subNames := make([]string, 0, len(subGroups))
	for k := range subGroups {
		subNames = append(subNames, k)
	}
	sort.Strings(subNames)
	for _, k := range subNames {
		evaluateBucket(subGroups[k])
	}

	lightCanvas := lightBucket.fallbackCanvas
	if lightBucket.primaryCanvas != nil {
		lightCanvas = lightBucket.primaryCanvas.rgba
	}
	darkCanvas := darkBucket.fallbackCanvas
	if darkBucket.primaryCanvas != nil {
		darkCanvas = darkBucket.primaryCanvas.rgba
	}

	statusGroupNames := make([]string, 0, len(statusGroups))
	for k := range statusGroups {
		statusGroupNames = append(statusGroupNames, k)
	}
	sort.Strings(statusGroupNames)
	for _, grp := range statusGroupNames {
		entries := statusGroups[grp]
		leaves := make([]string, 0, len(entries))
		for leaf := range entries {
			leaves = append(leaves, leaf)
		}
		sort.Strings(leaves)

		for _, leaf := range leaves {
			var stem string
			switch {
			case strings.HasSuffix(leaf, "_text"):
				stem = strings.TrimSuffix(leaf, "_text")
			case strings.HasSuffix(leaf, "_fg"):
				stem = strings.TrimSuffix(leaf, "_fg")
			default:
				continue
			}
			bgEntry, ok := entries[stem+"_bg"]
			if !ok {
				bgEntry, ok = entries[stem+"_background"]
			}
			if !ok {
				continue
			}
			fgEntry := entries[leaf]
			pol := "light"
			baseCanvas := lightCanvas
			if strings.Contains(stem, "dark") || strings.Contains(stem, "night") {
				pol = "dark"
				baseCanvas = darkCanvas
			}
			bgEff := bgEntry.rgba
			if bgEff.A < 1.0-1e-6 {
				bgEff = CompositeOver(bgEff, baseCanvas)
			}
			fgEff := fgEntry.rgba
			if fgEff.A < 1.0-1e-6 {
				fgEff = CompositeOver(fgEff, bgEff)
			}
			ratio := contrastLuminanceRatio(fgEff, bgEff)
			key := fgEntry.token.Path + "|" + bgEntry.token.Path + "|normal"
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, ContrastResult{
				Polarity:        pol,
				Level:           "normal",
				ForegroundToken: fgEntry.token.Path,
				ForegroundColor: fgEntry.token.Value,
				BackgroundToken: bgEntry.token.Path,
				BackgroundColor: bgEntry.token.Value,
				Ratio:           ratio,
				Required:        minNormal,
				Pass:            ratio >= minNormal,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Polarity != results[j].Polarity {
			return results[i].Polarity < results[j].Polarity
		}
		if results[i].Level != results[j].Level {
			return results[i].Level < results[j].Level
		}
		if results[i].BackgroundToken != results[j].BackgroundToken {
			return results[i].BackgroundToken < results[j].BackgroundToken
		}
		return results[i].ForegroundToken < results[j].ForegroundToken
	})

	return results, nil
}

func contrastLuminanceRatio(fg, bg RGBA) float64 {
	l1 := RelativeLuminance(fg)
	l2 := RelativeLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func isColorCheckTarget(tok Token) bool {
	lower := strings.ToLower(tok.Path)
	if strings.HasPrefix(lower, "typography.scale.") {
		return false
	}
	if tok.Type != "color" && !looksLikeColor(tok.Value) {
		return false
	}
	return strings.HasPrefix(lower, "color.") ||
		strings.HasPrefix(lower, "colors.") ||
		strings.Contains(lower, ".color.")
}

func splitGroupLeaf(path string) (string, string) {
	idx := strings.LastIndexByte(path, '.')
	if idx < 0 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

func classifyPolarity(path, leaf string) string {
	segs := strings.Split(strings.ToLower(path), ".")
	for _, seg := range segs {
		switch seg {
		case "dark", "night", "night_theme", "dark_theme":
			return "dark"
		case "light", "paper_theme", "light_theme", "print", "print_theme":
			return "light"
		}
	}
	if strings.HasSuffix(leaf, "_dark") || strings.HasPrefix(leaf, "dark_") {
		return "dark"
	}
	return ""
}

func isIsolatedColorGroup(group string) bool {
	lower := strings.ToLower(group)
	if lower == "color" || lower == "colors" ||
		lower == "color.brand" || lower == "color.data_viz" {
		return false
	}
	return strings.Contains(lower, ".") && !strings.HasPrefix(lower, "color.brand")
}

func findPrimaryCanvas(entries []colorEntry) *colorEntry {
	for _, candidate := range []string{"surface_canvas", "paper", "canvas", "background", "bg", "surface"} {
		for i := range entries {
			if entries[i].leaf == candidate {
				return &entries[i]
			}
		}
	}
	return nil
}

func findSecondarySurfaces(entries []colorEntry) []colorEntry {
	var out []colorEntry
	for _, e := range entries {
		switch e.leaf {
		case "paper_subtle", "paper_alt", "surface_card", "surface_subtle", "surface_inset":
			out = append(out, e)
		}
	}
	return out
}

func classifyContrastRole(leaf string) (level string, customReq float64, checkSecondary bool) {
	switch leaf {
	case "paper", "paper_subtle", "paper_alt", "surface_canvas", "surface_card",
		"surface_subtle", "surface_inset", "canvas", "background", "bg", "surface",
		"rule", "border_subtle", "border_strong", "divider", "border", "rule_neutral",
		"accent_subtle", "lagna_tint", "highlight":
		return "", 0, false
	case "rule_strong":
		return "large", 0, false
	case "ink", "ink_soft", "muted", "text_primary", "text_secondary", "text_muted":
		return "normal", 0, true
	case "text_caption", "text", "text_title", "text_mono", "fg", "foreground",
		"accent", "ochre", "forest", "primary", "primary_dark", "secondary",
		"positive", "negative":
		return "normal", 0, false
	}
	if strings.Contains(leaf, "large") || strings.HasSuffix(leaf, "_ui") || strings.HasPrefix(leaf, "ui_") || leaf == "ui" || leaf == "focus_ring" {
		return "large", 0, false
	}
	if strings.HasPrefix(leaf, "text_") || strings.HasPrefix(leaf, "ink_") {
		return "normal", 0, false
	}
	return "", 0, false
}
