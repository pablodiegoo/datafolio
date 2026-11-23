package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/assets/fonts"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

type doctorReport struct {
	Healthy bool              `json:"healthy"`
	Typst   doctorTypstStatus `json:"typst"`
	Fonts   doctorFontsStatus `json:"fonts"`
	Cache   doctorCacheStatus `json:"cache"`
}

type doctorTypstStatus struct {
	Healthy       bool   `json:"healthy"`
	Source        string `json:"source"`
	Path          string `json:"path,omitempty"`
	Version       string `json:"version,omitempty"`
	PinnedVersion string `json:"pinned_version"`
	Installed     bool   `json:"installed,omitempty"`
	Error         string `json:"error,omitempty"`
}

type doctorFontsStatus struct {
	Healthy   bool     `json:"healthy"`
	Path      string   `json:"path,omitempty"`
	Families  []string `json:"families"`
	FileCount int      `json:"file_count"`
	Error     string   `json:"error,omitempty"`
}

type doctorCacheStatus struct {
	Healthy bool   `json:"healthy"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

func runDoctor(ctx context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		installTypst bool
		asJSON       bool
	)
	fs.BoolVar(&installTypst, "install-typst", false, "download and verify the pinned Typst release into the managed cache")
	fs.BoolVar(&asJSON, "json", false, "emit machine-readable JSON diagnostics")

	pos, err := parsePositionalFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printDoctorUsage(opts.Stdout)
			return ExitSuccess
		}
		fmt.Fprintf(opts.Stderr, "doctor: %v\n", err)
		return ExitUsage
	}
	if len(pos) > 0 {
		fmt.Fprintf(opts.Stderr, "doctor: unexpected argument %q\n", pos[0])
		return ExitUsage
	}

	report := diagnoseToolchain(ctx, opts, installTypst)

	if asJSON {
		enc := json.NewEncoder(opts.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(opts.Stderr, "doctor: encode json: %v\n", err)
			return ExitError
		}
	} else {
		printDoctorReport(opts.Stdout, report)
	}

	if report.Healthy {
		return ExitSuccess
	}
	writeDoctorErrorHint(opts.Stderr, report)
	return ExitError
}

func diagnoseToolchain(ctx context.Context, opts Options, installTypst bool) doctorReport {
	fontsStatus := checkBundledFonts(opts.CacheDir)
	cacheStatus := checkCacheHealth(opts.CacheDir)
	typstStatus := checkTypstStatus(ctx, opts, installTypst)

	return doctorReport{
		Healthy: typstStatus.Healthy && fontsStatus.Healthy && cacheStatus.Healthy,
		Typst:   typstStatus,
		Fonts:   fontsStatus,
		Cache:   cacheStatus,
	}
}

func checkBundledFonts(cacheDir string) doctorFontsStatus {
	families := fonts.Families()
	names := make([]string, 0, len(families))
	fileCount := 0
	for _, fam := range families {
		names = append(names, fam.Name)
		fileCount += len(fam.Files) + 1 // TTF files + OFL notice
	}

	status := doctorFontsStatus{
		Families:  names,
		FileCount: fileCount,
	}
	fontPath, err := toolchain.EnsureFonts(cacheDir)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Healthy = true
	status.Path = fontPath
	return status
}

func checkCacheHealth(cacheDir string) doctorCacheStatus {
	status := doctorCacheStatus{Path: cacheDir}
	if strings.TrimSpace(cacheDir) == "" {
		status.Error = "cache directory is not configured"
		return status
	}
	info, err := os.Stat(cacheDir)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	if !info.IsDir() {
		status.Error = fmt.Sprintf("%s is not a directory", cacheDir)
		return status
	}
	probe, err := os.CreateTemp(cacheDir, ".doctor-probe-*")
	if err != nil {
		status.Error = fmt.Sprintf("cache directory not writable: %v", err)
		return status
	}
	probeName := probe.Name()
	probe.Close()
	_ = os.Remove(probeName)

	status.Healthy = true
	return status
}

func checkTypstStatus(ctx context.Context, opts Options, installTypst bool) doctorTypstStatus {
	status := doctorTypstStatus{
		Source:        toolchain.TypstSourceMissing,
		PinnedVersion: toolchain.PinnedTypstVersion,
	}

	res := toolchain.ResolveTypst(opts.LookPath, opts.CacheDir, opts.GOOS, opts.GOARCH)
	if !res.Found && installTypst {
		if _, err := toolchain.InstallTypst(ctx, opts.ReleaseFetcher, opts.CacheDir, opts.GOOS, opts.GOARCH); err != nil {
			status.Error = err.Error()
			return status
		}
		status.Installed = true
		res = toolchain.ResolveTypst(opts.LookPath, opts.CacheDir, opts.GOOS, opts.GOARCH)
	}

	if !res.Found {
		status.Error = fmt.Sprintf("typst not found on PATH or in %s (run \"datafolio doctor --install-typst\" to install %s)",
			toolchain.CachedTypstPath(opts.CacheDir, opts.GOOS), toolchain.PinnedTypstVersion)
		return status
	}

	status.Source = res.Source
	status.Path = res.Path

	ver, err := opts.TypstRunner.Version(ctx, res.Path)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Version = ver
	status.Healthy = true
	return status
}

func printDoctorReport(w io.Writer, r doctorReport) {
	fmt.Fprintln(w, "DataFolio Toolchain Doctor")

	if r.Typst.Healthy {
		fmt.Fprintf(w, "  typst:  %s (%s: %s)\n", r.Typst.Version, r.Typst.Source, r.Typst.Path)
	} else if r.Typst.Path != "" {
		fmt.Fprintf(w, "  typst:  unhealthy (%s: %s) — %s\n", r.Typst.Source, r.Typst.Path, r.Typst.Error)
	} else {
		fmt.Fprintf(w, "  typst:  %s (pinned %s)\n", r.Typst.Source, r.Typst.PinnedVersion)
	}

	if r.Fonts.Healthy {
		fmt.Fprintf(w, "  fonts:  ok — %s (%d assets in %s)\n",
			strings.Join(r.Fonts.Families, ", "), r.Fonts.FileCount, r.Fonts.Path)
	} else {
		fmt.Fprintf(w, "  fonts:  error — %s\n", r.Fonts.Error)
	}

	if r.Cache.Healthy {
		fmt.Fprintf(w, "  cache:  ok (%s)\n", r.Cache.Path)
	} else {
		fmt.Fprintf(w, "  cache:  error — %s\n", r.Cache.Error)
	}
}

func writeDoctorErrorHint(w io.Writer, r doctorReport) {
	for _, errMsg := range []string{r.Typst.Error, r.Fonts.Error, r.Cache.Error} {
		if errMsg != "" {
			fmt.Fprintf(w, "doctor: %s\n", errMsg)
			return
		}
	}
}

func printDoctorUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio doctor [flags]

Flags:
  --install-typst   Download and SHA-256 verify pinned Typst into the managed cache
  --json            Emit machine-readable JSON diagnostics`)
}
