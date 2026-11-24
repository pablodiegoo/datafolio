package tokens

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Token represents a single resolved design token inside a Token Contract.
type Token struct {
	Path        string
	RawValue    string
	Value       string
	Type        string
	Description string
	CSSVar      string
}

// Contract holds the resolved tokens and metadata parsed from a Token Contract or School.
type Contract struct {
	Name        string
	Version     string
	Description string
	SourcePath  string
	Tokens      map[string]Token
	Order       []string
}

// Get retrieves a resolved token by its dot-separated path.
func (c *Contract) Get(path string) (Token, bool) {
	if c == nil || c.Tokens == nil {
		return Token{}, false
	}
	tok, ok := c.Tokens[path]
	return tok, ok
}

// ParseFile reads and parses a standalone DTCG YAML/JSON file or a DESIGN.md file.
func ParseFile(path string) (*Contract, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading token contract %s: %w", path, err)
	}
	c, err := Parse(raw, path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.SourcePath = path
	return c, nil
}

// Parse parses raw bytes as JSON, YAML, or Markdown (DESIGN.md) into a resolved Contract.
func Parse(raw []byte, filename string) (*Contract, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	c := &Contract{
		SourcePath: filename,
		Tokens:     make(map[string]Token),
	}

	var roots []map[string]any
	switch ext {
	case ".json":
		m, err := decodeJSONMap(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid DTCG JSON: %w", err)
		}
		roots = append(roots, m)
	case ".yaml", ".yml":
		m, err := decodeYAMLMap(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid DTCG YAML: %w", err)
		}
		roots = append(roots, m)
	case ".md", ".markdown":
		mdRoots, err := extractMarkdownTokenRoots(raw)
		if err != nil {
			return nil, err
		}
		roots = append(roots, mdRoots...)
	default:
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			m, err := decodeJSONMap(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid DTCG JSON: %w", err)
			}
			roots = append(roots, m)
		} else {
			m, err := decodeYAMLMap(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid DTCG YAML: %w", err)
			}
			roots = append(roots, m)
		}
	}

	for _, root := range roots {
		collectContractRoot(c, root)
	}

	if err := resolveAliases(c); err != nil {
		return nil, err
	}
	return c, nil
}

func decodeJSONMap(raw []byte) (map[string]any, error) {
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func decodeYAMLMap(raw []byte) (map[string]any, error) {
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]any{}, nil
	}
	return m, nil
}

func collectContractRoot(c *Contract, root map[string]any) {
	if len(root) == 0 {
		return
	}
	if dt, ok := asStringMap(root["design_tokens"]); ok {
		root = dt
	}
	if v, ok := root["name"].(string); ok && c.Name == "" {
		c.Name = strings.TrimSpace(v)
	}
	if v, ok := root["version"].(string); ok && c.Version == "" {
		c.Version = strings.TrimSpace(v)
	}
	if v, ok := root["description"].(string); ok && c.Description == "" {
		c.Description = strings.TrimSpace(v)
	}
	walkTokenMap(c, root, "", "")
}

var rootNonTokenKeys = map[string]bool{
	"version":         true,
	"format":          true,
	"name":            true,
	"description":     true,
	"contrast_matrix": true,
}

var metadataOnlyKeys = map[string]bool{
	"description":     true,
	"note":            true,
	"role":            true,
	"cssVar":          true,
	"selector":        true,
	"prefix":          true,
	"mobileFallback":  true,
	"stitchRoundness": true,
	"stitchMapping":   true,
	"surfaces":        true,
	"wcag":            true,
	"ratio":           true,
}

func walkTokenMap(c *Contract, m map[string]any, prefix string, inheritedType string) {
	if m == nil {
		return
	}
	if t, ok := stringField(m, "$type", "type"); ok && t != "" {
		inheritedType = t
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if strings.HasPrefix(k, "$") {
			continue
		}
		if prefix == "" && rootNonTokenKeys[k] {
			continue
		}
		if prefix != "" && metadataOnlyKeys[k] {
			continue
		}

		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		val := m[k]
		if sub, ok := asStringMap(val); ok {
			if isTokenLeaf(sub) {
				tok := buildLeafToken(path, sub, inheritedType)
				addToken(c, tok)
				continue
			}
			walkTokenMap(c, sub, path, inheritedType)
			continue
		}

		if shouldAcceptShorthandScalar(path, k, val, inheritedType) {
			rawStr, ok := formatScalarOrList(val)
			if ok && rawStr != "" {
				typ := inheritedType
				if typ == "" {
					typ = inferTokenType(path, rawStr)
				}
				addToken(c, Token{
					Path:     path,
					RawValue: rawStr,
					Value:    rawStr,
					Type:     typ,
				})
			}
		}
	}
}

func isTokenLeaf(m map[string]any) bool {
	for _, key := range []string{"$value", "value"} {
		v, exists := m[key]
		if !exists {
			continue
		}
		if sub, ok := asStringMap(v); ok {
			if isTokenLeaf(sub) {
				return false
			}
			if _, hasColorSpace := sub["colorSpace"]; hasColorSpace {
				return true
			}
			if _, hasHex := sub["hex"]; hasHex {
				return true
			}
			return false
		}
		return true
	}
	return false
}

