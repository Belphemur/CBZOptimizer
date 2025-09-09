package webp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	converterrors "github.com/belphemur/CBZOptimizer/v2/pkg/converter/errors"
	"github.com/oliamb/cutter"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
	_ "golang.org/x/image/webp"
)

const webpMaxHeight = 16383

type Converter struct {
	maxHeight  int
	cropHeight int
	isPrepared bool
}

func (converter *Converter) Format() (format constant.ConversionFormat) {
	return constant.WebP
}

func New() *Converter {
	return &Converter{
		//maxHeight: 16383 / 2,
		maxHeight:  4000,
		cropHeight: 2000,
		isPrepared: false,
	}
}

func (converter *Converter) PrepareConverter() error {
	if converter.isPrepared {
		return nil
	}
	err := PrepareEncoder()
	if err != nil {
		return err
	}
	converter.isPrepared = true
	return nil
}

func (converter *Converter) ConvertChapter(ctx context.Context, chapter *manga.Chapter, quality uint8, split bool, progress func(message string, current uint32, total uint32)) (*manga.Chapter, error) {
	log.Debug().
		Str("chapter", chapter.FilePath).
		Int("pages", len(chapter.Pages)).
		Uint8("quality", quality).
		Bool("split", split).
		Int("max_goroutines", runtime.NumCPU()).
		Msg("Starting chapter conversion")

	err := converter.PrepareConverter()
	if err != nil {
		log.Error().Str("chapter", chapter.FilePath).Err(err).Msg("Failed to prepare converter")
		return nil, err
	}

	var wgConvertedPages sync.WaitGroup
	maxGoroutines := runtime.NumCPU()

	pagesChan := make(chan *manga.PageContainer, maxGoroutines)
	errChan := make(chan error, maxGoroutines)
	doneChan := make(chan struct{})

	var wgPages sync.WaitGroup
	wgPages.Add(len(chapter.Pages))

	guard := make(chan struct{}, maxGoroutines)
	pagesMutex := sync.Mutex{}
	var pages []*manga.Page
	var totalPages = uint32(len(chapter.Pages))

	log.Debug().
		Str("chapter", chapter.FilePath).
		Int("total_pages", len(chapter.Pages)).
		Int("worker_count", maxGoroutines).
		Msg("Initialized conversion worker pool")

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		log.Warn().Str("chapter", chapter.FilePath).Msg("Chapter conversion cancelled due to timeout")
		return nil, ctx.Err()
	default:
	}

	// Start the worker pool
	go func() {
		defer close(doneChan)
		for page := range pagesChan {
			select {
			case <-ctx.Done():
				return
			case guard <- struct{}{}: // would block if guard channel is already filled
			}

			go func(pageToConvert *manga.PageContainer) {
				defer func() {
					wgConvertedPages.Done()
					<-guard
				}()

				// Check context cancellation before processing
				select {
				case <-ctx.Done():
					return
				default:
				}

				convertedPage, err := converter.convertPage(pageToConvert, quality)
				if err != nil {
					if convertedPage == nil {
						select {
						case errChan <- err:
						case <-ctx.Done():
							return
						}
						return
					}
					buffer := new(bytes.Buffer)
					err := png.Encode(buffer, convertedPage.Image)
					if err != nil {
						select {
						case errChan <- err:
						case <-ctx.Done():
							return
						}
						return
					}
					convertedPage.Page.Contents = buffer
					convertedPage.Page.Extension = ".png"
					convertedPage.Page.Size = uint64(buffer.Len())
				}
				pagesMutex.Lock()
				defer pagesMutex.Unlock()
				pages = append(pages, convertedPage.Page)
				currentTotalPages := atomic.LoadUint32(&totalPages)
				progress(fmt.Sprintf("Converted %d/%d pages to %s format", len(pages), currentTotalPages, converter.Format()), uint32(len(pages)), currentTotalPages)
			}(page)
		}
	}()

	// Process pages
	for _, page := range chapter.Pages {
		select {
		case <-ctx.Done():
			log.Warn().Str("chapter", chapter.FilePath).Msg("Chapter conversion cancelled due to timeout")
			return nil, ctx.Err()
		default:
		}

		go func(page *manga.Page) {
			defer wgPages.Done()

			splitNeeded, img, format, err := converter.checkPageNeedsSplit(page, split)
			if err != nil {
				var pageIgnoredError *converterrors.PageIgnoredError
				if errors.As(err, &pageIgnoredError) {
					log.Info().Err(err).Msg("Page ignored due to image decode error")
				}

				select {
				case errChan <- err:
				case <-ctx.Done():
					return
				}

				wgConvertedPages.Add(1)
				select {
				case pagesChan <- manga.NewContainer(page, img, format, false):
				case <-ctx.Done():
					return
				}

				return
			}

			if !splitNeeded {
				wgConvertedPages.Add(1)
				select {
				case pagesChan <- manga.NewContainer(page, img, format, true):
				case <-ctx.Done():
					return
				}
				return
			}

			images, err := converter.cropImage(img)
			if err != nil {
				select {
				case errChan <- err:
				case <-ctx.Done():
					return
				}
				return
			}

			atomic.AddUint32(&totalPages, uint32(len(images)-1))
			for i, img := range images {
				select {
				case <-ctx.Done():
					return
				default:
				}

				newPage := &manga.Page{
					Index:          page.Index,
					IsSplitted:     true,
					SplitPartIndex: uint16(i),
				}
				wgConvertedPages.Add(1)
				select {
				case pagesChan <- manga.NewContainer(newPage, img, "N/A", true):
				case <-ctx.Done():
					return
				}
			}
		}(page)
	}

	wgPages.Wait()
	close(pagesChan)

	// Wait for all conversions to complete or context cancellation
	done := make(chan struct{})
	go func() {
		defer close(done)
		wgConvertedPages.Wait()
	}()

	select {
	case <-done:
		// Conversion completed successfully
	case <-ctx.Done():
		log.Warn().Str("chapter", chapter.FilePath).Msg("Chapter conversion cancelled due to timeout")
		return nil, ctx.Err()
	}

	close(errChan)
	close(guard)

	var errList []error
	for err := range errChan {
		errList = append(errList, err)
	}

	var aggregatedError error = nil
	if len(errList) > 0 {
		aggregatedError = errors.Join(errList...)
		log.Debug().
			Str("chapter", chapter.FilePath).
			Int("error_count", len(errList)).
			Err(errors.Join(errList...)).
			Msg("Conversion completed with errors")
	} else {
		log.Debug().
			Str("chapter", chapter.FilePath).
			Int("pages_converted", len(pages)).
			Msg("Conversion completed successfully")
	}

	slices.SortFunc(pages, func(a, b *manga.Page) int {
		if a.Index == b.Index {
			return int(a.SplitPartIndex) - int(b.SplitPartIndex)
		}
		return int(a.Index) - int(b.Index)
	})
	chapter.Pages = pages

	log.Debug().
		Str("chapter", chapter.FilePath).
		Int("final_page_count", len(pages)).
		Msg("Pages sorted and chapter updated")

	runtime.GC()
	log.Debug().Str("chapter", chapter.FilePath).Msg("Garbage collection completed")

	return chapter, aggregatedError
}

