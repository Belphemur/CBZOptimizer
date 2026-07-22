package cbz

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadChapter(t *testing.T) {
	type testCase struct {
		name               string
		filePath           string
		expectedPages      int
		expectedSeries     string
		expectedConversion bool
	}

	testCases := []testCase{
		{
			name:               "Original Chapter CBZ",
			filePath:           "../../testdata/Chapter 128.cbz",
			expectedPages:      14,
			expectedSeries:     "<Series>The Knight King Who Returned with a God</Series>",
			expectedConversion: false,
		},
		{
			name:               "Original Chapter CBR",
			filePath:           "../../testdata/Chapter 1.cbr",
			expectedPages:      16,
			expectedSeries:     "<Series>Boundless Necromancer</Series>",
			expectedConversion: false,
		},
		{
			name:               "Converted Chapter",
			filePath:           "../../testdata/Chapter 10_converted.cbz",
			expectedPages:      107,
			expectedSeries:     "<Series>Boundless Necromancer</Series>",
			expectedConversion: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chapter, err := LoadChapter(tc.filePath)
			require.NoError(t, err)
			defer func() { _ = chapter.Cleanup() }()

			assert.Equal(t, tc.expectedPages, len(chapter.Pages))

			assert.Contains(t, chapter.ComicInfoXml, tc.expectedSeries)

			assert.Equal(t, tc.expectedConversion, chapter.IsConverted)

			// Verify pages are on disk
			for _, page := range chapter.Pages {
			assert.NotEmpty(t, page.FilePath, "Page %d has no file path", page.Index)
			}
		})
	}
}

