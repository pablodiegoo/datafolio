package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/skill"
)

func runSkillCLI(t *testing.T, workDir, homeDir, ver string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := cli.Options{
		Stdout:   &stdout,
		Stderr:   &stderr,
		WorkDir:  workDir,
		HomeDir:  homeDir,
		CacheDir: filepath.Join(homeDir, ".cache", "datafolio"),
		Version:  ver,
		LookPath: func(string) (string, error) { return "", errors.New("disabled in test") },
	}
	code := cli.Run(context.Background(), args, opts)
	return code, stdout.String(), stderr.String()
}

func TestSkillUsageHelpAndInvalidArguments(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	homeDir := t.TempDir()

	for _, args := range [][]string{
		{"skill", "help"},
		{"skill", "--help"},
		{"skill", "-h"},
		{"skill", "install", "--help"},
		{"skill", "status", "--help"},
	} {
		code, out, errOut := runSkillCLI(t, workDir, homeDir, "0.1.0-test", args...)
		if code != cli.ExitSuccess {
			t.Fatalf("args %v: got exit %d, want %d (stderr=%s)", args, code, cli.ExitSuccess, errOut)
		}
		if !strings.Contains(out, "skill") {
			t.Fatalf("args %v: expected usage output on stdout, got %q", args, out)
		}
	}

	for _, args := range [][]string{
		{"skill"},
		{"skill", "bogus"},
		{"skill", "install", "--target", "bogus"},
		{"skill", "install", "--school", "unknown"},
		{"skill", "install", "--bogus-flag"},
		{"skill", "install", "unexpected-arg"},
		{"skill", "status", "--target", "bogus"},
		{"skill", "status", "--bogus-flag"},
		{"skill", "status", "unexpected-arg"},
	} {
		code, _, errOut := runSkillCLI(t, workDir, homeDir, "0.1.0-test", args...)
		if code != cli.ExitUsage {
			t.Fatalf("args %v: got exit %d, want %d (stderr=%s)", args, code, cli.ExitUsage, errOut)
		}
		if strings.TrimSpace(errOut) == "" {
			t.Fatalf("args %v: expected diagnostic on stderr", args)
		}
	}
}

func TestSkillInstallDefaultBundleAndPEP723Validation(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	homeDir := t.TempDir()

	code, out, errOut := runSkillCLI(t, workDir, homeDir, "0.1.0-test", "skill", "install")
	if code != cli.ExitSuccess {
		t.Fatalf("skill install failed (%d): stdout=%s stderr=%s", code, out, errOut)
	}

	installDir := filepath.Join(workDir, ".agents", "skills", "datafolio")
	if !strings.Contains(out, installDir) || !strings.Contains(out, "tufte") {
		t.Errorf("install stdout missing path or school:\n%s", out)
	}

	expectedFiles := []string{
		"SKILL.md",
		"references/markdown-grammar.md",
		"references/tufte-school.md",
		"references/token-contract.md",
		"scripts/plot_tufte.py",
		skill.ManifestFilename,
	}
	for _, rel := range expectedFiles {
		p := filepath.Join(installDir, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected installed file %s: %v", p, err)
		}
	}

	// Verify manifest fields.
	manifestRaw, err := os.ReadFile(filepath.Join(installDir, skill.ManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	var manifest skill.Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("invalid manifest JSON: %v", err)
	}
	if manifest.Version != "0.1.0-test" || manifest.School != "tufte" || manifest.Target != "agents" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}

	// Verify PEP 723 metadata validity in emitted plot_tufte.py.
	pyPath := filepath.Join(installDir, "scripts", "plot_tufte.py")
	pyBytes, err := os.ReadFile(pyPath)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := skill.ParsePEP723(pyBytes)
	if err != nil {
		t.Fatalf("emitted plot_tufte.py has invalid PEP 723 header: %v", err)
	}
	if meta.RequiresPython == "" || len(meta.Dependencies) == 0 {
		t.Fatalf("incomplete PEP 723 metadata: %+v", meta)
	}

	// Syntax check without executing matplotlib when python3 is present on host.
	if pyBin, lookErr := exec.LookPath("python3"); lookErr == nil {
		pycacheDir := t.TempDir()
		cmd := exec.Command(pyBin, "-m", "py_compile", pyPath)
		cmd.Env = append(os.Environ(), "PYTHONPYCACHEPREFIX="+pycacheDir)
		if compileOut, compErr := cmd.CombinedOutput(); compErr != nil {
			t.Fatalf("python3 -m py_compile failed on %s: %v\n%s", pyPath, compErr, string(compileOut))
		}
	}

	// Verify HomeDir was never touched during local install.
	if entries, err := os.ReadDir(homeDir); err != nil || len(entries) != 0 {
		t.Fatalf("local install must not touch HomeDir, found %v (err=%v)", entries, err)
	}
}

