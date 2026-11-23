package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitUntil(t *testing.T, timeout time.Duration, desc string, cond func() bool, diag ...func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	var extra string
	for _, fn := range diag {
		extra += "\n" + fn()
	}
	t.Fatalf("timed out after %v waiting for %s%s", timeout, desc, extra)
}

func atomicWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-save-*")
	if err != nil {
		t.Fatalf("create temp for %s: %v", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		t.Fatalf("write temp %s: %v", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		t.Fatalf("close temp %s: %v", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		t.Fatalf("rename %s -> %s: %v", tmpPath, path, err)
	}
}

func TestWatchInitialBuildAllFormatsAndCleanCancel(t *testing.T) {
	t.Run("html initial build and clean context cancellation", func(t *testing.T) {
		dir := t.TempDir()
		docPath := filepath.Join(dir, "report.md")
		if err := os.WriteFile(docPath, []byte("---\ntitle: Watch HTML\n---\n\nInitial paragraph.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var stdout, stderr syncBuffer
		done := make(chan int, 1)
		go func() {
			done <- cli.Run(ctx, []string{"build", "report.md", "--to", "html", "--watch"}, cli.Options{
				Stdout:   &stdout,
				Stderr:   &stderr,
				WorkDir:  dir,
				HomeDir:  dir,
				CacheDir: filepath.Join(dir, "cache"),
				Version:  "0.1.0-test",
			})
		}()

		outPath := filepath.Join(dir, "report.html")
		waitUntil(t, 3*time.Second, "initial HTML build output", func() bool {
			return strings.Contains(stdout.String(), "wrote "+outPath) &&
				strings.Contains(stdout.String(), "watching ")
		})

		htmlBytes, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("expected output HTML %s: %v", outPath, err)
		}
		if !strings.Contains(string(htmlBytes), "Initial paragraph.") {
			t.Fatalf("expected initial paragraph in HTML output")
		}

		cancel()
		select {
		case code := <-done:
			if code != cli.ExitSuccess {
				t.Fatalf("expected ExitSuccess (0) on cancel, got %d (stderr: %s)", code, stderr.String())
			}
		case <-time.After(3 * time.Second):
			t.Fatal("watch did not exit promptly on context cancellation")
		}
	})

	t.Run("document and slides initial builds via FakeTypstRunner", func(t *testing.T) {
		for _, format := range []string{"document", "slides"} {
			t.Run(format, func(t *testing.T) {
				dir := t.TempDir()
				docPath := filepath.Join(dir, "deck.md")
				content := "---\ntitle: Watch " + format + "\n---\n\n# Slide One\n\nBody text.\n"
				if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}

				var compileCount atomic.Int32
				runner := &toolchain.FakeTypstRunner{
					OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
						compileCount.Add(1)
						return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
					},
					OnQuery: func(_ context.Context, _ toolchain.QueryRequest) ([]byte, error) {
						return []byte(`[{"value":{"kind":"start","index":1,"title":"Watch slides","page":1}},{"value":{"kind":"end","index":1,"title":"Watch slides","page":1}}]`), nil
					},
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				var stdout, stderr syncBuffer
				done := make(chan int, 1)
				go func() {
					done <- cli.Run(ctx, []string{"build", "deck.md", "--to", format, "--watch"}, cli.Options{
						Stdout:      &stdout,
						Stderr:      &stderr,
						WorkDir:     dir,
						HomeDir:     dir,
						CacheDir:    filepath.Join(dir, "cache"),
						Version:     "0.1.0-test",
						TypstRunner: runner,
						LookPath:    func(string) (string, error) { return "/usr/bin/typst", nil },
						GOOS:        "linux",
						GOARCH:      "amd64",
					})
				}()

				outPath := filepath.Join(dir, "deck.pdf")
				waitUntil(t, 3*time.Second, "initial "+format+" build", func() bool {
					return strings.Contains(stdout.String(), "wrote "+outPath) &&
						strings.Contains(stdout.String(), "watching ")
				})
				if compileCount.Load() != 1 {
					t.Fatalf("expected 1 initial compile, got %d", compileCount.Load())
				}

				cancel()
				select {
				case code := <-done:
					if code != cli.ExitSuccess {
						t.Fatalf("got exit %d, want 0 (stderr: %s)", code, stderr.String())
					}
				case <-time.After(3 * time.Second):
					t.Fatal("watch did not exit on context cancellation")
				}
			})
		}
	})
}

type watchSession struct {
	t        *testing.T
	dir      string
	stdout   *syncBuffer
	stderr   *syncBuffer
	cancel   context.CancelFunc
	exitCode atomic.Int32
	exited   chan struct{}
}

func startWatch(t *testing.T, dir string, runner toolchain.TypstRunner, args ...string) *watchSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stdout := &syncBuffer{}
	stderr := &syncBuffer{}
	s := &watchSession{
		t:      t,
		dir:    dir,
		stdout: stdout,
		stderr: stderr,
		cancel: cancel,
		exited: make(chan struct{}),
	}
	go func() {
		defer close(s.exited)
		code := cli.Run(ctx, args, cli.Options{
			Stdout:      stdout,
			Stderr:      stderr,
			WorkDir:     dir,
			HomeDir:     dir,
			CacheDir:    filepath.Join(dir, "cache"),
			Version:     "0.1.0-test",
			TypstRunner: runner,
			LookPath:    func(string) (string, error) { return "/usr/bin/typst", nil },
			GOOS:        "linux",
			GOARCH:      "amd64",
		})
		s.exitCode.Store(int32(code))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.exited:
		case <-time.After(3 * time.Second):
			t.Errorf("watch session failed to terminate during cleanup")
		}
	})
	return s
}

