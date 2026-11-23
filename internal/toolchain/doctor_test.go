package toolchain_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/toolchain"
	"github.com/ulikunitz/xz"
)

func TestPinnedReleasesCoverAllSixPlatforms(t *testing.T) {
	t.Parallel()

	want := map[string]struct {
		urlSuffix string
		sha256    string
		archive   string
	}{
		"linux/amd64": {
			urlSuffix: "/v0.15.1/typst-x86_64-unknown-linux-musl.tar.xz",
			sha256:    "a6d077d0a95eed5a2eba715b2dae06be954f624ccbf85758a03f389ded33118c",
			archive:   "tar.xz",
		},
		"linux/arm64": {
			urlSuffix: "/v0.15.1/typst-aarch64-unknown-linux-musl.tar.xz",
			sha256:    "5aa8d74a3d906e60ea12a66ac2f37f8eef1b14cbad7182a745e393a10c23dcee",
			archive:   "tar.xz",
		},
		"darwin/amd64": {
			urlSuffix: "/v0.15.1/typst-x86_64-apple-darwin.tar.xz",
			sha256:    "7f9fdd9584866245de9a79e0add8f9236fae6f40a8a45e2c4771ccc14db4e0fa",
			archive:   "tar.xz",
		},
		"darwin/arm64": {
			urlSuffix: "/v0.15.1/typst-aarch64-apple-darwin.tar.xz",
			sha256:    "48f62ed034aa3a7978309579ac6ca00045e2ef0da73114e8af27cfd8e74dc05a",
			archive:   "tar.xz",
		},
		"windows/amd64": {
			urlSuffix: "/v0.15.1/typst-x86_64-pc-windows-msvc.zip",
			sha256:    "19ce3551153c2fe7ee9fa2f95208310c8f4d3209fedb699e0333faf8913f6736",
			archive:   "zip",
		},
		"windows/arm64": {
			urlSuffix: "/v0.15.1/typst-aarch64-pc-windows-msvc.zip",
			sha256:    "4ab28e1b71ec3184d38d580ab797f499b6770d952b6b19167be5cea5c2662e14",
			archive:   "zip",
		},
	}

	for key, expected := range want {
		goos, goarch, _ := strings.Cut(key, "/")
		asset, err := toolchain.PinnedRelease(goos, goarch)
		if err != nil {
			t.Fatalf("PinnedRelease(%s): %v", key, err)
		}
		if !strings.HasSuffix(asset.URL, expected.urlSuffix) {
			t.Errorf("%s URL = %q, want suffix %q", key, asset.URL, expected.urlSuffix)
		}
		if asset.SHA256 != expected.sha256 {
			t.Errorf("%s SHA256 = %q, want %q", key, asset.SHA256, expected.sha256)
		}
		if asset.Archive != expected.archive {
			t.Errorf("%s Archive = %q, want %q", key, asset.Archive, expected.archive)
		}
	}

	if _, err := toolchain.PinnedRelease("plan9", "386"); err == nil {
		t.Fatal("expected error for unsupported GOOS/GOARCH")
	}
}

