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
	"github.com/pablodiegoo/datafolio/internal/doc"
	htmlrender "github.com/pablodiegoo/datafolio/internal/render/html"
	typstrender "github.com/pablodiegoo/datafolio/internal/render/typst"
	"github.com/pablodiegoo/datafolio/internal/school"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

func runBuild(ctx context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		formatFlag string
		outputFlag string
		schoolFlag string
		themeFlag  string
		langFlag   string
		paperFlag  string
		tokensFlag string
		mathFlag   string
		coverFlag  bool
		tocFlag    bool
	)
	fs.StringVar(&formatFlag, "to", "", "target Format: document, slides, or html")
	fs.StringVar(&outputFlag, "o", "", "output file path")
	fs.StringVar(&outputFlag, "output", "", "output file path")
	fs.StringVar(&schoolFlag, "school", "", "visual School")
	fs.StringVar(&themeFlag, "theme", "", "Theme mode (paper, night, print)")
	fs.StringVar(&langFlag, "lang", "", "document language (en or pt-BR)")
	fs.StringVar(&paperFlag, "paper", "", "paper size (a4 or letter)")
	fs.StringVar(&tokensFlag, "tokens", "", "path to W3C DTCG Token Contract or DESIGN.md")
	fs.StringVar(&mathFlag, "math", "", "HTML math mode (mathml or katex)")
	fs.BoolVar(&coverFlag, "cover", false, "include cover page")
	fs.BoolVar(&tocFlag, "toc", false, "include table of contents")

	pos, err := parsePositionalFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printBuildUsage(opts.Stdout)
			return ExitSuccess
		}
		fmt.Fprintf(opts.Stderr, "build: %v\n", err)
		return ExitUsage
	}
	if len(pos) != 1 {
		fmt.Fprintln(opts.Stderr, "build: expected exactly one Source Document path")
		return ExitUsage
	}

	cliOverride := config.Override{BaseDir: opts.WorkDir}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "to":
			cliOverride.Format = &formatFlag
		case "school":
			cliOverride.School = &schoolFlag
		case "theme":
			cliOverride.Theme = &themeFlag
		case "lang":
			cliOverride.Lang = &langFlag
		case "paper":
			cliOverride.Paper = &paperFlag
		case "tokens":
			cliOverride.Tokens = &tokensFlag
		case "math":
			cliOverride.Math = &mathFlag
		case "cover":
			cliOverride.Cover = &coverFlag
		case "toc":
			cliOverride.TOC = &tocFlag
		}
	})
	if err := config.ValidateOverride(cliOverride); err != nil {
		fmt.Fprintf(opts.Stderr, "build: %v\n", err)
		return ExitUsage
	}

	inputPath := resolveWorkPath(opts.WorkDir, pos[0])
	projectOverride, _, err := config.DiscoverUpward(filepath.Dir(inputPath))
	if err != nil {
		fmt.Fprintf(opts.Stderr, "build: %v\n", err)
		return ExitUsage
	}

	parsedDoc, err := doc.ParseFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(opts.Stderr, "build: %v\n", err)
			return ExitError
		}
		fmt.Fprintf(opts.Stderr, "build: %v\n", err)
		return ExitUsage
	}

	fmOverride, err := config.ParseYAML(parsedDoc.RawFrontmatter, parsedDoc.Dir)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "build: %v\n", err)
		return ExitUsage
	}

	cfg, s, tok, err := config.Resolve(projectOverride, fmOverride, cliOverride)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "build: %v\n", err)
		return ExitUsage
	}

	outPath := outputFlag
	if outPath == "" {
		ext := ".html"
		if cfg.Format != "html" {
			ext = ".pdf"
		}
		outPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ext
	} else {
		outPath = resolveWorkPath(opts.WorkDir, outPath)
	}

	switch cfg.Format {
	case "html":
		rendered, err := htmlrender.Render(parsedDoc, cfg, tok, s.Labels(cfg.Lang))
		if err != nil {
			fmt.Fprintf(opts.Stderr, "build: %v\n", err)
			return ExitError
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintf(opts.Stderr, "build: %v\n", err)
			return ExitError
		}
		if err := os.WriteFile(outPath, []byte(rendered), 0o644); err != nil {
			fmt.Fprintf(opts.Stderr, "build: %v\n", err)
			return ExitError
		}
		fmt.Fprintf(opts.Stdout, "wrote %s\n", outPath)
		return ExitSuccess
	case "document":
		if err := buildTypstDocument(ctx, parsedDoc, cfg, tok, s.Labels(cfg.Lang), outPath, opts); err != nil {
			fmt.Fprintf(opts.Stderr, "build: %v\n", err)
			return ExitError
		}
		fmt.Fprintf(opts.Stdout, "wrote %s\n", outPath)
		return ExitSuccess
	default:
		fmt.Fprintf(opts.Stderr, "build: format %q compilation requires Typst backend (not available in this build)\n", cfg.Format)
		return ExitError
	}
}

func buildTypstDocument(
	ctx context.Context,
	parsedDoc *doc.SourceDocument,
	cfg config.Config,
	tok school.SchoolTokens,
	labels school.Labels,
	outPath string,
	opts Options,
) error {
	res := toolchain.ResolveTypst(opts.LookPath, opts.CacheDir, opts.GOOS, opts.GOARCH)
	if !res.Found {
		return fmt.Errorf("typst not found on PATH or in %s (run \"datafolio doctor --install-typst\" to install %s)",
			toolchain.CachedTypstPath(opts.CacheDir, opts.GOOS), toolchain.PinnedTypstVersion)
	}

	fontPath, err := toolchain.EnsureFonts(opts.CacheDir)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "datafolio-typst-*")
	if err != nil {
		return fmt.Errorf("create temp workspace: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	rootDir, renderOpts, err := typstrender.PrepareWorkspace(parsedDoc, cfg, tmpDir)
	if err != nil {
		return err
	}

	typSource, err := typstrender.RenderDocument(parsedDoc, cfg, tok, labels, renderOpts)
	if err != nil {
		return err
	}

	typPath := filepath.Join(tmpDir, "document.typ")
	if err := os.WriteFile(typPath, []byte(typSource), 0o644); err != nil {
		return fmt.Errorf("write typst source: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	return opts.TypstRunner.Compile(ctx, toolchain.CompileRequest{
		TypstBin:   res.Path,
		InputPath:  typPath,
		OutputPath: outPath,
		RootDir:    rootDir,
		FontPath:   fontPath,
	})
}

func printBuildUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio build <input.md> [flags]

Flags:
  --to string       Target Format: document, slides, or html
  -o, --output      Output file path
  --school string   Visual School
  --theme string    Theme mode: paper, night, or print
  --lang string     Document language: en or pt-BR
  --paper string    Paper size: a4 or letter
  --tokens string   Path to Token Contract file or DESIGN.md
  --math string     HTML math compiler: mathml or katex
  --cover           Include cover page
  --toc             Include table of contents`)
}