func (s *watchSession) waitWrites(want int) {
	s.t.Helper()
	waitUntil(s.t, 4*time.Second, "expected build write count", func() bool {
		return strings.Count(s.stdout.String(), "wrote ") >= want
	}, func() string {
		return "stdout:\n" + s.stdout.String() + "\nstderr:\n" + s.stderr.String()
	})
}

func (s *watchSession) waitReady() {
	s.t.Helper()
	waitUntil(s.t, 4*time.Second, "watch ready banner", func() bool {
		return strings.Contains(s.stdout.String(), "watching ")
	}, func() string {
		return "stdout:\n" + s.stdout.String() + "\nstderr:\n" + s.stderr.String()
	})
}

func (s *watchSession) stopAndExpectZero() {
	s.t.Helper()
	s.cancel()
	select {
	case <-s.exited:
		if code := int(s.exitCode.Load()); code != cli.ExitSuccess {
			s.t.Fatalf("got exit %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitSuccess, s.stdout.String(), s.stderr.String())
		}
	case <-time.After(3 * time.Second):
		s.t.Fatal("watch did not exit on context cancellation")
	}
}

func TestWatchDebouncesRapidSavesAndSurvivesAtomicRename(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "report.md")
	outPath := filepath.Join(dir, "report.html")
	if err := os.WriteFile(docPath, []byte("---\ntitle: Debounce Test\n---\n\nVersion 0.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := startWatch(t, dir, nil, "build", "report.md", "--to", "html", "--watch")
	ws.waitWrites(1)
	ws.waitReady()

	// Rapid burst of 5 consecutive saves within ~30ms (mixing direct WriteFile and atomic rename).
	for i := 1; i <= 4; i++ {
		body := "---\ntitle: Debounce Test\n---\n\nVersion " + string(rune('0'+i)) + ".\n"
		if err := os.WriteFile(docPath, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	atomicWriteFile(t, docPath, []byte("---\ntitle: Debounce Test\n---\n\nVersion 5 final.\n"))

	ws.waitWrites(2)

	// Allow more than 2x debounce window to ensure no duplicate rebuilds or outPath loop occur.
	time.Sleep(300 * time.Millisecond)
	if got := strings.Count(ws.stdout.String(), "wrote "); got != 2 {
		t.Fatalf("expected 5 rapid saves to debounce into 1 rebuild (total 2 writes), got %d writes:\n%s", got, ws.stdout.String())
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Version 5 final.") {
		t.Fatalf("expected final burst content in %s", outPath)
	}

	// Another atomic save-via-rename must still be observed (parent-dir watch not lost).
	atomicWriteFile(t, docPath, []byte("---\ntitle: Debounce Test\n---\n\nVersion 6 post-rename.\n"))
	ws.waitWrites(3)
	raw, err = os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Version 6 post-rename.") {
		t.Fatalf("expected post-rename content in %s", outPath)
	}

	ws.stopAndExpectZero()
}

func TestWatchTracksFiguresBibCSLDatafolioYAMLAndTokens(t *testing.T) {
	t.Run("html tracks figures, bib, upward datafolio.yaml, Token Contract, dynamic new dir, and ignores unrelated files", func(t *testing.T) {
		root := t.TempDir()
		docsDir := filepath.Join(root, "docs")
		assetsDir := filepath.Join(docsDir, "assets")
		newAssetsDir := filepath.Join(docsDir, "newassets")
		refsDir := filepath.Join(root, "refs")
		designDir := filepath.Join(root, "design")
		for _, d := range []string{docsDir, assetsDir, newAssetsDir, refsDir, designDir} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}

		// Active datafolio.yaml in project root (parent of docsDir).
		datafolioYAML := filepath.Join(root, "datafolio.yaml")
		if err := os.WriteFile(datafolioYAML, []byte("theme: paper\nformat: html\ntokens: design/tokens.yaml\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Active Token Contract.
		tokensYAML := filepath.Join(designDir, "tokens.yaml")
		if err := os.WriteFile(tokensYAML, []byte("color:\n  accent:\n    $type: color\n    $value: \"#1e3a8a\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Referenced .bib file.
		bibPath := filepath.Join(refsDir, "papers.bib")
		if err := os.WriteFile(bibPath, []byte("@article{knuth1984,\n  author = {Donald Knuth},\n  title = {Initial Bib Title},\n  year = {1984}\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Referenced local SVG figure.
		svgPath := filepath.Join(assetsDir, "chart.svg")
		if err := os.WriteFile(svgPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#111111"/></svg>`), 0o644); err != nil {
			t.Fatal(err)
		}

		// Dynamic figure that will be added on a later edit.
		addedSVG := filepath.Join(newAssetsDir, "added.svg")
		if err := os.WriteFile(addedSVG, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="#aaaaaa"/></svg>`), 0o644); err != nil {
			t.Fatal(err)
		}

		docPath := filepath.Join(docsDir, "report.md")
		docContent := `---
title: Dependency Tracking
bibliography: ../refs/papers.bib
---

See [@knuth1984] for details.

![Initial chart](assets/chart.svg)

Source: Telemetry
`
		if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
			t.Fatal(err)
		}

		ws := startWatch(t, root, nil, "build", "docs/report.md", "--watch")
		ws.waitWrites(1)
		ws.waitReady()

		outPath := filepath.Join(docsDir, "report.html")
		readOut := func() string {
			b, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}

		initialHTML := readOut()
		if !strings.Contains(initialHTML, "#1e3a8a") || !strings.Contains(initialHTML, "#111111") || !strings.Contains(initialHTML, "Initial Bib Title") {
			t.Fatalf("initial HTML missing expected token/figure/bib contents")
		}

		// Unrelated file change must NOT trigger rebuild.
		if err := os.WriteFile(filepath.Join(docsDir, "scratch.txt"), []byte("ignore me"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(250 * time.Millisecond)
		if got := strings.Count(ws.stdout.String(), "wrote "); got != 1 {
			t.Fatalf("unrelated file triggered rebuild (writes=%d)", got)
		}

		// 1. Edit referenced SVG figure.
		if err := os.WriteFile(svgPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#224466"/></svg>`), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(2)
		if !strings.Contains(readOut(), "#224466") {
			t.Fatalf("expected updated SVG figure in HTML after figure edit")
		}

		// 2. Edit referenced .bib file.
		if err := os.WriteFile(bibPath, []byte("@article{knuth1984,\n  author = {Donald Knuth},\n  title = {Updated Bib Title},\n  year = {1984}\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(3)
		if !strings.Contains(readOut(), "Updated Bib Title") {
			t.Fatalf("expected updated bib title in HTML after .bib edit")
		}

		// 3. Edit active Token Contract.
		if err := os.WriteFile(tokensYAML, []byte("color:\n  accent:\n    $type: color\n    $value: \"#9a3412\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(4)
		if !strings.Contains(readOut(), "#9a3412") {
			t.Fatalf("expected updated token accent in HTML after Token Contract edit")
		}

		// 4. Edit active datafolio.yaml.
		if err := os.WriteFile(datafolioYAML, []byte("theme: night\nformat: html\ntokens: design/tokens.yaml\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(5)
		if !strings.Contains(readOut(), `data-theme="night"`) {
			t.Fatalf("expected night theme in HTML after datafolio.yaml edit")
		}

		// 5. Edit Source Document to add a new figure in a new subdirectory, then edit that new figure.
		docWithNewFig := docContent + "\n![Added](newassets/added.svg)\n\nSource: New\n"
		if err := os.WriteFile(docPath, []byte(docWithNewFig), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(6)

		if err := os.WriteFile(addedSVG, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="#bbbbbb"/></svg>`), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(7)
		if !strings.Contains(readOut(), "#bbbbbb") {
			t.Fatalf("expected newly added figure edit to trigger rebuild")
		}

		ws.stopAndExpectZero()
	})

	t.Run("slides tracks local CSL file, slide figure, and sidenote inline image", func(t *testing.T) {
		root := t.TempDir()
		mediaDir := filepath.Join(root, "media")
		cslDir := filepath.Join(root, "styles")
		for _, d := range []string{mediaDir, cslDir} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}

		cslPath := filepath.Join(cslDir, "custom.csl")
		if err := os.WriteFile(cslPath, []byte("<style/>"), 0o644); err != nil {
			t.Fatal(err)
		}
		bibPath := filepath.Join(root, "refs.bib")
		if err := os.WriteFile(bibPath, []byte("@book{tufte2001,\n  author={Edward Tufte},\n  title={The Visual Display},\n  year={2001}\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		slideFig := filepath.Join(mediaDir, "slide-fig.svg")
		if err := os.WriteFile(slideFig, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#123456"/></svg>`), 0o644); err != nil {
			t.Fatal(err)
		}
		sideImg := filepath.Join(mediaDir, "side-fig.png")
		if err := os.WriteFile(sideImg, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, 0o644); err != nil {
			t.Fatal(err)
		}

		deckPath := filepath.Join(root, "deck.md")
		deck := `---
title: Slide Asset Deck
bibliography: refs.bib
csl: styles/custom.csl
---

Cover note[^sn1].

[^sn1]: Sidenote with inline image ![thumb](media/side-fig.png).

---

# Slide Two

Cited [@tufte2001].

![Slide chart](media/slide-fig.svg)

Source: Lab
`
		if err := os.WriteFile(deckPath, []byte(deck), 0o644); err != nil {
			t.Fatal(err)
		}

		var compiles atomic.Int32
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				compiles.Add(1)
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			OnQuery: func(_ context.Context, _ toolchain.QueryRequest) ([]byte, error) {
				return []byte(`[
					{"value":{"kind":"start","index":1,"title":"Slide Asset Deck","page":1}},
					{"value":{"kind":"end","index":1,"title":"Slide Asset Deck","page":1}},
					{"value":{"kind":"start","index":2,"title":"Slide Two","page":2}},
					{"value":{"kind":"end","index":2,"title":"Slide Two","page":2}}
				]`), nil
			},
		}

		ws := startWatch(t, root, runner, "build", "deck.md", "--to", "slides", "--watch")
		ws.waitWrites(1)
		ws.waitReady()

		// Edit local CSL file.
		if err := os.WriteFile(cslPath, []byte("<style version=\"1.0\"/>"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(2)

		// Edit slide figure.
		if err := os.WriteFile(slideFig, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#654321"/></svg>`), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(3)

		// Edit sidenote inline image.
		if err := os.WriteFile(sideImg, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}, 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(4)

		if compiles.Load() != 4 {
			t.Fatalf("expected 4 compiles across initial + CSL + slide fig + sidenote img, got %d", compiles.Load())
		}
		ws.stopAndExpectZero()
	})
}

func TestWatchRecoversFromSyntaxCompileAndSlideOverflowErrors(t *testing.T) {
	t.Run("recovers from frontmatter syntax error and broken Token Contract without exiting", func(t *testing.T) {
		dir := t.TempDir()
		tokensPath := filepath.Join(dir, "tokens.yaml")
		if err := os.WriteFile(tokensPath, []byte("color:\n  accent:\n    $type: color\n    $value: \"#8b1e1e\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		docPath := filepath.Join(dir, "report.md")
		validDoc := "---\ntitle: Recovery Doc\ntokens: tokens.yaml\n---\n\nValid body v1.\n"
		if err := os.WriteFile(docPath, []byte(validDoc), 0o644); err != nil {
			t.Fatal(err)
		}

		ws := startWatch(t, dir, nil, "build", "report.md", "--to", "html", "--watch")
		ws.waitWrites(1)
		ws.waitReady()

		// 1. Introduce invalid YAML frontmatter.
		if err := os.WriteFile(docPath, []byte("---\ntitle: [broken\n---\n\nBroken.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, 4*time.Second, "frontmatter error on stderr", func() bool {
			return strings.Contains(ws.stderr.String(), "invalid YAML frontmatter")
		})
		if got := strings.Count(ws.stdout.String(), "wrote "); got != 1 {
			t.Fatalf("expected no wrote line on failed build, got %d", got)
		}

		// 2. Recover with valid markdown.
		validDocV2 := "---\ntitle: Recovery Doc\ntokens: tokens.yaml\n---\n\nRecovered body v2.\n"
		if err := os.WriteFile(docPath, []byte(validDocV2), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(2)

		outBytes, err := os.ReadFile(filepath.Join(dir, "report.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(outBytes), "Recovered body v2.") {
			t.Fatalf("expected recovered HTML output")
		}

		// 3. Introduce invalid Token Contract YAML, then fix Token Contract directly.
		if err := os.WriteFile(tokensPath, []byte("color:\n  accent: [invalid\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		prevErrLen := len(ws.stderr.String())
		waitUntil(t, 4*time.Second, "token parse error on stderr", func() bool {
			return len(ws.stderr.String()) > prevErrLen
		})

		if err := os.WriteFile(tokensPath, []byte("color:\n  accent:\n    $type: color\n    $value: \"#166534\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(3)
		outBytes, err = os.ReadFile(filepath.Join(dir, "report.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(outBytes), "#166534") {
			t.Fatalf("expected recovered token color #166534 in HTML output")
		}

		ws.stopAndExpectZero()
	})

	t.Run("recovers from Typst compile error in document Format", func(t *testing.T) {
		dir := t.TempDir()
		docPath := filepath.Join(dir, "memo.md")
		if err := os.WriteFile(docPath, []byte("---\ntitle: Memo\n---\n\nInitial valid.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		var compileCalls atomic.Int32
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				call := compileCalls.Add(1)
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				if strings.Contains(string(raw), "TRIGGERCOMPILEERR") {
					return os.ErrInvalid
				}
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n%call="+string(rune('0'+call))+"\n"), 0o644)
			},
		}

		ws := startWatch(t, dir, runner, "build", "memo.md", "--to", "document", "--watch")
		ws.waitWrites(1)
		ws.waitReady()

		// Trigger compile failure.
		if err := os.WriteFile(docPath, []byte("---\ntitle: Memo\n---\n\nTRIGGERCOMPILEERR\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, 4*time.Second, "typst compile error on stderr", func() bool {
			return strings.Contains(ws.stderr.String(), "build:")
		})

		// Fix document and recover.
		if err := os.WriteFile(docPath, []byte("---\ntitle: Memo\n---\n\nFixed valid body.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(2)
		if compileCalls.Load() != 3 {
			t.Fatalf("expected 3 total compile attempts (ok, err, ok), got %d", compileCalls.Load())
		}

		ws.stopAndExpectZero()
	})

	t.Run("recovers from Slide Overflow error in slides Format", func(t *testing.T) {
		dir := t.TempDir()
		deckPath := filepath.Join(dir, "deck.md")
		if err := os.WriteFile(deckPath, []byte("---\ntitle: Deck\n---\n\nCover\n\n---\n\n# Slide Two\n\nConcise.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		var overflowNext atomic.Bool
		runner := &toolchain.FakeTypstRunner{
			OnCompile: func(_ context.Context, req toolchain.CompileRequest) error {
				raw, err := os.ReadFile(req.InputPath)
				if err != nil {
					return err
				}
				overflowNext.Store(strings.Contains(string(raw), "TOOMUCHCONTENT"))
				return os.WriteFile(req.OutputPath, []byte("%PDF-1.7\n"), 0o644)
			},
			OnQuery: func(_ context.Context, _ toolchain.QueryRequest) ([]byte, error) {
				if overflowNext.Load() {
					return []byte(`[
						{"value":{"kind":"start","index":1,"title":"Deck","page":1}},
						{"value":{"kind":"end","index":1,"title":"Deck","page":1}},
						{"value":{"kind":"start","index":2,"title":"Slide Two","page":2}},
						{"value":{"kind":"end","index":2,"title":"Slide Two","page":3}}
					]`), nil
				}
				return []byte(`[
					{"value":{"kind":"start","index":1,"title":"Deck","page":1}},
					{"value":{"kind":"end","index":1,"title":"Deck","page":1}},
					{"value":{"kind":"start","index":2,"title":"Slide Two","page":2}},
					{"value":{"kind":"end","index":2,"title":"Slide Two","page":2}}
				]`), nil
			},
		}

		ws := startWatch(t, dir, runner, "build", "deck.md", "--to", "slides", "--watch")
		ws.waitWrites(1)
		ws.waitReady()

		// Trigger Slide Overflow.
		if err := os.WriteFile(deckPath, []byte("---\ntitle: Deck\n---\n\nCover\n\n---\n\n# Slide Two\n\nTOOMUCHCONTENT\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, 4*time.Second, "slide overflow diagnostic on stderr", func() bool {
			return strings.Contains(ws.stderr.String(), "slide overflow") &&
				strings.Contains(ws.stderr.String(), "Slide Two")
		})
		// Overflowing PDF must not be left behind.
		if _, err := os.Stat(filepath.Join(dir, "deck.pdf")); !os.IsNotExist(err) {
			t.Fatalf("expected overflowing deck.pdf to be removed, stat err=%v", err)
		}

		// Trim slide and recover.
		if err := os.WriteFile(deckPath, []byte("---\ntitle: Deck\n---\n\nCover\n\n---\n\n# Slide Two\n\nTrimmed.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(2)
		if _, err := os.Stat(filepath.Join(dir, "deck.pdf")); err != nil {
			t.Fatalf("expected recovered deck.pdf to exist: %v", err)
		}

		ws.stopAndExpectZero()
	})

	t.Run("recovers when initial build has transient syntax error", func(t *testing.T) {
		dir := t.TempDir()
		docPath := filepath.Join(dir, "broken_start.md")
		if err := os.WriteFile(docPath, []byte("---\ntitle: [unclosed\n---\n\nOops.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		ws := startWatch(t, dir, nil, "build", "broken_start.md", "--to", "html", "--watch")
		ws.waitReady()
		if !strings.Contains(ws.stderr.String(), "invalid YAML frontmatter") {
			t.Fatalf("expected initial syntax error on stderr, got %q", ws.stderr.String())
		}

		if err := os.WriteFile(docPath, []byte("---\ntitle: Fixed Start\n---\n\nNow valid.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ws.waitWrites(1)
		outBytes, err := os.ReadFile(filepath.Join(dir, "broken_start.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(outBytes), "Now valid.") {
			t.Fatalf("expected recovered HTML output after initial transient failure")
		}

		ws.stopAndExpectZero()
	})
}

func TestWatchMissingInputAndHelpUsage(t *testing.T) {
	dir := t.TempDir()
	code, _, errOut := runCLI(t, dir, "build", "does-not-exist.md", "--watch")
	if code != cli.ExitError {
		t.Fatalf("missing input with --watch: got exit %d, want %d (stderr=%q)", code, cli.ExitError, errOut)
	}

	code, out, errOut := runCLI(t, dir, "build", "--help")
	if code != cli.ExitSuccess {
		t.Fatalf("build --help failed (%d): %s", code, errOut)
	}
	if !strings.Contains(out, "--watch") {
		t.Fatalf("expected build --help output to mention --watch, got:\n%s", out)
	}
}
