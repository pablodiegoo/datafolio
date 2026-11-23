package toolchain

import (
	"archive/tar"
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// PinnedTypstVersion is the Typst toolchain release managed by DataFolio v1.
const PinnedTypstVersion = "v0.15.1"

// TypstReleaseBaseURL is the official GitHub release download prefix for PinnedTypstVersion.
const TypstReleaseBaseURL = "https://github.com/typst/typst/releases/download/" + PinnedTypstVersion

// Resolution source identifiers returned by ResolveTypst.
const (
	TypstSourcePATH    = "PATH"
	TypstSourceCache   = "cache"
	TypstSourceMissing = "missing"
)

// ReleaseAsset describes a pinned Typst release archive and its expected SHA-256 digest.
type ReleaseAsset struct {
	Version string
	URL     string
	SHA256  string
	Archive string // "tar.xz" or "zip"
}

// TypstResolution describes where a `typst` executable was found.
type TypstResolution struct {
	Found  bool
	Source string
	Path   string
}

// ReleaseFetcher downloads a release archive stream from a URL.
type ReleaseFetcher interface {
	Fetch(ctx context.Context, url string) (io.ReadCloser, error)
}

// ReleaseCatalog is optionally implemented by a ReleaseFetcher to supply
// mirror URLs or test release metadata without mutating global state.
type ReleaseCatalog interface {
	ReleaseFor(goos, goarch string) (ReleaseAsset, bool, error)
}

// HTTPReleaseFetcher downloads release archives over HTTP(S).
// BaseURL optionally overrides TypstReleaseBaseURL while preserving pinned SHA-256 digests.
// Releases optionally overrides per-platform ("<goos>/<goarch>") ReleaseAsset entries.
type HTTPReleaseFetcher struct {
	Client   *http.Client
	BaseURL  string
	Releases map[string]ReleaseAsset
}

// Fetch performs an HTTP GET honoring ctx and returns the response body on HTTP 200.
func (f *HTTPReleaseFetcher) Fetch(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("toolchain: build request for %s: %w", rawURL, err)
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("toolchain: fetch %s: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("toolchain: fetch %s: unexpected HTTP status %s", rawURL, resp.Status)
	}
	return resp.Body, nil
}

// ReleaseFor resolves optional per-platform or BaseURL overrides for tests and mirrors.
func (f *HTTPReleaseFetcher) ReleaseFor(goos, goarch string) (ReleaseAsset, bool, error) {
	if f == nil {
		return ReleaseAsset{}, false, nil
	}
	key := platformKey(goos, goarch)
	if asset, ok := f.Releases[key]; ok {
		return asset, true, nil
	}
	if f.BaseURL != "" {
		asset, err := PinnedRelease(goos, goarch)
		if err != nil {
			return ReleaseAsset{}, false, err
		}
		base := strings.TrimRight(f.BaseURL, "/")
		asset.URL = base + strings.TrimPrefix(asset.URL, TypstReleaseBaseURL)
		return asset, true, nil
	}
	return ReleaseAsset{}, false, nil
}

var pinnedTypstReleases = map[string]ReleaseAsset{
	"linux/amd64": {
		Version: PinnedTypstVersion,
		URL:     TypstReleaseBaseURL + "/typst-x86_64-unknown-linux-musl.tar.xz",
		SHA256:  "a6d077d0a95eed5a2eba715b2dae06be954f624ccbf85758a03f389ded33118c",
		Archive: "tar.xz",
	},
	"linux/arm64": {
		Version: PinnedTypstVersion,
		URL:     TypstReleaseBaseURL + "/typst-aarch64-unknown-linux-musl.tar.xz",
		SHA256:  "5aa8d74a3d906e60ea12a66ac2f37f8eef1b14cbad7182a745e393a10c23dcee",
		Archive: "tar.xz",
	},
	"darwin/amd64": {
		Version: PinnedTypstVersion,
		URL:     TypstReleaseBaseURL + "/typst-x86_64-apple-darwin.tar.xz",
		SHA256:  "7f9fdd9584866245de9a79e0add8f9236fae6f40a8a45e2c4771ccc14db4e0fa",
		Archive: "tar.xz",
	},
	"darwin/arm64": {
		Version: PinnedTypstVersion,
		URL:     TypstReleaseBaseURL + "/typst-aarch64-apple-darwin.tar.xz",
		SHA256:  "48f62ed034aa3a7978309579ac6ca00045e2ef0da73114e8af27cfd8e74dc05a",
		Archive: "tar.xz",
	},
	"windows/amd64": {
		Version: PinnedTypstVersion,
		URL:     TypstReleaseBaseURL + "/typst-x86_64-pc-windows-msvc.zip",
		SHA256:  "19ce3551153c2fe7ee9fa2f95208310c8f4d3209fedb699e0333faf8913f6736",
		Archive: "zip",
	},
	"windows/arm64": {
		Version: PinnedTypstVersion,
		URL:     TypstReleaseBaseURL + "/typst-aarch64-pc-windows-msvc.zip",
		SHA256:  "4ab28e1b71ec3184d38d580ab797f499b6770d952b6b19167be5cea5c2662e14",
		Archive: "zip",
	},
}

func platformKey(goos, goarch string) string {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return goos + "/" + goarch
}

// BinaryName returns "typst.exe" on Windows and "typst" elsewhere.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "typst.exe"
	}
	return "typst"
}

