package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/config"
	"github.com/pablodiegoo/datafolio/internal/tokens"
)

func runTokens(ctx context.Context, args []string, opts Options) int {
	if len(args) == 0 {
		printTokensUsage(opts.Stderr)
		return ExitUsage
	}

	switch args[0] {
	case "help", "--help", "-h":
		printTokensUsage(opts.Stdout)
		return ExitSuccess
	case "export":
		return runTokensExport(ctx, args[1:], opts)
	case "check":
		return runTokensCheck(ctx, args[1:], opts)
	default:
		fmt.Fprintf(opts.Stderr, "tokens: unknown subcommand %q\n\n", args[0])
		printTokensUsage(opts.Stderr)
		return ExitUsage
	}
}

type tokensFlagValues struct {
	from   string
	to     string
	out    string
	school string
	theme  string
	check  bool
}

func runTokensExport(_ context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("tokens export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var vals tokensFlagValues
	fs.StringVar(&vals.from, "from", "", "path to W3C DTCG YAML/JSON file or DESIGN.md")
	fs.StringVar(&vals.to, "to", strings.Join(tokens.AllTargets, ","), "export targets (css,tailwind,scss,typst,matplotlib)")
	fs.StringVar(&vals.out, "out", "", "output directory (defaults to working directory)")
	fs.StringVar(&vals.school, "school", "", "visual School")
	fs.StringVar(&vals.theme, "theme", "", "Theme mode (paper, night, print)")
	fs.BoolVar(&vals.check, "check", false, "validate WCAG AA contrast without writing files unless --to or --out is set")

	return executeTokensCommand(fs, args, opts, false, &vals)
}

func runTokensCheck(_ context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("tokens check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var vals tokensFlagValues
	fs.StringVar(&vals.from, "from", "", "path to W3C DTCG YAML/JSON file or DESIGN.md")
	fs.StringVar(&vals.school, "school", "", "visual School")
	fs.StringVar(&vals.theme, "theme", "", "Theme mode (paper, night, print)")

	return executeTokensCommand(fs, args, opts, true, &vals)
}

func executeTokensCommand(fs *flag.FlagSet, args []string, opts Options, checkOnly bool, vals *tokensFlagValues) int {
	cmdName := fs.Name()

	pos, err := parsePositionalFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printTokensExportUsage(opts.Stdout, checkOnly)
			return ExitSuccess
		}
		fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
		return ExitUsage
	}

	var fromSet, toSet, outSet bool
	cliOv := config.Override{BaseDir: opts.WorkDir}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "from":
			fromSet = true
		case "to":
			toSet = true
		case "out":
			outSet = true
		case "school":
			cliOv.School = &vals.school
		case "theme":
			cliOv.Theme = &vals.theme
		}
	})

	fromFlag := vals.from
	toFlag := vals.to
	outFlag := vals.out
	checkFlag := vals.check

	if len(pos) == 1 && !fromSet {
		fromFlag = pos[0]
		fromSet = true
	} else if len(pos) > 0 {
		fmt.Fprintf(opts.Stderr, "%s: unexpected argument %q\n", cmdName, pos[0])
		return ExitUsage
	}

	var targets []string
	if !checkOnly {
		targets, err = tokens.ParseTargets(toFlag)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
			return ExitUsage
		}
	}

	if err := config.ValidateOverride(cliOv); err != nil {
		fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
		return ExitUsage
	}

	projectOv, _, err := config.DiscoverUpward(opts.WorkDir)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
		return ExitUsage
	}

	projectBase := projectOv
	projectBase.Tokens = nil
	_, s, baseTok, err := config.Resolve(projectBase, cliOv)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
		return ExitUsage
	}

	var contractPath string
	if fromSet {
		contractPath = resolveWorkPath(opts.WorkDir, fromFlag)
	} else if projectOv.Tokens != nil && strings.TrimSpace(*projectOv.Tokens) != "" {
		contractPath = resolveWorkPath(projectOv.BaseDir, *projectOv.Tokens)
	}

	var contract *tokens.Contract
	if contractPath != "" {
		contract, err = tokens.ParseFile(contractPath)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
			return ExitError
		}
	} else {
		contract = tokens.FromSchoolTokens(baseTok)
	}

	policy := s.Policy()
	results, err := tokens.CheckContrast(contract, policy.MinContrastNormal, policy.MinContrastLarge)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
		return ExitError
	}

	var failures []tokens.ContrastResult
	for _, r := range results {
		if !r.Pass {
			failures = append(failures, r)
		}
	}
	if len(failures) > 0 {
		fmt.Fprintf(opts.Stderr, "%s: WCAG AA contrast validation failed (%d failing pair(s)):\n", cmdName, len(failures))
		for _, f := range failures {
			fmt.Fprintf(opts.Stderr, "  %s\n", f.Format())
		}
		return ExitError
	}

	if checkOnly || (checkFlag && !toSet && !outSet) {
		fmt.Fprintf(opts.Stdout, "tokens: contrast check passed (%d pairs evaluated)\n", len(results))
		return ExitSuccess
	}

	artifacts, err := tokens.Export(contract, tokens.ExportOptions{
		Base:    baseTok,
		Targets: targets,
	})
	if err != nil {
		fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
		return ExitError
	}

	outDir := opts.WorkDir
	if outFlag != "" {
		outDir = resolveWorkPath(opts.WorkDir, outFlag)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(opts.Stderr, "%s: %v\n", cmdName, err)
		return ExitError
	}

	for _, art := range artifacts {
		dest := filepath.Join(outDir, art.Filename)
		if err := os.WriteFile(dest, art.Content, 0o644); err != nil {
			fmt.Fprintf(opts.Stderr, "%s: writing %s: %v\n", cmdName, dest, err)
			return ExitError
		}
		fmt.Fprintf(opts.Stdout, "wrote %s\n", dest)
	}
	return ExitSuccess
}

func printTokensUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio tokens <subcommand> [flags]

Subcommands:
  export    Validate WCAG AA contrast and export Token Contract targets
  check     Validate a Token Contract (alias resolution and WCAG AA contrast)

Run "datafolio tokens export --help" for export flags.`)
}

func printTokensExportUsage(w io.Writer, checkOnly bool) {
	if checkOnly {
		fmt.Fprintln(w, `Usage: datafolio tokens check [flags]

Flags:
  --from string     Path to W3C DTCG YAML/JSON file or DESIGN.md
  --school string   Visual School (default "tufte")
  --theme string    Theme mode: paper, night, or print (default "paper")`)
		return
	}
	fmt.Fprintln(w, `Usage: datafolio tokens export [flags]

Flags:
  --from string     Path to W3C DTCG YAML/JSON file or DESIGN.md
  --to string       Targets: css,tailwind,scss,typst,matplotlib (default all)
  --out string      Output directory (default current directory)
  --school string   Visual School (default "tufte")
  --theme string    Theme mode: paper, night, or print (default "paper")
  --check           Validate WCAG AA contrast (skips file writes if --to/--out omitted)`)
}
