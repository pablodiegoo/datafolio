package toolchain_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/assets/fonts"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

func TestEnsureFontsExtractsValidFontsAndNoticesIdempotently(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()

	dir1, err := toolchain.EnsureFonts(cacheDir)
	if err != nil {
		t.Fatalf("EnsureFonts returned unexpected error: %v", err)
	}

	fontsRoot := filepath.Join(cacheDir, "fonts")
	if filepath.Dir(dir1) != fontsRoot {
		t.Fatalf("expected extracted dir under %q, got %q", fontsRoot, dir1)
	}
	if filepath.Base(dir1) == "" || strings.HasPrefix(filepath.Base(dir1), ".tmp-") {
		t.Fatalf("expected content-hashed subdir name, got %q", dir1)
	}

	expectedFiles := []string{
		"EBGaramond[wght].ttf",
		"EBGaramond-Italic[wght].ttf",
		"OFL-EBGaramond.txt",
		"JetBrainsMono[wght].ttf",
		"JetBrainsMono-Italic[wght].ttf",
		"OFL-JetBrainsMono.txt",
		"SourceSans3[wght].ttf",
		"SourceSans3-Italic[wght].ttf",
		"OFL-SourceSans3.txt",
	}

	for _, name := range expectedFiles {
		extractedPath := filepath.Join(dir1, name)
		gotBytes, err := os.ReadFile(extractedPath)
		if err != nil {
			t.Fatalf("missing extracted asset %q: %v", name, err)
		}
		embeddedBytes, err := fonts.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("missing embedded asset %q: %v", name, err)
		}
		if !bytes.Equal(gotBytes, embeddedBytes) {
			t.Fatalf("extracted %q bytes differ from embedded asset", name)
		}

		if strings.HasSuffix(name, ".ttf") {
			assertValidTTF(t, name, gotBytes)
		} else {
			if !strings.Contains(string(gotBytes), "SIL Open Font License") {
				t.Fatalf("license file %q does not contain SIL Open Font License text", name)
			}
		}
	}

	// Idempotent second call returns the same deterministic path without temp leftovers.
	dir2, err := toolchain.EnsureFonts(cacheDir)
	if err != nil {
		t.Fatalf("second EnsureFonts returned error: %v", err)
	}
	if dir2 != dir1 {
		t.Fatalf("EnsureFonts not deterministic: first=%q second=%q", dir1, dir2)
	}

	entries, err := os.ReadDir(fontsRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", fontsRoot, err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 directory in %q, got %d", fontsRoot, len(entries))
	}
}