func (converter *Converter) cropImage(img image.Image) ([]image.Image, error) {
	bounds := img.Bounds()
	height := bounds.Dy()
	width := bounds.Dx()

	numParts := height / converter.cropHeight
	if height%converter.cropHeight != 0 {
		numParts++
	}

	log.Debug().
		Int("original_width", width).
		Int("original_height", height).
		Int("crop_height", converter.cropHeight).
		Int("num_parts", numParts).
		Msg("Starting image cropping for page splitting")

	parts := make([]image.Image, numParts)

	for i := 0; i < numParts; i++ {
		partHeight := converter.cropHeight
		if i == numParts-1 {
			partHeight = height - i*converter.cropHeight
		}

		log.Debug().
			Int("part_index", i).
			Int("part_height", partHeight).
			Int("y_offset", i*converter.cropHeight).
			Msg("Cropping image part")

		part, err := cutter.Crop(img, cutter.Config{
			Width:  bounds.Dx(),
			Height: partHeight,
			Anchor: image.Point{Y: i * converter.cropHeight},
			Mode:   cutter.TopLeft,
		})
		if err != nil {
			log.Error().
				Int("part_index", i).
				Err(err).
				Msg("Failed to crop image part")
			return nil, fmt.Errorf("error cropping part %d: %v", i+1, err)
		}

		parts[i] = part

		log.Debug().
			Int("part_index", i).
			Int("cropped_width", part.Bounds().Dx()).
			Int("cropped_height", part.Bounds().Dy()).
			Msg("Image part cropped successfully")
	}

	log.Debug().
		Int("total_parts", len(parts)).
		Msg("Image cropping completed")

	return parts, nil
}

