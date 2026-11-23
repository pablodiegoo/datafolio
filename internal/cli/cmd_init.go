package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pablodiegoo/datafolio/internal/school"
	"gopkg.in/yaml.v3"
)

func runInit(_ context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		withTokens bool
		schoolName string
		lang       string
		theme      string
		force      bool
	)
	fs.BoolVar(&withTokens, "tokens", false, "scaffold a starter W3C DTCG tokens.yaml Token Contract")
	fs.StringVar(&schoolName, "school", "tufte", "default visual School")
	fs.StringVar(&lang, "lang", "en", "default document language (en or pt-BR)")
	fs.StringVar(&theme, "theme", "paper", "default Theme (paper, night, print)")
	fs.BoolVar(&force, "force", false, "overwrite existing files")

	pos, err := parsePositionalFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printInitUsage(opts.Stdout)
			return ExitSuccess
		}
		fmt.Fprintf(opts.Stderr, "init: %v\n", err)
		return ExitUsage
	}
	if len(pos) > 0 {
		fmt.Fprintf(opts.Stderr, "init: unexpected argument %q\n", pos[0])
		return ExitUsage
	}

	s, err := school.Lookup(schoolName)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "init: %v\n", err)
		return ExitUsage
	}
	normLang, err := school.NormalizeLang(lang)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "init: %v\n", err)
		return ExitUsage
	}
	tok, err := s.Tokens(theme)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "init: %v\n", err)
		return ExitUsage
	}

	cfgPath := filepath.Join(opts.WorkDir, "datafolio.yaml")
	tokensPath := filepath.Join(opts.WorkDir, "tokens.yaml")

	targets := []string{cfgPath}
	if withTokens {
		targets = append(targets, tokensPath)
	}
	if !force {
		for _, target := range targets {
			if _, statErr := os.Stat(target); statErr == nil {
				fmt.Fprintf(opts.Stderr, "init: %s already exists (use --force to overwrite)\n", filepath.Base(target))
				return ExitError
			} else if !os.IsNotExist(statErr) {
				fmt.Fprintf(opts.Stderr, "init: checking %s: %v\n", target, statErr)
				return ExitError
			}
		}
	}

	if err := os.MkdirAll(opts.WorkDir, 0o755); err != nil {
		fmt.Fprintf(opts.Stderr, "init: %v\n", err)
		return ExitError
	}

	cfgDoc := initConfigFile{
		School: s.Name(),
		Theme:  tok.Theme,
		Lang:   normLang,
		Paper:  "a4",
	}
	if withTokens {
		cfgDoc.Tokens = "tokens.yaml"
	}
	cfgBytes, err := yaml.Marshal(cfgDoc)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "init: marshaling datafolio.yaml: %v\n", err)
		return ExitError
	}

	if withTokens {
		tokBytes, err := marshalStarterTokens(tok)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "init: marshaling tokens.yaml: %v\n", err)
			return ExitError
		}
		if err := os.WriteFile(tokensPath, tokBytes, 0o644); err != nil {
			fmt.Fprintf(opts.Stderr, "init: writing %s: %v\n", tokensPath, err)
			return ExitError
		}
		fmt.Fprintf(opts.Stdout, "created %s\n", tokensPath)
	}

	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
		fmt.Fprintf(opts.Stderr, "init: writing %s: %v\n", cfgPath, err)
		return ExitError
	}
	fmt.Fprintf(opts.Stdout, "created %s\n", cfgPath)
	return ExitSuccess
}

type initConfigFile struct {
	School string `yaml:"school"`
	Theme  string `yaml:"theme"`
	Lang   string `yaml:"lang"`
	Paper  string `yaml:"paper"`
	Tokens string `yaml:"tokens,omitempty"`
}

func marshalStarterTokens(tok school.SchoolTokens) ([]byte, error) {
	colorToken := func(val string) map[string]string {
		return map[string]string{
			"$type":  "color",
			"$value": val,
		}
	}
	dimToken := func(val string) map[string]string {
		return map[string]string{
			"$type":  "dimension",
			"$value": val,
		}
	}
	fontToken := func(val string) map[string]string {
		return map[string]string{
			"$type":  "fontFamily",
			"$value": val,
		}
	}

	contract := map[string]any{
		"color": map[string]any{
			"paper":        colorToken(tok.Paper),
			"paper_subtle": colorToken(tok.PaperSubtle),
			"paper_alt":    colorToken(tok.PaperAlt),
			"ink":          colorToken(tok.Ink),
			"ink_soft":     colorToken(tok.InkSoft),
			"muted":        colorToken(tok.Muted),
			"accent":       colorToken(tok.Accent),
			"ochre":        colorToken(tok.Ochre),
			"forest":       colorToken(tok.Forest),
			"rule":         colorToken(tok.Rule),
			"rule_strong":  colorToken(tok.RuleStrong),
		},
		"font": map[string]any{
			"serif": fontToken(tok.SerifFont),
			"mono":  fontToken(tok.MonoFont),
			"sans":  fontToken(tok.SansFont),
		},
		"radius": map[string]any{
			"default": dimToken(tok.BorderRadius),
		},
	}
	return yaml.Marshal(contract)
}

func printInitUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio init [flags]

Flags:
  --tokens          Scaffold a starter W3C DTCG tokens.yaml file
  --school string   Visual School name (default "tufte")
  --lang string     Document language: en or pt-BR (default "en")
  --theme string    Theme mode: paper, night, or print (default "paper")
  --force           Overwrite existing datafolio.yaml or tokens.yaml`)
}
