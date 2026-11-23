package review

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/school"
	"github.com/pablodiegoo/datafolio/internal/tokens"
)

const (
	RuleWebHardcodedColor    = "web-hardcoded-color"
	RuleWebOffContractRadius = "web-off-contract-radius"
	RuleWebOffContractShadow = "web-off-contract-shadow"
)

func init() {
	defaultWebReviewer = reviewWebFile
}

type webPolicy struct {
	schoolName     string
	policy         school.ReviewPolicy
	allowedColors  map[string]bool
	allowedRadii   map[string]bool
	allowedShadows map[string]bool
	allowAnyShadow bool
}

func reviewWebFile(_ context.Context, absPath, displayPath string, s school.School, contract *tokens.Contract) ([]Finding, error) {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return nil, usageErrorf("%s: %v", displayPath, err)
	}

	normalized := normalizeWebSource(raw)
	buf := []byte(normalized)
	wp := buildWebPolicy(s, contract)

	lowerPath := strings.ToLower(absPath)
	ext := strings.ToLower(filepath.Ext(lowerPath))
	var findings []Finding

	switch {
	case strings.HasSuffix(lowerPath, ".blade.php"):
		maskBladeTemplates(buf)
		findings = append(findings, scanHTMLMarkup(buf, displayPath, wp)...)
	case ext == ".templ":
		findings = append(findings, reviewTemplFile(buf, displayPath, wp)...)
	case ext == ".css" || ext == ".scss":
		maskCSSComments(buf, ext == ".scss")
		findings = append(findings, scanCSSDeclarations(buf, 0, buf, displayPath, wp)...)
	case ext == ".html" || ext == ".htm":
		maskYAMLFrontmatter(buf)
		maskLiquidAndTwigDirectives(buf)
		findings = append(findings, scanHTMLMarkup(buf, displayPath, wp)...)
	}

	findings = append(findings, scanWebDisallowedEmojis(buf, displayPath, wp)...)
	return findings, nil
}

func normalizeWebSource(raw []byte) string {
	text := strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
	return strings.ReplaceAll(text, "\r\n", "\n")
}

func buildWebPolicy(s school.School, contract *tokens.Contract) webPolicy {
	pol := s.Policy()
	allowedColors := make(map[string]bool)
	allowedRadii := map[string]bool{
		"0":    true,
		"none": true,
	}
	allowedShadows := map[string]bool{
		"none": true,
		"0":    true,
	}

	addColor := func(raw string) {
		c, err := parseWebColor(raw)
		if err != nil {
			return
		}
		allowedColors[c.Hex()] = true
	}

	addRadius := func(raw string) {
		for _, tok := range splitRadiusValues(raw) {
			if norm, ok := normalizeRadiusAtom(tok); ok {
				allowedRadii[norm] = true
			}
		}
	}

	for _, theme := range []string{"paper", "night", "print", ""} {
		st, err := s.Tokens(theme)
		if err != nil {
			continue
		}
		if contract != nil {
			st = tokens.Merge(st, contract)
		}
		for _, col := range []string{
			st.Paper, st.PaperSubtle, st.PaperAlt,
			st.Ink, st.InkSoft, st.Muted,
			st.Accent, st.Ochre, st.Forest,
			st.Rule, st.RuleStrong,
		} {
			addColor(col)
		}
		addRadius(st.BorderRadius)
		if normSh := normalizeShadowValue(st.BoxShadow); normSh != "" {
			allowedShadows[normSh] = true
		}
	}

	if pol.MaxBorderRadius != "" {
		addRadius(pol.MaxBorderRadius)
	}

	if contract != nil {
		for _, tok := range contract.Tokens {
			lowerPath := strings.ToLower(tok.Path)
			val := strings.TrimSpace(tok.Value)
			if val == "" {
				continue
			}
			if tok.Type == "color" || looksLikeWebColor(val) {
				addColor(val)
			}
			if strings.Contains(lowerPath, "radius") {
				addRadius(val)
			}
			if tok.Type == "shadow" || strings.Contains(lowerPath, "shadow") || strings.Contains(lowerPath, "elevation") {
				if norm := normalizeShadowValue(val); norm != "" {
					allowedShadows[norm] = true
				}
			}
		}
	}

	return webPolicy{
		schoolName:     s.Name(),
		policy:         pol,
		allowedColors:  allowedColors,
		allowedRadii:   allowedRadii,
		allowedShadows: allowedShadows,
		allowAnyShadow: pol.AllowBoxShadow && contract == nil,
	}
}

func maskBytesPreserveNewlines(buf []byte, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(buf) {
		end = len(buf)
	}
	for i := start; i < end; i++ {
		if buf[i] != '\n' {
			buf[i] = ' '
		}
	}
}

func lineNumberAt(buf []byte, offset int) int {
	if offset <= 0 {
		return 1
	}
	if offset > len(buf) {
		offset = len(buf)
	}
	return 1 + bytes.Count(buf[:offset], []byte{'\n'})
}

func maskYAMLFrontmatter(buf []byte) {
	firstNL := bytes.IndexByte(buf, '\n')
	if firstNL < 0 {
		return
	}
	if strings.TrimSpace(string(buf[:firstNL])) != "---" {
		return
	}
	pos := firstNL + 1
	for pos < len(buf) {
		nextNL := bytes.IndexByte(buf[pos:], '\n')
		lineEnd := len(buf)
		if nextNL >= 0 {
			lineEnd = pos + nextNL
		}
		trimmed := strings.TrimSpace(string(buf[pos:lineEnd]))
		if trimmed == "---" || trimmed == "..." {
			maskBytesPreserveNewlines(buf, 0, lineEnd)
			return
		}
		if nextNL < 0 {
			return
		}
		pos = lineEnd + 1
	}
}

