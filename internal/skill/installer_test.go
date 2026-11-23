package skill_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "github.com/pablodiegoo/datafolio/internal/school/tufte"
	"github.com/pablodiegoo/datafolio/internal/skill"
)

func TestInstallDefaultLocalAgentsAndManifest(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	homeDir := t.TempDir()

	res, err := skill.Install(skill.InstallOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	wantDir := filepath.Join(workDir, ".agents", "skills", "datafolio")
	if len(res.Installations) != 1 {
		t.Fatalf("Install() returned %d installations, want 1", len(res.Installations))
	}
	inst := res.Installations[0]
	if inst.Target != "agents" || inst.Path != wantDir || inst.School != "tufte" || inst.Version != "0.1.0-test" {
		t.Fatalf("unexpected installation summary: %+v", inst)
	}

	bundleFiles, err := skill.BundleFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, bf := range bundleFiles {
		dest := filepath.Join(wantDir, filepath.FromSlash(bf.RelPath))
		gotBytes, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("missing installed file %s: %v", dest, err)
		}
		if string(gotBytes) != string(bf.Content) {
			t.Errorf("content mismatch for %s", bf.RelPath)
		}
	}

	manifestRaw, err := os.ReadFile(filepath.Join(wantDir, skill.ManifestFilename))
	if err != nil {
		t.Fatalf("missing manifest: %v", err)
	}
	var manifest skill.Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("invalid manifest JSON: %v", err)
	}
	if manifest.Version != "0.1.0-test" || manifest.School != "tufte" || manifest.Target != "agents" {
		t.Fatalf("unexpected manifest metadata: %+v", manifest)
	}
	if len(manifest.Files) != len(bundleFiles) {
		t.Fatalf("manifest has %d file hashes, want %d", len(manifest.Files), len(bundleFiles))
	}
	for _, bf := range bundleFiles {
		sum := sha256.Sum256(bf.Content)
		wantHash := hex.EncodeToString(sum[:])
		if manifest.Files[bf.RelPath] != wantHash {
			t.Errorf("manifest hash for %s = %q, want %q", bf.RelPath, manifest.Files[bf.RelPath], wantHash)
		}
	}

	homeEntries, err := os.ReadDir(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(homeEntries) != 0 {
		t.Fatalf("local install must not write to HomeDir, found: %v", homeEntries)
	}
}

func TestInstallInvalidSchoolReturnsUsageError(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	_, err := skill.Install(skill.InstallOptions{
		WorkDir: workDir,
		HomeDir: workDir,
		Version: "0.1.0-test",
		School:  "unknown-school",
	})
	if err == nil {
		t.Fatal("expected error for unknown school")
	}
	if !skill.IsUsageError(err) {
		t.Fatalf("expected IsUsageError(err) == true, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, ".agents")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no files created on invalid school")
	}
}

func TestInstallTargetsAndGlobalPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		target    string
		global    bool
		wantPaths func(workDir, homeDir string) []string
	}{
		{
			name:   "local claude",
			target: "claude",
			wantPaths: func(w, _ string) []string {
				return []string{filepath.Join(w, ".claude", "skills", "datafolio")}
			},
		},
		{
			name:   "local gemini",
			target: "gemini",
			wantPaths: func(w, _ string) []string {
				return []string{filepath.Join(w, ".gemini", "skills", "datafolio")}
			},
		},
		{
			name:   "local all",
			target: "all",
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
			target: "agents",
			global: true,
			wantPaths: func(_, h string) []string {
				return []string{filepath.Join(h, ".agents", "skills", "datafolio")}
			},
		},
		{
			name:   "global claude",
			target: "claude",
			global: true,
			wantPaths: func(_, h string) []string {
				return []string{filepath.Join(h, ".claude", "skills", "datafolio")}
			},
		},
		{
			name:   "global gemini",
			target: "gemini",
			global: true,
			wantPaths: func(_, h string) []string {
				return []string{filepath.Join(h, ".gemini", "config", "skills", "datafolio")}
			},
		},
		{
			name:   "global all",
			target: "all",
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

			res, err := skill.Install(skill.InstallOptions{
				WorkDir: workDir,
				HomeDir: homeDir,
				Version: "0.2.0-test",
				Target:  tc.target,
				Global:  tc.global,
			})
			if err != nil {
				t.Fatalf("Install() error: %v", err)
			}

			want := tc.wantPaths(workDir, homeDir)
			if len(res.Installations) != len(want) {
				t.Fatalf("got %d installations, want %d (%+v)", len(res.Installations), len(want), res.Installations)
			}
			for i, wantDir := range want {
				if res.Installations[i].Path != wantDir {
					t.Errorf("installation[%d].Path = %q, want %q", i, res.Installations[i].Path, wantDir)
				}
				if _, err := os.Stat(filepath.Join(wantDir, "SKILL.md")); err != nil {
					t.Errorf("missing SKILL.md in %s: %v", wantDir, err)
				}
				if _, err := os.Stat(filepath.Join(wantDir, skill.ManifestFilename)); err != nil {
					t.Errorf("missing manifest in %s: %v", wantDir, err)
				}
			}

			if tc.global {
				workEntries, _ := os.ReadDir(workDir)
				if len(workEntries) != 0 {
					t.Errorf("global install wrote to WorkDir: %v", workEntries)
				}
			} else {
				homeEntries, _ := os.ReadDir(homeDir)
				if len(homeEntries) != 0 {
					t.Errorf("local install wrote to HomeDir: %v", homeEntries)
				}
			}
		})
	}
}

func TestInstallRefusesOverwriteWithoutForceAndAllowsForce(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	homeDir := t.TempDir()

	if _, err := skill.Install(skill.InstallOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.1.0",
	}); err != nil {
		t.Fatal(err)
	}

	skillPath := filepath.Join(workDir, ".agents", "skills", "datafolio", "SKILL.md")
	customContent := []byte("# Custom Local Edits\n")
	if err := os.WriteFile(skillPath, customContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Refusal without --force must mention local modifications and --force, and preserve file.
	_, err := skill.Install(skill.InstallOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.2.0",
	})
	if err == nil {
		t.Fatal("expected error refusing overwrite without --force")
	}
	if skill.IsUsageError(err) {
		t.Fatalf("overwrite refusal should be runtime error, not usage error: %v", err)
	}
	if !strings.Contains(err.Error(), "local modifications") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected error mentioning local modifications and --force, got: %v", err)
	}
	gotAfter, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotAfter) != string(customContent) {
		t.Fatalf("modified SKILL.md was clobbered without --force")
	}

	// Atomic refusal across --target all when one target already has a pre-existing file.
	if _, err := skill.Install(skill.InstallOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Target:  "all",
		Version: "0.2.0",
	}); err == nil {
		t.Fatal("expected refusal for --target all when agents already installed")
	}
	if _, statErr := os.Stat(filepath.Join(workDir, ".claude")); !os.IsNotExist(statErr) {
		t.Fatalf("expected atomic refusal: .claude should not be created when agents failed pre-check")
	}

	// With --force, overwrite succeeds and restores bundle + updates manifest version.
	if _, err := skill.Install(skill.InstallOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.2.0",
		Force:   true,
	}); err != nil {
		t.Fatalf("Install(--force) failed: %v", err)
	}
	restored, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) == string(customContent) {
		t.Fatalf("expected --force to overwrite modified SKILL.md")
	}
}