func (converter *Converter) checkPageNeedsSplit(page *manga.Page, splitRequested bool) (bool, image.Image, string, error) {
	log.Debug().
		Uint16("page_index", page.Index).
		Bool("split_requested", splitRequested).
		Int("page_size", len(page.Contents.Bytes())).
		Msg("Analyzing page for splitting")

	reader := bytes.NewBuffer(page.Contents.Bytes())
	img, format, err := image.Decode(reader)
	if err != nil {
		log.Debug().Uint16("page_index", page.Index).Err(err).Msg("Failed to decode page image")
		return false, nil, format, converterrors.NewPageIgnored(fmt.Sprintf("page %d: failed to decode image (%s)", page.Index, err.Error()))
	}

	bounds := img.Bounds()
	height := bounds.Dy()
	width := bounds.Dx()

	log.Debug().
		Uint16("page_index", page.Index).
		Int("width", width).
		Int("height", height).
		Str("format", format).
		Int("max_height", converter.maxHeight).
		Int("webp_max_height", webpMaxHeight).
		Msg("Page dimensions analyzed")

	if height >= webpMaxHeight && !splitRequested {
		log.Debug().
			Uint16("page_index", page.Index).
			Int("height", height).
			Int("webp_max", webpMaxHeight).
			Msg("Page too tall for WebP format, would be ignored")
		return false, img, format, converterrors.NewPageIgnored(fmt.Sprintf("page %d is too tall [max: %dpx] to be converted to webp format", page.Index, webpMaxHeight))
	}

	needsSplit := height >= converter.maxHeight && splitRequested
	log.Debug().
		Uint16("page_index", page.Index).
		Bool("needs_split", needsSplit).
		Msg("Page splitting decision made")

	return needsSplit, img, format, nil
}

func (converter *Converter) convertPage(container *manga.PageContainer, quality uint8) (*manga.PageContainer, error) {
	log.Debug().
		Uint16("page_index", container.Page.Index).
		Str("format", container.Format).
		Bool("to_be_converted", container.IsToBeConverted).
		Uint8("quality", quality).
		Msg("Converting page")

	// Fix WebP format detection (case insensitive)
	if container.Format == "webp" || container.Format == "WEBP" {
		log.Debug().
			Uint16("page_index", container.Page.Index).
			Msg("Page already in WebP format, skipping conversion")
		container.Page.Extension = ".webp"
		return container, nil
	}
	if !container.IsToBeConverted {
		log.Debug().
			Uint16("page_index", container.Page.Index).
			Msg("Page marked as not to be converted, skipping")
		return container, nil
	}

	log.Debug().
		Uint16("page_index", container.Page.Index).
		Uint8("quality", quality).
		Msg("Encoding page to WebP format")

	converted, err := converter.convert(container.Image, uint(quality))
	if err != nil {
		log.Error().
			Uint16("page_index", container.Page.Index).
			Err(err).
			Msg("Failed to convert page to WebP")
		return nil, err
	}

	container.SetConverted(converted, ".webp")

	log.Debug().
		Uint16("page_index", container.Page.Index).
		Int("original_size", len(container.Page.Contents.Bytes())).
		Int("converted_size", len(converted.Bytes())).
		Msg("Page conversion completed")

	return container, nil
}

// convert converts an image to the WebP format. It decodes the image from the input buffer,
// encodes it as a WebP file using the webp.Encode() function, and returns the resulting WebP
// file as a bytes.Buffer.
func (converter *Converter) convert(image image.Image, quality uint) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	err := Encode(&buf, image, quality)
	if err != nil {
		return nil, err
	}

	return &buf, nil
}
