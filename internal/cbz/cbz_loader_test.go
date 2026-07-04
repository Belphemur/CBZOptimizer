package cbz

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
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
			if err != nil {
				t.Fatalf("Failed to load chapter: %v", err)
			}
			defer func() { _ = chapter.Cleanup() }()

			actualPages := len(chapter.Pages)
			if actualPages != tc.expectedPages {
				t.Errorf("Expected %d pages, but got %d", tc.expectedPages, actualPages)
			}

			if !strings.Contains(chapter.ComicInfoXml, tc.expectedSeries) {
				t.Errorf("ComicInfoXml does not contain the expected series: %s", tc.expectedSeries)
			}

			if chapter.IsConverted != tc.expectedConversion {
				t.Errorf("Expected chapter to be converted: %t, but got %t", tc.expectedConversion, chapter.IsConverted)
			}

			// Verify pages are on disk
			for _, page := range chapter.Pages {
				if page.FilePath == "" {
					t.Errorf("Page %d has no file path", page.Index)
				}
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
			result, err := IsAlreadyConverted(tc.filePath)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("Expected %t, got %t", tc.expected, result)
			}
		})
	}
}

func TestIsAlreadyConverted_NonexistentFile(t *testing.T) {
	_, err := IsAlreadyConverted("/nonexistent/file.cbz")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestIsAlreadyConverted_InvalidExtension(t *testing.T) {
	// Create a temp file with unsupported extension
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	result, err := IsAlreadyConverted(tmpFile.Name())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result {
		t.Error("Expected false for unsupported extension")
	}
}

func TestIsAlreadyConverted_CBZWithConvertedComment(t *testing.T) {
	// Create a CBZ file with a zip comment containing a date (marks as converted)
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	_ = w.SetComment(time.Now().Format(time.RFC3339))
	// Add a dummy file
	fw, err := w.Create("page.webp")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("dummy"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(cbzPath)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !result {
		t.Error("Expected CBZ with date comment to be detected as converted")
	}
}

func TestIsAlreadyConverted_CBZWithNonDateComment(t *testing.T) {
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	_ = w.SetComment("this is not a date")
	fw, err := w.Create("page.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("dummy"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(cbzPath)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result {
		t.Error("Expected CBZ with non-date comment to NOT be detected as converted")
	}
}

func TestExtractChapter_NonexistentFile(t *testing.T) {
	_, err := ExtractChapter("/nonexistent/file.cbz")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestExtractChapter_PageExtensions(t *testing.T) {
	chapter, err := ExtractChapter("../../testdata/Chapter 128.cbz")
	if err != nil {
		t.Fatalf("Failed to extract chapter: %v", err)
	}
	defer func() { _ = chapter.Cleanup() }()

	// All pages should have valid image extensions
	validExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	for _, page := range chapter.Pages {
		if !validExts[page.Extension] {
			t.Errorf("Page %d has unexpected extension: %s", page.Index, page.Extension)
		}
	}
}

func TestExtractChapter_PagesHaveSequentialIndices(t *testing.T) {
	chapter, err := ExtractChapter("../../testdata/Chapter 128.cbz")
	if err != nil {
		t.Fatalf("Failed to extract chapter: %v", err)
	}
	defer func() { _ = chapter.Cleanup() }()

	for i, page := range chapter.Pages {
		if page.Index != uint16(i) {
			t.Errorf("Expected page index %d, got %d", i, page.Index)
		}
	}
}

func TestExtractChapter_Cleanup(t *testing.T) {
	chapter, err := ExtractChapter("../../testdata/Chapter 128.cbz")
	if err != nil {
		t.Fatalf("Failed to extract chapter: %v", err)
	}

	tempDir := chapter.TempDir
	// Verify temp dir exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Fatal("TempDir does not exist after extraction")
	}

	// Cleanup should remove temp dir
	err = chapter.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("TempDir should not exist after cleanup")
	}
}

func TestExtractChapter_CBR(t *testing.T) {
	chapter, err := ExtractChapter("../../testdata/Chapter 1.cbr")
	if err != nil {
		t.Fatalf("Failed to extract CBR chapter: %v", err)
	}
	defer func() { _ = chapter.Cleanup() }()

	if len(chapter.Pages) != 16 {
		t.Errorf("Expected 16 pages, got %d", len(chapter.Pages))
	}

	// All page files should exist on disk
	for _, page := range chapter.Pages {
		if _, err := os.Stat(page.FilePath); os.IsNotExist(err) {
			t.Errorf("Page file does not exist on disk: %s", page.FilePath)
		}
	}
}

func TestExtractChapter_ConvertedStatus(t *testing.T) {
	chapter, err := ExtractChapter("../../testdata/Chapter 10_converted.cbz")
	if err != nil {
		t.Fatalf("Failed to extract chapter: %v", err)
	}
	defer func() { _ = chapter.Cleanup() }()

	if !chapter.IsConverted {
		t.Error("Expected converted chapter to have IsConverted = true")
	}
	if chapter.ConvertedTime.IsZero() {
		t.Error("Expected non-zero ConvertedTime for converted chapter")
	}
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
	if err != nil {
		t.Fatalf("Should not error on empty chapter: %v", err)
	}

	// Verify the file is a valid zip
	r, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("Failed to open output CBZ: %v", err)
	}
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
	if err != nil {
		t.Fatalf("Failed to write chapter: %v", err)
	}

	// Verify ComicInfo.xml is present
	r, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("Failed to open output CBZ: %v", err)
	}
	defer func() { _ = r.Close() }()

	foundComicInfo := false
	for _, f := range r.File {
		if f.Name == "ComicInfo.xml" {
			foundComicInfo = true
		}
	}
	if !foundComicInfo {
		t.Error("ComicInfo.xml not found in output CBZ")
	}

	// Verify zip comment has converted timestamp
	if r.Comment == "" {
		t.Error("Expected zip comment with conversion timestamp")
	}
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
	if err == nil {
		t.Error("Expected error when page file does not exist")
	}
}

