package review

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/config"
	"github.com/pablodiegoo/datafolio/internal/school"
	"github.com/pablodiegoo/datafolio/internal/tokens"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

// UsageError indicates invalid Review arguments, unknown School, or missing/malformed configuration/target paths.
type UsageError struct {
	Err error
}

func (e *UsageError) Error() string {
	return e.Err.Error()
}

func (e *UsageError) Unwrap() error {
	return e.Err
}

func usageErrorf(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

// Options configures a static Review invocation.
type Options struct {
	Paths       []string
	TokensPath  string
	School      string
	WorkDir     string
	CacheDir    string
	TypstRunner toolchain.TypstRunner
	LookPath    func(string) (string, error)
	GOOS        string
	GOARCH      string
}

type fileKind int

const (
	kindIgnore fileKind = iota
	kindMarkdown
	kindTokenContract
	kindWeb
)

// Review statically audits all target Markdown Source Documents, Token Contracts, and web files.
func Review(ctx context.Context, opts Options) (Report, error) {
	opts = normalizeOptions(opts)

	if len(opts.Paths) == 0 {
		return Report{}, usageErrorf("expected at least one target path")
	}

	var schoolOverride school.School
	if strings.TrimSpace(opts.School) != "" {
		s, err := school.Lookup(opts.School)
		if err != nil {
			return Report{}, usageErrorf("%v", err)
		}
		schoolOverride = s
	}

	var report Report
	audited := make(map[string]bool)
	contractCache := make(map[string]*tokens.Contract)

	var explicitTokensPath string
	var explicitContract *tokens.Contract
	if strings.TrimSpace(opts.TokensPath) != "" {
		absTok, err := resolveExistingFile(opts.WorkDir, opts.TokensPath, "tokens")
		if err != nil {
			return Report{}, err
		}
		contract, err := tokens.ParseFile(absTok)
		if err != nil {
			return Report{}, usageErrorf("%v", err)
		}
		explicitTokensPath = absTok
		explicitContract = contract
		contractCache[absTok] = contract
		s, err := resolveTokenSchool(absTok, schoolOverride)
		if err != nil {
			return Report{}, err
		}
		findings, err := reviewTokenContract(absTok, displayPath(opts.WorkDir, absTok), contract, s.Policy())
		if err != nil {
			return Report{}, err
		}
		report.Findings = append(report.Findings, findings...)
		report.Files++
		audited[absTok] = true
	}

	for _, rawTarget := range opts.Paths {
		targetAbs := resolveWorkPath(opts.WorkDir, rawTarget)
		info, err := os.Stat(targetAbs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return Report{}, usageErrorf("target path does not exist: %s", rawTarget)
			}
			return Report{}, usageErrorf("stat %s: %v", rawTarget, err)
		}

		if !info.IsDir() {
			if err := auditTargetFile(ctx, targetAbs, true, opts, schoolOverride, explicitTokensPath, explicitContract, contractCache, audited, &report); err != nil {
				return Report{}, err
			}
			continue
		}

		walkErr := filepath.WalkDir(targetAbs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				base := d.Name()
				if path != targetAbs && (strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			return auditTargetFile(ctx, path, false, opts, schoolOverride, explicitTokensPath, explicitContract, contractCache, audited, &report)
		})
		if walkErr != nil {
			return Report{}, walkErr
		}
	}

	report.Finalize()
	return report, nil
}

func normalizeOptions(opts Options) Options {
	if opts.WorkDir == "" {
		opts.WorkDir, _ = os.Getwd()
	}
	if opts.TypstRunner == nil {
		opts.TypstRunner = toolchain.RealTypstRunner{}
	}
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	return opts
}

func auditTargetFile(
	ctx context.Context,
	absPath string,
	explicit bool,
	opts Options,
	schoolOverride school.School,
	explicitTokensPath string,
	explicitContract *tokens.Contract,
	contractCache map[string]*tokens.Contract,
	audited map[string]bool,
	report *Report,
) error {
	cleanAbs := filepath.Clean(absPath)
	if resolved, err := filepath.Abs(cleanAbs); err == nil {
		cleanAbs = resolved
	}
	if audited[cleanAbs] {
		return nil
	}

	kind := classifyTargetFile(cleanAbs, explicit)
	if kind == kindIgnore {
		return nil
	}
	audited[cleanAbs] = true
	disp := displayPath(opts.WorkDir, cleanAbs)

	switch kind {
	case kindTokenContract:
		contract, err := tokens.ParseFile(cleanAbs)
		if err != nil {
			if !explicit {
				lowerBase := strings.ToLower(filepath.Base(cleanAbs))
				if lowerBase == "design.md" && strings.Contains(err.Error(), "no design_tokens or token YAML blocks found") {
					return nil
				}
				if !strings.Contains(lowerBase, "token") && strings.Contains(err.Error(), "cannot unmarshal array") {
					return nil
				}
			}
			return usageErrorf("%v", err)
		}
		if !explicit && len(contract.Tokens) == 0 {
			return nil
		}
		s, err := resolveTokenSchool(cleanAbs, schoolOverride)
		if err != nil {
			return err
		}
		findings, err := reviewTokenContract(cleanAbs, disp, contract, s.Policy())
		if err != nil {
			return err
		}
		report.Findings = append(report.Findings, findings...)
		report.Files++
		return nil

	case kindMarkdown:
		findings, err := reviewMarkdownFile(ctx, cleanAbs, disp, opts, explicitTokensPath)
		if err != nil {
			return err
		}
		report.Findings = append(report.Findings, findings...)
		report.Files++
		return nil

	case kindWeb:
		report.Files++
		s, contract, err := resolveWebSchoolAndContract(cleanAbs, schoolOverride, explicitContract, contractCache)
		if err != nil {
			return err
		}
		findings, err := reviewWebFile(ctx, cleanAbs, disp, s, contract)
		if err != nil {
			return err
		}
		report.Findings = append(report.Findings, findings...)
		return nil
	}
	return nil
}

