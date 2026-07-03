package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	utils2 "github.com/belphemur/CBZOptimizer/v2/internal/utils"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// debounceDelay is the quiet period we wait, per path, after the last fsnotify
// event before triggering an optimization. This coalesces the bursts of
// Write/Create/Rename events fsnotify emits while a file is still being
// written (e.g. copied or downloaded into the watched folder).
const debounceDelay = 2 * time.Second

func init() {
	if runtime.GOOS != "linux" {
		return
	}
	command := &cobra.Command{
		Use:   "watch [folder]",
		Short: "Watch a folder for new CBZ/CBR files",
		Long:  "Watch a folder for new CBZ/CBR files.\nIt will watch a folder for new CBZ/CBR files and optimize them.",
		RunE:  WatchCommand,
		Args:  cobra.ExactArgs(1),
	}

	// Setup common flags (format, quality, override, split, timeout) with viper binding
	setupCommonFlags(command, &converterType, 85, true, false, true)

	AddCommand(command)
}
func WatchCommand(_ *cobra.Command, args []string) error {
	path := args[0]
	if path == "" {
		return fmt.Errorf("path is required")
	}

	if !utils2.IsValidFolder(path) {
		return fmt.Errorf("the path needs to be a folder")
	}

	quality := uint8(viper.GetUint16("quality"))
	if quality <= 0 || quality > 100 {
		return fmt.Errorf("invalid quality value")
	}

	override := viper.GetBool("override")

	split := viper.GetBool("split")

	timeout := viper.GetDuration("timeout")

	converterType := constant.FindConversionFormat(viper.GetString("format"))
	chapterConverter, err := converter.Get(converterType)
	if err != nil {
		return fmt.Errorf("failed to get chapterConverter: %w", err)
	}

	err = chapterConverter.PrepareConverter()
	if err != nil {
		return fmt.Errorf("failed to prepare converter: %w", err)
	}
	log.Info().Str("path", path).Bool("override", override).Uint8("quality", quality).Str("format", converterType.String()).Bool("split", split).Msg("Watching directory")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	defer func() {
		if closeErr := watcher.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("Failed to close file watcher")
		}
	}()

	// Optimization jobs run in a small worker pool so the fsnotify event loop
	// is never blocked waiting on a conversion.
	queue := newOptimizeQueue(runtime.NumCPU(), &utils2.OptimizeOptions{
		ChapterConverter: chapterConverter,
		Quality:          quality,
		Override:         override,
		Split:            split,
		Timeout:          timeout,
	})
	defer queue.Stop()

	debouncer := newEventDebouncer(debounceDelay, queue.Enqueue)
	defer debouncer.Stop()

	// Note: existing archives already present under path when the watch
	// starts are intentionally left untouched. Watch mode only reacts to
	// filesystem events going forward; use the `optimize` command to process
	// a library's existing contents. Archives inside a directory that is
	// created/moved into the watched tree *after* startup are still
	// back-filled below, since only the directory itself generates an
	// fsnotify event.
	if err := addRecursiveWatch(watcher, path); err != nil {
		return fmt.Errorf("failed to watch path %s: %w", path, err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			log.Debug().Str("file", event.Name).Str("event", event.Op.String()).Msg("File event")

			if event.Has(fsnotify.Create) {
				fileInfo, err := os.Stat(event.Name)
				if err == nil && fileInfo.IsDir() {
					if err := addRecursiveWatch(watcher, event.Name); err != nil {
						log.Error().Err(err).Str("path", event.Name).Msg("Failed to watch created directory")
					}
					// The newly discovered directory may already contain
					// archives (e.g. a folder moved/copied in); back-fill them
					// since no further fsnotify event will target them.
					backfillExistingArchives(event.Name, debouncer.Trigger)
					continue
				}
			}

			if !shouldProcessWatchEvent(event) {
				continue
			}

			if !isComicArchive(event.Name) {
				continue
			}

			debouncer.Trigger(event.Name)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Error().Err(err).Msg("Watch error")
		}
	}
}

// addRecursiveWatch registers a watch on rootPath and every subdirectory
// beneath it. Directories that can't be read (e.g. permission errors) are
// logged and skipped instead of aborting the whole walk, so a single bad
// subtree doesn't prevent the rest of the folder from being watched.
func addRecursiveWatch(watcher *fsnotify.Watcher, rootPath string) error {
	return filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Skipping path while setting up watch")
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if err := watcher.Add(path); err != nil {
			if errors.Is(err, fs.ErrPermission) {
				log.Warn().Err(err).Str("path", path).Msg("Skipping unreadable directory while setting up watch")
				return nil
			}
			return fmt.Errorf("failed to watch directory %s: %w", path, err)
		}
		return nil
	})
}