func buildLeafToken(path string, m map[string]any, inheritedType string) Token {
	rawVal := m["$value"]
	if _, ok := m["$value"]; !ok {
		rawVal = m["value"]
	}

	valStr := extractTokenValue(rawVal)
	typ, _ := stringField(m, "$type", "type")
	if typ == "" {
		typ = inheritedType
	}
	if typ == "" {
		typ = inferTokenType(path, valStr)
	}
	desc, _ := stringField(m, "$description", "description", "role")
	cssVar, _ := stringField(m, "cssVar")

	return Token{
		Path:        path,
		RawValue:    valStr,
		Value:       valStr,
		Type:        typ,
		Description: desc,
		CSSVar:      cssVar,
	}
}

func extractTokenValue(v any) string {
	if sub, ok := asStringMap(v); ok {
		if hex, ok := sub["hex"].(string); ok && hex != "" {
			return strings.TrimSpace(hex)
		}
	}
	s, _ := formatScalarOrList(v)
	return s
}

func formatScalarOrList(v any) (string, bool) {
	switch val := v.(type) {
	case nil:
		return "", false
	case string:
		return strings.TrimSpace(val), true
	case json.Number:
		return val.String(), true
	case int:
		return strconv.Itoa(val), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(val), true
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := formatScalarOrList(item)
			if !ok || s == "" {
				continue
			}
			if strings.ContainsRune(s, ' ') && !strings.HasPrefix(s, `"`) && !strings.HasPrefix(s, `'`) {
				s = `"` + s + `"`
			}
			parts = append(parts, s)
		}
		if len(parts) == 0 {
			return "", false
		}
		return strings.Join(parts, ", "), true
	default:
		return "", false
	}
}

func shouldAcceptShorthandScalar(path, key string, val any, inheritedType string) bool {
	s, ok := formatScalarOrList(val)
	if !ok || s == "" {
		return false
	}
	if inheritedType != "" {
		return true
	}
	if isAliasExpr(s) || looksLikeColor(s) {
		return true
	}
	lowerPath := strings.ToLower(path)
	lowerKey := strings.ToLower(key)
	switch {
	case strings.Contains(lowerPath, "font") || strings.Contains(lowerPath, "typography"):
		if lowerKey == "serif" || lowerKey == "sans" || lowerKey == "mono" ||
			lowerKey == "basefontsize" || lowerKey == "fontsize" || lowerKey == "lineheight" ||
			lowerKey == "fontweight" || lowerKey == "letterspacing" || lowerKey == "fontfamily" ||
			lowerKey == "fontstyle" || lowerKey == "fontvariant" || lowerKey == "fontvariantnumeric" ||
			lowerKey == "texttransform" || lowerKey == "color" {
			return true
		}
	case strings.Contains(lowerPath, "layout") || strings.Contains(lowerPath, "spacing") ||
		strings.Contains(lowerPath, "radius") || strings.Contains(lowerPath, "shape") ||
		strings.Contains(lowerPath, "rules") || strings.Contains(lowerPath, "dividers") ||
		strings.Contains(lowerPath, "elevation") || strings.Contains(lowerPath, "shadow"):
		return true
	}
	return false
}

func addToken(c *Contract, tok Token) {
	if _, exists := c.Tokens[tok.Path]; !exists {
		c.Order = append(c.Order, tok.Path)
	}
	c.Tokens[tok.Path] = tok
}

var aliasPattern = regexp.MustCompile(`\{([a-zA-Z0-9_-]+(?:\.[a-zA-Z0-9_-]+)+)\}`)

func isAliasExpr(s string) bool {
	return aliasPattern.MatchString(s)
}

func resolveAliases(c *Contract) error {
	resolved := make(map[string]Token, len(c.Tokens))
	visiting := make(map[string]bool)

	var resolveOne func(path string, stack []string) (Token, error)
	resolveOne = func(path string, stack []string) (Token, error) {
		if tok, ok := resolved[path]; ok {
			return tok, nil
		}
		tok, ok := c.Tokens[path]
		if !ok {
			return Token{}, fmt.Errorf("unresolved token reference %q", path)
		}
		if visiting[path] {
			cycle := append(append([]string{}, stack...), path)
			return Token{}, fmt.Errorf("token reference cycle detected: %s", strings.Join(cycle, " -> "))
		}

		visiting[path] = true
		nextStack := append(stack, path)

		matches := aliasPattern.FindAllStringSubmatchIndex(tok.RawValue, -1)
		if len(matches) > 0 {
			var b strings.Builder
			last := 0
			for _, loc := range matches {
				b.WriteString(tok.RawValue[last:loc[0]])
				refPath := tok.RawValue[loc[2]:loc[3]]
				if _, exists := c.Tokens[refPath]; !exists {
					delete(visiting, path)
					return Token{}, fmt.Errorf("unresolved token reference %q in token %q", "{"+refPath+"}", path)
				}
				target, err := resolveOne(refPath, nextStack)
				if err != nil {
					delete(visiting, path)
					return Token{}, err
				}
				b.WriteString(target.Value)
				if tok.Type == "" && strings.TrimSpace(tok.RawValue) == "{"+refPath+"}" {
					tok.Type = target.Type
				}
				last = loc[1]
			}
			b.WriteString(tok.RawValue[last:])
			tok.Value = b.String()
			if tok.Type == "" {
				tok.Type = inferTokenType(tok.Path, tok.Value)
			}
		}

		delete(visiting, path)
		resolved[path] = tok
		return tok, nil
	}

	keys := make([]string, 0, len(c.Tokens))
	for k := range c.Tokens {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		tok, err := resolveOne(k, nil)
		if err != nil {
			return err
		}
		c.Tokens[k] = tok
	}
	sort.Strings(c.Order)
	return nil
}