func classifyTargetFile(path string, explicit bool) fileKind {
	lowerPath := strings.ToLower(path)
	lowerBase := strings.ToLower(filepath.Base(path))

	if strings.HasSuffix(lowerPath, ".blade.php") {
		return kindWeb
	}
	ext := strings.ToLower(filepath.Ext(lowerBase))
	switch ext {
	case ".html", ".htm", ".templ", ".css", ".scss":
		return kindWeb
	case ".md", ".markdown":
		if lowerBase == "design.md" {
			return kindTokenContract
		}
		return kindMarkdown
	case ".yaml", ".yml", ".json":
		if !explicit && nonTokenConfigFiles[lowerBase] {
			return kindIgnore
		}
		return kindTokenContract
	default:
		return kindIgnore
	}
}

var nonTokenConfigFiles = map[string]bool{
	"datafolio.yaml":           true,
	"datafolio.yml":            true,
	".datafolio-manifest.json": true,
	"package.json":             true,
	"package-lock.json":        true,
	"tsconfig.json":            true,
	"composer.json":            true,
	"composer.lock":            true,
}

func resolveTokenSchool(filePath string, schoolOverride school.School) (school.School, error) {
	s, _, err := resolveProjectSchoolAndTokensPath(filePath, schoolOverride)
	return s, err
}

func resolveWebSchoolAndContract(
	filePath string,
	schoolOverride school.School,
	explicitContract *tokens.Contract,
	contractCache map[string]*tokens.Contract,
) (school.School, *tokens.Contract, error) {
	s, tokPath, err := resolveProjectSchoolAndTokensPath(filePath, schoolOverride)
	if err != nil {
		return nil, nil, err
	}
	if explicitContract != nil {
		return s, explicitContract, nil
	}
	if tokPath == "" {
		return s, nil, nil
	}
	if cached, ok := contractCache[tokPath]; ok {
		return s, cached, nil
	}
	contract, err := tokens.ParseFile(tokPath)
	if err != nil {
		return nil, nil, usageErrorf("%v", err)
	}
	contractCache[tokPath] = contract
	return s, contract, nil
}

func resolveProjectSchoolAndTokensPath(filePath string, schoolOverride school.School) (school.School, string, error) {
	projectOv, _, err := config.DiscoverUpward(filepath.Dir(filePath))
	if err != nil {
		return nil, "", usageErrorf("%v", err)
	}

	var tokPath string
	if projectOv.Tokens != nil && strings.TrimSpace(*projectOv.Tokens) != "" {
		tokPath = resolveWorkPath(projectOv.BaseDir, *projectOv.Tokens)
	}

	if schoolOverride != nil {
		return schoolOverride, tokPath, nil
	}
	if projectOv.School != nil && strings.TrimSpace(*projectOv.School) != "" {
		s, err := school.Lookup(*projectOv.School)
		if err != nil {
			return nil, "", usageErrorf("%v", err)
		}
		return s, tokPath, nil
	}
	s, err := school.Lookup("tufte")
	if err != nil {
		return nil, "", err
	}
	return s, tokPath, nil
}

func resolveExistingFile(workDir, p, flagName string) (string, error) {
	abs := resolveWorkPath(workDir, p)
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", usageErrorf("--%s file does not exist: %s", flagName, p)
		}
		return "", usageErrorf("--%s %s: %v", flagName, p, err)
	}
	if info.IsDir() {
		return "", usageErrorf("--%s path is a directory, expected file: %s", flagName, p)
	}
	return abs, nil
}

func resolveWorkPath(workDir, p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		abs, err := filepath.Abs(trimmed)
		if err == nil {
			return abs
		}
		return filepath.Clean(trimmed)
	}
	joined := filepath.Join(workDir, trimmed)
	if abs, err := filepath.Abs(joined); err == nil {
		return abs
	}
	return filepath.Clean(joined)
}

func displayPath(workDir, absPath string) string {
	if workDir != "" {
		if absWork, err := filepath.Abs(workDir); err == nil {
			if rel, err := filepath.Rel(absWork, absPath); err == nil && rel != "" && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
				return filepath.ToSlash(rel)
			}
		}
	}
	return absPath
}