func TestEnsureFontsRecoversFromIncompleteCacheDir(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	dir1, err := toolchain.EnsureFonts(cacheDir)
	if err != nil {
		t.Fatalf("EnsureFonts initial: %v", err)
	}

	// Simulate partial/corrupt cache by deleting one font and truncating another.
	if err := os.Remove(filepath.Join(dir1, "OFL-EBGaramond.txt")); err != nil {
		t.Fatalf("remove license: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "JetBrainsMono[wght].ttf"), []byte("corrupt"), 0o644); err != nil {
		t.Fatalf("corrupt font: %v", err)
	}

	dir2, err := toolchain.EnsureFonts(cacheDir)
	if err != nil {
		t.Fatalf("EnsureFonts recovery: %v", err)
	}
	if dir2 != dir1 {
		t.Fatalf("expected same dir %q after recovery, got %q", dir1, dir2)
	}

	repaired, err := os.ReadFile(filepath.Join(dir2, "JetBrainsMono[wght].ttf"))
	if err != nil {
		t.Fatalf("read repaired font: %v", err)
	}
	assertValidTTF(t, "JetBrainsMono[wght].ttf", repaired)
	if _, err := os.Stat(filepath.Join(dir2, "OFL-EBGaramond.txt")); err != nil {
		t.Fatalf("expected missing license file to be restored: %v", err)
	}
}

func TestEnsureFontsRejectsEmptyCacheDir(t *testing.T) {
	t.Parallel()

	if _, err := toolchain.EnsureFonts(""); err == nil {
		t.Fatal("expected error when cacheDir is empty")
	}
}

func TestBundledFontsSupportPortugueseDiacriticsAndSmallCaps(t *testing.T) {
	t.Parallel()

	ptBRRunes := []rune{'á', 'à', 'â', 'ã', 'ç', 'é', 'ê', 'í', 'ó', 'ô', 'õ', 'ú', 'Á', 'À', 'Â', 'Ã', 'Ç', 'É', 'Ê', 'Í', 'Ó', 'Ô', 'Õ', 'Ú'}

	for _, fam := range fonts.Families() {
		for _, name := range fam.Files {
			data, err := fonts.FS.ReadFile(name)
			if err != nil {
				t.Fatalf("read embedded font %q: %v", name, err)
			}
			assertValidTTF(t, name, data)
			for _, r := range ptBRRunes {
				if !ttfHasGlyph(t, data, r) {
					t.Errorf("font %q missing glyph for pt-BR rune %q (U+%04X)", name, r, r)
				}
			}
			if fam.Name == "EB Garamond" && !ttfHasGSUBFeature(t, data, "smcp") {
				t.Errorf("font %q missing OpenType small-caps (smcp) feature", name)
			}
		}
	}
}

func assertValidTTF(t *testing.T, name string, data []byte) {
	t.Helper()
	if len(data) < 12 {
		t.Fatalf("%s: file too small (%d bytes)", name, len(data))
	}
	sfntVersion := binary.BigEndian.Uint32(data[0:4])
	// TrueType (0x00010000), 'true' (0x74727565), or CFF OpenType 'OTTO' (0x4F54544F)
	if sfntVersion != 0x00010000 && sfntVersion != 0x74727565 && sfntVersion != 0x4F54544F {
		t.Fatalf("%s: invalid sfnt magic 0x%08x (prefix %q)", name, sfntVersion, data[:min(32, len(data))])
	}
}

func findSFNTTable(data []byte, tag string) ([]byte, bool) {
	if len(data) < 12 {
		return nil, false
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+numTables*16 {
		return nil, false
	}
	for i := 0; i < numTables; i++ {
		rec := data[12+i*16 : 12+(i+1)*16]
		if string(rec[0:4]) == tag {
			offset := int(binary.BigEndian.Uint32(rec[8:12]))
			length := int(binary.BigEndian.Uint32(rec[12:16]))
			if offset < 0 || length < 0 || offset+length > len(data) {
				return nil, false
			}
			return data[offset : offset+length], true
		}
	}
	return nil, false
}

func ttfHasGlyph(t *testing.T, data []byte, r rune) bool {
	t.Helper()
	cmap, ok := findSFNTTable(data, "cmap")
	if !ok || len(cmap) < 4 {
		t.Fatalf("missing cmap table")
	}
	numSubtables := int(binary.BigEndian.Uint16(cmap[2:4]))
	if len(cmap) < 4+numSubtables*8 {
		t.Fatalf("truncated cmap header")
	}
	cp := uint32(r)
	for i := 0; i < numSubtables; i++ {
		rec := cmap[4+i*8 : 4+(i+1)*8]
		offset := int(binary.BigEndian.Uint32(rec[4:8]))
		if offset+2 > len(cmap) {
			continue
		}
		sub := cmap[offset:]
		format := binary.BigEndian.Uint16(sub[0:2])
		switch format {
		case 4:
			if cp > 0xFFFF || len(sub) < 14 {
				continue
			}
			segCount := int(binary.BigEndian.Uint16(sub[6:8])) / 2
			endOff := 14
			startOff := endOff + segCount*2 + 2
			deltaOff := startOff + segCount*2
			rangeOff := deltaOff + segCount*2
			if len(sub) < rangeOff+segCount*2 {
				continue
			}
			for s := 0; s < segCount; s++ {
				endCode := uint32(binary.BigEndian.Uint16(sub[endOff+s*2:]))
				startCode := uint32(binary.BigEndian.Uint16(sub[startOff+s*2:]))
				if cp >= startCode && cp <= endCode {
					idRangeOffset := int(binary.BigEndian.Uint16(sub[rangeOff+s*2:]))
					if idRangeOffset == 0 {
						idDelta := int32(int16(binary.BigEndian.Uint16(sub[deltaOff+s*2:])))
						if uint16(int32(cp)+idDelta) != 0 {
							return true
						}
					} else {
						glyphPos := rangeOff + s*2 + idRangeOffset + int(cp-startCode)*2
						if glyphPos+2 <= len(sub) && binary.BigEndian.Uint16(sub[glyphPos:]) != 0 {
							return true
						}
					}
				}
			}
		case 12:
			if len(sub) < 16 {
				continue
			}
			numGroups := int(binary.BigEndian.Uint32(sub[12:16]))
			if len(sub) < 16+numGroups*12 {
				continue
			}
			for g := 0; g < numGroups; g++ {
				grp := sub[16+g*12:]
				startChar := binary.BigEndian.Uint32(grp[0:4])
				endChar := binary.BigEndian.Uint32(grp[4:8])
				startGlyph := binary.BigEndian.Uint32(grp[8:12])
				if cp >= startChar && cp <= endChar && startGlyph+(cp-startChar) != 0 {
					return true
				}
			}
		}
	}
	return false
}

func ttfHasGSUBFeature(t *testing.T, data []byte, featTag string) bool {
	t.Helper()
	gsub, ok := findSFNTTable(data, "GSUB")
	if !ok || len(gsub) < 10 {
		return false
	}
	featListOffset := int(binary.BigEndian.Uint16(gsub[6:8]))
	if featListOffset+2 > len(gsub) {
		return false
	}
	featList := gsub[featListOffset:]
	count := int(binary.BigEndian.Uint16(featList[0:2]))
	if len(featList) < 2+count*6 {
		return false
	}
	for i := 0; i < count; i++ {
		if string(featList[2+i*6:2+i*6+4]) == featTag {
			return true
		}
	}
	return false
}