func TestStatusReportsHealthyDriftModifiedMissingAndDiscovery(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	homeDir := t.TempDir()

	// 1. Not installed yet.
	rep, err := skill.Status(skill.StatusOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.1.0",
	})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if rep.Healthy {
		t.Fatalf("expected Healthy == false when not installed")
	}
	if !strings.Contains(rep.Hint, "datafolio skill install") {
		t.Fatalf("expected install hint, got %q", rep.Hint)
	}

	// 2. Install local claude target at v0.1.0 -> Status auto-discovers claude and reports healthy.
	if _, err := skill.Install(skill.InstallOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.1.0",
		Target:  "claude",
	}); err != nil {
		t.Fatal(err)
	}
	rep, err = skill.Status(skill.StatusOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Healthy || len(rep.Targets) != 1 {
		t.Fatalf("expected 1 healthy target, got %+v", rep)
	}
	tgt := rep.Targets[0]
	wantPath := filepath.Join(workDir, ".claude", "skills", "datafolio")
	if tgt.Path != wantPath || tgt.School != "tufte" || tgt.InstalledVersion != "0.1.0" || tgt.CurrentVersion != "0.1.0" {
		t.Fatalf("unexpected target status: %+v", tgt)
	}

	// 3. Explicit --target all must flag missing agents and gemini even though claude is healthy.
	repAll, err := skill.Status(skill.StatusOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.1.0",
		Target:  "all",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repAll.Healthy || len(repAll.Targets) != 3 {
		t.Fatalf("expected unhealthy 3-target report, got %+v", repAll)
	}

	// 4. Version drift when binary upgrades to 0.2.0.
	repDrift, err := skill.Status(skill.StatusOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repDrift.Healthy || !repDrift.Targets[0].VersionDrift {
		t.Fatalf("expected VersionDrift == true, got %+v", repDrift)
	}
	if !strings.Contains(repDrift.Hint, "datafolio skill install --force") {
		t.Fatalf("expected upgrade hint with --force, got %q", repDrift.Hint)
	}

	// 5. Modify file and delete another -> both reported.
	if err := os.WriteFile(filepath.Join(wantPath, "SKILL.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(wantPath, "references", "token-contract.md")); err != nil {
		t.Fatal(err)
	}
	repMod, err := skill.Status(skill.StatusOptions{
		WorkDir: workDir,
		HomeDir: homeDir,
		Version: "0.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repMod.Healthy {
		t.Fatalf("expected unhealthy when files modified/missing")
	}
	if !slices.Equal(repMod.Targets[0].ModifiedFiles, []string{"SKILL.md"}) {
		t.Errorf("ModifiedFiles = %v, want [SKILL.md]", repMod.Targets[0].ModifiedFiles)
	}
	if !slices.Equal(repMod.Targets[0].MissingFiles, []string{"references/token-contract.md"}) {
		t.Errorf("MissingFiles = %v, want [references/token-contract.md]", repMod.Targets[0].MissingFiles)
	}

	// 6. Unmanifested pre-existing SKILL.md -> Status flags invalid manifest, Install refuses without --force.
	unmanagedDir := t.TempDir()
	unmanagedSkillDir := filepath.Join(unmanagedDir, ".agents", "skills", "datafolio")
	if err := os.MkdirAll(unmanagedSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unmanagedFile := filepath.Join(unmanagedSkillDir, "SKILL.md")
	if err := os.WriteFile(unmanagedFile, []byte("# manual\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.Install(skill.InstallOptions{WorkDir: unmanagedDir, HomeDir: homeDir, Version: "0.1.0"}); err == nil {
		t.Fatal("expected refusal when pre-existing SKILL.md exists without manifest")
	}
	gotUnmanaged, err := os.ReadFile(unmanagedFile)
	if err != nil || string(gotUnmanaged) != "# manual\n" {
		t.Fatalf("pre-existing SKILL.md was altered without --force")
	}
	repUnmanaged, err := skill.Status(skill.StatusOptions{WorkDir: unmanagedDir, HomeDir: homeDir, Version: "0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if repUnmanaged.Healthy || !repUnmanaged.Targets[0].Installed || repUnmanaged.Targets[0].ManifestValid {
		t.Fatalf("expected installed+untracked status for pre-existing SKILL.md, got %+v", repUnmanaged)
	}
}
