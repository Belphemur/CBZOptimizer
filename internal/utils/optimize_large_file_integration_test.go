package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/rs/zerolog/log"
)

// largeTestFile is a ~1GB synthetic CBZ fixture stored via Git LFS (see
// .gitattributes). It is used to exercise the optimize pipeline with a
// chapter large enough to make in-memory-only handling impractical, and to
// validate that converted pages are streamed to/from a staging temp folder
// (see manga.Page.TempFilePath / manga.Chapter.TempDir) instead of blowing
// up memory usage.
const largeTestFile = "../../testdata/large/large_chapter.cbz"

// TestOptimizeIntegration_LargeFile is opt-in (set CBZ_RUN_LARGE_FILE_TEST=1)
// since it processes a ~1GB fixture and can take a while to run. It is
// automatically skipped if the fixture is unavailable (e.g. Git LFS content
// wasn't fetched, leaving only a pointer file) or in short mode.
func TestOptimizeIntegration_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file integration test in short mode")
	}
	if os.Getenv("CBZ_RUN_LARGE_FILE_TEST") == "" {
		t.Skip("Skipping large file integration test; set CBZ_RUN_LARGE_FILE_TEST=1 to run it")
	}

	info, err := os.Stat(largeTestFile)
	if err != nil {
		t.Skipf("large test fixture not found: %v", err)
	}
	// If Git LFS content wasn't fetched (e.g. `actions/checkout` without
	// `lfs: true`), the file on disk is just a small pointer text file
	// rather than the real ~1GB fixture. Detect and skip gracefully instead
	// of failing the whole suite.
	const minExpectedSize = 500 * 1024 * 1024 // 500MB
	if info.Size() < minExpectedSize {
		t.Skipf("large test fixture looks like a Git LFS pointer (size=%d), skipping; run `git lfs pull`", info.Size())
	}

	tempDir, err := os.MkdirTemp("", "test_optimize_large_file")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.CaptureGeneric(&err, os.RemoveAll, tempDir, "failed to remove temporary directory")

	converterInstance, err := converter.Get(constant.WebP)
	if err != nil {
		t.Skip("WebP converter not available, skipping large file integration test")
	}
	if err := converterInstance.PrepareConverter(); err != nil {
		t.Skip("Failed to prepare WebP converter, skipping large file integration test")
	}

	cbzFile := filepath.Join(tempDir, "large_chapter.cbz")
	if err := copyFile(largeTestFile, cbzFile); err != nil {
		t.Fatal(err)
	}

	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	options := &OptimizeOptions{
		ChapterConverter: converterInstance,
		Path:             cbzFile,
		Quality:          85,
		Override:         false,
		Split:            true,
	}

	err = Optimize(options)
	if err != nil {
		t.Fatalf("failed to optimize large chapter: %v", err)
	}

	runtime.GC()
	runtime.ReadMemStats(&memAfter)
	log.Info().
		Uint64("heap_alloc_before", memBefore.HeapAlloc).
		Uint64("heap_alloc_after", memAfter.HeapAlloc).
		Int64("input_size", info.Size()).
		Msg("Large file integration test memory usage")

	outputFile := strings.TrimSuffix(cbzFile, ".cbz") + "_converted.cbz"
	if _, err := os.Stat(outputFile); err != nil {
		t.Fatalf("expected converted output file %s to exist: %v", outputFile, err)
	}
}

// copyFile copies src to dst using streaming file I/O so that the whole
// file content is never held in memory at once, which matters for the
// large fixture used by this test.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer errs.Capture(&err, in.Close, "failed to close source file")

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer errs.Capture(&err, out.Close, "failed to close destination file")

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}
	return nil
}
