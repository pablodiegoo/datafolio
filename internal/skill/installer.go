package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/school"
	"github.com/pablodiegoo/datafolio/internal/version"
)

const (
	// ManifestFilename records the installed Skill Bundle version, School, target, and file hashes.
	ManifestFilename = ".datafolio-manifest.json"

	bundleDirName = "datafolio"
)

// Target constants supported by skill install and skill status.
const (
	TargetAgents = "agents"
	TargetClaude = "claude"
	TargetGemini = "gemini"
	TargetAll    = "all"
)

var allTargets = []string{TargetAgents, TargetClaude, TargetGemini}

// ErrUsage indicates invalid CLI/option inputs (target, school, or missing base directories).
var ErrUsage = errors.New("usage error")

// IsUsageError reports whether err represents a usage/configuration validation error.
func IsUsageError(err error) bool {
	return errors.Is(err, ErrUsage)
}

func usageErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUsage}, args...)...)
}

// InstallOptions configures Skill Bundle installation.
type InstallOptions struct {
	WorkDir string
	HomeDir string
	Version string
	Target  string
	Global  bool
	School  string
	Force   bool
}

// TargetInstallation summarizes a single target directory written by Install.
type TargetInstallation struct {
	Target       string `json:"target"`
	Path         string `json:"path"`
	School       string `json:"school"`
	Version      string `json:"version"`
	FilesWritten int    `json:"files_written"`
}

// InstallResult holds all target directories updated by Install.
type InstallResult struct {
	Installations []TargetInstallation `json:"installations"`
}

// Manifest is written as .datafolio-manifest.json inside each installed Skill Bundle directory.
type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	Version       string            `json:"version"`
	School        string            `json:"school"`
	Target        string            `json:"target"`
	Files         map[string]string `json:"files"`
}

type targetSpec struct {
	name string
	dir  string
}

// Install writes the embedded Skill Bundle and manifest into the requested target path(s).
func Install(opts InstallOptions) (InstallResult, error) {
	schoolName := strings.TrimSpace(opts.School)
	if schoolName == "" {
		schoolName = "tufte"
	}
	s, err := school.Lookup(schoolName)
	if err != nil {
		return InstallResult{}, fmt.Errorf("%w: %v", ErrUsage, err)
	}

	ver := strings.TrimSpace(opts.Version)
	if ver == "" {
		ver = version.Version
	}

	targets, err := resolveInstallTargets(opts.Target, opts.Global, opts.WorkDir, opts.HomeDir)
	if err != nil {
		return InstallResult{}, err
	}

	files, err := BundleFilesForSchool(s)
	if err != nil {
		return InstallResult{}, fmt.Errorf("loading skill bundle for %s: %w", s.Name(), err)
	}

	if !opts.Force {
		if err := ensureNoOverwrite(targets, files); err != nil {
			return InstallResult{}, err
		}
	}

	var res InstallResult
	for _, tgt := range targets {
		if err := writeTargetBundle(tgt, s.Name(), ver, files); err != nil {
			return InstallResult{}, err
		}
		res.Installations = append(res.Installations, TargetInstallation{
			Target:       tgt.name,
			Path:         tgt.dir,
			School:       s.Name(),
			Version:      ver,
			FilesWritten: len(files) + 1,
		})
	}
	return res, nil
}

func resolveInstallTargets(rawTarget string, global bool, workDir, homeDir string) ([]targetSpec, error) {
	norm := strings.ToLower(strings.TrimSpace(rawTarget))
	if norm == "" {
		norm = TargetAgents
	}

	var names []string
	switch norm {
	case TargetAgents, TargetClaude, TargetGemini:
		names = []string{norm}
	case TargetAll:
		names = allTargets
	default:
		return nil, usageErrorf("unknown target %q (supported: agents, claude, gemini, all)", rawTarget)
	}

	return buildTargetSpecs(names, global, workDir, homeDir)
}

func buildTargetSpecs(names []string, global bool, workDir, homeDir string) ([]targetSpec, error) {
	baseDir := strings.TrimSpace(workDir)
	if global {
		baseDir = strings.TrimSpace(homeDir)
		if baseDir == "" {
			return nil, usageErrorf("home directory is not configured for --global install")
		}
	} else if baseDir == "" {
		return nil, usageErrorf("working directory is not configured")
	}

	specs := make([]targetSpec, 0, len(names))
	for _, name := range names {
		dir, err := targetDir(name, global, baseDir)
		if err != nil {
			return nil, err
		}
		specs = append(specs, targetSpec{name: name, dir: dir})
	}
	return specs, nil
}

