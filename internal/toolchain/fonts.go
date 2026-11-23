package toolchain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/assets/fonts"
)

type embeddedAsset struct {
	name   string
	data   []byte
	digest [32]byte
}

// EnsureFonts extracts the bundled SIL OFL 1.1 fonts and license notices into a
// deterministic content-hashed subdirectory (<cacheDir>/fonts/<hash>/) and
// returns that directory path for Typst --font-path. Extraction is atomic and
// idempotent across repeated invocations.
func EnsureFonts(cacheDir string) (string, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return "", fmt.Errorf("toolchain: cache directory is empty")
	}

	assets, bundleHash, err := loadEmbeddedAssets()
	if err != nil {
		return "", err
	}

	fontsRoot := filepath.Join(cacheDir, "fonts")
	targetDir := filepath.Join(fontsRoot, bundleHash)

	if assetsComplete(targetDir, assets) {
		return targetDir, nil
	}

	if err := os.MkdirAll(fontsRoot, 0o755); err != nil {
		return "", fmt.Errorf("toolchain: create fonts cache dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(fontsRoot, ".tmp-"+bundleHash+"-*")
	if err != nil {
		return "", fmt.Errorf("toolchain: create temp fonts dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.Chmod(tmpDir, 0o755); err != nil {
		return "", fmt.Errorf("toolchain: chmod temp fonts dir: %w", err)
	}

	for _, asset := range assets {
		dst := filepath.Join(tmpDir, asset.name)
		if err := os.WriteFile(dst, asset.data, 0o644); err != nil {
			return "", fmt.Errorf("toolchain: write bundled asset %s: %w", asset.name, err)
		}
	}

	if _, statErr := os.Stat(targetDir); statErr == nil {
		// Another caller may have completed extraction concurrently; otherwise discard incomplete directory.
		if assetsComplete(targetDir, assets) {
			return targetDir, nil
		}
		if err := os.RemoveAll(targetDir); err != nil {
			return "", fmt.Errorf("toolchain: remove incomplete fonts dir: %w", err)
		}
	}

	if err := os.Rename(tmpDir, targetDir); err != nil {
		if assetsComplete(targetDir, assets) {
			return targetDir, nil
		}
		return "", fmt.Errorf("toolchain: install fonts dir: %w", err)
	}

	return targetDir, nil
}

func loadEmbeddedAssets() ([]embeddedAsset, string, error) {
	entries, err := fs.ReadDir(fonts.FS, ".")
	if err != nil {
		return nil, "", fmt.Errorf("toolchain: read embedded fonts: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, "", fmt.Errorf("toolchain: no embedded font assets found")
	}

	bundleHasher := sha256.New()
	assets := make([]embeddedAsset, 0, len(names))
	for _, name := range names {
		data, err := fonts.FS.ReadFile(name)
		if err != nil {
			return nil, "", fmt.Errorf("toolchain: read embedded asset %s: %w", name, err)
		}
		digest := sha256.Sum256(data)
		bundleHasher.Write([]byte(name))
		bundleHasher.Write([]byte{0})
		bundleHasher.Write(digest[:])
		assets = append(assets, embeddedAsset{
			name:   name,
			data:   data,
			digest: digest,
		})
	}

	sum := bundleHasher.Sum(nil)
	return assets, hex.EncodeToString(sum[:8]), nil
}

func assetsComplete(dir string, assets []embeddedAsset) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	for _, asset := range assets {
		data, err := os.ReadFile(filepath.Join(dir, asset.name))
		if err != nil {
			return false
		}
		if sha256.Sum256(data) != asset.digest {
			return false
		}
	}
	return true
}