func maskLiquidAndTwigDirectives(buf []byte) {
	n := len(buf)
	for i := 0; i < n; {
		if i+1 >= n {
			break
		}
		if buf[i] == '{' && buf[i+1] == '#' {
			end := findDelimitedClose(buf, i+2, "#}", false)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if buf[i] == '{' && buf[i+1] == '%' {
			tagEnd := findDelimitedClose(buf, i+2, "%}", true)
			if isLiquidTagNamed(buf[i+2:tagEnd], "comment") {
				closeStart, closeEnd := findLiquidEndTag(buf, tagEnd, "endcomment")
				if closeStart >= 0 {
					maskBytesPreserveNewlines(buf, i, closeEnd)
					i = closeEnd
					continue
				}
			}
			maskBytesPreserveNewlines(buf, i, tagEnd)
			i = tagEnd
			continue
		}
		if buf[i] == '{' && buf[i+1] == '{' {
			end := findDelimitedClose(buf, i+2, "}}", true)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		i++
	}
}

func findDelimitedClose(buf []byte, start int, closeDelim string, respectStrings bool) int {
	n := len(buf)
	for i := start; i < n; {
		if respectStrings && (buf[i] == '"' || buf[i] == '\'') {
			i = skipQuoted(buf, i)
			continue
		}
		if i+len(closeDelim) <= n && string(buf[i:i+len(closeDelim)]) == closeDelim {
			return i + len(closeDelim)
		}
		i++
	}
	return n
}

func isLiquidTagNamed(inner []byte, name string) bool {
	s := strings.TrimSuffix(string(inner), "%}")
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimSuffix(s, "-")
	s = strings.TrimSpace(s)
	return strings.EqualFold(s, name)
}

func findLiquidEndTag(buf []byte, from int, endName string) (int, int) {
	n := len(buf)
	for i := from; i+1 < n; i++ {
		if buf[i] == '{' && buf[i+1] == '%' {
			end := findDelimitedClose(buf, i+2, "%}", true)
			if isLiquidTagNamed(buf[i+2:end], endName) {
				return i, end
			}
			i = end - 1
		}
	}
	return -1, -1
}

var bladeStandaloneDirectives = map[string]bool{
	"else":            true,
	"stop":            true,
	"show":            true,
	"append":          true,
	"overwrite":       true,
	"csrf":            true,
	"parent":          true,
	"break":           true,
	"continue":        true,
	"empty":           true,
	"default":         true,
	"auth":            true,
	"guest":           true,
	"once":            true,
	"production":      true,
	"verbatim":        true,
	"livewirestyles":  true,
	"livewirescripts": true,
	"filamentstyles":  true,
	"filamentscripts": true,
	"inertia":         true,
	"inertiahead":     true,
	"routes":          true,
}

func maskBladeTemplates(buf []byte) {
	maskBladeCommentsPHPAndEchoes(buf)
	maskBladeDirectivesOutsideStyleAndScript(buf)
}

func maskBladeCommentsPHPAndEchoes(buf []byte) {
	n := len(buf)
	for i := 0; i < n; {
		if i+4 <= n && string(buf[i:i+4]) == "{{--" {
			end := findDelimitedClose(buf, i+4, "--}}", false)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if i+5 <= n && strings.EqualFold(string(buf[i:i+5]), "<?php") && (i+5 == n || isASCIIWhitespace(buf[i+5])) {
			end := findPHPClose(buf, i+5)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if i+3 <= n && string(buf[i:i+3]) == "<?=" {
			end := findPHPClose(buf, i+3)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if i+3 <= n && string(buf[i:i+3]) == "{!!" {
			end := findDelimitedClose(buf, i+3, "!!}", true)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if i+3 <= n && string(buf[i:i+3]) == "{{{" {
			end := findDelimitedClose(buf, i+3, "}}}", true)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if i+2 <= n && string(buf[i:i+2]) == "{{" {
			if i > 0 && buf[i-1] == '@' {
				i += 2
				continue
			}
			end := findDelimitedClose(buf, i+2, "}}", true)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if buf[i] == '@' && isDirectiveBoundaryBefore(buf, i) && i+4 <= n && strings.EqualFold(string(buf[i+1:i+4]), "php") {
			after := i + 4
			if after == n || !isASCIIAlphaNum(buf[after]) && buf[after] != '_' {
				j := skipHorizontalSpaces(buf, after)
				if j < n && buf[j] == '(' {
					end := skipParenGroup(buf, j)
					maskBytesPreserveNewlines(buf, i, end)
					i = end
					continue
				}
				if after < n && isASCIIWhitespace(buf[after]) {
					if endIdx := findBladeEndPHP(buf, after); endIdx > after {
						maskBytesPreserveNewlines(buf, i, endIdx)
						i = endIdx
						continue
					}
				}
			}
		}
		i++
	}
}

func findPHPClose(buf []byte, start int) int {
	n := len(buf)
	for i := start; i < n; {
		b := buf[i]
		if b == '"' || b == '\'' {
			i = skipQuoted(buf, i)
			continue
		}
		if b == '/' && i+1 < n && buf[i+1] == '/' {
			nl := bytes.IndexByte(buf[i+2:], '\n')
			if nl < 0 {
				return n
			}
			i = i + 2 + nl + 1
			continue
		}
		if b == '/' && i+1 < n && buf[i+1] == '*' {
			end := bytes.Index(buf[i+2:], []byte("*/"))
			if end < 0 {
				return n
			}
			i = i + 2 + end + 2
			continue
		}
		if b == '?' && i+1 < n && buf[i+1] == '>' {
			return i + 2
		}
		i++
	}
	return n
}

func findBladeEndPHP(buf []byte, start int) int {
	n := len(buf)
	for i := start; i+7 <= n; i++ {
		if buf[i] == '@' && isDirectiveBoundaryBefore(buf, i) && strings.EqualFold(string(buf[i+1:i+7]), "endphp") {
			after := i + 7
			if after == n || (!isASCIIAlphaNum(buf[after]) && buf[after] != '_') {
				return after
			}
		}
	}
	return -1
}

func maskBladeDirectivesOutsideStyleAndScript(buf []byte) {
	n := len(buf)
	for i := 0; i < n; {
		if i+4 <= n && string(buf[i:i+4]) == "<!--" {
			end := bytes.Index(buf[i+4:], []byte("-->"))
			if end < 0 {
				return
			}
			i = i + 4 + end + 3
			continue
		}
		if buf[i] == '<' && (matchHTMLTagStart(buf, i, "style") || matchHTMLTagStart(buf, i, "script")) {
			tag := "style"
			if matchHTMLTagStart(buf, i, "script") {
				tag = "script"
			}
			openEnd := findHTMLTagClose(buf, i)
			_, closeEnd := findClosingHTMLTag(buf, openEnd, tag)
			i = closeEnd
			continue
		}
		if buf[i] != '@' || !isDirectiveBoundaryBefore(buf, i) {
			i++
			continue
		}
		identStart := i + 1
		if identStart >= n || !isASCIIAlpha(buf[identStart]) {
			i++
			continue
		}
		identEnd := identStart + 1
		for identEnd < n && (isASCIIAlphaNum(buf[identEnd]) || buf[identEnd] == '_') {
			identEnd++
		}
		if identEnd < n && (buf[identEnd] == '=' || buf[identEnd] == '.' || buf[identEnd] == ':' || buf[identEnd] == '-') {
			i = identEnd
			continue
		}
		name := strings.ToLower(string(buf[identStart:identEnd]))
		j := skipHorizontalSpaces(buf, identEnd)
		if j < n && buf[j] == '(' {
			end := skipParenGroup(buf, j)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if bladeStandaloneDirectives[name] || strings.HasPrefix(name, "end") {
			maskBytesPreserveNewlines(buf, i, identEnd)
			i = identEnd
			continue
		}
		i = identEnd
	}
}

func isDirectiveBoundaryBefore(buf []byte, atIdx int) bool {
	if atIdx == 0 {
		return true
	}
	prev := buf[atIdx-1]
	return !isASCIIAlphaNum(prev) && prev != '_' && prev != '.' && prev != '-' && prev != '@'
}

func skipHorizontalSpaces(buf []byte, idx int) int {
	for idx < len(buf) && (buf[idx] == ' ' || buf[idx] == '\t') {
		idx++
	}
	return idx
}

type byteRange struct {
	start int
	end   int
}

func reviewTemplFile(buf []byte, displayPath string, wp webPolicy) []Finding {
	var cssRegions []byteRange
	maskTemplTopLevel(buf, &cssRegions)

	var findings []Finding
	for _, r := range cssRegions {
		if r.end > r.start {
			region := buf[r.start:r.end]
			maskCSSComments(region, false)
			findings = append(findings, scanCSSDeclarations(region, r.start, buf, displayPath, wp)...)
		}
	}
	findings = append(findings, scanHTMLMarkup(buf, displayPath, wp)...)
	return findings
}

func maskTemplTopLevel(buf []byte, cssRegions *[]byteRange) {
	n := len(buf)
	for i := 0; i < n; {
		if isASCIIWhitespace(buf[i]) {
			i++
			continue
		}
		if buf[i] == '/' && i+1 < n && buf[i+1] == '/' {
			end := findLineCommentEnd(buf, i)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if buf[i] == '/' && i+1 < n && buf[i+1] == '*' {
			end := findBlockCommentEnd(buf, i)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}

		if bodyStart, ok := matchTemplBlockDecl(buf, i, "templ"); ok {
			maskBytesPreserveNewlines(buf, i, bodyStart)
			i = scanTemplBody(buf, bodyStart, false, cssRegions)
			continue
		}
		if bodyStart, ok := matchTemplBlockDecl(buf, i, "css"); ok {
			blockEnd := skipCSSBraceBlock(buf, bodyStart-1)
			maskBytesPreserveNewlines(buf, i, bodyStart)
			if blockEnd > bodyStart {
				*cssRegions = append(*cssRegions, byteRange{start: bodyStart, end: blockEnd - 1})
				if buf[blockEnd-1] == '}' {
					buf[blockEnd-1] = ' '
				}
			}
			i = blockEnd
			continue
		}
		if bodyStart, ok := matchTemplBlockDecl(buf, i, "script"); ok {
			blockEnd := skipGoBraceBlock(buf, bodyStart-1)
			maskBytesPreserveNewlines(buf, i, blockEnd)
			i = blockEnd
			continue
		}

		end := scanGoTopLevelDeclEnd(buf, i)
		maskBytesPreserveNewlines(buf, i, end)
		i = end
	}
}

func matchTemplBlockDecl(buf []byte, start int, keyword string) (int, bool) {
	n := len(buf)
	if start+len(keyword) >= n || string(buf[start:start+len(keyword)]) != keyword {
		return 0, false
	}
	if start > 0 && isIdentChar(buf[start-1]) {
		return 0, false
	}
	j := start + len(keyword)
	if j >= n || !isASCIIWhitespace(buf[j]) {
		return 0, false
	}
	j = skipHorizontalSpaces(buf, j)
	if keyword == "templ" && j < n && buf[j] == '(' {
		j = skipGoParenGroup(buf, j)
		j = skipHorizontalSpaces(buf, j)
	}
	if j >= n || (!isASCIIAlpha(buf[j]) && buf[j] != '_') {
		return 0, false
	}
	for j < n && (isASCIIAlphaNum(buf[j]) || buf[j] == '_') {
		j++
	}
	j = skipHorizontalSpaces(buf, j)
	if j >= n || buf[j] != '(' {
		return 0, false
	}
	j = skipGoParenGroup(buf, j)
	for j < n {
		if isASCIIWhitespace(buf[j]) {
			j++
			continue
		}
		if buf[j] == '/' && j+1 < n && buf[j+1] == '/' {
			j = findLineCommentEnd(buf, j)
			continue
		}
		if buf[j] == '/' && j+1 < n && buf[j+1] == '*' {
			j = findBlockCommentEnd(buf, j)
			continue
		}
		if buf[j] == '{' {
			return j + 1, true
		}
		return 0, false
	}
	return 0, false
}

func scanGoTopLevelDeclEnd(buf []byte, start int) int {
	n := len(buf)
	parenDepth := 0
	braceDepth := 0
	for i := start; i < n; {
		b := buf[i]
		if next, handled := skipGoLiteralOrComment(buf, i); handled {
			i = next
			continue
		}
		switch b {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
				if braceDepth == 0 && parenDepth == 0 {
					return i + 1
				}
			}
		case ';', '\n':
			if parenDepth == 0 && braceDepth == 0 {
				return i + 1
			}
		}
		i++
	}
	return n
}

func scanTemplBody(buf []byte, start int, inSwitch bool, cssRegions *[]byteRange) int {
	n := len(buf)
	atStmtBoundary := true

	for i := start; i < n; {
		b := buf[i]
		if b == ' ' || b == '\t' || b == '\r' {
			i++
			continue
		}
		if b == '\n' {
			atStmtBoundary = true
			i++
			continue
		}
		if b == '}' {
			buf[i] = ' '
			return i + 1
		}
		if i+4 <= n && string(buf[i:i+4]) == "<!--" {
			end := bytes.Index(buf[i+4:], []byte("-->"))
			if end < 0 {
				maskBytesPreserveNewlines(buf, i, n)
				return n
			}
			maskEnd := i + 4 + end + 3
			maskBytesPreserveNewlines(buf, i, maskEnd)
			i = maskEnd
			atStmtBoundary = false
			continue
		}
		if b == '<' {
			if matchHTMLTagStart(buf, i, "script") {
				openEnd := findHTMLTagClose(buf, i)
				closeStart, closeEnd := findClosingHTMLTag(buf, openEnd, "script")
				maskBytesPreserveNewlines(buf, openEnd, closeStart)
				i = closeEnd
				atStmtBoundary = false
				continue
			}
			if matchHTMLTagStart(buf, i, "style") {
				openEnd := findHTMLTagClose(buf, i)
				_, closeEnd := findClosingHTMLTag(buf, openEnd, "style")
				i = closeEnd
				atStmtBoundary = false
				continue
			}
			if i+1 < n && (isASCIIAlpha(buf[i+1]) || buf[i+1] == '/') {
				i = maskTemplHTMLTagExprs(buf, i)
				atStmtBoundary = false
				continue
			}
		}
		if b == '{' {
			end := skipGoBraceBlock(buf, i)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			atStmtBoundary = false
			continue
		}
		if b == '@' && isDirectiveBoundaryBefore(buf, i) {
			if callEnd, hasBody, ok := matchTemplComponentCall(buf, i); ok {
				maskBytesPreserveNewlines(buf, i, callEnd)
				if hasBody {
					i = scanTemplBody(buf, callEnd, false, cssRegions)
				} else {
					i = callEnd
				}
				atStmtBoundary = true
				continue
			}
		}
		if atStmtBoundary {
			if hdrEnd, isSwitch, ok := matchTemplControlHeader(buf, i); ok {
				maskBytesPreserveNewlines(buf, i, hdrEnd)
				i = scanTemplBody(buf, hdrEnd, isSwitch, cssRegions)
				atStmtBoundary = true
				continue
			}
			if inSwitch {
				if caseEnd, ok := matchTemplSwitchCaseHeader(buf, i); ok {
					maskBytesPreserveNewlines(buf, i, caseEnd)
					i = caseEnd
					atStmtBoundary = true
					continue
				}
			}
		}
		atStmtBoundary = false
		i++
	}
	return n
}

func maskTemplHTMLTagExprs(buf []byte, tagStart int) int {
	n := len(buf)
	for i := tagStart + 1; i < n; {
		b := buf[i]
		if b == '"' || b == '\'' {
			i = skipHTMLQuoted(buf, i)
			continue
		}
		if b == '{' {
			end := skipGoBraceBlock(buf, i)
			maskBytesPreserveNewlines(buf, i, end)
			i = end
			continue
		}
		if b == '>' {
			return i + 1
		}
		i++
	}
	return n
}

func matchTemplComponentCall(buf []byte, atIdx int) (end int, hasBody bool, ok bool) {
	n := len(buf)
	i := atIdx + 1
	if i >= n || (!isASCIIAlpha(buf[i]) && buf[i] != '_') {
		return 0, false, false
	}
	for i < n && (isASCIIAlphaNum(buf[i]) || buf[i] == '_' || buf[i] == '.') {
		i++
	}
	i = skipHorizontalSpaces(buf, i)
	if i >= n || buf[i] != '(' {
		return 0, false, false
	}
	i = skipGoParenGroup(buf, i)
	j := skipHorizontalSpaces(buf, i)
	if j < n && buf[j] == '{' {
		return j + 1, true, true
	}
	return i, false, true
}

func matchTemplControlHeader(buf []byte, start int) (hdrEnd int, isSwitch bool, ok bool) {
	n := len(buf)
	var kw string
	for _, cand := range []string{"else", "if", "for", "switch"} {
		if start+len(cand) <= n && string(buf[start:start+len(cand)]) == cand {
			after := start + len(cand)
			if after < n && !isASCIIAlphaNum(buf[after]) && buf[after] != '_' {
				kw = cand
				break
			}
		}
	}
	if kw == "" {
		return 0, false, false
	}

	parenDepth := 0
	for i := start + len(kw); i < n; {
		b := buf[i]
		if next, handled := skipGoLiteralOrComment(buf, i); handled {
			if b == '/' && i+1 < n && buf[i+1] == '/' {
				return 0, false, false
			}
			i = next
			continue
		}
		switch b {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '\n':
			if parenDepth == 0 {
				return 0, false, false
			}
		case '{':
			if parenDepth == 0 {
				afterBrace := skipHorizontalSpaces(buf, i+1)
				if afterBrace == n || buf[afterBrace] == '\n' || buf[afterBrace] == '\r' ||
					(afterBrace+1 < n && buf[afterBrace] == '/' && buf[afterBrace+1] == '/') ||
					buf[afterBrace] == '<' {
					return i + 1, kw == "switch", true
				}
				return 0, false, false
			}
		}
		i++
	}
	return 0, false, false
}

func matchTemplSwitchCaseHeader(buf []byte, start int) (int, bool) {
	n := len(buf)
	if start+8 <= n && string(buf[start:start+7]) == "default" && !isASCIIAlphaNum(buf[start+7]) && buf[start+7] != '_' {
		j := skipHorizontalSpaces(buf, start+7)
		if j < n && buf[j] == ':' {
			return j + 1, true
		}
		return 0, false
	}
	if start+5 <= n && string(buf[start:start+4]) == "case" && isASCIIWhitespace(buf[start+4]) {
		parenDepth := 0
		for i := start + 5; i < n; {
			b := buf[i]
			if next, handled := skipGoLiteralOrComment(buf, i); handled {
				if b == '/' && i+1 < n && buf[i+1] == '/' {
					return 0, false
				}
				i = next
				continue
			}
			switch b {
			case '(':
				parenDepth++
			case ')':
				if parenDepth > 0 {
					parenDepth--
				}
			case '\n':
				return 0, false
			case ':':
				if parenDepth == 0 {
					return i + 1, true
				}
			}
			i++
		}
	}
	return 0, false
}

func skipGoLiteralOrComment(buf []byte, idx int) (int, bool) {
	n := len(buf)
	b := buf[idx]
	switch b {
	case '"', '\'':
		return skipQuoted(buf, idx), true
	case '`':
		for i := idx + 1; i < n; i++ {
			if buf[i] == '`' {
				return i + 1, true
			}
		}
		return n, true
	case '/':
		if idx+1 < n && buf[idx+1] == '/' {
			return findLineCommentEnd(buf, idx), true
		}
		if idx+1 < n && buf[idx+1] == '*' {
			return findBlockCommentEnd(buf, idx), true
		}
	}
	return idx, false
}

func findLineCommentEnd(buf []byte, start int) int {
	nl := bytes.IndexByte(buf[start:], '\n')
	if nl < 0 {
		return len(buf)
	}
	return start + nl
}

func findBlockCommentEnd(buf []byte, start int) int {
	end := bytes.Index(buf[start+2:], []byte("*/"))
	if end < 0 {
		return len(buf)
	}
	return start + 2 + end + 2
}

func skipGoParenGroup(buf []byte, openIdx int) int {
	return skipGoDelimGroup(buf, openIdx, '(', ')')
}

func skipGoBraceBlock(buf []byte, openIdx int) int {
	return skipGoDelimGroup(buf, openIdx, '{', '}')
}

func skipGoDelimGroup(buf []byte, openIdx int, openChar, closeChar byte) int {
	n := len(buf)
	if openIdx >= n || buf[openIdx] != openChar {
		return openIdx
	}
	depth := 1
	for i := openIdx + 1; i < n; {
		if next, handled := skipGoLiteralOrComment(buf, i); handled {
			i = next
			continue
		}
		switch buf[i] {
		case openChar:
			depth++
		case closeChar:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return n
}

func skipCSSBraceBlock(buf []byte, openIdx int) int {
	n := len(buf)
	if openIdx >= n || buf[openIdx] != '{' {
		return openIdx
	}
	depth := 1
	for i := openIdx + 1; i < n; {
		b := buf[i]
		if b == '"' || b == '\'' {
			i = skipQuoted(buf, i)
			continue
		}
		if b == '/' && i+1 < n && buf[i+1] == '*' {
			i = findBlockCommentEnd(buf, i)
			continue
		}
		switch b {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return n
}

func scanHTMLMarkup(buf []byte, displayPath string, wp webPolicy) []Finding {
	var findings []Finding
	n := len(buf)

	for i := 0; i < n; {
		if i+4 <= n && string(buf[i:i+4]) == "<!--" {
			end := bytes.Index(buf[i+4:], []byte("-->"))
			if end < 0 {
				maskBytesPreserveNewlines(buf, i, n)
				return findings
			}
			maskEnd := i + 4 + end + 3
			maskBytesPreserveNewlines(buf, i, maskEnd)
			i = maskEnd
			continue
		}

		if buf[i] != '<' || i+1 >= n {
			i++
			continue
		}

		if matchHTMLTagStart(buf, i, "script") {
			openEnd := findHTMLTagClose(buf, i)
			closeStart, closeEnd := findClosingHTMLTag(buf, openEnd, "script")
			maskBytesPreserveNewlines(buf, openEnd, closeStart)
			i = closeEnd
			continue
		}

		if matchHTMLTagStart(buf, i, "style") {
			openEnd := findHTMLTagClose(buf, i)
			closeStart, closeEnd := findClosingHTMLTag(buf, openEnd, "style")
			if closeStart > openEnd {
				styleBody := buf[openEnd:closeStart]
				allowLineComments := strings.Contains(strings.ToLower(string(buf[i:openEnd])), "scss")
				maskCSSComments(styleBody, allowLineComments)
				findings = append(findings, scanCSSDeclarations(styleBody, openEnd, buf, displayPath, wp)...)
			}
			i = closeEnd
			continue
		}

		if isASCIIAlpha(buf[i+1]) {
			tagEnd := findHTMLTagClose(buf, i)
			findings = append(findings, scanHTMLTagAttributes(buf, i, tagEnd, displayPath, wp)...)
			i = tagEnd
			continue
		}

		i++
	}

	return findings
}

func matchHTMLTagStart(buf []byte, idx int, tagName string) bool {
	end := idx + 1 + len(tagName)
	if end > len(buf) {
		return false
	}
	if !strings.EqualFold(string(buf[idx+1:end]), tagName) {
		return false
	}
	if end == len(buf) {
		return true
	}
	next := buf[end]
	return next == '>' || next == '/' || isASCIIWhitespace(next)
}

func findHTMLTagClose(buf []byte, start int) int {
	n := len(buf)
	for i := start + 1; i < n; i++ {
		if buf[i] == '"' || buf[i] == '\'' {
			i = skipHTMLQuoted(buf, i) - 1
			continue
		}
		if buf[i] == '>' {
			return i + 1
		}
	}
	return n
}

func skipHTMLQuoted(buf []byte, start int) int {
	quote := buf[start]
	for i := start + 1; i < len(buf); i++ {
		if buf[i] == quote {
			return i + 1
		}
	}
	return len(buf)
}

func findClosingHTMLTag(buf []byte, from int, tagName string) (int, int) {
	n := len(buf)
	for i := from; i < n; i++ {
		if buf[i] == '<' && i+2+len(tagName) <= n && buf[i+1] == '/' &&
			strings.EqualFold(string(buf[i+2:i+2+len(tagName)]), tagName) {
			after := i + 2 + len(tagName)
			if after < n && (buf[after] == '>' || isASCIIWhitespace(buf[after])) {
				return i, findHTMLTagClose(buf, i)
			}
		}
	}
	return n, n
}

func scanHTMLTagAttributes(fullBuf []byte, tagStart, tagEnd int, displayPath string, wp webPolicy) []Finding {
	var findings []Finding
	i := tagStart + 1
	for i < tagEnd && !isASCIIWhitespace(fullBuf[i]) && fullBuf[i] != '>' && fullBuf[i] != '/' {
		i++
	}

	for i < tagEnd {
		for i < tagEnd && (isASCIIWhitespace(fullBuf[i]) || fullBuf[i] == '/' || fullBuf[i] == '>') {
			i++
		}
		if i >= tagEnd {
			break
		}

		nameStart := i
		for i < tagEnd && !isASCIIWhitespace(fullBuf[i]) && fullBuf[i] != '=' && fullBuf[i] != '>' && fullBuf[i] != '/' {
			i++
		}
		attrName := strings.ToLower(string(fullBuf[nameStart:i]))

		for i < tagEnd && isASCIIWhitespace(fullBuf[i]) {
			i++
		}
		if i >= tagEnd || fullBuf[i] != '=' {
			continue
		}
		i++ // skip '='
		for i < tagEnd && isASCIIWhitespace(fullBuf[i]) {
			i++
		}
		if i >= tagEnd {
			break
		}

		var valStart, valEnd int
		if fullBuf[i] == '"' || fullBuf[i] == '\'' {
			quote := fullBuf[i]
			valStart = i + 1
			closePos := valStart
			for closePos < tagEnd && fullBuf[closePos] != quote {
				closePos++
			}
			valEnd = closePos
			i = closePos + 1
		} else {
			valStart = i
			for i < tagEnd && !isASCIIWhitespace(fullBuf[i]) && fullBuf[i] != '>' {
				i++
			}
			valEnd = i
		}

		if valEnd <= valStart {
			continue
		}
		attrVal := fullBuf[valStart:valEnd]

		switch {
		case attrName == "style":
			findings = append(findings, scanCSSDeclarations(attrVal, valStart, fullBuf, displayPath, wp)...)
		case attrName == "class" || attrName == "classname":
			findings = append(findings, scanTailwindArbitraryColors(attrVal, valStart, fullBuf, displayPath, wp)...)
		case isHTMLColorAttr(attrName):
			findings = append(findings, scanColorLiterals(attrVal, valStart, fullBuf, displayPath, wp)...)
		}
	}

	return findings
}

func isHTMLColorAttr(name string) bool {
	switch name {
	case "fill", "stroke", "stop-color", "flood-color", "lighting-color", "bgcolor", "color":
		return true
	default:
		return false
	}
}

func scanTailwindArbitraryColors(valBytes []byte, valOffset int, fullBuf []byte, displayPath string, wp webPolicy) []Finding {
	var findings []Finding
	n := len(valBytes)
	for i := 0; i < n; i++ {
		if valBytes[i] != '[' {
			continue
		}
		end := findMatchingBracket(valBytes, i)
		if end <= i+1 {
			continue
		}
		findings = append(findings, scanColorLiterals(valBytes[i+1:end], valOffset+i+1, fullBuf, displayPath, wp)...)
		i = end
	}
	return findings
}

func findMatchingBracket(buf []byte, openIdx int) int {
	depth := 1
	for i := openIdx + 1; i < len(buf); i++ {
		switch buf[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(buf) - 1
}

func maskCSSComments(buf []byte, allowLineComments bool) {
	n := len(buf)
	for i := 0; i < n; {
		b := buf[i]
		if b == '"' || b == '\'' {
			i = skipQuoted(buf, i)
			continue
		}
		if allowLineComments && hasFoldPrefixAt(buf, i, "url(") && (i == 0 || !isIdentChar(buf[i-1])) {
			i = skipParenGroup(buf, i+3)
			continue
		}
		if b == '/' && i+1 < n && buf[i+1] == '*' {
			end := bytes.Index(buf[i+2:], []byte("*/"))
			if end < 0 {
				maskBytesPreserveNewlines(buf, i, n)
				return
			}
			maskEnd := i + 2 + end + 2
			maskBytesPreserveNewlines(buf, i, maskEnd)
			i = maskEnd
			continue
		}
		if allowLineComments && b == '/' && i+1 < n && buf[i+1] == '/' {
			end := bytes.IndexByte(buf[i+2:], '\n')
			if end < 0 {
				maskBytesPreserveNewlines(buf, i, n)
				return
			}
			maskEnd := i + 2 + end
			maskBytesPreserveNewlines(buf, i, maskEnd)
			i = maskEnd
			continue
		}
		i++
	}
}

func skipQuoted(buf []byte, start int) int {
	quote := buf[start]
	for i := start + 1; i < len(buf); i++ {
		if buf[i] == '\\' {
			i++
			continue
		}
		if buf[i] == quote {
			return i + 1
		}
	}
	return len(buf)
}

func skipParenGroup(buf []byte, openIdx int) int {
	if openIdx >= len(buf) || buf[openIdx] != '(' {
		return openIdx
	}
	depth := 1
	for i := openIdx + 1; i < len(buf); i++ {
		switch buf[i] {
		case '"', '\'':
			i = skipQuoted(buf, i) - 1
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(buf)
}

func scanCSSDeclarations(buf []byte, baseOffset int, fullBuf []byte, displayPath string, wp webPolicy) []Finding {
	var findings []Finding
	segStart := 0
	parenDepth := 0
	n := len(buf)

	for i := 0; i <= n; i++ {
		if i < n {
			b := buf[i]
			if b == '"' || b == '\'' {
				i = skipQuoted(buf, i) - 1
				continue
			}
			switch b {
			case '(':
				parenDepth++
				continue
			case ')':
				if parenDepth > 0 {
					parenDepth--
				}
				continue
			case '{':
				if parenDepth == 0 {
					// Segment before '{' is a selector or at-rule header, never a declaration.
					segStart = i + 1
				}
				continue
			case ';', '}':
				if parenDepth > 0 {
					continue
				}
			default:
				continue
			}
		}

		seg := buf[segStart:i]
		segOffset := baseOffset + segStart
		segStart = i + 1

		colon := findTopLevelColon(seg)
		if colon < 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(string(seg[:colon])))
		if !isCSSPropertyName(prop) {
			continue
		}

		valBytes := seg[colon+1:]
		valOffset := segOffset + colon + 1
		valTrimmed := strings.TrimSpace(string(valBytes))
		if valTrimmed == "" {
			continue
		}
		valTrimmed = stripImportant(valTrimmed)

		if isBorderRadiusProp(prop) {
			if f, ok := checkBorderRadiusDecl(valBytes, valOffset, fullBuf, valTrimmed, displayPath, wp); ok {
				findings = append(findings, f)
			}
			continue
		}

		if isBoxShadowProp(prop) {
			if f, ok := checkBoxShadowDecl(valBytes, valOffset, fullBuf, valTrimmed, displayPath, wp); ok {
				findings = append(findings, f)
				continue
			}
			if !wp.allowAnyShadow && !strings.Contains(strings.ToLower(valTrimmed), "var(") {
				continue
			}
		}

		findings = append(findings, scanColorLiterals(valBytes, valOffset, fullBuf, displayPath, wp)...)
	}

	return findings
}

func findTopLevelColon(seg []byte) int {
	parenDepth := 0
	for i := 0; i < len(seg); i++ {
		b := seg[i]
		if b == '"' || b == '\'' {
			i = skipQuoted(seg, i) - 1
			continue
		}
		switch b {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ':':
			if parenDepth == 0 {
				return i
			}
		}
	}
	return -1
}

func isCSSPropertyName(prop string) bool {
	if prop == "" {
		return false
	}
	start := 0
	if strings.HasPrefix(prop, "--") {
		start = 2
	} else if strings.HasPrefix(prop, "$") {
		start = 1
	} else if !isASCIIAlpha(prop[0]) && prop[0] != '-' && prop[0] != '_' {
		return false
	}
	if start >= len(prop) {
		return false
	}
	for i := start; i < len(prop); i++ {
		c := prop[i]
		if !isASCIIAlphaNum(c) && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func stripImportant(val string) string {
	lower := strings.ToLower(val)
	if idx := strings.LastIndex(lower, "!important"); idx >= 0 && strings.TrimSpace(lower[idx+len("!important"):]) == "" {
		return strings.TrimSpace(val[:idx])
	}
	return val
}

func isBorderRadiusProp(prop string) bool {
	if prop == "border-radius" {
		return true
	}
	return strings.HasPrefix(prop, "border-") && strings.HasSuffix(prop, "-radius")
}

func isBoxShadowProp(prop string) bool {
	return prop == "box-shadow" || prop == "-webkit-box-shadow" || prop == "-moz-box-shadow"
}

func checkBorderRadiusDecl(valBytes []byte, valOffset int, fullBuf []byte, valTrimmed, displayPath string, wp webPolicy) (Finding, bool) {
	if valTrimmed == "" || isDynamicOrKeywordCSSValue(valTrimmed) {
		return Finding{}, false
	}
	for _, atom := range splitRadiusValues(valTrimmed) {
		if atom == "/" || isDynamicOrKeywordCSSValue(atom) || isOrphanUnitFromTemplate(atom) {
			continue
		}
		norm, ok := normalizeRadiusAtom(atom)
		if !ok {
			continue
		}
		if !wp.allowedRadii[norm] {
			line := firstNonSpaceLine(valBytes, valOffset, fullBuf)
			return Finding{
				RuleID:   RuleWebOffContractRadius,
				File:     displayPath,
				Line:     line,
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("off-contract border-radius %q violates flat geometry policy", valTrimmed),
				Hint:     "Set border-radius to 0px or use a radius token defined in your Token Contract.",
			}, true
		}
	}
	return Finding{}, false
}

func splitRadiusValues(val string) []string {
	cleaned := strings.ReplaceAll(val, "/", " ")
	return strings.Fields(cleaned)
}

func normalizeRadiusAtom(atom string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(atom))
	if lower == "" {
		return "", false
	}
	if lower == "none" || lower == "0" {
		return "0", true
	}
	for _, unit := range []string{"px", "rem", "em", "pt", "%"} {
		if strings.HasSuffix(lower, unit) {
			numPart := strings.TrimSuffix(lower, unit)
			v, err := strconv.ParseFloat(numPart, 64)
			if err != nil {
				return "", false
			}
			if v == 0 {
				return "0", true
			}
			return strconv.FormatFloat(v, 'f', -1, 64) + unit, true
		}
	}
	if v, err := strconv.ParseFloat(lower, 64); err == nil {
		if v == 0 {
			return "0", true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	}
	return lower, true
}

func isOrphanUnitFromTemplate(atom string) bool {
	switch strings.ToLower(strings.TrimSpace(atom)) {
	case "px", "rem", "em", "pt", "%", "vh", "vw":
		return true
	default:
		return false
	}
}

func checkBoxShadowDecl(valBytes []byte, valOffset int, fullBuf []byte, valTrimmed, displayPath string, wp webPolicy) (Finding, bool) {
	if wp.allowAnyShadow || valTrimmed == "" || isDynamicOrKeywordCSSValue(valTrimmed) {
		return Finding{}, false
	}
	norm := normalizeShadowValue(valTrimmed)
	if norm == "" || wp.allowedShadows[norm] {
		return Finding{}, false
	}
	line := firstNonSpaceLine(valBytes, valOffset, fullBuf)
	return Finding{
		RuleID:   RuleWebOffContractShadow,
		File:     displayPath,
		Line:     line,
		Severity: SeverityWarn,
		Message:  fmt.Sprintf("off-contract box-shadow %q violates flat elevation policy", valTrimmed),
		Hint:     "Set box-shadow to none and convey hierarchy with hairline rules or subtle surface contrast.",
	}, true
}

func normalizeShadowValue(val string) string {
	trimmed := strings.TrimSpace(strings.ToLower(val))
	if trimmed == "" {
		return ""
	}
	if trimmed == "none" || trimmed == "0" || trimmed == "0px" {
		return "none"
	}
	fields := strings.Fields(trimmed)
	for idx, f := range fields {
		switch f {
		case "0px", "0rem", "0em":
			fields[idx] = "0"
		}
	}
	joined := strings.Join(fields, " ")
	joined = strings.ReplaceAll(joined, ", ", ",")
	joined = strings.ReplaceAll(joined, " ,", ",")
	joined = strings.ReplaceAll(joined, "( ", "(")
	joined = strings.ReplaceAll(joined, " )", ")")
	return joined
}

func isDynamicOrKeywordCSSValue(val string) bool {
	lower := strings.ToLower(strings.TrimSpace(val))
	switch lower {
	case "inherit", "initial", "unset", "revert", "revert-layer", "auto", "transparent", "currentcolor":
		return true
	}
	return strings.Contains(lower, "var(") || strings.HasPrefix(lower, "$")
}

func firstNonSpaceLine(slice []byte, sliceOffset int, fullBuf []byte) int {
	for i, b := range slice {
		if !isASCIIWhitespace(b) {
			return lineNumberAt(fullBuf, sliceOffset+i)
		}
	}
	return lineNumberAt(fullBuf, sliceOffset)
}

func scanColorLiterals(valBytes []byte, valOffset int, fullBuf []byte, displayPath string, wp webPolicy) []Finding {
	var findings []Finding
	n := len(valBytes)

	for i := 0; i < n; {
		b := valBytes[i]
		if b == '"' || b == '\'' {
			i = skipQuoted(valBytes, i)
			continue
		}
		if hasFoldPrefixAt(valBytes, i, "url(") && (i == 0 || !isIdentChar(valBytes[i-1])) {
			i = skipParenGroup(valBytes, i+3)
			continue
		}

		// Functional colors: rgb(...), rgba(...), hsl(...), hsla(...)
		if fnLen := matchColorFuncPrefix(valBytes, i); fnLen > 0 {
			openIdx := i + fnLen - 1
			endIdx := skipParenGroup(valBytes, openIdx)
			lit := string(valBytes[i:endIdx])
			if c, err := parseWebColor(lit); err == nil {
				if !wp.allowedColors[c.Hex()] {
					findings = append(findings, newHardcodedColorFinding(displayPath, lineNumberAt(fullBuf, valOffset+i), lit))
				}
			}
			i = endIdx
			continue
		}

		// Hex colors: #RGB, #RGBA, #RRGGBB, #RRGGBBAA
		if b == '#' && (i == 0 || (valBytes[i-1] != '&' && !isIdentChar(valBytes[i-1]))) {
			j := i + 1
			for j < n && isHexDigit(valBytes[j]) {
				j++
			}
			hexLen := j - (i + 1)
			validEnd := j == n || !isIdentChar(valBytes[j])
			if validEnd && (hexLen == 3 || hexLen == 4 || hexLen == 6 || hexLen == 8) {
				lit := string(valBytes[i:j])
				if c, err := parseWebColor(lit); err == nil {
					if !wp.allowedColors[c.Hex()] {
						findings = append(findings, newHardcodedColorFinding(displayPath, lineNumberAt(fullBuf, valOffset+i), lit))
					}
				}
			}
			i = j
			continue
		}

		i++
	}

	return findings
}

func newHardcodedColorFinding(displayPath string, line int, lit string) Finding {
	return Finding{
		RuleID:   RuleWebHardcodedColor,
		File:     displayPath,
		Line:     line,
		Severity: SeverityWarn,
		Message:  fmt.Sprintf("hardcoded color literal %s is outside the active Token Contract", lit),
		Hint:     "Replace the literal color with a Token Contract CSS custom property (var(--df-*)) or define the token in --tokens.",
	}
}

func matchColorFuncPrefix(buf []byte, idx int) int {
	if idx > 0 && isIdentChar(buf[idx-1]) {
		return 0
	}
	for _, fn := range []string{"rgba(", "rgb(", "hsla(", "hsl("} {
		if hasFoldPrefixAt(buf, idx, fn) {
			return len(fn)
		}
	}
	return 0
}

func hasFoldPrefixAt(buf []byte, idx int, prefix string) bool {
	if idx+len(prefix) > len(buf) {
		return false
	}
	return strings.EqualFold(string(buf[idx:idx+len(prefix)]), prefix)
}

func looksLikeWebColor(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lower, "#") ||
		strings.HasPrefix(lower, "rgb(") ||
		strings.HasPrefix(lower, "rgba(") ||
		strings.HasPrefix(lower, "hsl(") ||
		strings.HasPrefix(lower, "hsla(")
}

func parseWebColor(raw string) (tokens.RGBA, error) {
	if c, err := tokens.ParseColor(raw); err == nil {
		return c, nil
	}
	s := strings.TrimSpace(raw)
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "hsla(") && strings.HasSuffix(lower, ")") {
		return parseHSLFunc(s[5 : len(s)-1])
	}
	if strings.HasPrefix(lower, "hsl(") && strings.HasSuffix(lower, ")") {
		return parseHSLFunc(s[4 : len(s)-1])
	}
	return tokens.RGBA{}, fmt.Errorf("unsupported web color %q", raw)
}

func parseHSLFunc(inner string) (tokens.RGBA, error) {
	var parts []string
	var alphaStr string

	if slashIdx := strings.IndexByte(inner, '/'); slashIdx >= 0 {
		alphaStr = strings.TrimSpace(inner[slashIdx+1:])
		parts = splitChannelList(inner[:slashIdx])
	} else {
		parts = splitChannelList(inner)
		if len(parts) == 4 {
			alphaStr = parts[3]
			parts = parts[:3]
		}
	}
	if len(parts) != 3 {
		return tokens.RGBA{}, fmt.Errorf("expected 3 HSL channels in %q", inner)
	}

	hStr := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parts[0])), "deg")
	h, err := strconv.ParseFloat(hStr, 64)
	if err != nil {
		return tokens.RGBA{}, fmt.Errorf("invalid hue %q", parts[0])
	}
	sPct, err := parsePercentFloat(parts[1])
	if err != nil {
		return tokens.RGBA{}, err
	}
	lPct, err := parsePercentFloat(parts[2])
	if err != nil {
		return tokens.RGBA{}, err
	}

	a := 1.0
	if alphaStr != "" {
		a, err = parseWebAlpha(alphaStr)
		if err != nil {
			return tokens.RGBA{}, err
		}
	}

	r, g, b := hslToRGB(h, sPct/100.0, lPct/100.0)
	return tokens.RGBA{R: r, G: g, B: b, A: a}, nil
}

func splitChannelList(s string) []string {
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

func parsePercentFloat(s string) (float64, error) {
	trimmed := strings.TrimSpace(s)
	if !strings.HasSuffix(trimmed, "%") {
		return 0, fmt.Errorf("expected percentage in %q", s)
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid percentage %q", s)
	}
	if v < 0 {
		return 0, nil
	}
	if v > 100 {
		return 100, nil
	}
	return v, nil
}

func parseWebAlpha(s string) (float64, error) {
	trimmed := strings.TrimSpace(s)
	if strings.HasSuffix(trimmed, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid alpha %q", s)
		}
		return clampUnit(v / 100.0), nil
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid alpha %q", s)
	}
	return clampUnit(v), nil
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func hslToRGB(hDeg, s, l float64) (uint8, uint8, uint8) {
	h := math.Mod(hDeg, 360.0)
	if h < 0 {
		h += 360.0
	}
	c := (1.0 - math.Abs(2.0*l-1.0)) * s
	x := c * (1.0 - math.Abs(math.Mod(h/60.0, 2.0)-1.0))
	m := l - c/2.0

	var rp, gp, bp float64
	switch {
	case h < 60:
		rp, gp, bp = c, x, 0
	case h < 120:
		rp, gp, bp = x, c, 0
	case h < 180:
		rp, gp, bp = 0, c, x
	case h < 240:
		rp, gp, bp = 0, x, c
	case h < 300:
		rp, gp, bp = x, 0, c
	default:
		rp, gp, bp = c, 0, x
	}

	toByte := func(v float64) uint8 {
		return uint8(math.Round(clampUnit(v+m) * 255.0))
	}
	return toByte(rp), toByte(gp), toByte(bp)
}

func scanWebDisallowedEmojis(buf []byte, displayPath string, wp webPolicy) []Finding {
	if !wp.policy.DisallowEmojis {
		return nil
	}
	var findings []Finding
	lines := strings.Split(string(buf), "\n")
	for idx, lineText := range lines {
		for _, emoji := range extractEmojis(lineText) {
			findings = append(findings, Finding{
				RuleID:   RuleEmojiDisallowed,
				File:     displayPath,
				Line:     idx + 1,
				Severity: SeverityError,
				Message:  fmt.Sprintf("emoji %s is disallowed under the %s School", emoji, wp.schoolName),
				Hint:     fmt.Sprintf("Remove %s and replace it with plain typography or domain prose.", emoji),
			})
		}
	}
	return findings
}

func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isASCIIAlphaNum(b byte) bool {
	return isASCIIAlpha(b) || (b >= '0' && b <= '9')
}

func isIdentChar(b byte) bool {
	return isASCIIAlphaNum(b) || b == '_' || b == '-'
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
