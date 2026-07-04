package cbz

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
)

func TestWriteChapterToCBZ(t *testing.T) {
	currentTime := time.Now()

	// Helper to create a temp file with content and return path
	createTempPage := func(t *testing.T, dir, content, ext string) string {
		t.Helper()
		f, err := os.CreateTemp(dir, "page-*"+ext)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(content)
		if err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
		return f.Name()
	}

	testCases := []struct {
		name            string
		chapter         func(t *testing.T, dir string) *manga.Chapter
		expectedFiles   []string
		expectedComment string
	}{
		{
			name: "Single page, ComicInfo, converted",
			chapter: func(t *testing.T, dir string) *manga.Chapter {
				return &manga.Chapter{
					Pages: []*manga.PageFile{
						{
							Index:     0,
							Extension: ".jpg",
							FilePath:  createTempPage(t, dir, "image data", ".jpg"),
						},
					},
					ComicInfoXml:  "<Series>Boundless Necromancer</Series>",
					IsConverted:   true,
					ConvertedTime: currentTime,
				}
			},
			expectedFiles:   []string{"0000.jpg", "ComicInfo.xml"},
			expectedComment: fmt.Sprintf("%s\nThis chapter has been converted by CBZOptimizer.", currentTime),
		},
		{
			name: "Single page, no ComicInfo",
			chapter: func(t *testing.T, dir string) *manga.Chapter {
				return &manga.Chapter{
					Pages: []*manga.PageFile{
						{
							Index:     0,
							Extension: ".jpg",
							FilePath:  createTempPage(t, dir, "image data", ".jpg"),
						},
					},
				}
			},
			expectedFiles: []string{"0000.jpg"},
		},
		{
			name: "Multiple pages with ComicInfo",
			chapter: func(t *testing.T, dir string) *manga.Chapter {
				return &manga.Chapter{
					Pages: []*manga.PageFile{
						{Index: 0, Extension: ".jpg", FilePath: createTempPage(t, dir, "image data 1", ".jpg")},
						{Index: 1, Extension: ".jpg", FilePath: createTempPage(t, dir, "image data 2", ".jpg")},
					},
					ComicInfoXml: "<Series>Boundless Necromancer</Series>",
				}
			},
			expectedFiles: []string{"0000.jpg", "0001.jpg", "ComicInfo.xml"},
		},
		{
			name: "Split page",
			chapter: func(t *testing.T, dir string) *manga.Chapter {
				return &manga.Chapter{
					Pages: []*manga.PageFile{
						{
							Index:          0,
							Extension:      ".jpg",
							FilePath:       createTempPage(t, dir, "split image data", ".jpg"),
							IsSplitted:     true,
							SplitPartIndex: 1,
						},
					},
				}
			},
			expectedFiles: []string{"0000-01.jpg"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp dir for page files
			pageDir := t.TempDir()
			chapter := tc.chapter(t, pageDir)

			// Create temp file for output CBZ
			tempFile, err := os.CreateTemp("", "*.cbz")
			if err != nil {
				t.Fatalf("Failed to create temporary file: %v", err)
			}
			_ = tempFile.Close()
			defer errs.CaptureGeneric(&err, os.Remove, tempFile.Name(), "failed to remove temporary file")

			// Write chapter
			err = WriteChapterToCBZ(chapter, tempFile.Name())
			if err != nil {
				t.Fatalf("Failed to write chapter to CBZ: %v", err)
			}

			// Verify the archive
			r, err := zip.OpenReader(tempFile.Name())
			if err != nil {
				t.Fatalf("Failed to open CBZ file: %v", err)
			}
			defer func() { _ = r.Close() }()

			var filesInArchive []string
			for _, f := range r.File {
				filesInArchive = append(filesInArchive, f.Name)
			}

			for _, expectedFile := range tc.expectedFiles {
				found := false
				for _, actualFile := range filesInArchive {
					if actualFile == expectedFile {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected file %s not found in archive", expectedFile)
				}
			}

			if tc.expectedComment != "" && r.Comment != tc.expectedComment {
				t.Errorf("Expected comment %s, but found %s", tc.expectedComment, r.Comment)
			}

			if len(filesInArchive) != len(tc.expectedFiles) {
				t.Errorf("Expected %d files, but found %d: %v", len(tc.expectedFiles), len(filesInArchive), filesInArchive)
			}
		})
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	// Create a temp directory with page files
	pageDir := t.TempDir()

	// Create some page files
	for i := 0; i < 3; i++ {
		pagePath := filepath.Join(pageDir, fmt.Sprintf("%04d.jpg", i))
		err := os.WriteFile(pagePath, []byte(fmt.Sprintf("image data %d", i)), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create chapter
	chapter := &manga.Chapter{
		FilePath: "/test/chapter.cbz",
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".jpg", FilePath: filepath.Join(pageDir, "0000.jpg")},
			{Index: 1, Extension: ".jpg", FilePath: filepath.Join(pageDir, "0001.jpg")},
			{Index: 2, Extension: ".jpg", FilePath: filepath.Join(pageDir, "0002.jpg")},
		},
		ComicInfoXml: "<Series>Test Series</Series>",
	}
	chapter.SetConverted()

	// Write to CBZ
	outputPath := filepath.Join(t.TempDir(), "output.cbz")
	err := WriteChapterToCBZ(chapter, outputPath)
	if err != nil {
		t.Fatalf("Failed to write chapter: %v", err)
	}

	// Read it back
	loaded, err := LoadChapter(outputPath)
	if err != nil {
		t.Fatalf("Failed to load chapter: %v", err)
	}
	defer func() { _ = loaded.Cleanup() }()

	if !loaded.IsConverted {
		t.Error("Loaded chapter should be marked as converted")
	}

	if len(loaded.Pages) != 3 {
		t.Errorf("Expected 3 pages, got %d", len(loaded.Pages))
	}

	if loaded.ComicInfoXml != "<Series>Test Series</Series>" {
		t.Errorf("ComicInfoXml mismatch: %s", loaded.ComicInfoXml)
	}
}
