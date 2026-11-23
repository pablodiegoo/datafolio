package watch_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pablodiegoo/datafolio/internal/watch"
)

type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitCond(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s", timeout, desc)
}

func TestWatchRunValidationAndPrecancelledContext(t *testing.T) {
	if err := watch.Run(context.Background(), watch.Options{}); err == nil {
		t.Fatal("expected error when Build callback is nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var called atomic.Bool
	err := watch.Run(ctx, watch.Options{
		Build: func(context.Context) watch.BuildResult {
			called.Store(true)
			return watch.BuildResult{}
		},
	})
	if err != nil {
		t.Fatalf("expected nil on pre-cancelled context, got %v", err)
	}
	if called.Load() {
		t.Fatal("Build must not run when context is already cancelled")
	}
}

func TestWatchPrunesRemovedDirectoriesIgnoresOutPathAndCreatesMissingSubdir(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, "dirA")
	dirB := filepath.Join(root, "dirB")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}

	depA := filepath.Join(dirA, "a.txt")
	depB := filepath.Join(dirB, "b.txt")
	missingSubdirFile := filepath.Join(root, "late", "asset.svg")
	outPath := filepath.Join(dirA, "out.html")

	if err := os.WriteFile(depA, []byte("a0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(depB, []byte("b0"), 0o644); err != nil {
		t.Fatal(err)
	}

	var phase atomic.Int32
	var builds atomic.Int32
	var stdout, stderr syncBuf

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- watch.Run(ctx, watch.Options{
			Stdout:   &stdout,
			Stderr:   &stderr,
			Debounce: 40 * time.Millisecond,
			Build: func(context.Context) watch.BuildResult {
				builds.Add(1)
				// Always write outPath in watched dirA to verify outPath events are ignored.
				_ = os.WriteFile(outPath, []byte("out"), 0o644)

				switch phase.Load() {
				case 0:
					return watch.BuildResult{OutPath: outPath, Deps: []string{depA}}
				case 1:
					// Switch dependency from dirA/a.txt to dirB/b.txt and a not-yet-existing subdir file.
					return watch.BuildResult{OutPath: outPath, Deps: []string{depB, missingSubdirFile}}
				default:
					if _, err := os.Stat(missingSubdirFile); err != nil {
						return watch.BuildResult{
							OutPath: outPath,
							Deps:    []string{depB, missingSubdirFile},
							Err:     errors.New("missing late asset"),
						}
					}
					return watch.BuildResult{OutPath: outPath, Deps: []string{depB, missingSubdirFile}}
				}
			},
		})
	}()

	waitCond(t, 2*time.Second, "watch ready", func() bool {
		return builds.Load() == 1 && strings.Contains(stdout.String(), "watching ")
	})

	// Switch dependency to dirB/b.txt + missingSubdirFile via depA trigger.
	phase.Store(1)
	if err := os.WriteFile(depA, []byte("a1"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitCond(t, 2*time.Second, "second build pruning dirA", func() bool {
		return builds.Load() == 2
	})

	// Editing pruned depA must NOT trigger a rebuild.
	if err := os.WriteFile(depA, []byte("a2"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := builds.Load(); got != 2 {
		t.Fatalf("expected pruned depA edit not to trigger rebuild, builds=%d", got)
	}

	// Editing depB triggers build #3 (which fails because late/asset.svg doesn't exist yet).
	phase.Store(2)
	if err := os.WriteFile(depB, []byte("b1"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitCond(t, 2*time.Second, "third build missing late asset", func() bool {
		return builds.Load() == 3 && strings.Contains(stderr.String(), "missing late asset")
	})

	// Creating late/ and late/asset.svg recovers cleanly.
	if err := os.MkdirAll(filepath.Dir(missingSubdirFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(missingSubdirFile, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitCond(t, 2*time.Second, "fourth build after late asset creation", func() bool {
		return builds.Load() == 4
	})

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("watch.Run returned error on cancel: %v", err)
	}
}

func TestWatchReleasesGoroutinesOnCancel(t *testing.T) {
	// Warm up any runtime background goroutines.
	runtime.GC()
	baseline := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		f := filepath.Join(dir, "doc.md")
		if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout syncBuf
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- watch.Run(ctx, watch.Options{
				Stdout:   &stdout,
				Debounce: 30 * time.Millisecond,
				Build: func(context.Context) watch.BuildResult {
					return watch.BuildResult{OutPath: filepath.Join(dir, "doc.html"), Deps: []string{f}}
				},
			})
		}()
		waitCond(t, 2*time.Second, "watch ready", func() bool {
			return strings.Contains(stdout.String(), "watching ")
		})
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("iteration %d: unexpected err %v", i, err)
		}
	}

	waitCond(t, 2*time.Second, "goroutines to return to baseline", func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= baseline
	})
}
