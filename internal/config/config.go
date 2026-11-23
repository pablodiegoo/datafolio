package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pablodiegoo/datafolio/internal/school"
	"gopkg.in/yaml.v3"
)

// ChromeSpec configures optional Running Chrome (top header or bottom footer).
type ChromeSpec struct {
	Enabled   bool
	Left      string
	Center    string
	Right     string
	SkipFirst bool
	Rule      bool
}

// ChromeOverride captures an explicit Running Chrome setting from YAML or CLI flags.
type ChromeOverride struct {
	Enabled   *bool   `yaml:"enabled"`
	Left      *string `yaml:"left"`
	Center    *string `yaml:"center"`
	Right     *string `yaml:"right"`
	SkipFirst *bool   `yaml:"skip_first"`
	Rule      *bool   `yaml:"rule"`
}

// UnmarshalYAML supports boolean shorthand (`false` / `true`) or an object with slot fields.
func (c *ChromeOverride) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		if value.Tag == "!!null" {
			return nil
		}
		var b bool
		if err := value.Decode(&b); err != nil {
			return fmt.Errorf("expected boolean or mapping for running chrome: %w", err)
		}
		c.Enabled = &b
		return nil
	}
	if value.Kind == yaml.MappingNode {
		type rawChrome ChromeOverride
		var decoded rawChrome
		if err := value.Decode(&decoded); err != nil {
			return err
		}
		*c = ChromeOverride(decoded)
		if c.Enabled == nil {
			enabled := true
			c.Enabled = &enabled
		}
		return nil
	}
	return fmt.Errorf("expected boolean or mapping for running chrome")
}

// Config holds the fully resolved build configuration for a Source Document.
type Config struct {
	School       string
	Theme        string
	Format       string
	Lang         string
	Paper        string
	Cover        bool
	TOC          bool
	Header       ChromeSpec
	Footer       ChromeSpec
	Tokens       string
	Bibliography string
	CSL          string
	Math         string
}

// Override captures optional explicitly-set configuration values at a single precedence level.
// Pointer fields distinguish unset fields from explicit zero values (false, "").
type Override struct {
	School       *string         `yaml:"school"`
	Theme        *string         `yaml:"theme"`
	Format       *string         `yaml:"format"`
	Lang         *string         `yaml:"lang"`
	Paper        *string         `yaml:"paper"`
	Cover        *bool           `yaml:"cover"`
	TOC          *bool           `yaml:"toc"`
	Header       *ChromeOverride `yaml:"header"`
	Footer       *ChromeOverride `yaml:"footer"`
	Tokens       *string         `yaml:"tokens"`
	Bibliography *string         `yaml:"bibliography"`
	CSL          *string         `yaml:"csl"`
	Math         *string         `yaml:"math"`
	BaseDir      string          `yaml:"-"`
}

// ParseYAML decodes configuration overrides from YAML bytes (from datafolio.yaml or frontmatter).
func ParseYAML(raw []byte, baseDir string) (Override, error) {
	var ov Override
	if len(raw) == 0 {
		ov.BaseDir = baseDir
		return ov, nil
	}
	if err := yaml.Unmarshal(raw, &ov); err != nil {
		return Override{}, fmt.Errorf("invalid config YAML: %w", err)
	}
	ov.BaseDir = baseDir
	return ov, nil
}

// DiscoverUpward walks from startDir upward looking for datafolio.yaml.
// If found, it parses and returns the Override and file path. If not found, returns zero Override and empty path.
func DiscoverUpward(startDir string) (Override, string, error) {
	if startDir == "" {
		return Override{}, "", nil
	}
	dir := filepath.Clean(startDir)
	for {
		candidate := filepath.Join(dir, "datafolio.yaml")
		raw, err := os.ReadFile(candidate)
		if err == nil {
			ov, parseErr := ParseYAML(raw, dir)
			if parseErr != nil {
				return Override{}, candidate, fmt.Errorf("%s: %w", candidate, parseErr)
			}
			return ov, candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Override{}, candidate, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Override{}, "", nil
		}
		dir = parent
	}
}

// Defaults returns baseline DataFolio configuration defaults before overrides.
func Defaults() Config {
	return Config{
		School: "tufte",
		Theme:  "paper",
		Format: "document",
		Lang:   "en",
		Paper:  "a4",
		Math:   "mathml",
		Header: ChromeSpec{Enabled: true, SkipFirst: true},
		Footer: ChromeSpec{Enabled: true, SkipFirst: true},
	}
}

