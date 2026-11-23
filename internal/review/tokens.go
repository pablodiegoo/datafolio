package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/doc"
	"github.com/pablodiegoo/datafolio/internal/school"
	"github.com/pablodiegoo/datafolio/internal/tokens"
	"gopkg.in/yaml.v3"
)

const RuleTokenContrastAA = "token-contrast-aa"

func reviewTokenContract(absPath, displayPath string, contract *tokens.Contract, policy school.ReviewPolicy) ([]Finding, error) {
	results, err := tokens.CheckContrast(contract, policy.MinContrastNormal, policy.MinContrastLarge)
	if err != nil {
		return nil, usageErrorf("contrast check %s: %v", displayPath, err)
	}

	var raw []byte
	for _, r := range results {
		if r.Pass {
			continue
		}
		if raw == nil {
			raw, _ = os.ReadFile(absPath)
		}
		break
	}

	var findings []Finding
	for _, r := range results {
		if r.Pass {
			continue
		}
		line := locateTokenLine(raw, absPath, r.ForegroundToken)
		msg := fmt.Sprintf(
			"WCAG AA contrast failure [%s]: %s (%s) on %s (%s) has ratio %.2f:1, below required %.1f:1 for %s",
			r.Polarity,
			r.ForegroundToken,
			r.ForegroundColor,
			r.BackgroundToken,
			r.BackgroundColor,
			r.Ratio,
			r.Required,
			r.Level,
		)
		hint := fmt.Sprintf(
			"Adjust %s or %s so their WCAG 2.1 contrast ratio is at least %.1f:1 (%s).",
			r.ForegroundToken,
			r.BackgroundToken,
			r.Required,
			r.Level,
		)
		findings = append(findings, Finding{
			RuleID:   RuleTokenContrastAA,
			File:     displayPath,
			Line:     line,
			Severity: SeverityError,
			Message:  msg,
			Hint:     hint,
		})
	}
	return findings, nil
}

func locateTokenLine(raw []byte, filename, tokenPath string) int {
	if len(raw) == 0 || strings.TrimSpace(tokenPath) == "" {
		return 1
	}
	segments := strings.Split(tokenPath, ".")
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".yaml", ".yml":
		if line := findYAMLTokenLine(raw, segments); line > 0 {
			return line
		}
	case ".md", ".markdown":
		if line := findMarkdownTokenLine(raw, segments); line > 0 {
			return line
		}
	case ".json":
		if line := findJSONTokenLine(raw, segments); line > 0 {
			return line
		}
	}
	return 1
}

func findYAMLTokenLine(raw []byte, segments []string) int {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil || len(root.Content) == 0 {
		return 0
	}
	top := root.Content[0]
	if line := walkYAMLMapping(top, segments); line > 0 {
		return line
	}
	if dt := findYAMLChild(top, "design_tokens"); dt != nil {
		if line := walkYAMLMapping(dt, segments); line > 0 {
			return line
		}
	}
	return 0
}

func walkYAMLMapping(node *yaml.Node, segments []string) int {
	cur := node
	for i, seg := range segments {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return 0
		}
		var matchedKey, matchedVal *yaml.Node
		for j := 0; j+1 < len(cur.Content); j += 2 {
			k := cur.Content[j]
			v := cur.Content[j+1]
			if k.Value == seg {
				matchedKey = k
				matchedVal = v
				break
			}
		}
		if matchedKey == nil {
			return 0
		}
		if i == len(segments)-1 {
			return matchedKey.Line
		}
		cur = matchedVal
	}
	return 0
}

func findYAMLChild(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for j := 0; j+1 < len(node.Content); j += 2 {
		if node.Content[j].Value == key {
			return node.Content[j+1]
		}
	}
	return nil
}

func findMarkdownTokenLine(raw []byte, segments []string) int {
	fmBytes, _, bodyOffset, err := doc.SplitFrontmatter(raw)
	if err == nil && len(fmBytes) > 0 {
		if relLine := findYAMLTokenLine(fmBytes, segments); relLine > 0 {
			return 1 + relLine
		}
	}

	text := strings.ReplaceAll(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	startIdx := 0
	if bodyOffset > 1 && bodyOffset-1 <= len(lines) {
		startIdx = bodyOffset - 1
	}

	inFence := false
	fenceChar := byte(0)
	fenceLen := 0
	isYAML := false
	blockStartLine := 0
	var buf []string

	for i := startIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !inFence {
			if fc, fl, lang, ok := parseFenceStart(trimmed); ok {
				inFence = true
				fenceChar = fc
				fenceLen = fl
				isYAML = lang == "yaml" || lang == "yml"
				blockStartLine = i + 2 // 1-based line of first content line inside fence
				buf = buf[:0]
			}
			continue
		}
		if isFenceClose(trimmed, fenceChar, fenceLen) {
			if isYAML && len(buf) > 0 {
				if relLine := findYAMLTokenLine([]byte(strings.Join(buf, "\n")), segments); relLine > 0 {
					return blockStartLine + relLine - 1
				}
			}
			inFence = false
			isYAML = false
			buf = buf[:0]
			continue
		}
		if isYAML {
			buf = append(buf, lines[i])
		}
	}
	return 0
}

func parseFenceStart(trimmed string) (byte, int, string, bool) {
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, "", false
	}
	fc := trimmed[0]
	n := 0
	for n < len(trimmed) && trimmed[n] == fc {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	info := strings.ToLower(strings.TrimSpace(trimmed[n:]))
	fields := strings.Fields(info)
	lang := ""
	if len(fields) > 0 {
		lang = fields[0]
	}
	return fc, n, lang, true
}

func isFenceClose(trimmed string, fc byte, minLen int) bool {
	if len(trimmed) < minLen {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != fc {
			return false
		}
	}
	return true
}

func findJSONTokenLine(raw []byte, segments []string) int {
	if len(segments) == 0 {
		return 0
	}
	leafKey := `"` + segments[len(segments)-1] + `"`
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	bestLine := 0
	bestScore := -1
	for i, line := range lines {
		if !strings.Contains(line, leafKey) {
			continue
		}
		// Prefer lines whose preceding lines contain parent path segments in order.
		score := 0
		searchFrom := 0
		for _, seg := range segments[:len(segments)-1] {
			segPat := `"` + seg + `"`
			for j := searchFrom; j <= i; j++ {
				if strings.Contains(lines[j], segPat) {
					score++
					searchFrom = j
					break
				}
			}
		}
		if score > bestScore {
			bestScore = score
			bestLine = i + 1
		}
	}
	return bestLine
}
