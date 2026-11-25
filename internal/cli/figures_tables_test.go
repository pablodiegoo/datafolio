package cli_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
)

func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 139, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBuildBooktabsTablesNumericAlignmentAndLocalFiguresEN(t *testing.T) {
	root := t.TempDir()
	docDir := filepath.Join(root, "analysis")
	assetsDir := filepath.Join(docDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	svgContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 40"><rect width="100" height="40" fill="#8b1e1e"/></svg>`
	if err := os.WriteFile(filepath.Join(assetsDir, "trend.svg"), []byte(svgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	pngBytes := makeTestPNG(t)
	if err := os.WriteFile(filepath.Join(assetsDir, "snapshot.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	md := `---
title: Quarterly Revenue Review
lang: en
---

Table: Regional performance summary

| Region | Revenue | Margin |
| --- | --- | --- |
| North | $1,240.50 | 18.4% |
| South | $980.00 | -2.1% |

*Source: Internal ERP ledger*

![Revenue trajectory across quarters](assets/trend.svg)
*Source: Telemetry pipeline*

![System snapshot](assets/snapshot.png "Source: Operations log")
`
	if err := os.WriteFile(filepath.Join(docDir, "report.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	// Note: WorkDir is root, NOT docDir — figures must resolve relative to docDir!
	code, _, errOut := runCLI(t, root, "build", "analysis/report.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}

	outBytes, err := os.ReadFile(filepath.Join(docDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(outBytes)

	expectedPNGDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	for _, want := range []string{
		"Table 1:",
		"Regional performance summary",
		"Source:",
		"Internal ERP ledger",
		"tabular-nums",
		"border-left: none",
		"border-right: none",
		`<td class="num">$1,240.50</td>`,
		`<td class="num">18.4%</td>`,
		`<td>North</td>`,
		"Figure 1:",
		"Revenue trajectory across quarters",
		"&middot;",
		"Telemetry pipeline",
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 40">`,
		"Figure 2:",
		"System snapshot",
		"Operations log",
		expectedPNGDataURI,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected fragment %q", want)
		}
	}

	if strings.Contains(html, "<?xml") || strings.Contains(html, "PUBLIC \"-//W3C//DTD SVG") {
		t.Errorf("inlined SVG should strip XML declaration / SVG DOCTYPE preamble")
	}
}

func TestBuildBilingualPortugueseLabels(t *testing.T) {
	dir := t.TempDir()
	svgContent := `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"></svg>`
	if err := os.WriteFile(filepath.Join(dir, "grafico.svg"), []byte(svgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	md := `---
title: Relatório Analítico
lang: pt-BR
---

Tabela: Receita por região

| Região | Valor |
| --- | --- |
| Sudeste | 1.500,00 |

*Fonte: Contabilidade interna*

![Evolução mensal](grafico.svg)
*Fonte: Base histórica*
`
	if err := os.WriteFile(filepath.Join(dir, "relatorio.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "build", "relatorio.md", "--to", "html")
	if code != cli.ExitSuccess {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}

	outBytes, err := os.ReadFile(filepath.Join(dir, "relatorio.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(outBytes)

	for _, want := range []string{
		`<html lang="pt-BR"`,
		"Tabela 1:",
		"Receita por região",
		"Fonte:",
		"Contabilidade interna",
		"Figura 1:",
		"Evolução mensal",
		"Base histórica",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("pt-BR HTML missing expected fragment %q", want)
		}
	}
}

func TestBuildMissingLocalFigureReturnsRuntimeError(t *testing.T) {
	dir := t.TempDir()
	md := `---
title: Broken Image
---

![Missing figure](assets/does-not-exist.svg)
`
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, dir, "build", "broken.md", "--to", "html")
	if code != cli.ExitError {
		t.Fatalf("expected exit %d for missing local figure, got %d (stderr: %s)", cli.ExitError, code, errOut)
	}
	if !strings.Contains(errOut, "does-not-exist.svg") {
		t.Fatalf("expected missing figure filename in stderr, got %q", errOut)
	}
}
