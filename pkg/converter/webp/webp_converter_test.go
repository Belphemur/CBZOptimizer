package webp

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestImageFile(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	f, err := os.Create(path)
	require.NoError(t, err)
	err = jpeg.Encode(f, img, nil)
	require.NoError(t, err)
	_ = f.Close()
}

func createTestChapter(t *testing.T, pages []struct{ w, h int }) (*manga.Chapter, string) {
	t.Helper()
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "output"), 0755))

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
	}

	for i, p := range pages {
		pagePath := filepath.Join(inputDir, fmt.Sprintf("%04d.jpg", i))
		createTestImageFile(t, pagePath, p.w, p.h)
		chapter.Pages = append(chapter.Pages, &manga.PageFile{
			Index:     uint16(i),
			Extension: ".jpg",
			FilePath:  pagePath,
		})
	}

	return chapter, dir
}

func TestConverter_ConvertChapter(t *testing.T) {
	tests := []struct {
		name        string
		pages       []struct{ w, h int }
		split       bool
		expectSplit bool
		expectError bool
		numExpected int
	}{
		{
			name:        "Single normal image",
			pages:       []struct{ w, h int }{{800, 1200}},
			split:       false,
			numExpected: 1,
		},
		{
			name: "Multiple normal images",
			pages: []struct{ w, h int }{
				{800, 1200},
				{800, 1200},
				{800, 1200},
			},
			split:       false,
			numExpected: 3,
		},
		{
			name:        "Tall image with split enabled",
			pages:       []struct{ w, h int }{{800, 5000}},
			split:       true,
			expectSplit: false, // cwebp handles 5000px fine (< 16383 webp max), no split needed
			numExpected: 1,
		},
		{
			name:        "Tall image without split",
			pages:       []struct{ w, h int }{{800, webpMaxHeight + 100}},
			split:       false,
			expectError: true,
			numExpected: 1, // kept as-is
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := New()
			err := converter.PrepareConverter()
			require.NoError(t, err)

			chapter, _ := createTestChapter(t, tt.pages)

			var progressMutex sync.Mutex
			var lastProgress uint32
			progress := func(message string, current uint32, total uint32) {
				progressMutex.Lock()
				defer progressMutex.Unlock()
				assert.GreaterOrEqual(t, current, lastProgress)
				lastProgress = current
				assert.LessOrEqual(t, current, total)
			}

			convertedChapter, err := converter.ConvertChapter(context.Background(), chapter, 80, tt.split, progress)

			if tt.expectError {
				assert.Error(t, err)
				if convertedChapter != nil {
					assert.LessOrEqual(t, len(convertedChapter.Pages), tt.numExpected)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, convertedChapter)
			assert.Len(t, convertedChapter.Pages, tt.numExpected)

			// Verify page order
			for i := 1; i < len(convertedChapter.Pages); i++ {
				prev := convertedChapter.Pages[i-1]
				curr := convertedChapter.Pages[i]
				if prev.Index == curr.Index {
					assert.Less(t, prev.SplitPartIndex, curr.SplitPartIndex)
				} else {
					assert.Less(t, prev.Index, curr.Index)
				}
			}

			if tt.expectSplit {
				splitFound := false
				for _, page := range convertedChapter.Pages {
					if page.IsSplitted {
						splitFound = true
						break
					}
				}
				assert.True(t, splitFound, "Expected split pages")
			}

			// Verify all output files exist
			for _, page := range convertedChapter.Pages {
				assert.FileExists(t, page.FilePath)
			}
		})
	}
}

func TestConverter_Format(t *testing.T) {
	converter := New()
	assert.Equal(t, constant.WebP, converter.Format())
}

