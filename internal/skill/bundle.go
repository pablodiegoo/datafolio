package skill

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed bundle
var bundleFS embed.FS

// BundleFile represents a single file inside the embedded Skill Bundle.
type BundleFile struct {
	RelPath string
	Content []byte
	Mode    fs.FileMode
	SHA256  string
}

// PEP723Metadata holds parsed inline script metadata from a `# /// script` block.
type PEP723Metadata struct {
	RequiresPython string
	Dependencies   []string
	RawTOML        string
}

// BundleFiles returns all embedded Skill Bundle files sorted deterministically by relative path.
func BundleFiles() ([]BundleFile, error) {
	var files []BundleFile
	err := fs.WalkDir(bundleFS, "bundle", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "bundle/")
		raw, err := fs.ReadFile(bundleFS, p)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if path.Ext(rel) == ".py" {
			mode = 0o755
		}
		files = append(files, BundleFile{
			RelPath: rel,
			Content: raw,
			Mode:    mode,
			SHA256:  sha256Hex(raw),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	return files, nil
}

var (
	pep723StartRe = regexp.MustCompile(`^# /// script\s*$`)
	pep723EndRe   = regexp.MustCompile(`^# ///\s*$`)
	reqPyRe       = regexp.MustCompile(`(?m)^\s*requires-python\s*=\s*("([^"]*)"|'([^']*)')\s*$`)
	depsRe        = regexp.MustCompile(`(?s)dependencies\s*=\s*\[(.*?)\]`)
	tomlStringRe  = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
)

// ParsePEP723 extracts and validates a PEP 723 `# /// script` metadata block from Python source.
func ParsePEP723(src []byte) (PEP723Metadata, error) {
	normalized := strings.ReplaceAll(string(src), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	startIdx := -1
	endIdx := -1
	for i, line := range lines {
		if startIdx == -1 {
			if pep723StartRe.MatchString(line) {
				startIdx = i
			}
			continue
		}
		if pep723EndRe.MatchString(line) {
			endIdx = i
			break
		}
		if line != "#" && !strings.HasPrefix(line, "# ") {
			return PEP723Metadata{}, fmt.Errorf("invalid PEP 723 line %d inside script block: %q", i+1, line)
		}
	}

	if startIdx == -1 {
		return PEP723Metadata{}, errors.New("missing PEP 723 '# /// script' header")
	}
	if endIdx == -1 {
		return PEP723Metadata{}, errors.New("unterminated PEP 723 '# /// script' block")
	}

	tomlLines := make([]string, 0, endIdx-startIdx-1)
	for _, line := range lines[startIdx+1 : endIdx] {
		if line == "#" {
			tomlLines = append(tomlLines, "")
		} else {
			tomlLines = append(tomlLines, strings.TrimPrefix(line, "# "))
		}
	}
	rawTOML := strings.Join(tomlLines, "\n")

	meta := PEP723Metadata{RawTOML: rawTOML}
	if m := reqPyRe.FindStringSubmatch(rawTOML); m != nil {
		if m[2] != "" {
			meta.RequiresPython = m[2]
		} else {
			meta.RequiresPython = m[3]
		}
	}
	if m := depsRe.FindStringSubmatch(rawTOML); m != nil {
		for _, sm := range tomlStringRe.FindAllStringSubmatch(m[1], -1) {
			val := sm[1]
			if val == "" {
				val = sm[2]
			}
			if val != "" {
				if unq, err := strconv.Unquote(`"` + strings.ReplaceAll(val, `"`, `\"`) + `"`); err == nil {
					val = unq
				}
				meta.Dependencies = append(meta.Dependencies, val)
			}
		}
	}

	if meta.RequiresPython == "" {
		return PEP723Metadata{}, errors.New("PEP 723 metadata missing requires-python")
	}
	if len(meta.Dependencies) == 0 {
		return PEP723Metadata{}, errors.New("PEP 723 metadata missing dependencies")
	}
	return meta, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
