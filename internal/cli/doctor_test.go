package cli_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
	"github.com/ulikunitz/xz"
)

func runDoctorCLI(t *testing.T, opts cli.Options, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if opts.Version == "" {
		opts.Version = "0.1.0-test"
	}
	code := cli.Run(context.Background(), args, opts)
	return code, stdout.String(), stderr.String()
}

func TestDefaultOptionsPopulatesToolchainSeams(t *testing.T) {
	opts := cli.DefaultOptions()
	if opts.TypstRunner == nil {
		t.Fatal("DefaultOptions().TypstRunner is nil")
	}
	if opts.ReleaseFetcher == nil {
		t.Fatal("DefaultOptions().ReleaseFetcher is nil")
	}
	if opts.LookPath == nil {
		t.Fatal("DefaultOptions().LookPath is nil")
	}
	if opts.GOOS == "" || opts.GOARCH == "" {
		t.Fatalf("DefaultOptions() GOOS/GOARCH empty: %q/%q", opts.GOOS, opts.GOARCH)
	}
}

func TestDoctorMissingTypstReportsStatusAndExtractsFonts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")

	code, out, errOut := runDoctorCLI(t, cli.Options{
		WorkDir:  dir,
		HomeDir:  dir,
		CacheDir: cacheDir,
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		GOOS:     "linux",
		GOARCH:   "amd64",
	}, "doctor")

	if code != cli.ExitError {
		t.Fatalf("got exit %d, want %d (stdout=%s stderr=%s)", code, cli.ExitError, out, errOut)
	}
	for _, want := range []string{"missing", "EB Garamond", "JetBrains Mono", "Source Sans 3", filepath.Join(cacheDir, "fonts")} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor stdout missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, "--install-typst") {
		t.Errorf("doctor stderr missing install hint:\n%s", errOut)
	}

	// Verify fonts were extracted under cacheDir/fonts/<hash>/.
	fontDirs, err := os.ReadDir(filepath.Join(cacheDir, "fonts"))
	if err != nil || len(fontDirs) != 1 {
		t.Fatalf("expected 1 font hash dir under %s, got %v (err=%v)", cacheDir, fontDirs, err)
	}
	fontDir := filepath.Join(cacheDir, "fonts", fontDirs[0].Name())
	for _, f := range []string{"EBGaramond[wght].ttf", "JetBrainsMono[wght].ttf", "SourceSans3[wght].ttf", "OFL-EBGaramond.txt"} {
		if _, err := os.Stat(filepath.Join(fontDir, f)); err != nil {
			t.Errorf("missing extracted font asset %q: %v", f, err)
		}
	}
}

