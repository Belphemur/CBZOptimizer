package converter

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
)

func TestConvertChapter(t *testing.T) {
	testCases := []struct {
		name               string
		genTestChapter     func(t *testing.T, dir string) (*manga.Chapter, []string)
		split              bool
		expectError        bool
	}{
		{
			name:           "All split pages",
			genTestChapter: genHugePage,
			split:          true,
		},
		{
			name:           "Big Pages, no split",
			genTestChapter: genHugePageNoSplit,
			split:          false,
			expectError:    true,
		},
		{
			name:           "No split pages",
			genTestChapter: genSmallPages,
			split:          false,
		},
		{
			name:           "Mix of split and no split pages",
			genTestChapter: genMixSmallBig,
			split:          true,
		},
		{
			name:           "Mix of Huge and small page",
			genTestChapter: genMixSmallHuge,
			split:          false,
			expectError:    true,
		},
	}

	for _, converterFormat := range Available() {
		conv, err := Get(converterFormat)
		if err != nil {
			t.Fatalf("failed to get converter: %v", err)
		}
		t.Run(conv.Format().String(), func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					tempDir := t.TempDir()
					chapter, expectedExtensions := tc.genTestChapter(t, tempDir)

					quality := uint8(80)
					progress := func(msg string, current uint32, total uint32) {
						t.Log(msg)
					}

					convertedChapter, err := conv.ConvertChapter(context.Background(), chapter, quality, tc.split, progress)
					if err != nil && !tc.expectError {
						t.Fatalf("failed to convert chapter: %v", err)
					}

					if convertedChapter == nil {
						t.Fatal("convertedChapter is nil")
					}

					if len(convertedChapter.Pages) == 0 {
						t.Fatal("no pages were converted")
					}

					if len(convertedChapter.Pages) != len(expectedExtensions) {
						t.Fatalf("converted chapter has %d pages but expected %d", len(convertedChapter.Pages), len(expectedExtensions))
					}

					for i, page := range convertedChapter.Pages {
						expectedExt := expectedExtensions[i]
						if page.Extension != expectedExt {
							t.Errorf("page %d has extension %s but expected %s", page.Index, page.Extension, expectedExt)
						}
					}
				})
			}
		})
	}
}

// createTestImageFile creates a JPEG image file at the given path with the specified dimensions.
func createTestImageFile(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = jpeg.Encode(f, img, nil)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func genHugePage(t *testing.T, dir string) (*manga.Chapter, []string) {
	inputDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "output"), 0755); err != nil {
		t.Fatal(err)
	}

	pagePath := filepath.Join(inputDir, "0000.jpg")
	createTestImageFile(t, pagePath, 1, 17000)

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".jpg", FilePath: pagePath},
		},
	}

	// With split: 17000/2000 = 9 parts (8*2000 + 1*1000)
	// Without split: page > webpMaxHeight → kept as .jpg with error
	// The caller decides split=true or split=false and we return expectations
	// for the split=true case (this test case is always called with split=true)
	expectedExtensions := []string{".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp"}

	return chapter, expectedExtensions
}

func genHugePageNoSplit(t *testing.T, dir string) (*manga.Chapter, []string) {
	inputDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "output"), 0755); err != nil {
		t.Fatal(err)
	}

	pagePath := filepath.Join(inputDir, "0000.jpg")
	createTestImageFile(t, pagePath, 1, 17000)

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".jpg", FilePath: pagePath},
		},
	}

	// Without split: page > webpMaxHeight → kept as .jpg with error
	return chapter, []string{".jpg"}
}

func genSmallPages(t *testing.T, dir string) (*manga.Chapter, []string) {
	inputDir := filepath.Join(dir, "input")
	_ = os.MkdirAll(inputDir, 0755)
	_ = os.MkdirAll(filepath.Join(dir, "output"), 0755)

	var pages []*manga.PageFile
	for i := 0; i < 5; i++ {
		pagePath := filepath.Join(inputDir, fmt.Sprintf("%04d.jpg", i))
		createTestImageFile(t, pagePath, 300, 1000)
		pages = append(pages, &manga.PageFile{
			Index:     uint16(i),
			Extension: ".jpg",
			FilePath:  pagePath,
		})
	}

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages:    pages,
	}

	return chapter, []string{".webp", ".webp", ".webp", ".webp", ".webp"}
}

func genMixSmallBig(t *testing.T, dir string) (*manga.Chapter, []string) {
	inputDir := filepath.Join(dir, "input")
	_ = os.MkdirAll(inputDir, 0755)
	_ = os.MkdirAll(filepath.Join(dir, "output"), 0755)

	var pages []*manga.PageFile
	for i := 0; i < 5; i++ {
		pagePath := filepath.Join(inputDir, fmt.Sprintf("%04d.jpg", i))
		createTestImageFile(t, pagePath, 300, 1000*(i+1))
		pages = append(pages, &manga.PageFile{
			Index:     uint16(i),
			Extension: ".jpg",
			FilePath:  pagePath,
		})
	}

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages:    pages,
	}

	// Pages heights: 1000, 2000, 3000, 4000, 5000
	// With new disk-first architecture: cwebp handles all these directly
	// (all < 16383 webp max), no splitting needed
	return chapter, []string{".webp", ".webp", ".webp", ".webp", ".webp"}
}

func genMixSmallHuge(t *testing.T, dir string) (*manga.Chapter, []string) {
	inputDir := filepath.Join(dir, "input")
	_ = os.MkdirAll(inputDir, 0755)
	_ = os.MkdirAll(filepath.Join(dir, "output"), 0755)

	var pages []*manga.PageFile
	for i := 0; i < 10; i++ {
		pagePath := filepath.Join(inputDir, fmt.Sprintf("%04d.jpg", i))
		createTestImageFile(t, pagePath, 1, 2000*(i+1))
		pages = append(pages, &manga.PageFile{
			Index:     uint16(i),
			Extension: ".jpg",
			FilePath:  pagePath,
		})
	}

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages:    pages,
	}

	// Heights: 2000, 4000, 6000, 8000, 10000, 12000, 14000, 16000, 18000, 20000
	// Without split, pages > webpMaxHeight (16383) are kept as .jpg
	// Pages 0-7 (2000-16000): should convert to .webp
	// Pages 8-9 (18000, 20000): > 16383, no split → kept as .jpg
	return chapter, []string{".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".jpg", ".jpg"}
}