// backfillExistingArchives walks rootPath for comic archives that already
// exist on disk and hands them to process. This covers the case where a
// directory (potentially already containing archives) is created/moved into
// the watched tree: only the directory itself generates an fsnotify event,
// so the files inside it would otherwise never be picked up.
func backfillExistingArchives(rootPath string, process func(path string)) {
	err := filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Skipping path while scanning for existing archives")
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !isComicArchive(path) {
			return nil
		}
		process(path)
		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("path", rootPath).Msg("Failed to scan directory for existing archives")
	}
}

func shouldProcessWatchEvent(event fsnotify.Event) bool {
	return event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Rename)
}

func isComicArchive(path string) bool {
	filename := strings.ToLower(path)
	return strings.HasSuffix(filename, ".cbz") || strings.HasSuffix(filename, ".cbr")
}

// eventDebouncer coalesces bursts of fsnotify events targeting the same path
// into a single call to onQuiet, fired after the path has been quiet for
// delay. This prevents repeated conversions being triggered while a file is
// still being written, and avoids running Optimize against a Rename event
// that reports the *old* (already gone) file name once the quiet period lets
// us re-check the path.
type eventDebouncer struct {
	delay   time.Duration
	onQuiet func(path string)

	mu       sync.Mutex
	timers   map[string]*debounceTimer
	inFlight sync.WaitGroup
	stopping bool
}

type debounceTimer struct {
	timer *time.Timer
}

func newEventDebouncer(delay time.Duration, onQuiet func(path string)) *eventDebouncer {
	return &eventDebouncer{
		delay:   delay,
		onQuiet: onQuiet,
		timers:  make(map[string]*debounceTimer),
	}
}

// Trigger (re)schedules onQuiet to run for path after the debounce delay,
// resetting any previously pending timer for the same path.
func (d *eventDebouncer) Trigger(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopping {
		return
	}

	if existing, exists := d.timers[path]; exists {
		if existing.timer.Stop() {
			d.inFlight.Done()
		}
	}

	entry := &debounceTimer{}
	d.inFlight.Add(1)
	entry.timer = time.AfterFunc(d.delay, func() {
		defer d.inFlight.Done()

		d.mu.Lock()
		// Only the still-current timer for this path is allowed to clear the
		// entry and fire onQuiet. If Trigger raced with this callback and
		// already installed a newer timer, this stale invocation must not
		// delete that newer entry or fire early.
		owns := d.timers[path] == entry && !d.stopping
		if owns {
			delete(d.timers, path)
		}
		d.mu.Unlock()
		if owns {
			d.onQuiet(path)
		}
	})
	d.timers[path] = entry
}

// Stop cancels all pending timers.
func (d *eventDebouncer) Stop() {
	d.mu.Lock()
	d.stopping = true
	for path, timer := range d.timers {
		if timer.timer.Stop() {
			d.inFlight.Done()
		}
		delete(d.timers, path)
	}
	d.mu.Unlock()
	d.inFlight.Wait()
}

// optimizeQueue is a small worker pool that runs Optimize jobs off of the
// fsnotify event loop, so a slow conversion never blocks event draining.
type optimizeQueue struct {
	jobs     chan string
	wg       sync.WaitGroup
	mu       sync.RWMutex
	stopped  bool
	options  *utils2.OptimizeOptions
	optimize func(options *utils2.OptimizeOptions) error
}

func newOptimizeQueue(workerCount int, options *utils2.OptimizeOptions) *optimizeQueue {
	if workerCount < 1 {
		workerCount = 1
	}
	q := &optimizeQueue{
		jobs:     make(chan string, 64),
		options:  options,
		optimize: utils2.Optimize,
	}
	q.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go q.worker()
	}
	return q
}

func (q *optimizeQueue) worker() {
	defer q.wg.Done()
	for path := range q.jobs {
		q.process(path)
	}
}

// process re-checks the path right before optimizing: fsnotify Rename events
// commonly report the *old* file name (already gone by the time we act on
// it), and Write can fire while a file is still being written elsewhere. In
// override mode, Optimize may overwrite/delete the source file, so skipping
// paths that no longer exist (or that are no longer regular files) avoids
// noisy failures.
func (q *optimizeQueue) process(path string) {
	info, err := os.Stat(path)
	if err != nil {
		log.Debug().Err(err).Str("file", path).Msg("Skipping watch event: path no longer accessible")
		return
	}
	if info.IsDir() {
		return
	}

	options := *q.options
	options.Path = path
	if err := q.optimize(&options); err != nil {
		log.Error().Err(err).Str("file", path).Msg("Error processing file")
	}
}

// Enqueue submits path for processing by the worker pool.
func (q *optimizeQueue) Enqueue(path string) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.stopped {
		return
	}
	q.jobs <- path
}

// Stop closes the job queue and waits for in-flight jobs to finish.
func (q *optimizeQueue) Stop() {
	q.mu.Lock()
	q.stopped = true
	close(q.jobs)
	q.mu.Unlock()
	q.wg.Wait()
}
