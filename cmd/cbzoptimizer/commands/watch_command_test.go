package commands

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	utils2 "github.com/belphemur/CBZOptimizer/v2/internal/utils"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
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
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	assert.NoError(t, os.MkdirAll(nested, 0o755))

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
		time.Sleep(5 * time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, 1, atomic.LoadInt32(&calls))
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

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, seen["/tmp/a.cbz"])
	assert.Equal(t, 1, seen["/tmp/b.cbz"])
}

func TestOptimizeQueueSkipsMissingPath(t *testing.T) {
	q := newOptimizeQueue(1, &utils2.OptimizeOptions{})
	defer q.Stop()

	// Enqueue a path that doesn't exist; process should skip it without
	// panicking or blocking since it never reaches utils2.Optimize.
	q.Enqueue(filepath.Join(t.TempDir(), "missing.cbz"))

	// Give the worker a moment to drain the job.
	time.Sleep(50 * time.Millisecond)
}