func inferTokenType(path, val string) string {
	lowerPath := strings.ToLower(path)
	switch {
	case looksLikeColor(val) || strings.HasPrefix(lowerPath, "color.") || strings.HasPrefix(lowerPath, "colors."):
		return "color"
	case strings.Contains(lowerPath, "fontfamily") || strings.Contains(lowerPath, "font.serif") ||
		strings.Contains(lowerPath, "font.sans") || strings.Contains(lowerPath, "font.mono") ||
		strings.Contains(lowerPath, "typography.family"):
		return "fontFamily"
	case strings.Contains(lowerPath, "shadow"):
		return "shadow"
	case strings.Contains(lowerPath, "radius") || strings.Contains(lowerPath, "spacing") ||
		strings.Contains(lowerPath, "width") || strings.Contains(lowerPath, "gap") ||
		strings.Contains(lowerPath, "size") || strings.HasSuffix(val, "px") ||
		strings.HasSuffix(val, "rem") || strings.HasSuffix(val, "em") || strings.HasSuffix(val, "%"):
		return "dimension"
	default:
		return "string"
	}
}

func looksLikeColor(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lower, "#") ||
		strings.HasPrefix(lower, "rgb(") ||
		strings.HasPrefix(lower, "rgba(")
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	default:
		return nil, false
	}
}

func stringField(m map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s), true
			}
		}
	}
	return "", false
}

var markdownTokenRootKeys = map[string]bool{
	"design_tokens": true,
	"color":         true,
	"colors":        true,
	"typography":    true,
	"font":          true,
	"fonts":         true,
	"spacing":       true,
	"layout":        true,
	"shape":         true,
	"radius":        true,
	"elevation":     true,
	"rules":         true,
	"dividers":      true,
	"tokens":        true,
}

func hasTokenRootKey(m map[string]any) bool {
	for k := range m {
		if markdownTokenRootKeys[k] {
			return true
		}
	}
	return false
}

// YAMLBlock represents an extracted YAML frontmatter or fenced YAML block from Markdown,
// paired with the 1-based line number of its first content line.
type YAMLBlock struct {
	Content     []byte
	StartLine   int
	Frontmatter bool
}

// ExtractYAMLBlocks extracts YAML frontmatter (when present) and fenced yaml/yml code blocks
// from Markdown source in document order along with their 1-based content start line numbers.
func ExtractYAMLBlocks(raw []byte) []YAMLBlock {
	text := strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	var blocks []YAMLBlock
	startIdx := 0

	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			t := strings.TrimSpace(lines[i])
			if t == "---" || t == "..." {
				endIdx = i
				break
			}
		}
		if endIdx > 0 {
			fmBody := strings.Join(lines[1:endIdx], "\n")
			blocks = append(blocks, YAMLBlock{
				Content:     []byte(fmBody),
				StartLine:   2,
				Frontmatter: true,
			})
			startIdx = endIdx + 1
		}
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
				blockStartLine = i + 2
				buf = buf[:0]
			}
			continue
		}

		if isClosingFence(trimmed, fenceChar, fenceLen) {
			if isYAML {
				blocks = append(blocks, YAMLBlock{
					Content:   []byte(strings.Join(buf, "\n")),
					StartLine: blockStartLine,
				})
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

	return blocks
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

func extractMarkdownTokenRoots(raw []byte) ([]map[string]any, error) {
	var roots []map[string]any
	for _, blk := range ExtractYAMLBlocks(raw) {
		m, err := decodeYAMLMap(blk.Content)
		if err != nil {
			if blk.Frontmatter {
				return nil, fmt.Errorf("invalid YAML frontmatter in Markdown token contract: %w", err)
			}
			return nil, fmt.Errorf("invalid fenced YAML block in Markdown token contract: %w", err)
		}
		if hasTokenRootKey(m) {
			roots = append(roots, m)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no design_tokens or token YAML blocks found in Markdown document")
	}
	return roots, nil
}

func isClosingFence(trimmed string, fc byte, minLen int) bool {
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
