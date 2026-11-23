package toolchain

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// CompileRequest describes an invocation of `typst compile`.
type CompileRequest struct {
	TypstBin   string
	InputPath  string
	OutputPath string
	RootDir    string
	FontPath   string
}

// QueryRequest describes an invocation of `typst query`.
type QueryRequest struct {
	TypstBin  string
	InputPath string
	Selector  string
	RootDir   string
	FontPath  string
	Field     string
	One       bool
}

// TypstRunner abstracts external Typst CLI execution for compilation, metadata queries, and version probes.
type TypstRunner interface {
	Compile(ctx context.Context, req CompileRequest) error
	Query(ctx context.Context, req QueryRequest) ([]byte, error)
	Version(ctx context.Context, typstBin string) (string, error)
}

// RealTypstRunner executes the external `typst` binary via os/exec.
type RealTypstRunner struct{}

// Compile invokes `typst compile` with --root and --font-path when supplied.
func (RealTypstRunner) Compile(ctx context.Context, req CompileRequest) error {
	if strings.TrimSpace(req.InputPath) == "" {
		return fmt.Errorf("typst compile: input path is required")
	}
	if strings.TrimSpace(req.OutputPath) == "" {
		return fmt.Errorf("typst compile: output path is required")
	}

	args := buildTypstArgs("compile", req.RootDir, req.FontPath)
	args = append(args, req.InputPath, req.OutputPath)

	_, err := runTypstCommand(ctx, req.TypstBin, args)
	return err
}

// Query invokes `typst query` and returns its stdout JSON payload.
func (RealTypstRunner) Query(ctx context.Context, req QueryRequest) ([]byte, error) {
	if strings.TrimSpace(req.InputPath) == "" {
		return nil, fmt.Errorf("typst query: input path is required")
	}
	if strings.TrimSpace(req.Selector) == "" {
		return nil, fmt.Errorf("typst query: selector is required")
	}

	args := buildTypstArgs("query", req.RootDir, req.FontPath)
	if req.Field != "" {
		args = append(args, "--field", req.Field)
	}
	if req.One {
		args = append(args, "--one")
	}
	args = append(args, req.InputPath, req.Selector)

	return runTypstCommand(ctx, req.TypstBin, args)
}

// Version runs `typst --version` and returns the parsed version string.
func (RealTypstRunner) Version(ctx context.Context, typstBin string) (string, error) {
	out, err := runTypstCommand(ctx, typstBin, []string{"--version"})
	if err != nil {
		return "", err
	}
	ver := parseTypstVersion(string(out))
	if ver == "" {
		return "", fmt.Errorf("typst --version: empty version output")
	}
	return ver, nil
}

func buildTypstArgs(subcmd, rootDir, fontPath string) []string {
	args := []string{subcmd}
	if rootDir != "" {
		args = append(args, "--root", rootDir)
	}
	if fontPath != "" {
		args = append(args, "--font-path", fontPath)
	}
	return args
}

func runTypstCommand(ctx context.Context, typstBin string, args []string) ([]byte, error) {
	bin := strings.TrimSpace(typstBin)
	if bin == "" {
		bin = "typst"
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		sub := args[0]
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		if errMsg != "" {
			return nil, fmt.Errorf("typst %s: %w: %s", sub, err, errMsg)
		}
		return nil, fmt.Errorf("typst %s: %w", sub, err)
	}
	return stdout.Bytes(), nil
}

func parseTypstVersion(raw string) string {
	line := strings.TrimSpace(raw)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	if strings.EqualFold(fields[0], "typst") && len(fields) >= 2 {
		return strings.TrimPrefix(fields[1], "v")
	}
	return strings.TrimPrefix(fields[0], "v")
}

// FakeTypstRunner is a deterministic in-memory TypstRunner for offline tests.
type FakeTypstRunner struct {
	mu            sync.Mutex
	CompileCalls  []CompileRequest
	QueryCalls    []QueryRequest
	VersionCalls  []string
	CompileErr    error
	OnCompile     func(ctx context.Context, req CompileRequest) error
	QueryOutput   []byte
	QueryErr      error
	OnQuery       func(ctx context.Context, req QueryRequest) ([]byte, error)
	VersionOutput string
	VersionErr    error
}

// Compile records the CompileRequest and delegates to OnCompile or returns CompileErr.
func (f *FakeTypstRunner) Compile(ctx context.Context, req CompileRequest) error {
	f.mu.Lock()
	f.CompileCalls = append(f.CompileCalls, req)
	onCompile := f.OnCompile
	compileErr := f.CompileErr
	f.mu.Unlock()

	if onCompile != nil {
		return onCompile(ctx, req)
	}
	return compileErr
}

// Query records the QueryRequest and delegates to OnQuery or returns configured QueryOutput/QueryErr.
func (f *FakeTypstRunner) Query(ctx context.Context, req QueryRequest) ([]byte, error) {
	f.mu.Lock()
	f.QueryCalls = append(f.QueryCalls, req)
	onQuery := f.OnQuery
	out := append([]byte(nil), f.QueryOutput...)
	queryErr := f.QueryErr
	f.mu.Unlock()

	if onQuery != nil {
		return onQuery(ctx, req)
	}
	if queryErr != nil {
		return nil, queryErr
	}
	return out, nil
}

// Version records the typstBin argument and returns VersionOutput/VersionErr.
func (f *FakeTypstRunner) Version(_ context.Context, typstBin string) (string, error) {
	f.mu.Lock()
	f.VersionCalls = append(f.VersionCalls, typstBin)
	ver := f.VersionOutput
	verErr := f.VersionErr
	f.mu.Unlock()

	if verErr != nil {
		return "", verErr
	}
	if ver == "" {
		return strings.TrimPrefix(PinnedTypstVersion, "v"), nil
	}
	return ver, nil
}