func targetDir(target string, global bool, baseDir string) (string, error) {
	switch target {
	case TargetAgents:
		return filepath.Join(baseDir, ".agents", "skills", bundleDirName), nil
	case TargetClaude:
		return filepath.Join(baseDir, ".claude", "skills", bundleDirName), nil
	case TargetGemini:
		if global {
			return filepath.Join(baseDir, ".gemini", "config", "skills", bundleDirName), nil
		}
		return filepath.Join(baseDir, ".gemini", "skills", bundleDirName), nil
	default:
		return "", usageErrorf("unknown target %q (supported: agents, claude, gemini, all)", target)
	}
}

func ensureNoOverwrite(targets []targetSpec, files []BundleFile) error {
	for _, tgt := range targets {
		manifestPath := filepath.Join(tgt.dir, ManifestFilename)
		manifest, hasManifest := readManifestIfValid(manifestPath)

		var existingPath string
		for _, f := range files {
			dest := filepath.Join(tgt.dir, filepath.FromSlash(f.RelPath))
			existing, err := os.ReadFile(dest)
			if err == nil {
				if hasManifest {
					if recorded, ok := manifest.Files[f.RelPath]; ok && sha256Hex(existing) != recorded {
						return fmt.Errorf("%s has local modifications (use --force to overwrite)", dest)
					}
				}
				if existingPath == "" {
					existingPath = dest
				}
				continue
			}
			if !os.IsNotExist(err) {
				return fmt.Errorf("checking %s: %w", dest, err)
			}
		}

		if existingPath != "" {
			return fmt.Errorf("%s already exists (use --force to overwrite)", existingPath)
		}

		if _, err := os.Stat(manifestPath); err == nil {
			return fmt.Errorf("skill bundle already installed at %s (use --force to overwrite)", tgt.dir)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking %s: %w", manifestPath, err)
		}
	}
	return nil
}

func writeTargetBundle(tgt targetSpec, schoolName, ver string, files []BundleFile) error {
	if err := os.MkdirAll(tgt.dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", tgt.dir, err)
	}

	hashes := make(map[string]string, len(files))
	for _, f := range files {
		dest := filepath.Join(tgt.dir, filepath.FromSlash(f.RelPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", dest, err)
		}
		if err := os.WriteFile(dest, f.Content, f.Mode); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
		hashes[f.RelPath] = f.SHA256
	}

	manifest := Manifest{
		SchemaVersion: 1,
		Version:       ver,
		School:        schoolName,
		Target:        tgt.name,
		Files:         hashes,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding manifest: %w", err)
	}
	encoded = append(encoded, '\n')

	manifestPath := filepath.Join(tgt.dir, ManifestFilename)
	if err := os.WriteFile(manifestPath, encoded, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", manifestPath, err)
	}
	return nil
}

// StatusOptions configures Skill Bundle status and drift inspection.
type StatusOptions struct {
	WorkDir string
	HomeDir string
	Version string
	Target  string
	Global  bool
}

// TargetStatus captures installation health and drift diagnostics for one target directory.
type TargetStatus struct {
	Target           string   `json:"target"`
	Path             string   `json:"path"`
	Installed        bool     `json:"installed"`
	ManifestValid    bool     `json:"manifest_valid"`
	School           string   `json:"school,omitempty"`
	InstalledVersion string   `json:"installed_version,omitempty"`
	CurrentVersion   string   `json:"current_version"`
	VersionDrift     bool     `json:"version_drift"`
	ModifiedFiles    []string `json:"modified_files,omitempty"`
	MissingFiles     []string `json:"missing_files,omitempty"`
	Healthy          bool     `json:"healthy"`
	Error            string   `json:"error,omitempty"`
}

// StatusReport summarizes Skill Bundle state across inspected target directories.
type StatusReport struct {
	Healthy        bool           `json:"healthy"`
	CurrentVersion string         `json:"current_version"`
	Targets        []TargetStatus `json:"targets"`
	Hint           string         `json:"hint,omitempty"`
}

// Status inspects installed Skill Bundle targets, reporting versions, active School,
// modified or missing files, and version drift against opts.Version.
func Status(opts StatusOptions) (StatusReport, error) {
	ver := strings.TrimSpace(opts.Version)
	if ver == "" {
		ver = version.Version
	}

	targets, err := resolveStatusTargets(opts.Target, opts.Global, opts.WorkDir, opts.HomeDir)
	if err != nil {
		return StatusReport{}, err
	}

	report := StatusReport{
		Healthy:        true,
		CurrentVersion: ver,
		Targets:        make([]TargetStatus, 0, len(targets)),
	}

	anyInstalled := false
	for _, tgt := range targets {
		st, err := inspectTarget(tgt, ver)
		if err != nil {
			return StatusReport{}, fmt.Errorf("loading embedded skill bundle: %w", err)
		}
		if st.Installed {
			anyInstalled = true
		}
		if !st.Healthy {
			report.Healthy = false
		}
		report.Targets = append(report.Targets, st)
	}

	if !report.Healthy {
		report.Hint = buildStatusHint(opts, anyInstalled)
	}
	return report, nil
}

func resolveStatusTargets(rawTarget string, global bool, workDir, homeDir string) ([]targetSpec, error) {
	trimmed := strings.TrimSpace(rawTarget)
	if trimmed != "" {
		return resolveInstallTargets(trimmed, global, workDir, homeDir)
	}

	candidates, err := buildTargetSpecs(allTargets, global, workDir, homeDir)
	if err != nil {
		return nil, err
	}

	var installed []targetSpec
	for _, cand := range candidates {
		if targetExists(cand.dir) {
			installed = append(installed, cand)
		}
	}
	if len(installed) > 0 {
		return installed, nil
	}
	return candidates[:1], nil
}

func targetExists(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ManifestFilename)); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		return true
	}
	return false
}

