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
		formatFlag          string
		outputFlag          string
		schoolFlag          string
		themeFlag           string
		langFlag            string
		paperFlag           string
		tokensFlag          string
		mathFlag            string
		headerFlag          string
		footerFlag          string
		coverFlag           bool
		tocFlag             bool
		headerRuleFlag      bool
		footerRuleFlag      bool
		headerSkipFirstFlag bool
		footerSkipFirstFlag bool
		allowOverflowFlag   bool
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
	fs.StringVar(&headerFlag, "header", "", "Running Chrome header (false, true, left|center|right, or JSON)")
	fs.StringVar(&footerFlag, "footer", "", "Running Chrome footer (false, true, left|center|right, or JSON)")
	fs.BoolVar(&coverFlag, "cover", false, "include cover page")
	fs.BoolVar(&tocFlag, "toc", false, "include table of contents")
	fs.BoolVar(&headerRuleFlag, "header-rule", false, "render separator rule under Running Chrome header")
	fs.BoolVar(&footerRuleFlag, "footer-rule", false, "render separator rule above Running Chrome footer")
	fs.BoolVar(&headerSkipFirstFlag, "header-skip-first", false, "hide Running Chrome header on page/slide 1")
	fs.BoolVar(&footerSkipFirstFlag, "footer-skip-first", false, "hide Running Chrome footer on page/slide 1")
	fs.BoolVar(&allowOverflowFlag, "allow-overflow", false, "downgrade Slide Overflow errors in slides Format to warnings")

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

	visited := make(map[string]bool)
	cliOverride := config.Override{BaseDir: opts.WorkDir}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
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
	if hdrOv, err := parseCLIChromeFlag(visited["header"], headerFlag, visited["header-rule"], headerRuleFlag, visited["header-skip-first"], headerSkipFirstFlag); err != nil {
		fmt.Fprintf(opts.Stderr, "build: --header: %v\n", err)
		return ExitUsage
	} else if hdrOv != nil {
		cliOverride.Header = hdrOv
	}
	if ftrOv, err := parseCLIChromeFlag(visited["footer"], footerFlag, visited["footer-rule"], footerRuleFlag, visited["footer-skip-first"], footerSkipFirstFlag); err != nil {
		fmt.Fprintf(opts.Stderr, "build: --footer: %v\n", err)
		return ExitUsage
	} else if ftrOv != nil {
		cliOverride.Footer = ftrOv
	}
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
	case "document", "slides":
		if err := buildTypst(ctx, parsedDoc, cfg, tok, s.Labels(cfg.Lang), outPath, allowOverflowFlag, opts); err != nil {
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

func buildTypst(
	ctx context.Context,
	parsedDoc *doc.SourceDocument,
	cfg config.Config,
	tok school.SchoolTokens,
	labels school.Labels,
	outPath string,
	allowOverflow bool,
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

	var typSource string
	fileName := "document.typ"
	if cfg.Format == "slides" {
		fileName = "slides.typ"
		typSource, err = typstrender.RenderSlides(parsedDoc, cfg, tok, labels, renderOpts)
	} else {
		typSource, err = typstrender.RenderDocument(parsedDoc, cfg, tok, labels, renderOpts)
	}
	if err != nil {
		return err
	}

	typPath := filepath.Join(tmpDir, fileName)
	if err := os.WriteFile(typPath, []byte(typSource), 0o644); err != nil {
		return fmt.Errorf("write typst source: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	if err := opts.TypstRunner.Compile(ctx, toolchain.CompileRequest{
		TypstBin:   res.Path,
		InputPath:  typPath,
		OutputPath: outPath,
		RootDir:    rootDir,
		FontPath:   fontPath,
	}); err != nil {
		return err
	}

	if cfg.Format != "slides" {
		return nil
	}

	overflows, err := typstrender.CheckSlideOverflow(ctx, opts.TypstRunner, toolchain.QueryRequest{
		TypstBin:  res.Path,
		InputPath: typPath,
		Selector:  typstrender.SlideMarkerSelector,
		RootDir:   rootDir,
		FontPath:  fontPath,
	})
	if err != nil {
		_ = os.Remove(outPath)
		return err
	}
	if len(overflows) > 0 {
		msg := typstrender.FormatSlideOverflows(overflows)
		if allowOverflow {
			fmt.Fprintf(opts.Stderr, "warning: %s\n", msg)
			return nil
		}
		_ = os.Remove(outPath)
		return fmt.Errorf("%s (pass --allow-overflow to keep the PDF)", msg)
	}
	return nil
}

func parseCLIChromeFlag(hasMain bool, raw string, hasRule, rule, hasSkip, skip bool) (*config.ChromeOverride, error) {
	if !hasMain && !hasRule && !hasSkip {
		return nil, nil
	}
	out := &config.ChromeOverride{}
	if hasMain {
		ov, err := doc.ParseSlideChromeOverride(raw)
		if err != nil {
			return nil, err
		}
		out.Enabled = ov.Enabled
		out.SkipFirst = ov.SkipFirst
		out.Rule = ov.Rule
		if ov.ReplaceAllSlots || ov.HasLeft {
			l := ov.Left
			out.Left = &l
		}
		if ov.ReplaceAllSlots || ov.HasCenter {
			c := ov.Center
			out.Center = &c
		}
		if ov.ReplaceAllSlots || ov.HasRight {
			r := ov.Right
			out.Right = &r
		}
	}
	if hasRule {
		r := rule
		out.Rule = &r
	}
	if hasSkip {
		s := skip
		out.SkipFirst = &s
	}
	return out, nil
}

func printBuildUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio build <input.md> [flags]

Flags:
  --to string           Target Format: document, slides, or html
  -o, --output          Output file path
  --school string       Visual School
  --theme string        Theme mode: paper, night, or print
  --lang string         Document language: en or pt-BR
  --paper string        Paper size: a4 or letter
  --tokens string       Path to Token Contract file or DESIGN.md
  --math string         HTML math compiler: mathml or katex
  --header string       Running Chrome header (false, true, "left|center|right", or JSON)
  --footer string       Running Chrome footer (false, true, "left|center|right", or JSON)
  --header-rule         Render separator rule under Running Chrome header
  --footer-rule         Render separator rule above Running Chrome footer
  --header-skip-first   Hide Running Chrome header on page/slide 1
  --footer-skip-first   Hide Running Chrome footer on page/slide 1
  --cover               Include cover page
  --toc                 Include table of contents
  --allow-overflow      Keep slides PDF and warn on Slide Overflow instead of failing`)
}