func TestIsAlreadyConverted(t *testing.T) {
	testCases := []struct {
		name     string
		filePath string
		expected bool
	}{
		{
			name:     "Converted CBZ",
			filePath: "../../testdata/Chapter 10_converted.cbz",
			expected: true,
		},
		{
			name:     "Original CBZ",
			filePath: "../../testdata/Chapter 128.cbz",
			expected: false,
		},
		{
			name:     "Original CBR",
			filePath: "../../testdata/Chapter 1.cbr",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := IsAlreadyConverted(context.Background(), tc.filePath)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsAlreadyConverted_NonexistentFile(t *testing.T) {
	_, err := IsAlreadyConverted(context.Background(), "/nonexistent/file.cbz")
	require.Error(t, err)
}

func TestIsAlreadyConverted_InvalidExtension(t *testing.T) {
	// Create a temp file with unsupported extension
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	result, err := IsAlreadyConverted(context.Background(), tmpFile.Name())
	require.NoError(t, err)
	assert.False(t, result, "Expected false for unsupported extension")
}

func TestIsAlreadyConverted_CBZWithConvertedComment(t *testing.T) {
	// Create a CBZ file with a zip comment containing a date (marks as converted)
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	_ = w.SetComment(time.Now().Format(time.RFC3339))
	// Add a dummy file
	fw, err := w.Create("page.webp")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("dummy"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(context.Background(), cbzPath)
	require.NoError(t, err)
	assert.True(t, result, "Expected CBZ with date comment to be detected as converted")
}

func TestIsAlreadyConverted_CBZWithNonDateComment(t *testing.T) {
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	_ = w.SetComment("this is not a date")
	fw, err := w.Create("page.jpg")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("dummy"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(context.Background(), cbzPath)
	require.NoError(t, err)
	assert.False(t, result, "Expected CBZ with non-date comment to NOT be detected as converted")
}

func TestExtractChapter_NonexistentFile(t *testing.T) {
	_, err := ExtractChapter(context.Background(), "/nonexistent/file.cbz", false)
	require.Error(t, err)
}

func TestExtractChapter_PageExtensions(t *testing.T) {
	chapter, err := ExtractChapter(context.Background(), "../../testdata/Chapter 128.cbz", false)
	require.NoError(t, err)
	defer func() { _ = chapter.Cleanup() }()

	// All pages should have valid image extensions
	validExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	for _, page := range chapter.Pages {
		assert.True(t, validExts[page.Extension], "Page %d has unexpected extension: %s", page.Index, page.Extension)
	}
}

func TestExtractChapter_PagesHaveSequentialIndices(t *testing.T) {
	t.Run("default sequential naming", func(t *testing.T) {
		chapter, err := ExtractChapter(context.Background(), "../../testdata/Chapter 128.cbz", false)
		require.NoError(t, err)
		defer func() { _ = chapter.Cleanup() }()

		for i, page := range chapter.Pages {
			assert.Equal(t, uint16(i), page.Index)
			assert.Empty(t, page.OriginalName, "keep-filenames disabled: OriginalName must stay empty")
		}
	})

	t.Run("keep-filenames records OriginalName", func(t *testing.T) {
		chapter, err := ExtractChapter(context.Background(), "../../testdata/Chapter 128.cbz", true)
		require.NoError(t, err)
		defer func() { _ = chapter.Cleanup() }()

		require.NotEmpty(t, chapter.Pages)
		for i, page := range chapter.Pages {
			assert.Equal(t, uint16(i), page.Index)
			assert.NotEmpty(t, page.OriginalName, "keep-filenames enabled: OriginalName must be set for every page")
			// OriginalName must be a bare base name (no directory prefix), even
			// when the source archive nests pages in subdirectories.
			assert.Equal(t, filepath.Base(page.OriginalName), page.OriginalName,
				"OriginalName should be a base filename with no path components")
		}
	})
}

// writeCollisionCBZ builds a CBZ containing the same image base name in two
// different subdirectories plus a normal unique page. The test fixture is
// specifically crafted to reproduce the race fixed in ExtractChapter: when
// keep-filenames naively copied filepath.Base(path) into OriginalName, both
// a/page.png and b/page.png would have ended up with the same stem and the
// WebP converter would have raced on a single shared intermediate path.
func writeCollisionCBZ(t *testing.T, dir string) string {
	t.Helper()
	cbzPath := filepath.Join(dir, "collisions.cbz")
	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)

	for _, entry := range []string{"a/page.png", "b/page.png", "cover.png"} {
		fw, err := w.Create(entry)
		require.NoError(t, err)
		_, err = fw.Write([]byte("dummy image bytes for " + entry))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
	return cbzPath
}

func TestExtractChapter_KeepFilenames_DeduplicatesCollidingNames(t *testing.T) {
	tmpDir := t.TempDir()
	cbzPath := writeCollisionCBZ(t, tmpDir)

	t.Run("keep-filenames resolves colliding base names", func(t *testing.T) {
		chapter, err := ExtractChapter(context.Background(), cbzPath, true)
		require.NoError(t, err)
		defer func() { _ = chapter.Cleanup() }()

		require.Len(t, chapter.Pages, 3, "expected 3 pages: two colliding + one unique")

		// All OriginalNames must be non-empty bare base names and unique.
		seen := make(map[string]struct{}, len(chapter.Pages))
		for _, page := range chapter.Pages {
			assert.NotEmpty(t, page.OriginalName, "page %d must have OriginalName set", page.Index)
			assert.Equal(t, filepath.Base(page.OriginalName), page.OriginalName,
				"page %d OriginalName should be a bare base filename", page.Index)
			if _, dup := seen[page.OriginalName]; dup {
				t.Errorf("duplicate OriginalName across pages: %q", page.OriginalName)
			}
			seen[page.OriginalName] = struct{}{}
		}

		// The unique page keeps its original name; the two colliding pages
		// must come out as one bare "page.png" and one indexed variant.
		assert.Contains(t, seen, "page.png", "one colliding page should keep the bare page.png name")
		assert.Contains(t, seen, "cover.png", "the unique page should keep its original name")

		indexedPattern := regexp.MustCompile(`^page_\d{4}\.png$`)
		indexedCount := 0
		for name := range seen {
			if indexedPattern.MatchString(name) {
				indexedCount++
			}
		}
		assert.Equal(t, 1, indexedCount,
			"exactly one colliding page should be resolved to the page_NNNN.png pattern, got %v", seen)
	})

	t.Run("keep-filenames off leaves OriginalName empty (regression guard)", func(t *testing.T) {
		// Same collision-prone archive, but with keep-filenames off: the
		// dedup logic must not run, so OriginalName stays empty for every
		// page. This is the historical default and must not regress.
		chapter, err := ExtractChapter(context.Background(), cbzPath, false)
		require.NoError(t, err)
		defer func() { _ = chapter.Cleanup() }()

		require.Len(t, chapter.Pages, 3)
		for _, page := range chapter.Pages {
			assert.Empty(t, page.OriginalName, "page %d must have empty OriginalName when keep-filenames is off", page.Index)
		}
	})
}

// writeBackslashCBZ builds a CBZ whose entry names mix Windows-style
// backslash separators with a normal forward-slash page. The fixture
// reproduces the CodeRabbit finding for PR #217: archive entries like
// "subdir\page.png" and "..\evil.png" must not surface verbatim as the
// OriginalName written into the output CBZ, since Windows ZIP consumers
// can interpret backslashes as path traversal.
func writeBackslashCBZ(t *testing.T, dir string) string {
	t.Helper()
	cbzPath := filepath.Join(dir, "backslash.cbz")
	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)

	// archive/zip stores entry names verbatim, so the bytes on the wire
	// carry the backslashes through to fs.WalkDir in ExtractChapter.
	for _, entry := range []string{`subdir\page.png`, `..\evil.png`, "cover.png"} {
		fw, err := w.Create(entry)
		require.NoError(t, err)
		_, err = fw.Write([]byte("dummy image bytes for " + entry))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
	return cbzPath
}

func TestExtractChapter_KeepFilenames_NormalizesWindowsSeparators(t *testing.T) {
	tmpDir := t.TempDir()
	cbzPath := writeBackslashCBZ(t, tmpDir)

	chapter, err := ExtractChapter(context.Background(), cbzPath, true)
	require.NoError(t, err)
	defer func() { _ = chapter.Cleanup() }()

	require.Len(t, chapter.Pages, 3, "expected 3 pages: two backslash entries + one normal")

	// Every OriginalName must be a bare base name: no backslashes, no
	// forward slashes, and unique across the chapter.
	seen := make(map[string]struct{}, len(chapter.Pages))
	for _, page := range chapter.Pages {
		assert.NotEmpty(t, page.OriginalName, "page %d must have OriginalName set", page.Index)
		assert.NotContains(t, page.OriginalName, `\`, "page %d OriginalName must not contain backslash, got %q", page.Index, page.OriginalName)
		assert.NotContains(t, page.OriginalName, "/", "page %d OriginalName must not contain forward slash, got %q", page.Index, page.OriginalName)
		assert.Equal(t, filepath.Base(page.OriginalName), page.OriginalName,
			"page %d OriginalName should be a bare base filename", page.Index)
		if _, dup := seen[page.OriginalName]; dup {
			t.Errorf("duplicate OriginalName across pages: %q", page.OriginalName)
		}
		seen[page.OriginalName] = struct{}{}
	}

	// The two backslash entries must collapse to their bare bases; the
	// "..\evil.png" entry's leading "..\evil" must not be carried through.
	assert.Contains(t, seen, "page.png", "subdir\\page.png should normalize to bare page.png")
	assert.Contains(t, seen, "evil.png", "..\\evil.png should normalize to bare evil.png")
	assert.Contains(t, seen, "cover.png", "cover.png should pass through unchanged")
}

// writeSameStemDifferentExtCBZ builds a CBZ containing two same-stem pages
// with different extensions plus a distinct unique page. The fixture
// reproduces the CodeRabbit finding for PR #217: a/page.png and b/page.jpg
// share a stem but do NOT share a full filename, so naive full-name
// deduplication lets both OriginalNames through and the WebP converter then
// races on a single shared intermediate output path (outputDir/page.webp).
// The fix flips the dedup tracking to STEM-uniqueness, so one of the
// colliding pair must come out as a bare "page.<ext>" and the other as
// "page_NNNN.<ext>" — each preserving its original extension.
func writeSameStemDifferentExtCBZ(t *testing.T, dir string) string {
	t.Helper()
	cbzPath := filepath.Join(dir, "same_stem.cbz")
	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)

	for _, entry := range []string{"a/page.png", "b/page.jpg", "cover.png"} {
		fw, err := w.Create(entry)
		require.NoError(t, err)
		_, err = fw.Write([]byte("dummy image bytes for " + entry))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
	return cbzPath
}

func TestExtractChapter_KeepFilenames_DeduplicatesSameStemDifferentExtensions(t *testing.T) {
	tmpDir := t.TempDir()
	cbzPath := writeSameStemDifferentExtCBZ(t, tmpDir)

	chapter, err := ExtractChapter(context.Background(), cbzPath, true)
	require.NoError(t, err)
	defer func() { _ = chapter.Cleanup() }()

	require.Len(t, chapter.Pages, 3, "expected 3 pages: two same-stem + one unique")

	// Every OriginalName must be a bare base name; full names AND stems
	// must each be unique across the chapter. The stem-uniqueness check is
	// the contract the WebP converter relies on (it strips the extension
	// and appends ".webp" when computing intermediatePageName).
	seenNames := make(map[string]struct{}, len(chapter.Pages))
	seenStems := make(map[string]struct{}, len(chapter.Pages))
	for _, page := range chapter.Pages {
		assert.NotEmpty(t, page.OriginalName, "page %d must have OriginalName set", page.Index)
		assert.Equal(t, filepath.Base(page.OriginalName), page.OriginalName,
			"page %d OriginalName should be a bare base filename", page.Index)
		if _, dup := seenNames[page.OriginalName]; dup {
			t.Errorf("duplicate OriginalName full name across pages: %q", page.OriginalName)
		}
		seenNames[page.OriginalName] = struct{}{}

		stem := strings.TrimSuffix(page.OriginalName, filepath.Ext(page.OriginalName))
		if _, dup := seenStems[stem]; dup {
			t.Errorf("duplicate OriginalName stem across pages: %q (from %q)", stem, page.OriginalName)
		}
		seenStems[stem] = struct{}{}
	}

	// The unique page keeps its original name untouched.
	assert.Contains(t, seenNames, "cover.png", "the unique page should keep its original name")

	// The colliding pair must split into one bare "page.<ext>" and one
	// "page_NNNN.<ext>". The extensions must differ — each must keep the
	// extension of the archive entry it came from. Walk order is not
	// guaranteed, so the test checks both shapes regardless of which
	// entry comes first.
	barePattern := regexp.MustCompile(`^page\.(png|jpg)$`)
	indexedPattern := regexp.MustCompile(`^page_\d{4}\.(png|jpg)$`)

	var bareExt, indexedExt string
	for name := range seenNames {
		if barePattern.MatchString(name) {
			bareExt = filepath.Ext(name)
		}
		if indexedPattern.MatchString(name) {
			indexedExt = filepath.Ext(name)
		}
	}
	assert.NotEmpty(t, bareExt,
		"expected one bare page.<ext> OriginalName from the colliding pair, got %v", seenNames)
	assert.NotEmpty(t, indexedExt,
		"expected one page_NNNN.<ext> OriginalName from the colliding pair, got %v", seenNames)
	assert.NotEqual(t, bareExt, indexedExt,
		"the bare and indexed variants must keep their original different extensions, got bare=%q indexed=%q",
		bareExt, indexedExt)
}

func TestExtractChapter_Cleanup(t *testing.T) {
	chapter, err := ExtractChapter(context.Background(), "../../testdata/Chapter 128.cbz", false)
	require.NoError(t, err)

	tempDir := chapter.TempDir
	// Verify temp dir exists
	assert.DirExists(t, tempDir)

	// Cleanup should remove temp dir
	err = chapter.Cleanup()
	require.NoError(t, err)

	assert.NoDirExists(t, tempDir)
}

func TestExtractChapter_CBR(t *testing.T) {
	chapter, err := ExtractChapter(context.Background(), "../../testdata/Chapter 1.cbr", false)
	require.NoError(t, err)
	defer func() { _ = chapter.Cleanup() }()

	assert.Len(t, chapter.Pages, 16)

	// All page files should exist on disk
	for _, page := range chapter.Pages {
		assert.FileExists(t, page.FilePath)
		}
}

func TestExtractChapter_ConvertedStatus(t *testing.T) {
	chapter, err := ExtractChapter(context.Background(), "../../testdata/Chapter 10_converted.cbz", false)
	require.NoError(t, err)
	defer func() { _ = chapter.Cleanup() }()

	assert.True(t, chapter.IsConverted, "Expected converted chapter to have IsConverted = true")
	assert.False(t, chapter.ConvertedTime.IsZero(), "Expected non-zero ConvertedTime for converted chapter")
}

func TestWriteChapterToCBZ_EmptyChapter(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "empty.cbz")

	chapter := &manga.Chapter{
		FilePath: filepath.Join(tmpDir, "source.cbz"),
		TempDir:  tmpDir,
		Pages:    []*manga.PageFile{},
	}

	err := WriteChapterToCBZ(chapter, outputPath)
	require.NoError(t, err)

	// Verify the file is a valid zip
	r, err := zip.OpenReader(outputPath)
	require.NoError(t, err)
	_ = r.Close()
}

func TestWriteChapterToCBZ_WithComicInfo(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.cbz")
	inputDir := filepath.Join(tmpDir, "input")
	_ = os.MkdirAll(inputDir, 0755)

	// Create a dummy page file
	pagePath := filepath.Join(inputDir, "0000.jpg")
	_ = os.WriteFile(pagePath, []byte("fake image data"), 0644)

	chapter := &manga.Chapter{
		FilePath:     filepath.Join(tmpDir, "source.cbz"),
		TempDir:      tmpDir,
		ComicInfoXml: `<?xml version="1.0"?><ComicInfo><Series>Test</Series></ComicInfo>`,
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".jpg", FilePath: pagePath},
		},
	}
	chapter.SetConverted()

	err := WriteChapterToCBZ(chapter, outputPath)
	require.NoError(t, err)

	// Verify ComicInfo.xml is present
	r, err := zip.OpenReader(outputPath)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	foundComicInfo := false
	for _, f := range r.File {
		if f.Name == "ComicInfo.xml" {
			foundComicInfo = true
		}
	}
	assert.True(t, foundComicInfo, "ComicInfo.xml not found in output CBZ")

	// Verify zip comment has converted timestamp
	assert.NotEmpty(t, r.Comment, "Expected zip comment with conversion timestamp")
}

func TestWriteChapterToCBZ_InvalidPagePath(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.cbz")

	chapter := &manga.Chapter{
		FilePath: filepath.Join(tmpDir, "source.cbz"),
		TempDir:  tmpDir,
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".jpg", FilePath: "/nonexistent/page.jpg"},
		},
	}

	err := WriteChapterToCBZ(chapter, outputPath)
	require.Error(t, err)
}

func TestIsAlreadyConverted_CBZWithConvertedTxt(t *testing.T) {
	// Create a CBZ with converted.txt inside (no zip comment)
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	// Add converted.txt with a date
	fw, err := w.Create("converted.txt")
	require.NoError(t, err)
	_, _ = fw.Write([]byte(time.Now().Format(time.RFC3339)))
	// Add a dummy page
	fw, err = w.Create("page.webp")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("dummy"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(context.Background(), cbzPath)
	require.NoError(t, err)
	assert.True(t, result, "Expected CBZ with converted.txt to be detected as converted")
}

func TestIsAlreadyConverted_CBZWithInvalidConvertedTxt(t *testing.T) {
	// Create a CBZ with converted.txt that has invalid date
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	fw, err := w.Create("converted.txt")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("not a valid date"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(context.Background(), cbzPath)
	require.NoError(t, err)
	assert.False(t, result, "Expected CBZ with invalid date in converted.txt to NOT be detected as converted")
}

func TestExtractChapter_WithConvertedTxt(t *testing.T) {
	// Create a CBZ with converted.txt inside
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	fw, err := w.Create("converted.txt")
	require.NoError(t, err)
	convertedTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	_, _ = fw.Write([]byte(convertedTime.Format(time.RFC3339)))
	fw, err = w.Create("page001.jpg")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("fake image"))
	_ = w.Close()
	_ = f.Close()

	chapter, err := ExtractChapter(context.Background(), cbzPath, false)
	require.NoError(t, err)
	defer func() { _ = chapter.Cleanup() }()

	assert.True(t, chapter.IsConverted, "Expected chapter with converted.txt to be marked as converted")
	assert.False(t, chapter.ConvertedTime.IsZero(), "Expected non-zero ConvertedTime")
}

func TestWriteChapterToCBZ_MultiplePages(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.cbz")
	inputDir := filepath.Join(tmpDir, "input")
	_ = os.MkdirAll(inputDir, 0755)

	// Create multiple dummy page files
	var pages []*manga.PageFile
	for i := 0; i < 5; i++ {
		pagePath := filepath.Join(inputDir, fmt.Sprintf("%04d.webp", i))
		_ = os.WriteFile(pagePath, []byte(fmt.Sprintf("page %d content", i)), 0644)
		pages = append(pages, &manga.PageFile{
			Index:     uint16(i),
			Extension: ".webp",
			FilePath:  pagePath,
		})
	}

	chapter := &manga.Chapter{
		FilePath: filepath.Join(tmpDir, "source.cbz"),
		TempDir:  tmpDir,
		Pages:    pages,
	}
	chapter.SetConverted()

	err := WriteChapterToCBZ(chapter, outputPath)
	require.NoError(t, err)

	// Verify archive contents
	r, err := zip.OpenReader(outputPath)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	// Should have 5 pages = 5 files (conversion status stored in zip comment, not as file)
	expectedFiles := 5
	assert.Len(t, r.File, expectedFiles)

	// Verify zip comment is set
	assert.NotEmpty(t, r.Comment, "Expected zip comment for converted chapter")
}

func TestWriteChapterToCBZ_InvalidOutputPath(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	_ = os.MkdirAll(inputDir, 0755)

	pagePath := filepath.Join(inputDir, "0000.webp")
	_ = os.WriteFile(pagePath, []byte("content"), 0644)

	chapter := &manga.Chapter{
		FilePath: filepath.Join(tmpDir, "source.cbz"),
		TempDir:  tmpDir,
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".webp", FilePath: pagePath},
		},
	}

	err := WriteChapterToCBZ(chapter, "/nonexistent/dir/output.cbz")
	require.Error(t, err)
}