func inspectTarget(tgt targetSpec, currentVer string) (TargetStatus, error) {
	st := TargetStatus{
		Target:         tgt.name,
		Path:           tgt.dir,
		CurrentVersion: currentVer,
	}

	if !targetExists(tgt.dir) {
		st.Error = "skill bundle not installed"
		return st, nil
	}
	st.Installed = true

	manifestPath := filepath.Join(tgt.dir, ManifestFilename)
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		st.Error = fmt.Sprintf("missing %s", ManifestFilename)
		return st, nil
	}
	var manifest Manifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		st.Error = fmt.Sprintf("invalid %s: %v", ManifestFilename, err)
		return st, nil
	}

	st.ManifestValid = true
	st.School = manifest.School
	st.InstalledVersion = manifest.Version
	st.VersionDrift = manifest.Version != currentVer

	var files []BundleFile
	if strings.TrimSpace(manifest.School) != "" {
		var installedSchool school.School
		if s, lookupErr := school.Lookup(manifest.School); lookupErr == nil {
			installedSchool = s
		}
		files, err = BundleFilesForSchool(installedSchool)
	} else {
		files, err = BundleFiles()
	}
	if err != nil {
		return TargetStatus{}, err
	}

	for _, f := range files {
		dest := filepath.Join(tgt.dir, filepath.FromSlash(f.RelPath))
		raw, err := os.ReadFile(dest)
		if err != nil {
			if os.IsNotExist(err) {
				st.MissingFiles = append(st.MissingFiles, f.RelPath)
			} else {
				st.ModifiedFiles = append(st.ModifiedFiles, f.RelPath)
			}
			continue
		}
		if recorded, ok := manifest.Files[f.RelPath]; ok {
			if sha256Hex(raw) != recorded {
				st.ModifiedFiles = append(st.ModifiedFiles, f.RelPath)
			}
		}
	}

	var reasons []string
	if st.VersionDrift {
		reasons = append(reasons, fmt.Sprintf("version drift (installed %s, current %s)", st.InstalledVersion, currentVer))
	}
	if len(st.ModifiedFiles) > 0 {
		reasons = append(reasons, fmt.Sprintf("modified files: %s", strings.Join(st.ModifiedFiles, ", ")))
	}
	if len(st.MissingFiles) > 0 {
		reasons = append(reasons, fmt.Sprintf("missing files: %s", strings.Join(st.MissingFiles, ", ")))
	}

	if len(reasons) == 0 {
		st.Healthy = true
	} else {
		st.Error = strings.Join(reasons, "; ")
	}
	return st, nil
}

func buildStatusHint(opts StatusOptions, anyInstalled bool) string {
	parts := []string{"datafolio skill install"}
	if anyInstalled {
		parts = append(parts, "--force")
	}
	if opts.Global {
		parts = append(parts, "--global")
	}
	if t := strings.ToLower(strings.TrimSpace(opts.Target)); t != "" && t != TargetAgents {
		parts = append(parts, "--target", t)
	}
	action := "install"
	if anyInstalled {
		action = "upgrade or restore"
	}
	return fmt.Sprintf("run %q to %s", strings.Join(parts, " "), action)
}

func readManifestIfValid(path string) (Manifest, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, false
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, false
	}
	return m, true
}
