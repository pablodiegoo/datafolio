package skill_test

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	_ "github.com/pablodiegoo/datafolio/internal/school/tufte"
	"github.com/pablodiegoo/datafolio/internal/skill"
)

func TestBundleFilesContainsRequiredAssets(t *testing.T) {
	t.Parallel()

	files, err := skill.BundleFiles()
	if err != nil {
		t.Fatalf("BundleFiles() error: %v", err)
	}

	gotPaths := make([]string, 0, len(files))
	for _, f := range files {
		if len(f.Content) == 0 {
			t.Errorf("bundle file %q has empty content", f.RelPath)
		}
		gotPaths = append(gotPaths, f.RelPath)
	}

	wantPaths := []string{
		"SKILL.md",
		"references/markdown-grammar.md",
		"references/token-contract.md",
		"references/tufte-school.md",
		"scripts/plot_tufte.py",
	}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("BundleFiles() paths = %v, want %v", gotPaths, wantPaths)
	}
}

func TestBundlePEP723MetadataAndZeroEmojis(t *testing.T) {
	t.Parallel()

	files, err := skill.BundleFiles()
	if err != nil {
		t.Fatalf("BundleFiles() error: %v", err)
	}

	byPath := make(map[string]skill.BundleFile, len(files))
	for _, f := range files {
		byPath[f.RelPath] = f
		if hasEmoji(string(f.Content)) {
			t.Errorf("bundle file %q must not contain emojis", f.RelPath)
		}
	}

	py := byPath["scripts/plot_tufte.py"]
	meta, err := skill.ParsePEP723(py.Content)
	if err != nil {
		t.Fatalf("ParsePEP723(plot_tufte.py) error: %v", err)
	}
	if strings.TrimSpace(meta.RequiresPython) == "" {
		t.Fatalf("PEP 723 requires-python is empty")
	}
	if !slices.Contains(meta.Dependencies, "matplotlib>=3.9") {
		t.Fatalf("PEP 723 dependencies = %v, want matplotlib>=3.9", meta.Dependencies)
	}

	pyText := string(py.Content)
	for _, want := range []string{"datafolio.mplstyle", "tokens.json", "colors", "palette", "paper", "accent"} {
		if !strings.Contains(pyText, want) {
			t.Errorf("plot_tufte.py missing expected integration token %q", want)
		}
	}

	skillMD := string(byPath["SKILL.md"].Content)
	for _, cmd := range []string{"datafolio init", "datafolio build", "datafolio tokens export", "datafolio review", "datafolio skill install", "datafolio skill status", "datafolio doctor"} {
		if !strings.Contains(skillMD, cmd) {
			t.Errorf("SKILL.md missing CLI command %q", cmd)
		}
	}
}

func TestParsePEP723RejectsInvalidHeaders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
	}{
		{name: "missing header", src: "import matplotlib\n"},
		{name: "unterminated block", src: "# /// script\n# requires-python = \">=3.11\"\n"},
		{name: "uncommented line inside block", src: "# /// script\nrequires-python = \">=3.11\"\n# ///\n"},
		{name: "missing requires-python", src: "# /// script\n# dependencies = [\"matplotlib>=3.9\"]\n# ///\n"},
		{name: "missing dependencies", src: "# /// script\n# requires-python = \">=3.11\"\n# ///\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := skill.ParsePEP723([]byte(tc.src)); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func hasEmoji(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x1F300 && r <= 0x1FAFF:
			return true
		case r >= 0x2600 && r <= 0x27BF:
			return true
		case r == 0xFE0F || r == 0x200D:
			return true
		case unicode.Is(unicode.So, r) && r > 0x25FF:
			return true
		}
	}
	return false
}
