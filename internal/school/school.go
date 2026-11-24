package school

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// School defines a pluggable visual and editorial grammar for DataFolio.
type School interface {
	Name() string
	Tokens(theme string) (SchoolTokens, error)
	Labels(lang string) Labels
	Policy() ReviewPolicy
	SkillReference() (relPath string, content []byte)
}

// SchoolTokens holds the resolved surface, ink, typography, and layout tokens for a Theme.
type SchoolTokens struct {
	Theme           string
	Paper           string
	PaperSubtle     string
	PaperAlt        string
	Ink             string
	InkSoft         string
	Muted           string
	Accent          string
	Ochre           string
	Forest          string
	Rule            string
	RuleStrong      string
	SerifFont       string
	MonoFont        string
	SansFont        string
	MainWidth       string
	MarginWidth     string
	ColumnGap       string
	BorderRadius    string
	BoxShadow       string
	BooktabsTopRule string
	BooktabsMidRule string
	BooktabsRowRule string
}

// Labels provides localized generated captions and headings for a Source Document language.
type Labels struct {
	Figure     string
	Table      string
	Source     string
	MarginNote string
	References string
	Contents   string
}

// ReviewPolicy configures static visual and editorial governance defaults for a School.
type ReviewPolicy struct {
	DisallowEmojis    bool
	MaxBorderRadius   string
	AllowBoxShadow    bool
	RequireCaptions   bool
	RequireSources    bool
	MinContrastNormal float64
	MinContrastLarge  float64
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]School)
)

// Register adds a School implementation to the global registry by its Name().
func Register(s School) {
	if s == nil {
		panic("school: cannot register nil School")
	}
	name := strings.ToLower(strings.TrimSpace(s.Name()))
	if name == "" {
		panic("school: cannot register School with empty name")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = s
}

// Lookup retrieves a registered School by name (case-insensitive).
func Lookup(name string) (School, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	registryMu.RLock()
	defer registryMu.RUnlock()
	s, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("unknown school %q (available: %s)", name, strings.Join(namesLocked(), ", "))
	}
	return s, nil
}

// Names returns all registered School names in sorted order.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return namesLocked()
}

func namesLocked() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// NormalizeLang validates and canonicalizes supported Source Document languages ("en" or "pt-BR").
func NormalizeLang(lang string) (string, error) {
	trimmed := strings.TrimSpace(lang)
	switch strings.ToLower(trimmed) {
	case "en":
		return "en", nil
	case "pt-br", "pt_br", "pt":
		return "pt-BR", nil
	default:
		return "", fmt.Errorf("unsupported lang %q (supported: en, pt-BR)", lang)
	}
}