func TestConverter_ConvertChapter_Timeout(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	chapter, _ := createTestChapter(t, []struct{ w, h int }{
		{800, 1200},
		{800, 1200},
		{800, 1200},
	})

	progress := func(message string, current uint32, total uint32) {}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	convertedChapter, err := converter.ConvertChapter(ctx, chapter, 80, false, progress)

	assert.Error(t, err)
	assert.Nil(t, convertedChapter)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestConverter_ConvertChapter_ManyPages_NoDeadlock(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	pages := make([]struct{ w, h int }, 50)
	for i := range pages {
		pages[i] = struct{ w, h int }{100, 100}
	}

	chapter, _ := createTestChapter(t, pages)

	progress := func(message string, current uint32, total uint32) {}

	for iteration := 0; iteration < 10; iteration++ {
		t.Run(fmt.Sprintf("iteration_%d", iteration), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
			defer cancel()

			done := make(chan struct{})
			var convertErr error
			go func() {
				defer close(done)
				_, convertErr = converter.ConvertChapter(ctx, chapter, 80, false, progress)
			}()

			select {
			case <-done:
				assert.Error(t, convertErr)
			case <-time.After(5 * time.Second):
				t.Fatal("Deadlock detected")
			}
		})
	}
}

func TestConverter_ConvertChapter_ConcurrentChapters_NoDeadlock(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	numChapters := 20
	chapters := make([]*manga.Chapter, numChapters)

	pages := make([]struct{ w, h int }, 30)
	for i := range pages {
		pages[i] = struct{ w, h int }{100, 100}
	}

	for c := 0; c < numChapters; c++ {
		chapters[c], _ = createTestChapter(t, pages)
	}

	progress := func(message string, current uint32, total uint32) {}

	parallelism := 4
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, parallelism)

	testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer testCancel()

	for _, chapter := range chapters {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(ch *manga.Chapter) {
			defer wg.Done()
			defer func() { <-semaphore }()

			ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
			defer cancel()

			_, _ = converter.ConvertChapter(ctx, ch, 80, false, progress)
		}(chapter)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-testCtx.Done():
		t.Fatal("Deadlock detected")
	}
}

func TestConverter_SplitAndConvert(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	// Create a very tall image that exceeds webpMaxHeight
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "output"), 0755))

	// Create image taller than webpMaxHeight (16383)
	tallImagePath := filepath.Join(inputDir, "0000.jpg")
	createTestImageFile(t, tallImagePath, 800, webpMaxHeight+500)

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".jpg", FilePath: tallImagePath},
		},
	}

	progress := func(message string, current uint32, total uint32) {}

	// With split=true, the oversized page should be split
	convertedChapter, err := converter.ConvertChapter(context.Background(), chapter, 80, true, progress)
	require.NoError(t, err)
	require.NotNil(t, convertedChapter)

	// Should have more pages due to splitting
	assert.Greater(t, len(convertedChapter.Pages), 1, "Tall image should be split into multiple parts")

	// All parts should be split and have webp extension
	for _, page := range convertedChapter.Pages {
		assert.True(t, page.IsSplitted, "All pages should be marked as split")
		assert.Equal(t, ".webp", page.Extension)
		assert.FileExists(t, page.FilePath)
		assert.Equal(t, uint16(0), page.Index, "All split parts should have same original index")
	}

	// Verify sequential split part indices
	for i, page := range convertedChapter.Pages {
		assert.Equal(t, uint16(i), page.SplitPartIndex)
	}
}

func TestConverter_OversizedImageNoSplit(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	require.NoError(t, os.MkdirAll(inputDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "output"), 0755))

	tallImagePath := filepath.Join(inputDir, "0000.jpg")
	createTestImageFile(t, tallImagePath, 800, webpMaxHeight+500)

	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages: []*manga.PageFile{
			{Index: 0, Extension: ".jpg", FilePath: tallImagePath},
		},
	}

	progress := func(message string, current uint32, total uint32) {}

	// With split=false, the oversized page should be kept as-is with error
	convertedChapter, err := converter.ConvertChapter(context.Background(), chapter, 80, false, progress)
	assert.Error(t, err, "Should return error for oversized page without split")
	if convertedChapter != nil {
		// The page should be kept in original format
		for _, page := range convertedChapter.Pages {
			assert.Equal(t, ".jpg", page.Extension, "Page should keep original extension")
		}
	}
}

