package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"gopkg.in/yaml.v3"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s missing go.mod: %v", root, err)
	}
	return root
}

func TestReleaseLicenseAndThirdPartyNotices(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	licenseBytes, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatalf("read LICENSE: %v", err)
	}
	licenseText := string(licenseBytes)
	for _, want := range []string{
		"Apache License",
		"Version 2.0, January 2004",
		"http://www.apache.org/licenses/",
		"Copyright 2026 Pablo Diego Pereira",
	} {
		if !strings.Contains(licenseText, want) {
			t.Errorf("LICENSE missing required text %q", want)
		}
	}

	thirdPartyBytes, err := os.ReadFile(filepath.Join(root, "THIRD_PARTY_LICENSES"))
	if err != nil {
		t.Fatalf("read THIRD_PARTY_LICENSES: %v", err)
	}
	thirdPartyText := normalizeLF(string(thirdPartyBytes))

	fontLicenseFiles := []string{
		"OFL-EBGaramond.txt",
		"OFL-JetBrainsMono.txt",
		"OFL-SourceSans3.txt",
	}
	for _, name := range fontLicenseFiles {
		raw, err := os.ReadFile(filepath.Join(root, "internal", "assets", "fonts", name))
		if err != nil {
			t.Fatalf("read font license %s: %v", name, err)
		}
		wantText := strings.TrimSpace(normalizeLF(string(raw)))
		if !strings.Contains(thirdPartyText, wantText) {
			t.Errorf("THIRD_PARTY_LICENSES missing verbatim text of %s", name)
		}
	}

	goModBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, modPath := range parseGoModDependencies(string(goModBytes)) {
		if !strings.Contains(thirdPartyText, modPath) {
			t.Errorf("THIRD_PARTY_LICENSES missing Go module notice for %q", modPath)
		}
	}
}

func normalizeLF(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func parseGoModDependencies(goMod string) []string {
	var deps []string
	inRequireBlock := false
	for _, rawLine := range strings.Split(normalizeLF(goMod), "\n") {
		line := strings.TrimSpace(rawLine)
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		if line == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			fields := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(fields) >= 2 {
				deps = append(deps, fields[0])
			}
			continue
		}
		if inRequireBlock {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				deps = append(deps, fields[0])
			}
		}
	}
	return deps
}

func TestReadmeAndDocsPresent(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	if _, err := os.Stat(filepath.Join(root, "readme.md")); !os.IsNotExist(err) {
		t.Fatalf("placeholder readme.md must be removed, stat err = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	readme := string(raw)

	requiredPhrases := []string{
		"go install github.com/pablodiegoo/datafolio/cmd/datafolio@latest",
		"CGO_ENABLED=0",
		"GitHub Releases",
		"datafolio init",
		"datafolio build",
		"datafolio tokens check",
		"datafolio tokens export",
		"datafolio review",
		"datafolio skill install",
		"datafolio skill status",
		"datafolio doctor",
		"datafolio version",
		"Running Chrome",
		"Token Contract",
		"Source Document",
		"Skill Bundle",
		"Slide Overflow",
		"Sidenote",
		"Margin Note",
		":::wide",
		":::kpi",
		":::columns",
		":::note",
		"DESIGN.md",
		"tailwind",
		"matplotlib",
		".templ",
		".blade.php",
		"uv run",
		"token-contrast-aa",
		"prose-deletion-test",
		"web-hardcoded-color",
		"THIRD_PARTY_LICENSES",
	}
	for _, want := range requiredPhrases {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md missing required section/term %q", want)
		}
	}

	for _, r := range readme {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) || r == 0xFE0F || r == 0x200D || (unicode.Is(unicode.So, r) && r > 0x25FF) {
			t.Errorf("README.md must not contain emoji rune %U", r)
			break
		}
	}

	lower := strings.ToLower(readme)
	for _, filler := range []string{
		"it's important to note that",
		"it is important to note that",
		"based on the data provided",
		"it is worth mentioning",
		"one can observe that",
		"the data suggests that",
		"in conclusion",
		"as we can see",
	} {
		if strings.Contains(lower, filler) {
			t.Errorf("README.md contains Deletion Test phrase %q", filler)
		}
	}
}