// ValidateOverride checks any explicitly set fields in an Override without requiring a full config.
func ValidateOverride(ov Override) error {
	if ov.School != nil {
		if _, err := school.Lookup(*ov.School); err != nil {
			return err
		}
	}
	if ov.Lang != nil {
		if _, err := school.NormalizeLang(*ov.Lang); err != nil {
			return err
		}
	}
	if ov.Format != nil {
		if err := validateFormat(*ov.Format); err != nil {
			return err
		}
	}
	if ov.Paper != nil {
		if err := validatePaper(*ov.Paper); err != nil {
			return err
		}
	}
	if ov.Math != nil {
		if err := validateMath(*ov.Math); err != nil {
			return err
		}
	}
	if ov.Theme != nil {
		sName := "tufte"
		if ov.School != nil {
			sName = *ov.School
		}
		if s, err := school.Lookup(sName); err == nil {
			if _, err := s.Tokens(*ov.Theme); err != nil {
				return err
			}
		}
	}
	return nil
}

// Resolve applies overrides in order (lowest precedence to highest precedence) and validates the result.
func Resolve(overrides ...Override) (Config, school.School, school.SchoolTokens, error) {
	cfg := Defaults()
	for _, ov := range overrides {
		applyScalarOverride(&cfg, ov)
	}

	switch cfg.Format {
	case "document":
		cfg.Header = ChromeSpec{Enabled: true, SkipFirst: true}
		cfg.Footer = ChromeSpec{Enabled: true, SkipFirst: true}
	case "slides", "html":
		cfg.Header = ChromeSpec{Enabled: false}
		cfg.Footer = ChromeSpec{Enabled: false}
	}

	for _, ov := range overrides {
		applyChromeOverride(&cfg.Header, ov.Header)
		applyChromeOverride(&cfg.Footer, ov.Footer)
	}

	s, err := school.Lookup(cfg.School)
	if err != nil {
		return Config{}, nil, school.SchoolTokens{}, err
	}
	cfg.School = s.Name()

	normLang, err := school.NormalizeLang(cfg.Lang)
	if err != nil {
		return Config{}, nil, school.SchoolTokens{}, err
	}
	cfg.Lang = normLang

	if err := validateFormat(cfg.Format); err != nil {
		return Config{}, nil, school.SchoolTokens{}, err
	}
	if err := validatePaper(cfg.Paper); err != nil {
		return Config{}, nil, school.SchoolTokens{}, err
	}
	if err := validateMath(cfg.Math); err != nil {
		return Config{}, nil, school.SchoolTokens{}, err
	}

	tok, err := s.Tokens(cfg.Theme)
	if err != nil {
		return Config{}, nil, school.SchoolTokens{}, err
	}
	cfg.Theme = tok.Theme

	return cfg, s, tok, nil
}

func validateFormat(v string) error {
	switch v {
	case "document", "slides", "html":
		return nil
	default:
		return fmt.Errorf("unsupported format %q (supported: document, slides, html)", v)
	}
}

func validatePaper(v string) error {
	switch v {
	case "a4", "letter":
		return nil
	default:
		return fmt.Errorf("unsupported paper %q (supported: a4, letter)", v)
	}
}

func validateMath(v string) error {
	switch v {
	case "mathml", "katex":
		return nil
	default:
		return fmt.Errorf("unsupported math %q (supported: mathml, katex)", v)
	}
}

func applyScalarOverride(cfg *Config, ov Override) {
	if ov.School != nil {
		cfg.School = *ov.School
	}
	if ov.Theme != nil {
		cfg.Theme = *ov.Theme
	}
	if ov.Format != nil {
		cfg.Format = *ov.Format
	}
	if ov.Lang != nil {
		cfg.Lang = *ov.Lang
	}
	if ov.Paper != nil {
		cfg.Paper = *ov.Paper
	}
	if ov.Cover != nil {
		cfg.Cover = *ov.Cover
	}
	if ov.TOC != nil {
		cfg.TOC = *ov.TOC
	}
	if ov.Tokens != nil {
		cfg.Tokens = resolveRel(ov.BaseDir, *ov.Tokens)
	}
	if ov.Bibliography != nil {
		cfg.Bibliography = resolveRel(ov.BaseDir, *ov.Bibliography)
	}
	if ov.CSL != nil {
		cfg.CSL = resolveRel(ov.BaseDir, *ov.CSL)
	}
	if ov.Math != nil {
		cfg.Math = *ov.Math
	}
}

func applyChromeOverride(spec *ChromeSpec, ov *ChromeOverride) {
	if ov == nil {
		return
	}
	if ov.Enabled != nil {
		spec.Enabled = *ov.Enabled
	}
	if ov.Left != nil {
		spec.Left = *ov.Left
	}
	if ov.Center != nil {
		spec.Center = *ov.Center
	}
	if ov.Right != nil {
		spec.Right = *ov.Right
	}
	if ov.SkipFirst != nil {
		spec.SkipFirst = *ov.SkipFirst
	}
	if ov.Rule != nil {
		spec.Rule = *ov.Rule
	}
}

func resolveRel(baseDir, p string) string {
	if p == "" || filepath.IsAbs(p) || baseDir == "" {
		return p
	}
	return filepath.Join(baseDir, p)
}
