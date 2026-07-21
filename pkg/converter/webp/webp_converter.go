package webp

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	converterrors "github.com/belphemur/CBZOptimizer/v2/pkg/converter/errors"
	"github.com/rs/zerolog/log"
	_ "golang.org/x/image/webp"
)

const webpMaxHeight = 16383

// intermediatePageName returns the on-disk filename used for a page's WebP
// intermediate during conversion. When the page carries an OriginalName
// (recorded by --keep-filenames), its stem is reused so the temp file line
// up with the name the archive writer will pick. Otherwise the historical
// %04d indexed naming is kept. The splitSuffix argument is appended verbatim
// after the stem (e.g. "-00", "-01") for split parts and is empty for the
// happy-path single output. A leading dash is only added when both the
// split suffix and the original-name stem are present, so the indexed form
// stays as %04d-%02d.
func intermediatePageName(page *manga.PageFile, splitSuffix string) string {
	if page.OriginalName != "" {
		stem := strings.TrimSuffix(page.OriginalName, filepath.Ext(page.OriginalName))
		return stem + splitSuffix + ".webp"
	}
	if splitSuffix == "" {
		return fmt.Sprintf("%04d.webp", page.Index)
	}
	return fmt.Sprintf("%04d%s.webp", page.Index, splitSuffix)
}

type Converter struct {
	maxHeight  int
	cropHeight int
	isPrepared bool
	// pageWorkerGuard limits concurrent cwebp processes across all chapters.
	pageWorkerGuard chan struct{}
}

func (converter *Converter) Format() constant.ConversionFormat {
	return constant.WebP
}

