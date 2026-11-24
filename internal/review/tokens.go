package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	for _, blk := range tokens.ExtractYAMLBlocks(raw) {
		if relLine := findYAMLTokenLine(blk.Content, segments); relLine > 0 {
			return blk.StartLine + relLine - 1
		}
	}
	return 0
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
