package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	_ "github.com/pablodiegoo/datafolio/internal/school/tufte"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
	"github.com/pablodiegoo/datafolio/internal/version"
)

// Exit codes returned by Run.
const (
	ExitSuccess = 0
	ExitError   = 1
	ExitUsage   = 2
)

// Options configures external IO and host environment paths for an in-process CLI execution.
type Options struct {
	Stdout         io.Writer
	Stderr         io.Writer
	WorkDir        string
	HomeDir        string
	CacheDir       string
	Version        string
	TypstRunner    toolchain.TypstRunner
	ReleaseFetcher toolchain.ReleaseFetcher
	LookPath       func(string) (string, error)
	GOOS           string
	GOARCH         string
}

// DefaultOptions constructs Options populated from the current OS environment.
func DefaultOptions() Options {
	workDir, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()
	cacheBase, err := os.UserCacheDir()
	if err != nil || cacheBase == "" {
		if homeDir != "" {
			cacheBase = filepath.Join(homeDir, ".cache")
		}
	}
	var cacheDir string
	if cacheBase != "" {
		cacheDir = filepath.Join(cacheBase, "datafolio")
	}
	return Options{
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
		WorkDir:        workDir,
		HomeDir:        homeDir,
		CacheDir:       cacheDir,
		Version:        version.Version,
		TypstRunner:    toolchain.RealTypstRunner{},
		ReleaseFetcher: &toolchain.HTTPReleaseFetcher{},
		LookPath:       exec.LookPath,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
	}
}

func (o Options) normalized() Options {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	if o.WorkDir == "" {
		o.WorkDir, _ = os.Getwd()
	}
	if o.CacheDir == "" && o.HomeDir != "" {
		o.CacheDir = filepath.Join(o.HomeDir, ".cache", "datafolio")
	}
	if o.Version == "" {
		o.Version = version.Version
	}
	if o.TypstRunner == nil {
		o.TypstRunner = toolchain.RealTypstRunner{}
	}
	if o.ReleaseFetcher == nil {
		o.ReleaseFetcher = &toolchain.HTTPReleaseFetcher{}
	}
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	return o
}

// Run executes the datafolio CLI for the provided arguments and environment options.
func Run(ctx context.Context, args []string, opts Options) int {
	opts = opts.normalized()

	if len(args) == 0 {
		printUsage(opts.Stderr)
		return ExitUsage
	}

	switch args[0] {
	case "help", "--help", "-h":
		printUsage(opts.Stdout)
		return ExitSuccess
	case "version", "--version", "-v":
		fmt.Fprintf(opts.Stdout, "datafolio %s\n", opts.Version)
		return ExitSuccess
	case "init":
		return runInit(ctx, args[1:], opts)
	case "build":
		return runBuild(ctx, args[1:], opts)
	case "tokens":
		return runTokens(ctx, args[1:], opts)
	case "review":
		return runReview(ctx, args[1:], opts)
	case "skill":
		return runSkill(ctx, args[1:], opts)
	case "doctor":
		return runDoctor(ctx, args[1:], opts)
	default:
		fmt.Fprintf(opts.Stderr, "unknown command %q\n\n", args[0])
		printUsage(opts.Stderr)
		return ExitUsage
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `datafolio - editorial publishing, token governance, and visual review CLI

Usage:
  datafolio <command> [flags] [arguments]

Commands:
  init      Scaffold datafolio.yaml and optional Token Contract
  build     Compile a Source Document to document, slides, or html
  tokens    Validate and export W3C DTCG Token Contracts
  review    Run deterministic visual and editorial Review checks
  skill     Install or inspect the embedded Skill Bundle
  doctor    Verify Typst toolchain and bundled OFL font health
  version   Print the datafolio version`)
}