func TestGetImageDimensions(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name           string
		width, height  int
		expectW, expectH int
	}{
		{"small image", 100, 200, 100, 200},
		{"wide image", 1920, 1080, 1920, 1080},
		{"tall image", 800, 5000, 800, 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".jpg")
			createTestImageFile(t, path, tt.width, tt.height)

			w, h, err := getImageDimensions(path)
			require.NoError(t, err)
			assert.Equal(t, tt.expectW, w)
			assert.Equal(t, tt.expectH, h)
		})
	}
}

func TestGetImageDimensions_NonexistentFile(t *testing.T) {
	_, _, err := getImageDimensions("/nonexistent/file.jpg")
	assert.Error(t, err)
}

func TestGetImageDimensions_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.jpg")
	require.NoError(t, os.WriteFile(path, []byte("not an image"), 0644))

	_, _, err := getImageDimensions(path)
	assert.Error(t, err)
}

func TestConverter_ConvertChapter_EmptyChapter(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	dir := t.TempDir()
	chapter := &manga.Chapter{
		FilePath: filepath.Join(dir, "test.cbz"),
		TempDir:  dir,
		Pages:    []*manga.PageFile{},
	}

	progress := func(message string, current uint32, total uint32) {}

	_, err = converter.ConvertChapter(context.Background(), chapter, 80, false, progress)
	assert.Error(t, err, "Should error on empty chapter")
}

func TestConverter_ConvertChapter_PreservesComicInfo(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	chapter, _ := createTestChapter(t, []struct{ w, h int }{{400, 600}})
	chapter.ComicInfoXml = `<?xml version="1.0"?><ComicInfo><Series>Test</Series></ComicInfo>`

	progress := func(message string, current uint32, total uint32) {}

	convertedChapter, err := converter.ConvertChapter(context.Background(), chapter, 80, false, progress)
	require.NoError(t, err)
	require.NotNil(t, convertedChapter)
	assert.Equal(t, chapter.ComicInfoXml, convertedChapter.ComicInfoXml)
}

func TestConverter_ConvertChapter_OutputInCorrectDir(t *testing.T) {
	converter := New()
	err := converter.PrepareConverter()
	require.NoError(t, err)

	chapter, dir := createTestChapter(t, []struct{ w, h int }{{400, 600}})

	progress := func(message string, current uint32, total uint32) {}

	convertedChapter, err := converter.ConvertChapter(context.Background(), chapter, 80, false, progress)
	require.NoError(t, err)
	require.NotNil(t, convertedChapter)

	// All output files should be in the output directory
	expectedOutputDir := filepath.Join(dir, "output")
	for _, page := range convertedChapter.Pages {
		pageDir := filepath.Dir(page.FilePath)
		assert.Equal(t, expectedOutputDir, pageDir, "Output file should be in output directory")
	}
}

func TestEncodeFile(t *testing.T) {
	err := PrepareEncoder()
	require.NoError(t, err)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.jpg")
	outputPath := filepath.Join(dir, "output.webp")

	createTestImageFile(t, inputPath, 200, 300)

	err = EncodeFile(inputPath, outputPath, 80)
	require.NoError(t, err)

	// Verify output exists and is non-empty
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestEncodeFileWithCrop(t *testing.T) {
	err := PrepareEncoder()
	require.NoError(t, err)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.jpg")
	outputPath := filepath.Join(dir, "output.webp")

	createTestImageFile(t, inputPath, 200, 600)

	err = EncodeFileWithCrop(inputPath, outputPath, 80, 0, 0, 200, 300)
	require.NoError(t, err)

	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}