func TestResolveTypstPrecedence(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cachedBin := filepath.Join(cacheDir, "typst", toolchain.PinnedTypstVersion, "typst")
	if err := os.MkdirAll(filepath.Dir(cachedBin), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(cachedBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write cached bin: %v", err)
	}

	// PATH takes precedence over managed cache.
	res := toolchain.ResolveTypst(func(name string) (string, error) {
		if name != "typst" {
			t.Fatalf("lookPath called with %q, want %q", name, "typst")
		}
		return "/usr/local/bin/typst", nil
	}, cacheDir, "linux", "amd64")

	if !res.Found || res.Source != toolchain.TypstSourcePATH || res.Path != "/usr/local/bin/typst" {
		t.Fatalf("expected PATH resolution, got %+v", res)
	}

	// Falls back to managed cache when lookPath misses.
	res = toolchain.ResolveTypst(func(string) (string, error) {
		return "", exec.ErrNotFound
	}, cacheDir, "linux", "amd64")
	if !res.Found || res.Source != toolchain.TypstSourceCache || res.Path != cachedBin {
		t.Fatalf("expected cache resolution %q, got %+v", cachedBin, res)
	}

	// Reports missing when neither PATH nor cache has the binary.
	emptyCache := t.TempDir()
	res = toolchain.ResolveTypst(func(string) (string, error) {
		return "", exec.ErrNotFound
	}, emptyCache, "linux", "amd64")
	if res.Found || res.Source != toolchain.TypstSourceMissing || res.Path != "" {
		t.Fatalf("expected missing resolution, got %+v", res)
	}
}

func TestInstallTypstValidTarXZ(t *testing.T) {
	t.Parallel()

	wantBinary := []byte("#!/bin/sh\necho 'typst 0.15.1'\n")
	archive := buildTarXZArchive(t, "typst-x86_64-unknown-linux-musl/typst", wantBinary)
	digest := sha256Hex(archive)

	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	fetcher := &toolchain.HTTPReleaseFetcher{
		Client: srv.Client(),
		Releases: map[string]toolchain.ReleaseAsset{
			"linux/amd64": {
				Version: toolchain.PinnedTypstVersion,
				URL:     srv.URL + "/typst-linux.tar.xz",
				SHA256:  digest,
				Archive: "tar.xz",
			},
		},
	}

	installedPath, err := toolchain.InstallTypst(context.Background(), fetcher, cacheDir, "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallTypst failed: %v", err)
	}
	if requestedPath != "/typst-linux.tar.xz" {
		t.Fatalf("unexpected HTTP request path %q", requestedPath)
	}

	wantPath := filepath.Join(cacheDir, "typst", toolchain.PinnedTypstVersion, "typst")
	if installedPath != wantPath {
		t.Fatalf("installed path = %q, want %q", installedPath, wantPath)
	}

	gotBytes, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if !bytes.Equal(gotBytes, wantBinary) {
		t.Fatalf("installed binary content mismatch")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(installedPath)
		if err != nil {
			t.Fatalf("stat installed binary: %v", err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("expected executable bit on %q, got mode %v", installedPath, info.Mode())
		}
	}

	assertNoTempLeftovers(t, filepath.Dir(installedPath))
}

func TestInstallTypstValidZipWindows(t *testing.T) {
	t.Parallel()

	wantBinary := []byte("MZ-fake-windows-typst-binary")
	archive := buildZipArchive(t, "typst-x86_64-pc-windows-msvc/typst.exe", wantBinary)
	digest := sha256Hex(archive)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	fetcher := &toolchain.HTTPReleaseFetcher{
		Client: srv.Client(),
		Releases: map[string]toolchain.ReleaseAsset{
			"windows/amd64": {
				Version: toolchain.PinnedTypstVersion,
				URL:     srv.URL + "/typst-win.zip",
				SHA256:  digest,
				Archive: "zip",
			},
		},
	}

	installedPath, err := toolchain.InstallTypst(context.Background(), fetcher, cacheDir, "windows", "amd64")
	if err != nil {
		t.Fatalf("InstallTypst windows/amd64 failed: %v", err)
	}

	wantPath := filepath.Join(cacheDir, "typst", toolchain.PinnedTypstVersion, "typst.exe")
	if installedPath != wantPath {
		t.Fatalf("installed path = %q, want %q", installedPath, wantPath)
	}
	gotBytes, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if !bytes.Equal(gotBytes, wantBinary) {
		t.Fatalf("installed windows binary mismatch")
	}
	assertNoTempLeftovers(t, filepath.Dir(installedPath))
}

func TestInstallTypstRejectsSHA256MismatchWithZeroPartialFiles(t *testing.T) {
	t.Parallel()

	archive := buildTarXZArchive(t, "typst-x86_64-unknown-linux-musl/typst", []byte("tampered"))
	var requested string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	// BaseURL overrides only the download prefix, keeping the REAL pinned SHA-256 digest!
	fetcher := &toolchain.HTTPReleaseFetcher{
		Client:  srv.Client(),
		BaseURL: srv.URL,
	}

	_, err := toolchain.InstallTypst(context.Background(), fetcher, cacheDir, "linux", "amd64")
	if err == nil {
		t.Fatal("expected SHA-256 mismatch error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sha256") && !strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Fatalf("expected checksum error detail, got %v", err)
	}
	if requested != "/typst-x86_64-unknown-linux-musl.tar.xz" {
		t.Fatalf("unexpected request path %q", requested)
	}

	assertEmptyDir(t, cacheDir)
}

func TestInstallTypstCorruptArchiveAndHTTPErrorLeaveZeroPartialFiles(t *testing.T) {
	t.Parallel()

	t.Run("http 404", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()

		cacheDir := t.TempDir()
		fetcher := &toolchain.HTTPReleaseFetcher{
			Client:  srv.Client(),
			BaseURL: srv.URL,
		}
		if _, err := toolchain.InstallTypst(context.Background(), fetcher, cacheDir, "linux", "amd64"); err == nil {
			t.Fatal("expected error for HTTP 404")
		}
		assertEmptyDir(t, cacheDir)
	})

	t.Run("corrupt xz payload matching checksum", func(t *testing.T) {
		t.Parallel()
		corrupt := []byte("not-a-valid-tar-xz-payload")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(corrupt)
		}))
		defer srv.Close()

		cacheDir := t.TempDir()
		fetcher := &toolchain.HTTPReleaseFetcher{
			Client: srv.Client(),
			Releases: map[string]toolchain.ReleaseAsset{
				"linux/amd64": {
					Version: toolchain.PinnedTypstVersion,
					URL:     srv.URL + "/bad.tar.xz",
					SHA256:  sha256Hex(corrupt),
					Archive: "tar.xz",
				},
			},
		}
		err := error(nil)
		if _, err = toolchain.InstallTypst(context.Background(), fetcher, cacheDir, "linux", "amd64"); err == nil {
			t.Fatal("expected extraction error for corrupt xz archive")
		}
		if errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected cancellation error: %v", err)
		}
		assertEmptyDir(t, cacheDir)
	})
}

func buildTarXZArchive(t *testing.T, entryName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz.NewWriter: %v", err)
	}
	tw := tar.NewWriter(xzw)
	hdr := &tar.Header{
		Name: entryName,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
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

func buildZipArchive(t *testing.T, entryName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(entryName)
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("zip.Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected %q to contain zero partial files/dirs, found: %v", dir, names)
	}
}

func assertNoTempLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("unexpected temp leftover %q in %q", e.Name(), dir)
		}
	}
}