// CachedTypstPath returns the managed cache binary path for PinnedTypstVersion.
func CachedTypstPath(cacheDir, goos string) string {
	if strings.TrimSpace(cacheDir) == "" {
		return ""
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	return filepath.Join(cacheDir, "typst", PinnedTypstVersion, BinaryName(goos))
}

// PinnedRelease returns the official pinned Typst v0.15.1 archive metadata for goos/goarch.
func PinnedRelease(goos, goarch string) (ReleaseAsset, error) {
	key := platformKey(goos, goarch)
	asset, ok := pinnedTypstReleases[key]
	if !ok {
		return ReleaseAsset{}, fmt.Errorf("toolchain: unsupported platform %s", key)
	}
	return asset, nil
}

// ResolveTypst resolves the `typst` binary, checking PATH via lookPath("typst")
// before falling back to <cacheDir>/typst/v0.15.1/typst(.exe).
func ResolveTypst(lookPath func(string) (string, error), cacheDir, goos, goarch string) TypstResolution {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if p, err := lookPath("typst"); err == nil && strings.TrimSpace(p) != "" {
		return TypstResolution{
			Found:  true,
			Source: TypstSourcePATH,
			Path:   p,
		}
	}

	if goos == "" {
		goos = runtime.GOOS
	}

	cached := CachedTypstPath(cacheDir, goos)
	if cached != "" && isExecutableBinary(cached, goos) {
		return TypstResolution{
			Found:  true,
			Source: TypstSourceCache,
			Path:   cached,
		}
	}

	return TypstResolution{
		Found:  false,
		Source: TypstSourceMissing,
	}
}

func isExecutableBinary(p, goos string) bool {
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// InstallTypst fetches the pinned Typst release archive via fetcher, verifies its
// SHA-256 checksum prior to extraction, and atomically installs the binary into
// <cacheDir>/typst/v0.15.1/typst(.exe). Any failure leaves zero partial files in cacheDir.
func InstallTypst(ctx context.Context, fetcher ReleaseFetcher, cacheDir, goos, goarch string) (string, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return "", fmt.Errorf("toolchain: cache directory is empty")
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if fetcher == nil {
		fetcher = &HTTPReleaseFetcher{}
	}

	asset, err := resolveReleaseAsset(fetcher, goos, goarch)
	if err != nil {
		return "", err
	}

	// Stage download outside cacheDir so a failed fetch or SHA-256 mismatch never
	// creates files or directories inside cacheDir.
	archiveFile, err := os.CreateTemp("", "datafolio-typst-archive-*")
	if err != nil {
		return "", fmt.Errorf("toolchain: create temp archive file: %w", err)
	}
	defer func() {
		archiveFile.Close()
		os.Remove(archiveFile.Name())
	}()

	gotDigest, err := downloadAndHash(ctx, fetcher, asset.URL, archiveFile)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(gotDigest, asset.SHA256) {
		return "", fmt.Errorf("toolchain: sha256 checksum mismatch for %s: got %s, want %s", asset.URL, gotDigest, asset.SHA256)
	}

	if _, err := archiveFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("toolchain: rewind verified archive: %w", err)
	}

	return installVerifiedArchive(archiveFile, asset, cacheDir, goos)
}

func resolveReleaseAsset(fetcher ReleaseFetcher, goos, goarch string) (ReleaseAsset, error) {
	if catalog, ok := fetcher.(ReleaseCatalog); ok {
		override, found, err := catalog.ReleaseFor(goos, goarch)
		if err != nil {
			return ReleaseAsset{}, err
		}
		if found {
			return normalizeReleaseAsset(override)
		}
	}
	asset, err := PinnedRelease(goos, goarch)
	if err != nil {
		return ReleaseAsset{}, err
	}
	return normalizeReleaseAsset(asset)
}

func normalizeReleaseAsset(asset ReleaseAsset) (ReleaseAsset, error) {
	if strings.TrimSpace(asset.URL) == "" {
		return ReleaseAsset{}, fmt.Errorf("toolchain: release URL is empty")
	}
	if strings.TrimSpace(asset.SHA256) == "" {
		return ReleaseAsset{}, fmt.Errorf("toolchain: release SHA-256 digest is empty")
	}
	if asset.Version == "" {
		asset.Version = PinnedTypstVersion
	}
	if asset.Archive == "" {
		if strings.HasSuffix(strings.ToLower(asset.URL), ".zip") {
			asset.Archive = "zip"
		} else {
			asset.Archive = "tar.xz"
		}
	}
	return asset, nil
}

func downloadAndHash(ctx context.Context, fetcher ReleaseFetcher, url string, dst io.Writer) (string, error) {
	rc, err := fetcher.Fetch(ctx, url)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, hasher), rc); err != nil {
		return "", fmt.Errorf("toolchain: download %s: %w", url, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func installVerifiedArchive(archiveFile *os.File, asset ReleaseAsset, cacheDir, goos string) (string, error) {
	typstRoot := filepath.Join(cacheDir, "typst")
	versionDir := filepath.Join(typstRoot, asset.Version)
	targetPath := filepath.Join(versionDir, BinaryName(goos))

	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return "", fmt.Errorf("toolchain: create cache dir %s: %w", versionDir, err)
	}

	tmpBin, err := os.CreateTemp(versionDir, ".typst-*.tmp")
	if err != nil {
		cleanupEmptyDirs(versionDir, typstRoot)
		return "", fmt.Errorf("toolchain: create staging binary: %w", err)
	}

	renamed := false
	defer func() {
		tmpBin.Close()
		if !renamed {
			os.Remove(tmpBin.Name())
			cleanupEmptyDirs(versionDir, typstRoot)
		}
	}()

	binaryName := BinaryName(goos)
	switch asset.Archive {
	case "tar.xz":
		err = extractBinaryFromTarXZ(archiveFile, binaryName, tmpBin)
	case "zip":
		err = extractBinaryFromZip(archiveFile, binaryName, tmpBin)
	default:
		err = fmt.Errorf("toolchain: unsupported archive format %q", asset.Archive)
	}
	if err != nil {
		return "", err
	}

	if err := tmpBin.Chmod(0o755); err != nil {
		return "", fmt.Errorf("toolchain: chmod staged binary: %w", err)
	}
	if err := tmpBin.Close(); err != nil {
		return "", fmt.Errorf("toolchain: close staged binary: %w", err)
	}
	if err := os.Rename(tmpBin.Name(), targetPath); err != nil {
		return "", fmt.Errorf("toolchain: install binary to %s: %w", targetPath, err)
	}
	renamed = true
	return targetPath, nil
}

func cleanupEmptyDirs(dirs ...string) {
	for _, d := range dirs {
		_ = os.Remove(d) // succeeds only if empty
	}
}

func extractBinaryFromTarXZ(r io.Reader, binaryName string, dst io.Writer) error {
	xzr, err := xz.NewReader(r)
	if err != nil {
		return fmt.Errorf("toolchain: open xz stream: %w", err)
	}
	tr := tar.NewReader(xzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("toolchain: binary %q not found in tar.xz archive", binaryName)
		}
		if err != nil {
			return fmt.Errorf("toolchain: read tar archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if isTargetArchiveEntry(hdr.Name, binaryName) {
			if _, err := io.Copy(dst, tr); err != nil {
				return fmt.Errorf("toolchain: extract %s: %w", binaryName, err)
			}
			return nil
		}
	}
}

func extractBinaryFromZip(archiveFile *os.File, binaryName string, dst io.Writer) error {
	info, err := archiveFile.Stat()
	if err != nil {
		return fmt.Errorf("toolchain: stat zip archive: %w", err)
	}
	zr, err := zip.NewReader(archiveFile, info.Size())
	if err != nil {
		return fmt.Errorf("toolchain: open zip archive: %w", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if isTargetArchiveEntry(f.Name, binaryName) {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("toolchain: open zip entry %s: %w", f.Name, err)
			}
			_, copyErr := io.Copy(dst, rc)
			rc.Close()
			if copyErr != nil {
				return fmt.Errorf("toolchain: extract %s: %w", binaryName, copyErr)
			}
			return nil
		}
	}
	return fmt.Errorf("toolchain: binary %q not found in zip archive", binaryName)
}

func isTargetArchiveEntry(entryName, binaryName string) bool {
	clean := path.Clean(filepath.ToSlash(entryName))
	if clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	return path.Base(clean) == binaryName
}
