package toolchain_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

func writeFakeTypstScript(t *testing.T, dir, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fake binary test")
	}
	binPath := filepath.Join(dir, "typst")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake typst script: %v", err)
	}
	return binPath
}

func TestRealTypstRunnerCompilePassesRootAndFontPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	argsLog := filepath.Join(dir, "args.txt")
	bin := writeFakeTypstScript(t, dir, `printf '%s\n' "$@" > "`+argsLog+`"`)

	runner := toolchain.RealTypstRunner{}
	req := toolchain.CompileRequest{
		TypstBin:   bin,
		InputPath:  filepath.Join(dir, "doc.typ"),
		OutputPath: filepath.Join(dir, "doc.pdf"),
		RootDir:    filepath.Join(dir, "workspace"),
		FontPath:   filepath.Join(dir, "fonts"),
	}
	if err := runner.Compile(context.Background(), req); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	want := []string{
		"compile",
		"--root", req.RootDir,
		"--font-path", req.FontPath,
		req.InputPath,
		req.OutputPath,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Compile argv mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestRealTypstRunnerCompileSurfacesStderrOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bin := writeFakeTypstScript(t, dir, `echo "error: unknown font" >&2; exit 1`)

	runner := toolchain.RealTypstRunner{}
	err := runner.Compile(context.Background(), toolchain.CompileRequest{
		TypstBin:   bin,
		InputPath:  "in.typ",
		OutputPath: "out.pdf",
	})
	if err == nil {
		t.Fatal("expected error from failing typst compile")
	}
	if !strings.Contains(err.Error(), "unknown font") {
		t.Fatalf("expected stderr detail in error, got %v", err)
	}
}

func TestRealTypstRunnerQueryPassesFlagsAndReturnsJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	argsLog := filepath.Join(dir, "query-args.txt")
	bin := writeFakeTypstScript(t, dir, `
printf '%s\n' "$@" > "`+argsLog+`"
printf '[{"slide":1,"page":1}]\n'
`)

	runner := toolchain.RealTypstRunner{}
	req := toolchain.QueryRequest{
		TypstBin:  bin,
		InputPath: filepath.Join(dir, "slides.typ"),
		Selector:  "<datafolio-slide>",
		RootDir:   filepath.Join(dir, "project"),
		FontPath:  filepath.Join(dir, "fonts"),
		Field:     "value",
		One:       true,
	}
	out, err := runner.Query(context.Background(), req)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if strings.TrimSpace(string(out)) != `[{"slide":1,"page":1}]` {
		t.Fatalf("unexpected query output: %q", string(out))
	}

	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read query args: %v", err)
	}
	got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	want := []string{
		"query",
		"--root", req.RootDir,
		"--font-path", req.FontPath,
		"--field", "value",
		"--one",
		req.InputPath,
		req.Selector,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Query argv mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestRealTypstRunnerVersionParsesSemver(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bin := writeFakeTypstScript(t, dir, `
if [ "${1:-}" = "--version" ]; then
  echo "typst 0.15.1 (8958f15c)"
  exit 0
fi
exit 2
`)

	runner := toolchain.RealTypstRunner{}
	ver, err := runner.Version(context.Background(), bin)
	if err != nil {
		t.Fatalf("Version failed: %v", err)
	}
	if ver != "0.15.1" {
		t.Fatalf("got version %q, want %q", ver, "0.15.1")
	}
}

func TestFakeTypstRunnerRecordsCallsDeterministically(t *testing.T) {
	t.Parallel()

	fake := &toolchain.FakeTypstRunner{
		VersionOutput: "0.15.1",
		QueryOutput:   []byte(`[{"value":{"index":1,"page":1}}]`),
	}
	var runner toolchain.TypstRunner = fake

	ver, err := runner.Version(context.Background(), "/cache/typst")
	if err != nil || ver != "0.15.1" {
		t.Fatalf("Version got (%q, %v), want (0.15.1, nil)", ver, err)
	}

	compileReq := toolchain.CompileRequest{
		TypstBin:   "/cache/typst",
		InputPath:  "doc.typ",
		OutputPath: "doc.pdf",
		RootDir:    "/work",
		FontPath:   "/fonts",
	}
	if err := runner.Compile(context.Background(), compileReq); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	queryReq := toolchain.QueryRequest{
		TypstBin:  "/cache/typst",
		InputPath: "slides.typ",
		Selector:  "<datafolio-slide>",
		Field:     "value",
	}
	qOut, err := runner.Query(context.Background(), queryReq)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if string(qOut) != `[{"value":{"index":1,"page":1}}]` {
		t.Fatalf("Query output mismatch: %q", string(qOut))
	}

	if !reflect.DeepEqual(fake.VersionCalls, []string{"/cache/typst"}) {
		t.Fatalf("VersionCalls = %v", fake.VersionCalls)
	}
	if !reflect.DeepEqual(fake.CompileCalls, []toolchain.CompileRequest{compileReq}) {
		t.Fatalf("CompileCalls = %v", fake.CompileCalls)
	}
	if !reflect.DeepEqual(fake.QueryCalls, []toolchain.QueryRequest{queryReq}) {
		t.Fatalf("QueryCalls = %v", fake.QueryCalls)
	}
}