func New() *Converter {
	return &Converter{
		maxHeight:       4000,
		cropHeight:      2000,
		isPrepared:      false,
		pageWorkerGuard: make(chan struct{}, runtime.NumCPU()),
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

// ConvertChapter converts all pages in a chapter using file-to-file cwebp operations.
// In the happy path, no image data is loaded into Go memory.
// Splitting is attempted only if direct conversion fails due to dimension limits.
func (converter *Converter) ConvertChapter(ctx context.Context, chapter *manga.Chapter, quality uint8, split bool, progress func(message string, current uint32, total uint32)) (*manga.Chapter, error) {
	log.Debug().
		Str("chapter", chapter.FilePath).
		Int("pages", len(chapter.Pages)).
		Uint8("quality", quality).
		Bool("split", split).
		Msg("Starting file-to-file chapter conversion")

	err := converter.PrepareConverter()
	if err != nil {
		return nil, err
	}

	// Validate TempDir is set to prevent writing to cwd
	if chapter.TempDir == "" {
		return nil, fmt.Errorf("chapter TempDir is empty, cannot create output directory")
	}

	// Create output directory for converted files
	outputDir := filepath.Join(chapter.TempDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Check for early context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	guard := converter.pageWorkerGuard
	var totalPages atomic.Uint32
	totalPages.Store(uint32(len(chapter.Pages)))

	type pageResult struct {
		pages []*manga.PageFile
		err   error
	}

	results := make([]pageResult, len(chapter.Pages))
	var wg sync.WaitGroup
	var convertedCount atomic.Uint32

	for i, page := range chapter.Pages {
		wg.Add(1)

		go func(idx int, p *manga.PageFile) {
			defer wg.Done()

			// Check context before acquiring worker slot
			select {
			case <-ctx.Done():
				results[idx] = pageResult{err: ctx.Err()}
				return
			case guard <- struct{}{}:
			}
			defer func() { <-guard }()

			// Check context after acquiring worker slot
			select {
			case <-ctx.Done():
				results[idx] = pageResult{err: ctx.Err()}
				return
			default:
			}

			pages, err := converter.convertPageFile(ctx, p, outputDir, quality, split)
			results[idx] = pageResult{pages: pages, err: err}

			current := convertedCount.Add(1)
			total := totalPages.Load()
			progress(fmt.Sprintf("Converted %d/%d pages to %s format", current, total, converter.Format()), current, total)
		}(i, page)
	}

	// Wait for completion or context cancellation
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		// Wait for in-flight goroutines to finish
		<-done
		return nil, ctx.Err()
	}

	// Collect results — separate fatal errors from page-ignored errors
	var convertedPages []*manga.PageFile
	var fatalErrors []error
	var ignoredErrors []error

	for _, result := range results {
		if result.err != nil {
			if errors.Is(result.err, context.DeadlineExceeded) || errors.Is(result.err, context.Canceled) {
				return nil, result.err
			}
			var pageIgnored *converterrors.PageIgnoredError
			if errors.As(result.err, &pageIgnored) {
				ignoredErrors = append(ignoredErrors, result.err)
			} else {
				fatalErrors = append(fatalErrors, result.err)
			}
		}
		if result.pages != nil {
			convertedPages = append(convertedPages, result.pages...)
		}
	}

	// Fatal errors take priority over ignored-page errors
	if len(fatalErrors) > 0 {
		return nil, errors.Join(fatalErrors...)
	}

	if len(convertedPages) == 0 {
		if len(ignoredErrors) > 0 {
			return nil, errors.Join(ignoredErrors...)
		}
		return nil, fmt.Errorf("no pages were converted")
	}

	// Sort pages by index and split part
	sort.Slice(convertedPages, func(i, j int) bool {
		a, b := convertedPages[i], convertedPages[j]
		if a.Index == b.Index {
			return a.SplitPartIndex < b.SplitPartIndex
		}
		return a.Index < b.Index
	})

	chapter.Pages = convertedPages

	var aggregatedError error
	if len(ignoredErrors) > 0 {
		aggregatedError = errors.Join(ignoredErrors...)
	}

	log.Debug().
		Str("chapter", chapter.FilePath).
		Int("converted_pages", len(convertedPages)).
		Msg("Chapter conversion completed")

	return chapter, aggregatedError
}

// convertPageFile converts a single page file to WebP format.
// Returns the converted page(s) — multiple if splitting was needed.
func (converter *Converter) convertPageFile(ctx context.Context, page *manga.PageFile, outputDir string, quality uint8, split bool) ([]*manga.PageFile, error) {
	log.Debug().
		Uint16("page_index", page.Index).
		Str("input", page.FilePath).
		Msg("Converting page file")

	// If the page is already WebP, just return it as-is. The returned page
	// keeps any OriginalName set during extraction so the archive writer can
	// honor --keep-filenames for the final entry name.
	if strings.ToLower(page.Extension) == ".webp" {
		log.Debug().Uint16("page_index", page.Index).Msg("Page already WebP, skipping")
		return []*manga.PageFile{page}, nil
	}

	// Try direct file-to-file conversion first (happy path — no memory allocation)
	outputPath := filepath.Join(outputDir, intermediatePageName(page, ""))
	err := EncodeFile(page.FilePath, outputPath, uint(quality))

	if err == nil {
		// Success! No image decoding needed. Preserve OriginalName so
		// --keep-filenames carries through to the final zip entry name.
		return []*manga.PageFile{{
			Index:        page.Index,
			Extension:    ".webp",
			FilePath:     outputPath,
			OriginalName: page.OriginalName,
		}}, nil
	}

	// Direct conversion failed. Check if it's a dimension issue.
	log.Debug().
		Uint16("page_index", page.Index).
		Err(err).
		Msg("Direct conversion failed, checking dimensions")

	// Read just the image header to get dimensions (no full decode)
	width, height, decodeErr := getImageDimensions(page.FilePath)
	if decodeErr != nil {
		// Can't even read the image header — keep the original file
		log.Info().
			Uint16("page_index", page.Index).
			Err(decodeErr).
			Msg("Cannot decode image, keeping original")
		return []*manga.PageFile{page}, converterrors.NewPageIgnored(
			fmt.Sprintf("page %d: failed to decode image (%s)", page.Index, decodeErr.Error()))
	}

	log.Debug().
		Uint16("page_index", page.Index).
		Int("width", width).
		Int("height", height).
		Msg("Image dimensions read")

	// If height exceeds WebP max and split is not enabled, keep original
	if height >= webpMaxHeight && !split {
		log.Info().
			Uint16("page_index", page.Index).
			Int("height", height).
			Msg("Page too tall for WebP, keeping original")
		return []*manga.PageFile{page}, converterrors.NewPageIgnored(
			fmt.Sprintf("page %d is too tall [max: %dpx] to be converted to webp format", page.Index, webpMaxHeight))
	}

	// If height exceeds our split threshold and split is enabled, use cwebp -crop
	if height >= converter.maxHeight && split {
		return converter.splitAndConvert(ctx, page, outputDir, quality, width, height)
	}

	// Height is within limits but conversion still failed for another reason.
	// Keep the original file.
	log.Warn().
		Uint16("page_index", page.Index).
		Err(err).
		Msg("Conversion failed for non-dimension reason, keeping original")
	return []*manga.PageFile{page}, converterrors.NewPageIgnored(
		fmt.Sprintf("page %d: conversion failed (%s)", page.Index, err.Error()))
}

// splitAndConvert splits a tall image into multiple parts using cwebp -crop
// and converts each part. No Go-side image decode is needed.
func (converter *Converter) splitAndConvert(ctx context.Context, page *manga.PageFile, outputDir string, quality uint8, width, height int) ([]*manga.PageFile, error) {
	log.Debug().
		Uint16("page_index", page.Index).
		Int("width", width).
		Int("height", height).
		Int("crop_height", converter.cropHeight).
		Msg("Splitting and converting page using cwebp -crop")

	numParts := height / converter.cropHeight
	if height%converter.cropHeight != 0 {
		numParts++
	}

	var pages []*manga.PageFile

	for i := 0; i < numParts; i++ {
		select {
		case <-ctx.Done():
			return pages, ctx.Err()
		default:
		}

		yOffset := i * converter.cropHeight
		partHeight := converter.cropHeight
		if i == numParts-1 {
			partHeight = height - yOffset
		}

		outputPath := filepath.Join(outputDir, intermediatePageName(page, fmt.Sprintf("-%02d", i)))
		err := EncodeFileWithCrop(page.FilePath, outputPath, uint(quality), 0, yOffset, width, partHeight)

		if err != nil {
			log.Error().
				Uint16("page_index", page.Index).
				Int("part", i).
				Err(err).
				Msg("Failed to convert split part")
			return nil, fmt.Errorf("failed to convert split part %d of page %d: %w", i, page.Index, err)
		}

		pages = append(pages, &manga.PageFile{
			Index:          page.Index,
			Extension:      ".webp",
			FilePath:       outputPath,
			IsSplitted:     true,
			SplitPartIndex: uint16(i),
			OriginalName:   page.OriginalName,
		})
	}

	log.Debug().
		Uint16("page_index", page.Index).
		Int("parts", len(pages)).
		Msg("Split conversion completed")

	return pages, nil
}

// getImageDimensions reads only the image header to determine dimensions.
// This is much cheaper than a full image.Decode — only a few bytes are read.
func getImageDimensions(filePath string) (width, height int, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()

	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}

	return config.Width, config.Height, nil
}
