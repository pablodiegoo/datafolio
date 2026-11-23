package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"gopkg.in/yaml.v3"
)

func TestInitScaffoldsProjectAndRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()

	code, out, errOut := runCLI(t, dir, "init")
	if code != cli.ExitSuccess {
		t.Fatalf("init failed with %d: stdout=%s stderr=%s", code, out, errOut)
	}

	cfgPath := filepath.Join(dir, "datafolio.yaml")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("expected datafolio.yaml to be created: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("datafolio.yaml is invalid YAML: %v\n%s", err, string(raw))
	}
	if parsed["school"] != "tufte" || parsed["theme"] != "paper" || parsed["lang"] != "en" {
		t.Fatalf("unexpected datafolio.yaml defaults: %#v", parsed)
	}

	if _, err := os.Stat(filepath.Join(dir, "tokens.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected tokens.yaml not to be created without --tokens")
	}

	custom := []byte("school: tufte\ntheme: night\n")
	if err := os.WriteFile(cfgPath, custom, 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut = runCLI(t, dir, "init")
	if code != cli.ExitError {
		t.Fatalf("expected exit %d when refusing overwrite without --force, got %d (stderr: %s)", cli.ExitError, code, errOut)
	}
	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(custom) {
		t.Fatalf("existing datafolio.yaml was modified without --force: got %q", string(after))
	}

	code, _, errOut = runCLI(t, dir, "init", "--force")
	if code != cli.ExitSuccess {
		t.Fatalf("init --force failed with %d: %s", code, errOut)
	}
	forced, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(forced) == string(custom) {
		t.Fatalf("expected --force to overwrite datafolio.yaml")
	}
}

func TestInitWithTokensAndBilingualLang(t *testing.T) {
	dir := t.TempDir()

	code, _, errOut := runCLI(t, dir, "init", "--tokens", "--school", "tufte", "--lang", "pt-BR")
	if code != cli.ExitSuccess {
		t.Fatalf("init --tokens failed with %d: %s", code, errOut)
	}

	cfgBytes, err := os.ReadFile(filepath.Join(dir, "datafolio.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["lang"] != "pt-BR" || cfg["school"] != "tufte" || cfg["tokens"] != "tokens.yaml" {
		t.Fatalf("unexpected datafolio.yaml with --tokens: %#v", cfg)
	}

	tokBytes, err := os.ReadFile(filepath.Join(dir, "tokens.yaml"))
	if err != nil {
		t.Fatalf("expected tokens.yaml to be scaffolded: %v", err)
	}
	tokStr := string(tokBytes)
	if !strings.Contains(tokStr, "$value") || !strings.Contains(tokStr, "#fffff8") || !strings.Contains(tokStr, "#8b1e1e") {
		t.Fatalf("tokens.yaml missing expected DTCG Archival Carmine tokens:\n%s", tokStr)
	}
}

func TestInitAtomicRefusalWhenTokensExistsAndUsageErrors(t *testing.T) {
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "tokens.yaml")
	if err := os.WriteFile(tokPath, []byte("keep: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, _ := runCLI(t, dir, "init", "--tokens")
	if code != cli.ExitError {
		t.Fatalf("expected exit %d when tokens.yaml already exists, got %d", cli.ExitError, code)
	}
	if _, err := os.Stat(filepath.Join(dir, "datafolio.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected atomic refusal: datafolio.yaml should not be written if tokens.yaml exists")
	}

	for _, args := range [][]string{
		{"init", "--school", "unknown"},
		{"init", "--lang", "fr"},
		{"init", "--bogus"},
	} {
		code, _, _ := runCLI(t, dir, args...)
		if code != cli.ExitUsage {
			t.Fatalf("args %v: expected exit %d, got %d", args, cli.ExitUsage, code)
		}
	}
}
