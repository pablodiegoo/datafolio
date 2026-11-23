package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
)

func runCLI(t *testing.T, workDir string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := cli.Options{
		Stdout:   &stdout,
		Stderr:   &stderr,
		WorkDir:  workDir,
		HomeDir:  workDir,
		CacheDir: workDir,
		Version:  "0.1.0-test",
	}
	code := cli.Run(context.Background(), args, opts)
	return code, stdout.String(), stderr.String()
}

func TestRunHelpAndVersion(t *testing.T) {
	dir := t.TempDir()

	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		code, out, errOut := runCLI(t, dir, args...)
		if code != cli.ExitSuccess {
			t.Fatalf("args %v: got exit %d, want %d (stderr: %s)", args, code, cli.ExitSuccess, errOut)
		}
		if !strings.Contains(out, "datafolio") || !strings.Contains(out, "init") || !strings.Contains(out, "build") {
			t.Fatalf("args %v: help output missing expected commands, got %q", args, out)
		}
	}

	for _, args := range [][]string{{"--version"}, {"version"}} {
		code, out, errOut := runCLI(t, dir, args...)
		if code != cli.ExitSuccess {
			t.Fatalf("args %v: got exit %d, want %d (stderr: %s)", args, code, cli.ExitSuccess, errOut)
		}
		if !strings.Contains(out, "0.1.0-test") {
			t.Fatalf("args %v: version output %q did not include injected version", args, out)
		}
	}
}

func TestRunUsageAndStubExitCodes(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "no args", args: nil, want: cli.ExitUsage},
		{name: "unknown subcommand", args: []string{"bogus"}, want: cli.ExitUsage},
		{name: "tokens stub", args: []string{"tokens"}, want: cli.ExitUsage},
		{name: "review stub", args: []string{"review"}, want: cli.ExitUsage},
		{name: "skill stub", args: []string{"skill"}, want: cli.ExitUsage},
		{name: "doctor stub", args: []string{"doctor"}, want: cli.ExitUsage},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runCLI(t, dir, tc.args...)
			if code != tc.want {
				t.Fatalf("args %v: got exit %d, want %d (stderr: %s)", tc.args, code, tc.want, errOut)
			}
			if strings.TrimSpace(errOut) == "" {
				t.Fatalf("args %v: expected diagnostic message on stderr", tc.args)
			}
		})
	}
}