func TestDoctorPATHPrecedenceOverManagedCacheAndJSONOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	cachedBin := filepath.Join(cacheDir, "typst", toolchain.PinnedTypstVersion, "typst")
	if err := os.MkdirAll(filepath.Dir(cachedBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachedBin, []byte("cached"), 0o755); err != nil {
		t.Fatal(err)
	}

	runner := &toolchain.FakeTypstRunner{VersionOutput: "0.15.1"}
	code, out, errOut := runDoctorCLI(t, cli.Options{
		WorkDir:     dir,
		HomeDir:     dir,
		CacheDir:    cacheDir,
		TypstRunner: runner,
		LookPath:    func(string) (string, error) { return "/opt/bin/typst", nil },
		GOOS:        "linux",
		GOARCH:      "amd64",
	}, "doctor", "--json")

	if code != cli.ExitSuccess {
		t.Fatalf("got exit %d, want %d (stderr=%s)", code, cli.ExitSuccess, errOut)
	}

	var report struct {
		Healthy bool `json:"healthy"`
		Typst   struct {
			Healthy bool   `json:"healthy"`
			Source  string `json:"source"`
			Path    string `json:"path"`
			Version string `json:"version"`
		} `json:"typst"`
		Fonts struct {
			Healthy   bool     `json:"healthy"`
			Path      string   `json:"path"`
			Families  []string `json:"families"`
			FileCount int      `json:"file_count"`
		} `json:"fonts"`
		Cache struct {
			Healthy bool   `json:"healthy"`
			Path    string `json:"path"`
		} `json:"cache"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid doctor --json output: %v\n%s", err, out)
	}
	if !report.Healthy || !report.Typst.Healthy || !report.Fonts.Healthy || !report.Cache.Healthy {
		t.Fatalf("expected all checks healthy, got %+v", report)
	}
	if report.Typst.Source != toolchain.TypstSourcePATH || report.Typst.Path != "/opt/bin/typst" || report.Typst.Version != "0.15.1" {
		t.Fatalf("expected PATH precedence, got %+v", report.Typst)
	}
	if report.Fonts.FileCount != 9 || !strings.HasPrefix(report.Fonts.Path, filepath.Join(cacheDir, "fonts")) {
		t.Fatalf("unexpected fonts report: %+v", report.Fonts)
	}
	if len(runner.VersionCalls) != 1 || runner.VersionCalls[0] != "/opt/bin/typst" {
		t.Fatalf("expected Version called with /opt/bin/typst, got %v", runner.VersionCalls)
	}
}

func TestDoctorUsesManagedCacheWhenNotOnPATH(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	cachedBin := filepath.Join(cacheDir, "typst", toolchain.PinnedTypstVersion, "typst")
	if err := os.MkdirAll(filepath.Dir(cachedBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachedBin, []byte("cached"), 0o755); err != nil {
		t.Fatal(err)
	}

	runner := &toolchain.FakeTypstRunner{VersionOutput: "0.15.1"}
	code, out, errOut := runDoctorCLI(t, cli.Options{
		WorkDir:     dir,
		HomeDir:     dir,
		CacheDir:    cacheDir,
		TypstRunner: runner,
		LookPath:    func(string) (string, error) { return "", exec.ErrNotFound },
		GOOS:        "linux",
		GOARCH:      "amd64",
	}, "doctor", "--json")

	if code != cli.ExitSuccess {
		t.Fatalf("got exit %d, want %d (stderr=%s)", code, cli.ExitSuccess, errOut)
	}
	if !strings.Contains(out, `"source": "cache"`) || !strings.Contains(out, cachedBin) {
		t.Fatalf("expected cache resolution in JSON output:\n%s", out)
	}
}

func TestDoctorInstallTypstViaHTTPTestServerValidArchive(t *testing.T) {
	t.Parallel()

	wantBin := []byte("#!/bin/sh\necho 'typst 0.15.1'\n")
	archive := buildCLITestTarXZ(t, "typst-x86_64-unknown-linux-musl/typst", wantBin)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	runner := &toolchain.FakeTypstRunner{VersionOutput: "0.15.1"}

	opts := cli.Options{
		WorkDir:     dir,
		HomeDir:     dir,
		CacheDir:    cacheDir,
		TypstRunner: runner,
		ReleaseFetcher: &toolchain.HTTPReleaseFetcher{
			Client: srv.Client(),
			Releases: map[string]toolchain.ReleaseAsset{
				"linux/amd64": {
					Version: toolchain.PinnedTypstVersion,
					URL:     srv.URL + "/typst.tar.xz",
					SHA256:  digest,
					Archive: "tar.xz",
				},
			},
		},
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		GOOS:     "linux",
		GOARCH:   "amd64",
	}

	code, out, errOut := runDoctorCLI(t, opts, "doctor", "--install-typst", "--json")
	if code != cli.ExitSuccess {
		t.Fatalf("doctor --install-typst failed with %d: stdout=%s stderr=%s", code, out, errOut)
	}

	var report struct {
		Healthy bool `json:"healthy"`
		Typst   struct {
			Healthy   bool   `json:"healthy"`
			Source    string `json:"source"`
			Path      string `json:"path"`
			Version   string `json:"version"`
			Installed bool   `json:"installed"`
		} `json:"typst"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	wantPath := filepath.Join(cacheDir, "typst", toolchain.PinnedTypstVersion, "typst")
	if !report.Healthy || !report.Typst.Installed || report.Typst.Source != toolchain.TypstSourceCache || report.Typst.Path != wantPath {
		t.Fatalf("unexpected report after install: %+v", report)
	}

	gotBytes, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if !bytes.Equal(gotBytes, wantBin) {
		t.Fatalf("installed binary content mismatch")
	}

	// Second call without --install-typst is healthy from cache.
	code, out2, errOut2 := runDoctorCLI(t, opts, "doctor")
	if code != cli.ExitSuccess {
		t.Fatalf("follow-up doctor failed with %d: stdout=%s stderr=%s", code, out2, errOut2)
	}
	if !strings.Contains(out2, "cache:") {
		t.Fatalf("expected cache source in follow-up output:\n%s", out2)
	}
}

func TestDoctorInstallTypstRejectsSHA256MismatchLeavesZeroPartialFiles(t *testing.T) {
	t.Parallel()

	archive := buildCLITestTarXZ(t, "typst-x86_64-unknown-linux-musl/typst", []byte("tampered"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")

	code, out, errOut := runDoctorCLI(t, cli.Options{
		WorkDir:  dir,
		HomeDir:  dir,
		CacheDir: cacheDir,
		ReleaseFetcher: &toolchain.HTTPReleaseFetcher{
			Client:  srv.Client(),
			BaseURL: srv.URL,
		},
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		GOOS:     "linux",
		GOARCH:   "amd64",
	}, "doctor", "--install-typst", "--json")

	if code != cli.ExitError {
		t.Fatalf("expected ExitError (%d) on checksum mismatch, got %d (stderr=%s)", cli.ExitError, code, errOut)
	}
	if !strings.Contains(strings.ToLower(errOut), "sha256") && !strings.Contains(strings.ToLower(errOut), "checksum") {
		t.Fatalf("expected checksum mismatch diagnostic on stderr, got %q", errOut)
	}
	var report struct {
		Healthy bool `json:"healthy"`
		Typst   struct {
			Healthy bool   `json:"healthy"`
			Error   string `json:"error"`
		} `json:"typst"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("expected valid JSON output even when install fails: %v\n%s", err, out)
	}
	if report.Healthy || report.Typst.Healthy || !strings.Contains(strings.ToLower(report.Typst.Error), "sha256") {
		t.Fatalf("unexpected failure report: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "typst")); !os.IsNotExist(err) {
		t.Fatalf("expected zero partial typst cache files/dirs after failed checksum, stat err=%v", err)
	}
}

func TestDoctorUnhealthyTypstVersionAndNonDirectoryCache(t *testing.T) {
	t.Parallel()

	t.Run("broken typst binary version check", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		code, out, errOut := runDoctorCLI(t, cli.Options{
			WorkDir:     dir,
			HomeDir:     dir,
			CacheDir:    filepath.Join(dir, "cache"),
			TypstRunner: &toolchain.FakeTypstRunner{VersionErr: errors.New("exec format error")},
			LookPath:    func(string) (string, error) { return "/bad/typst", nil },
			GOOS:        "linux",
			GOARCH:      "amd64",
		}, "doctor")

		if code != cli.ExitError {
			t.Fatalf("expected ExitError, got %d", code)
		}
		if !strings.Contains(out, "unhealthy") || !strings.Contains(errOut, "exec format error") {
			t.Fatalf("unexpected diagnostics: stdout=%q stderr=%q", out, errOut)
		}
	})

	t.Run("cache path is a file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fileCache := filepath.Join(dir, "not-a-dir")
		if err := os.WriteFile(fileCache, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, out, _ := runDoctorCLI(t, cli.Options{
			WorkDir:     dir,
			HomeDir:     dir,
			CacheDir:    fileCache,
			TypstRunner: &toolchain.FakeTypstRunner{VersionOutput: "0.15.1"},
			LookPath:    func(string) (string, error) { return "/usr/bin/typst", nil },
			GOOS:        "linux",
			GOARCH:      "amd64",
		}, "doctor", "--json")

		if code != cli.ExitError {
			t.Fatalf("expected ExitError for file cache path, got %d (out=%s)", code, out)
		}
		if !strings.Contains(out, `"healthy": false`) {
			t.Fatalf("expected unhealthy JSON output, got %s", out)
		}
	})
}

func TestDoctorUsageAndHelp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	opts := cli.Options{WorkDir: dir, HomeDir: dir, CacheDir: filepath.Join(dir, "cache")}

	code, out, _ := runDoctorCLI(t, opts, "doctor", "--help")
	if code != cli.ExitSuccess || !strings.Contains(out, "--install-typst") || !strings.Contains(out, "--json") {
		t.Fatalf("doctor --help got (%d, %q)", code, out)
	}

	for _, args := range [][]string{
		{"doctor", "--unknown-flag"},
		{"doctor", "unexpected-arg"},
	} {
		code, _, errOut := runDoctorCLI(t, opts, args...)
		if code != cli.ExitUsage || strings.TrimSpace(errOut) == "" {
			t.Fatalf("args %v: got (%d, %q), want ExitUsage (%d) with stderr", args, code, errOut, cli.ExitUsage)
		}
	}
}

func buildCLITestTarXZ(t *testing.T, entryName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz.NewWriter: %v", err)
	}
	tw := tar.NewWriter(xzw)
	if err := tw.WriteHeader(&tar.Header{
		Name: entryName,
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatalf("tar.WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar.Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Close: %v", err)
	}
	if err := xzw.Close(); err != nil {
		t.Fatalf("xz.Close: %v", err)
	}
	return buf.Bytes()
}