func TestIsAlreadyConverted_CBZWithConvertedTxt(t *testing.T) {
	// Create a CBZ with converted.txt inside (no zip comment)
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// Add converted.txt with a date
	fw, err := w.Create("converted.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(time.Now().Format(time.RFC3339)))
	// Add a dummy page
	fw, err = w.Create("page.webp")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("dummy"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(cbzPath)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !result {
		t.Error("Expected CBZ with converted.txt to be detected as converted")
	}
}

func TestIsAlreadyConverted_CBZWithInvalidConvertedTxt(t *testing.T) {
	// Create a CBZ with converted.txt that has invalid date
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	fw, err := w.Create("converted.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("not a valid date"))
	_ = w.Close()
	_ = f.Close()

	result, err := IsAlreadyConverted(cbzPath)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result {
		t.Error("Expected CBZ with invalid date in converted.txt to NOT be detected as converted")
	}
}

func TestExtractChapter_WithConvertedTxt(t *testing.T) {
	// Create a CBZ with converted.txt inside
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	fw, err := w.Create("converted.txt")
	if err != nil {
		t.Fatal(err)
	}
	convertedTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	_, _ = fw.Write([]byte(convertedTime.Format(time.RFC3339)))
	fw, err = w.Create("page001.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("fake image"))
	_ = w.Close()
	_ = f.Close()

	chapter, err := ExtractChapter(cbzPath)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}
	defer func() { _ = chapter.Cleanup() }()

	if !chapter.IsConverted {
		t.Error("Expected chapter with converted.txt to be marked as converted")
	}
	if chapter.ConvertedTime.IsZero() {
		t.Error("Expected non-zero ConvertedTime")
	}
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
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Verify archive contents
	r, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("Failed to open: %v", err)
	}
	defer func() { _ = r.Close() }()

	// Should have 5 pages = 5 files (conversion status stored in zip comment, not as file)
	expectedFiles := 5
	if len(r.File) != expectedFiles {
		t.Errorf("Expected %d files in archive, got %d", expectedFiles, len(r.File))
	}

	// Verify zip comment is set
	if r.Comment == "" {
		t.Error("Expected zip comment for converted chapter")
	}
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
	if err == nil {
		t.Error("Expected error for invalid output path")
	}
}
