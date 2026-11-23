package watch

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounce is the default quiet period after a tracked file changes before rebuilding.
const DefaultDebounce = 120 * time.Millisecond

const relevantOps = fsnotify.Write | fsnotify.Create | fsnotify.Rename | fsnotify.Chmod

// BuildResult captures the outcome and discovered dependencies of a single build pass.
type BuildResult struct {
	OutPath string
	Deps    []string
	Err     error
}

// BuildFunc executes one build pass and reports the output path, tracked file dependencies, and any error.
type BuildFunc func(ctx context.Context) BuildResult

// Options configures the filesystem watch loop.
type Options struct {
	Build    BuildFunc
	Stdout   io.Writer
	Stderr   io.Writer
	Debounce time.Duration
	Label    string
}

// Run executes an initial build and watches tracked parent directories until ctx is cancelled.
func Run(ctx context.Context, opts Options) error {
	if opts.Build == nil {
		return fmt.Errorf("watch: Build callback is required")
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx.Err() != nil {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watch: create fsnotify watcher: %w", err)
	}
	defer watcher.Close()

	tracked := make(map[string]struct{})
	watchedDirs := make(map[string]struct{})
	var ignoredOutPath string

	initial := opts.Build(ctx)
	if ctx.Err() != nil {
		return nil
	}
	updateTracking(watcher, tracked, watchedDirs, &ignoredOutPath, initial)
	reportBuild(stdout, stderr, initial)

	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = DefaultDebounce
	}

	if opts.Label != "" {
		fmt.Fprintf(stdout, "watching %s (Ctrl+C to stop)\n", opts.Label)
	} else {
		fmt.Fprintln(stdout, "watching for changes (Ctrl+C to stop)")
	}

	var (
		timer   *time.Timer
		timerCh <-chan time.Time
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	scheduleRebuild := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounce)
		}
		timerCh = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if ev.Op&relevantOps == 0 {
				continue
			}
			evPath := normalizePath(ev.Name)
			if evPath == "" || evPath == ignoredOutPath {
				continue
			}
			if _, isTracked := tracked[evPath]; isTracked {
				scheduleRebuild()
				continue
			}
			if ev.Has(fsnotify.Create) && isAncestorOfAny(evPath, tracked) {
				syncWatchDirs(watcher, tracked, watchedDirs)
				scheduleRebuild()
			}
		case <-timerCh:
			timerCh = nil
			if ctx.Err() != nil {
				return nil
			}
			res := opts.Build(ctx)
			if ctx.Err() != nil {
				return nil
			}
			updateTracking(watcher, tracked, watchedDirs, &ignoredOutPath, res)
			reportBuild(stdout, stderr, res)
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(stderr, "watch: %v\n", watchErr)
		}
	}
}

func updateTracking(watcher *fsnotify.Watcher, tracked, watchedDirs map[string]struct{}, ignoredOut *string, res BuildResult) {
	if res.OutPath != "" {
		*ignoredOut = normalizePath(res.OutPath)
	}
	if res.Err == nil {
		clear(tracked)
	}
	for _, dep := range res.Deps {
		clean := normalizePath(dep)
		if clean == "" || clean == *ignoredOut {
			continue
		}
		tracked[clean] = struct{}{}
	}
	if *ignoredOut != "" {
		delete(tracked, *ignoredOut)
	}
	syncWatchDirs(watcher, tracked, watchedDirs)
}

func syncWatchDirs(watcher *fsnotify.Watcher, tracked, watchedDirs map[string]struct{}) {
	wantDirs := make(map[string]struct{})
	for f := range tracked {
		dir := nearestExistingDir(filepath.Clean(filepath.Dir(f)))
		if dir != "" {
			wantDirs[dir] = struct{}{}
		}
	}
	for dir := range wantDirs {
		if _, ok := watchedDirs[dir]; ok {
			continue
		}
		if err := watcher.Add(dir); err == nil {
			watchedDirs[dir] = struct{}{}
		}
	}
	for dir := range watchedDirs {
		if _, ok := wantDirs[dir]; !ok {
			_ = watcher.Remove(dir)
			delete(watchedDirs, dir)
		}
	}
}

func nearestExistingDir(dir string) string {
	cur := filepath.Clean(dir)
	for cur != "" {
		info, err := os.Stat(cur)
		if err == nil && info.IsDir() {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
	return ""
}

func isAncestorOfAny(dir string, tracked map[string]struct{}) bool {
	prefix := dir + string(filepath.Separator)
	for f := range tracked {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

func reportBuild(stdout, stderr io.Writer, res BuildResult) {
	if res.Err != nil {
		fmt.Fprintf(stderr, "build: %v\n", res.Err)
		return
	}
	fmt.Fprintf(stdout, "wrote %s\n", res.OutPath)
}

func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}
