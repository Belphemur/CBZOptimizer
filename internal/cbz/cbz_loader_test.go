package cbz

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
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