func TestGoReleaserAndCIWorkflowConfig(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	// 1. Validate .goreleaser.yaml v2 structure, 6 OS/arch targets, CGO_ENABLED=0, and archived files.
	grBytes, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var gr struct {
		Version     int    `yaml:"version"`
		ProjectName string `yaml:"project_name"`
		Builds      []struct {
			ID      string   `yaml:"id"`
			Main    string   `yaml:"main"`
			Binary  string   `yaml:"binary"`
			Env     []string `yaml:"env"`
			Flags   []string `yaml:"flags"`
			Ldflags []string `yaml:"ldflags"`
			GOOS    []string `yaml:"goos"`
			GOARCH  []string `yaml:"goarch"`
		} `yaml:"builds"`
		Archives []struct {
			ID              string   `yaml:"id"`
			Formats         []string `yaml:"formats"`
			FormatOverrides []struct {
				GOOS    string   `yaml:"goos"`
				Formats []string `yaml:"formats"`
			} `yaml:"format_overrides"`
			Files []string `yaml:"files"`
		} `yaml:"archives"`
		Checksum struct {
			NameTemplate string `yaml:"name_template"`
			Algorithm    string `yaml:"algorithm"`
		} `yaml:"checksum"`
	}
	if err := yaml.Unmarshal(grBytes, &gr); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}
	if gr.Version != 2 {
		t.Errorf(".goreleaser.yaml version = %d, want 2", gr.Version)
	}
	if len(gr.Builds) == 0 {
		t.Fatal(".goreleaser.yaml has no builds")
	}
	b := gr.Builds[0]
	if b.ID != "datafolio" || b.Main != "./cmd/datafolio" || b.Binary != "datafolio" {
		t.Errorf(".goreleaser.yaml build identity = (%q, %q, %q), want (datafolio, ./cmd/datafolio, datafolio)", b.ID, b.Main, b.Binary)
	}
	if !slices.Contains(b.Env, "CGO_ENABLED=0") {
		t.Errorf(".goreleaser.yaml env missing CGO_ENABLED=0: %v", b.Env)
	}
	if !slices.Contains(b.Flags, "-trimpath") {
		t.Errorf(".goreleaser.yaml flags missing -trimpath: %v", b.Flags)
	}
	ldflagsJoined := strings.Join(b.Ldflags, " ")
	if !strings.Contains(ldflagsJoined, "github.com/pablodiegoo/datafolio/internal/version.Version") {
		t.Errorf(".goreleaser.yaml ldflags missing version injection: %v", b.Ldflags)
	}
	for _, wantOS := range []string{"linux", "darwin", "windows"} {
		if !slices.Contains(b.GOOS, wantOS) {
			t.Errorf(".goreleaser.yaml goos missing %q: %v", wantOS, b.GOOS)
		}
	}
	for _, wantArch := range []string{"amd64", "arm64"} {
		if !slices.Contains(b.GOARCH, wantArch) {
			t.Errorf(".goreleaser.yaml goarch missing %q: %v", wantArch, b.GOARCH)
		}
	}
	if len(gr.Archives) == 0 {
		t.Fatal(".goreleaser.yaml has no archives")
	}
	arch := gr.Archives[0]
	if !slices.Contains(arch.Formats, "tar.gz") {
		t.Errorf(".goreleaser.yaml archives.formats missing tar.gz: %v", arch.Formats)
	}
	hasWinZip := false
	for _, ov := range arch.FormatOverrides {
		if ov.GOOS == "windows" && slices.Contains(ov.Formats, "zip") {
			hasWinZip = true
		}
	}
	if !hasWinZip {
		t.Errorf(".goreleaser.yaml archives missing windows zip format_override: %+v", arch.FormatOverrides)
	}
	for _, reqFile := range []string{"LICENSE", "THIRD_PARTY_LICENSES", "README.md"} {
		if !slices.Contains(arch.Files, reqFile) {
			t.Errorf(".goreleaser.yaml archives.files missing %q: %v", reqFile, arch.Files)
		}
		if _, err := os.Stat(filepath.Join(root, reqFile)); err != nil {
			t.Errorf("archived file %q does not exist at repo root: %v", reqFile, err)
		}
	}
	if gr.Checksum.NameTemplate != "checksums.txt" || gr.Checksum.Algorithm != "sha256" {
		t.Errorf(".goreleaser.yaml checksum = %+v, want checksums.txt + sha256", gr.Checksum)
	}

	// 2. Validate .github/workflows/ci.yml structure and required checks.
	ciBytes, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read .github/workflows/ci.yml: %v", err)
	}
	var wf map[string]any
	if err := yaml.Unmarshal(ciBytes, &wf); err != nil {
		t.Fatalf("parse .github/workflows/ci.yml: %v", err)
	}
	ciText := string(ciBytes)
	for _, want := range []string{
		"go test -race -count=1 ./...",
		"CGO_ENABLED=0",
		"./cmd/datafolio",
		"doctor --install-typst",
		"datafolio doctor",
		"DATAFOLIO_TEST_REAL_TYPST=1",
		"TestSkillBundleMatchesCLI",
		"pull_request",
		"feat/*",
	} {
		if !strings.Contains(ciText, want) {
			t.Errorf(".github/workflows/ci.yml missing %q", want)
		}
	}
}
