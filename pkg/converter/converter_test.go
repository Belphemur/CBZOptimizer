package converter

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"testing"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
)

func TestConvertChapter(t *testing.T) {

	testCases := []struct {
		name           string
		genTestChapter func(path string, isSplit bool) (*manga.Chapter, []string, error)
		split          bool
		expectError    bool
	}{
		{
			name:           "All split pages",
			genTestChapter: genHugePage,
			split:          true,
		},
		{
			name:           "Big Pages, no split",
			genTestChapter: genHugePage,
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
		{
			name:           "Two corrupted pages",
			genTestChapter: genTwoCorrupted,
			split:          false,
			expectError:    true,
		},
	}
	// Load test genTestChapter from testdata
	temp, err := os.CreateTemp("", "test_chapter_*.cbz")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)

	}
	defer errs.CaptureGeneric(&err, os.Remove, temp.Name(), "failed to remove temporary file")
	for _, converter := range Available() {
		converter, err := Get(converter)
		if err != nil {
			t.Fatalf("failed to get converter: %v", err)
		}
		t.Run(converter.Format().String(), func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					chapter, expectedExtensions, err := tc.genTestChapter(temp.Name(), tc.split)
					if err != nil {
						t.Fatalf("failed to load test genTestChapter: %v", err)
					}

					quality := uint8(80)

					progress := func(msg string, current uint32, total uint32) {
						t.Log(msg)
					}

					convertedChapter, err := converter.ConvertChapter(context.Background(), chapter, quality, tc.split, progress)
					if err != nil && !tc.expectError {
						t.Fatalf("failed to convert genTestChapter: %v", err)
					}

					if len(convertedChapter.Pages) == 0 {
						t.Fatalf("no pages were converted")
					}

					if len(convertedChapter.Pages) != len(expectedExtensions) {
						t.Fatalf("converted chapter has %d pages but expected %d", len(convertedChapter.Pages), len(expectedExtensions))
					}

					// Check each page's extension against the expected array
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

func genHugePage(path string, isSplit bool) (*manga.Chapter, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer errs.Capture(&err, file.Close, "failed to close file")

	var pages []*manga.Page
	expectedExtensions := []string{".jpg"} // One image that's generated as JPEG
	if isSplit {
		expectedExtensions = []string{".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp"}
	}

	// Create one tall page
	img := image.NewRGBA(image.Rect(0, 0, 1, 17000))
	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, img, nil)
	if err != nil {
		return nil, nil, err
	}
	page := &manga.Page{
		Index:     0,
		Contents:  buf,
		Extension: ".jpg",
	}
	pages = append(pages, page)

	return &manga.Chapter{
		FilePath: path,
		Pages:    pages,
	}, expectedExtensions, nil
}

func genSmallPages(path string, isSplit bool) (*manga.Chapter, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer errs.Capture(&err, file.Close, "failed to close file")

	var pages []*manga.Page
	for i := 0; i < 5; i++ { // Assuming there are 5 pages for the test
		img := image.NewRGBA(image.Rect(0, 0, 300, 1000))
		buf := new(bytes.Buffer)
		err = jpeg.Encode(buf, img, nil)
		if err != nil {
			return nil, nil, err
		}
		page := &manga.Page{
			Index:     uint16(i),
			Contents:  buf,
			Extension: ".jpg",
		}
		pages = append(pages, page)
	}

	return &manga.Chapter{
		FilePath: path,
		Pages:    pages,
	}, []string{".webp", ".webp", ".webp", ".webp", ".webp"}, nil
}

func genMixSmallBig(path string, isSplit bool) (*manga.Chapter, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer errs.Capture(&err, file.Close, "failed to close file")

	var pages []*manga.Page
	for i := 0; i < 5; i++ { // Assuming there are 5 pages for the test
		img := image.NewRGBA(image.Rect(0, 0, 300, 1000*(i+1)))
		buf := new(bytes.Buffer)
		err := jpeg.Encode(buf, img, nil)
		if err != nil {
			return nil, nil, err
		}
		page := &manga.Page{
			Index:     uint16(i),
			Contents:  buf,
			Extension: ".jpg",
		}
		pages = append(pages, page)
	}
	expectedExtensions := []string{".webp", ".webp", ".webp", ".webp", ".webp"}
	if isSplit {
		expectedExtensions = []string{".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp"}
	}

	return &manga.Chapter{
		FilePath: path,
		Pages:    pages,
	}, expectedExtensions, nil
}

func genMixSmallHuge(path string, isSplit bool) (*manga.Chapter, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer errs.Capture(&err, file.Close, "failed to close file")

	var pages []*manga.Page
	for i := 0; i < 10; i++ { // Assuming there are 5 pages for the test
		img := image.NewRGBA(image.Rect(0, 0, 1, 2000*(i+1)))
		buf := new(bytes.Buffer)
		err := jpeg.Encode(buf, img, nil)
		if err != nil {
			return nil, nil, err
		}
		page := &manga.Page{
			Index:     uint16(i),
			Contents:  buf,
			Extension: ".jpg",
		}
		pages = append(pages, page)
	}

	return &manga.Chapter{
		FilePath: path,
		Pages:    pages,
	}, []string{".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".webp", ".jpg", ".jpg"}, nil
}

func genTwoCorrupted(path string, isSplit bool) (*manga.Chapter, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer errs.Capture(&err, file.Close, "failed to close file")

	var pages []*manga.Page
	numPages := 5
	corruptedIndices := []int{2, 4} // Pages 2 and 4 are too tall to convert without splitting
	for i := 0; i < numPages; i++ {
		var buf *bytes.Buffer
		var ext string
		isCorrupted := false
		for _, ci := range corruptedIndices {
			if i == ci {
				isCorrupted = true
				break
			}
		}
		if isCorrupted {
			buf = bytes.NewBufferString("corrupted data") // Invalid data, can't decode as image
			ext = ".jpg"
		} else {
			img := image.NewRGBA(image.Rect(0, 0, 300, 1000))
			buf = new(bytes.Buffer)
			err = jpeg.Encode(buf, img, nil)
			if err != nil {
				return nil, nil, err
			}
			ext = ".jpg"
		}
		page := &manga.Page{
			Index:     uint16(i),
			Contents:  buf,
			Extension: ext,
		}
		pages = append(pages, page)
	}

	// Expected: small pages to .webp, corrupted pages to .jpg (kept as is)
	expectedExtensions := []string{".webp", ".webp", ".jpg", ".webp", ".jpg"}
	// Even with split, corrupted pages can't be decoded so stay as is

	return &manga.Chapter{
		FilePath: path,
		Pages:    pages,
	}, expectedExtensions, nil
}