func TestSkillInstallTargetsAndGlobalPathsThroughRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		args      []string
		global    bool
		wantPaths func(workDir, homeDir string) []string
	}{
		{
			name: "local claude",
			args: []string{"skill", "install", "--target", "claude", "--school", "tufte"},
			wantPaths: func(w, _ string) []string {
				return []string{filepath.Join(w, ".claude", "skills", "datafolio")}
			},
		},
		{
			name: "local gemini",
			args: []string{"skill", "install", "--target", "gemini"},
			wantPaths: func(w, _ string) []string {
				return []string{filepath.Join(w, ".gemini", "skills", "datafolio")}
			},
		},
		{
			name: "local all",
			args: []string{"skill", "install", "--target", "all"},
			wantPaths: func(w, _ string) []string {
				return []string{
					filepath.Join(w, ".agents", "skills", "datafolio"),
					filepath.Join(w, ".claude", "skills", "datafolio"),
					filepath.Join(w, ".gemini", "skills", "datafolio"),
				}
			},
		},
		{
			name:   "global agents",
			args:   []string{"skill", "install", "--global"},
			global: true,
			wantPaths: func(_, h string) []string {
				return []string{filepath.Join(h, ".agents", "skills", "datafolio")}
			},
		},
		{
			name:   "global claude",
			args:   []string{"skill", "install", "--target", "claude", "--global"},
			global: true,
			wantPaths: func(_, h string) []string {
				return []string{filepath.Join(h, ".claude", "skills", "datafolio")}
			},
		},
		{
			name:   "global gemini",
			args:   []string{"skill", "install", "--target", "gemini", "--global"},
			global: true,
			wantPaths: func(_, h string) []string {
				return []string{filepath.Join(h, ".gemini", "config", "skills", "datafolio")}
			},
		},
		{
			name:   "global all",
			args:   []string{"skill", "install", "--target", "all", "--global"},
			global: true,
			wantPaths: func(_, h string) []string {
				return []string{
					filepath.Join(h, ".agents", "skills", "datafolio"),
					filepath.Join(h, ".claude", "skills", "datafolio"),
					filepath.Join(h, ".gemini", "config", "skills", "datafolio"),
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			homeDir := t.TempDir()

			code, out, errOut := runSkillCLI(t, workDir, homeDir, "0.1.0-test", tc.args...)
			if code != cli.ExitSuccess {
				t.Fatalf("args %v failed (%d): stdout=%s stderr=%s", tc.args, code, out, errOut)
			}

			for _, wantDir := range tc.wantPaths(workDir, homeDir) {
				if _, err := os.Stat(filepath.Join(wantDir, "SKILL.md")); err != nil {
					t.Errorf("missing SKILL.md at %s: %v", wantDir, err)
				}
				if _, err := os.Stat(filepath.Join(wantDir, skill.ManifestFilename)); err != nil {
					t.Errorf("missing manifest at %s: %v", wantDir, err)
				}
			}

			if tc.global {
				if entries, _ := os.ReadDir(workDir); len(entries) != 0 {
					t.Errorf("global install wrote into WorkDir: %v", entries)
				}
			} else {
				if entries, _ := os.ReadDir(homeDir); len(entries) != 0 {
					t.Errorf("local install wrote into HomeDir: %v", entries)
				}
			}
		})
	}
}

