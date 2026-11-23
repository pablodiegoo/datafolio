package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/config"
	_ "github.com/pablodiegoo/datafolio/internal/school/tufte"
)

func TestChromeSpecPrecedenceAndFormatDefaults(t *testing.T) {
	// Default document format enables header and footer with skip_first: true.
	docCfg, _, _, err := config.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if !docCfg.Header.Enabled || !docCfg.Header.SkipFirst || !docCfg.Footer.Enabled || !docCfg.Footer.SkipFirst {
		t.Fatalf("expected document default Running Chrome enabled with skip_first=true, got header=%+v footer=%+v", docCfg.Header, docCfg.Footer)
	}

	// Default slides format disables header and footer.
	slidesFormat := "slides"
	slidesCfg, _, _, err := config.Resolve(config.Override{Format: &slidesFormat})
	if err != nil {
		t.Fatal(err)
	}
	if slidesCfg.Header.Enabled || slidesCfg.Footer.Enabled {
		t.Fatalf("expected slides default Running Chrome disabled, got header=%+v footer=%+v", slidesCfg.Header, slidesCfg.Footer)
	}

	// Merging object header across levels and boolean false disable.
	lvl1, err := config.ParseYAML([]byte(`
cover: true
header:
  left: "ACME Analytics"
  right: "{page}/{pages}"
  skip_first: false
  rule: true
footer:
  center: "Confidential"
`), "")
	if err != nil {
		t.Fatal(err)
	}

	lvl2, err := config.ParseYAML([]byte(`
header:
  center: "{section}"
footer: false
`), "")
	if err != nil {
		t.Fatal(err)
	}

	// Explicit false on CLI overrides true from lower level, while unset zero values do not.
	cliFalse := false
	lvl3 := config.Override{Cover: &cliFalse}

	merged, _, _, err := config.Resolve(lvl1, lvl2, lvl3)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Cover {
		t.Fatalf("expected explicit CLI --cover=false to override cover: true")
	}
	if !merged.Header.Enabled || merged.Header.Left != "ACME Analytics" || merged.Header.Center != "{section}" || merged.Header.Right != "{page}/{pages}" || merged.Header.SkipFirst || !merged.Header.Rule {
		t.Fatalf("unexpected merged header ChromeSpec: %+v", merged.Header)
	}
	if merged.Footer.Enabled {
		t.Fatalf("expected footer: false in frontmatter to disable footer, got %+v", merged.Footer)
	}
}

func TestDiscoverProjectConfigUpwardAndRelativePathResolution(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "datafolio.yaml"), []byte("tokens: design/tokens.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ov, foundPath, err := config.DiscoverUpward(nested)
	if err != nil {
		t.Fatal(err)
	}
	if foundPath != filepath.Join(root, "datafolio.yaml") {
		t.Fatalf("got path %q, want %q", foundPath, filepath.Join(root, "datafolio.yaml"))
	}

	cfg, _, _, err := config.Resolve(ov)
	if err != nil {
		t.Fatal(err)
	}
	wantTok := filepath.Join(root, "design", "tokens.yaml")
	if cfg.Tokens != wantTok {
		t.Fatalf("got tokens path %q, want %q", cfg.Tokens, wantTok)
	}
}
