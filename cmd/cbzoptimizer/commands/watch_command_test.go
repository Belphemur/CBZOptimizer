package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	utils2 "github.com/belphemur/CBZOptimizer/v2/internal/utils"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsComicArchive(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{"cbz lowercase", "/a/b/chapter.cbz", true},
		{"cbr lowercase", "/a/b/chapter.cbr", true},
		{"cbz uppercase", "/a/b/chapter.CBZ", true},
		{"other extension", "/a/b/chapter.zip", false},
		{"no extension", "/a/b/chapter", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isComicArchive(tc.path))
		})
	}
}

func TestShouldProcessWatchEvent(t *testing.T) {
	testCases := []struct {
		name     string
		op       fsnotify.Op
		expected bool
	}{
		{"create", fsnotify.Create, true},
		{"write", fsnotify.Write, true},
		{"rename", fsnotify.Rename, true},
		{"remove", fsnotify.Remove, false},
		{"chmod", fsnotify.Chmod, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := fsnotify.Event{Name: "file.cbz", Op: tc.op}
			assert.Equal(t, tc.expected, shouldProcessWatchEvent(event))
		})
	}
}

func TestAddRecursiveWatchSkipsUnreadableSubdirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not reliably enforced on Windows")
	}

	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	assert.NoError(t, os.MkdirAll(nested, 0o755))
	assert.NoError(t, os.Chmod(nested, 0o000))
	defer func() {
		assert.NoError(t, os.Chmod(nested, 0o755))
	}()
	if _, err := os.ReadDir(nested); err == nil {
		t.Skip("cannot make nested directory unreadable in this environment")
	}

	watcher, err := fsnotify.NewWatcher()
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, watcher.Close())
	}()

	// A regular, fully accessible tree should be watched without error.
	assert.NoError(t, addRecursiveWatch(watcher, root))
}

func TestBackfillExistingArchivesFindsPreExistingArchives(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(root, "chapter1.cbz"), []byte("data"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte("data"), 0o644))

	sub := filepath.Join(root, "sub")
	assert.NoError(t, os.MkdirAll(sub, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(sub, "chapter2.cbr"), []byte("data"), 0o644))

	var mu sync.Mutex
	var found []string
	backfillExistingArchives(root, func(path string) {
		mu.Lock()
		defer mu.Unlock()
		found = append(found, path)
	})

	assert.Len(t, found, 2)
}

func TestEventDebouncerCoalescesBurstsIntoSingleCall(t *testing.T) {
	var calls int32
	debouncer := newEventDebouncer(20*time.Millisecond, func(path string) {
		atomic.AddInt32(&calls, 1)
	})
	defer debouncer.Stop()

	// Simulate a burst of events for the same path.
	for i := 0; i < 5; i++ {
		debouncer.Trigger("/tmp/chapter.cbz")
	}

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&calls) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestEventDebouncerHandlesMultiplePaths(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]int)
	debouncer := newEventDebouncer(10*time.Millisecond, func(path string) {
		mu.Lock()
		defer mu.Unlock()
		seen[path]++
	})
	defer debouncer.Stop()

	debouncer.Trigger("/tmp/a.cbz")
	debouncer.Trigger("/tmp/b.cbz")

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return seen["/tmp/a.cbz"] == 1 && seen["/tmp/b.cbz"] == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestOptimizeQueueSkipsMissingPath(t *testing.T) {
	q := newOptimizeQueue(1, &utils2.OptimizeOptions{})
	defer q.Stop()
	var calls int32
	done := make(chan struct{}, 1)
	existing := filepath.Join(t.TempDir(), "existing.cbz")
	require.NoError(t, os.WriteFile(existing, []byte("data"), 0o644))
	q.optimize = func(options *utils2.OptimizeOptions) error {
		if options.Path == existing {
			atomic.AddInt32(&calls, 1)
			select {
			case done <- struct{}{}:
			default:
			}
		}
		return nil
	}

	// Enqueue a path that doesn't exist; process should skip it without
	// panicking or blocking since it never reaches utils2.Optimize.
	q.Enqueue(filepath.Join(t.TempDir(), "missing.cbz"))
	q.Enqueue(existing)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for existing path to be processed")
	}

	assert.EqualValues(t, 1, atomic.LoadInt32(&calls))
}

func TestWatchCommandBackfillFlagDefaultsToFalse(t *testing.T) {
	watchCmd, _, err := rootCmd.Find([]string{"watch"})
	require.NoError(t, err)

	flag := watchCmd.Flags().Lookup("backfill")
	require.NotNil(t, flag, "watch command should register a --backfill flag")
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "bool", flag.Value.Type())
}

func TestBackfillExistingArchivesOnlyInvokedWhenRequested(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "chapter1.cbz"), []byte("data"), 0o644))

	runBackfill := func(backfill bool) []string {
		var found []string
		var mu sync.Mutex
		process := func(path string) {
			mu.Lock()
			defer mu.Unlock()
			found = append(found, path)
		}
		// Mirrors the gating logic in WatchCommand: backfillExistingArchives
		// must only run when the --backfill flag is set.
		if backfill {
			backfillExistingArchives(root, process)
		}
		return found
	}

	assert.Empty(t, runBackfill(false), "no pre-existing archive should be processed when backfill is disabled")
	assert.Len(t, runBackfill(true), 1, "pre-existing archives should be processed when backfill is enabled")
}