func TestSkillInstallOverwriteSafetyAndForceThroughRun(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	homeDir := t.TempDir()

	code, _, errOut := runSkillCLI(t, workDir, homeDir, "0.1.0-test", "skill", "install")
	if code != cli.ExitSuccess {
		t.Fatalf("initial install failed: %s", errOut)
	}

	skillPath := filepath.Join(workDir, ".agents", "skills", "datafolio", "SKILL.md")
	edited := []byte("# User customized skill\n")
	if err := os.WriteFile(skillPath, edited, 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut = runSkillCLI(t, workDir, homeDir, "0.2.0-test", "skill", "install")
	if code != cli.ExitError {
		t.Fatalf("expected ExitError (%d) without --force, got %d (stderr=%s)", cli.ExitError, code, errOut)
	}
	if !strings.Contains(errOut, "--force") {
		t.Errorf("expected stderr to mention --force, got %q", errOut)
	}

	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(edited) {
		t.Fatalf("modified SKILL.md was overwritten without --force")
	}

	code, _, errOut = runSkillCLI(t, workDir, homeDir, "0.2.0-test", "skill", "install", "--force")
	if code != cli.ExitSuccess {
		t.Fatalf("install --force failed (%d): %s", code, errOut)
	}
	restored, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) == string(edited) {
		t.Fatalf("expected --force to restore embedded SKILL.md")
	}
}

func TestSkillStatusAndDriftDetectionThroughRun(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	homeDir := t.TempDir()

	// Not installed -> exit 1 + install hint.
	code, _, errOut := runSkillCLI(t, workDir, homeDir, "0.1.0-test", "skill", "status")
	if code != cli.ExitError {
		t.Fatalf("status before install: got %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(errOut, "datafolio skill install") {
		t.Errorf("expected install hint in stderr, got %q", errOut)
	}

	// Install at 0.1.0-test -> status at 0.1.0-test succeeds.
	code, _, errOut = runSkillCLI(t, workDir, homeDir, "0.1.0-test", "skill", "install")
	if code != cli.ExitSuccess {
		t.Fatalf("install failed: %s", errOut)
	}

	installDir := filepath.Join(workDir, ".agents", "skills", "datafolio")
	code, out, errOut := runSkillCLI(t, workDir, homeDir, "0.1.0-test", "skill", "status")
	if code != cli.ExitSuccess {
		t.Fatalf("status after install failed (%d): stdout=%s stderr=%s", code, out, errOut)
	}
	for _, want := range []string{installDir, "tufte", "0.1.0-test"} {
		if !strings.Contains(out, want) {
			t.Errorf("status stdout missing %q:\n%s", want, out)
		}
	}

	// Upgrade binary to 0.2.0-test -> status detects drift, exits 1, shows both versions and upgrade hint.
	code, out, errOut = runSkillCLI(t, workDir, homeDir, "0.2.0-test", "skill", "status")
	if code != cli.ExitError {
		t.Fatalf("status on version drift: got %d, want %d (stdout=%s)", code, cli.ExitError, out)
	}
	for _, want := range []string{installDir, "tufte", "0.1.0-test", "0.2.0-test", "drift"} {
		if !strings.Contains(out, want) {
			t.Errorf("drift stdout missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, "datafolio skill install --force") {
		t.Errorf("drift stderr missing upgrade hint 'datafolio skill install --force': %q", errOut)
	}

	// JSON status output also reflects drift clearly.
	code, jsonOut, _ := runSkillCLI(t, workDir, homeDir, "0.2.0-test", "skill", "status", "--json")
	if code != cli.ExitError {
		t.Fatalf("status --json on drift: got %d, want %d", code, cli.ExitError)
	}
	var report skill.StatusReport
	if err := json.Unmarshal([]byte(jsonOut), &report); err != nil {
		t.Fatalf("invalid status --json output: %v\n%s", err, jsonOut)
	}
	if report.Healthy || len(report.Targets) != 1 || !report.Targets[0].VersionDrift {
		t.Fatalf("unexpected JSON drift report: %+v", report)
	}

	// Install --force at 0.2.0-test resolves drift -> status returns ExitSuccess.
	code, _, errOut = runSkillCLI(t, workDir, homeDir, "0.2.0-test", "skill", "install", "--force")
	if code != cli.ExitSuccess {
		t.Fatalf("upgrade install --force failed: %s", errOut)
	}
	code, out, errOut = runSkillCLI(t, workDir, homeDir, "0.2.0-test", "skill", "status")
	if code != cli.ExitSuccess {
		t.Fatalf("status after upgrade failed (%d): stdout=%s stderr=%s", code, out, errOut)
	}

	// Local file edit -> status flags modified file and exits 1.
	if err := os.WriteFile(filepath.Join(installDir, "references", "tufte-school.md"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = runSkillCLI(t, workDir, homeDir, "0.2.0-test", "skill", "status")
	if code != cli.ExitError {
		t.Fatalf("status on modified file: got %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(out, "references/tufte-school.md") {
		t.Errorf("status stdout missing modified file name:\n%s", out)
	}
}
