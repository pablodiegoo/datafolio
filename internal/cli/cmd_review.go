package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/review"
	"github.com/pablodiegoo/datafolio/internal/school"
)

func runReview(ctx context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		tokensFlag string
		schoolFlag string
		formatFlag string
		strictFlag bool
	)
	fs.StringVar(&tokensFlag, "tokens", "", "path to W3C DTCG Token Contract or DESIGN.md")
	fs.StringVar(&schoolFlag, "school", "", "visual School")
	fs.StringVar(&formatFlag, "format", "text", "output format (text, json, md)")
	fs.BoolVar(&strictFlag, "strict", false, "exit with code 1 when any warning Finding is emitted")

	pos, err := parsePositionalFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printReviewUsage(opts.Stdout)
			return ExitSuccess
		}
		fmt.Fprintf(opts.Stderr, "review: %v\n", err)
		return ExitUsage
	}

	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})

	format := strings.ToLower(strings.TrimSpace(formatFlag))
	switch format {
	case "text", "json", "md":
	default:
		fmt.Fprintf(opts.Stderr, "review: unsupported --format %q (supported: text, json, md)\n", formatFlag)
		return ExitUsage
	}

	if visited["school"] {
		if _, err := school.Lookup(schoolFlag); err != nil {
			fmt.Fprintf(opts.Stderr, "review: %v\n", err)
			return ExitUsage
		}
	}

	if visited["tokens"] && strings.TrimSpace(tokensFlag) == "" {
		fmt.Fprintln(opts.Stderr, "review: --tokens path cannot be empty")
		return ExitUsage
	}

	if len(pos) == 0 {
		fmt.Fprintln(opts.Stderr, "review: expected at least one target path")
		return ExitUsage
	}

	var schoolName string
	if visited["school"] {
		schoolName = schoolFlag
	}

	report, err := review.Review(ctx, review.Options{
		Paths:       pos,
		TokensPath:  tokensFlag,
		School:      schoolName,
		WorkDir:     opts.WorkDir,
		CacheDir:    opts.CacheDir,
		TypstRunner: opts.TypstRunner,
		LookPath:    opts.LookPath,
		GOOS:        opts.GOOS,
		GOARCH:      opts.GOARCH,
	})
	if err != nil {
		fmt.Fprintf(opts.Stderr, "review: %v\n", err)
		var usageErr *review.UsageError
		if errors.As(err, &usageErr) {
			return ExitUsage
		}
		return ExitError
	}

	formatted, err := report.Format(format)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "review: %v\n", err)
		return ExitUsage
	}
	fmt.Fprint(opts.Stdout, formatted)

	if report.Failed(strictFlag) {
		return ExitError
	}
	return ExitSuccess
}

func printReviewUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio review <path...> [flags]

Flags:
  --tokens string   Path to W3C DTCG Token Contract file or DESIGN.md
  --school string   Visual School (default "tufte")
  --format string   Output format: text, json, or md (default "text")
  --strict          Treat warning Findings as errors (exit code 1)`)
}
