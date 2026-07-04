package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/cbz"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter"
	errors2 "github.com/belphemur/CBZOptimizer/v2/pkg/converter/errors"
	"github.com/rs/zerolog/log"
)

type OptimizeOptions struct {
	ChapterConverter converter.Converter
	Path             string
	Quality          uint8
	Override         bool
	Split            bool
	Timeout          time.Duration
}

// Optimize optimizes a CBZ/CBR file using the specified converter.
// The new pipeline is disk-first:
// 1. Fast check if already converted (no extraction)
// 2. Extract archive to temp directory on disk
// 3. Convert pages file-to-file (no image data in memory)
// 4. Create output CBZ by streaming from disk
// 5. Cleanup temp files
func Optimize(options *OptimizeOptions) error {
	log.Info().Str("file", options.Path).Msg("Processing file")
	log.Debug().
		Str("file", options.Path).
		Uint8("quality", options.Quality).
		Bool("override", options.Override).
		Bool("split", options.Split).
		Msg("Optimization parameters")

	// Step 1: Fast conversion check before extracting (new requirement)
	alreadyConverted, err := cbz.IsAlreadyConverted(options.Path)
	if err != nil {
		log.Debug().Str("file", options.Path).Err(err).Msg("Conversion check failed, proceeding with extraction")
	}
	if alreadyConverted {
		log.Info().Str("file", options.Path).Msg("Chapter already converted")
		return nil
	}

	// Step 2: Extract chapter to disk
	log.Debug().Str("file", options.Path).Msg("Extracting chapter")
	chapter, err := cbz.ExtractChapter(options.Path)
	if err != nil {
		log.Error().Str("file", options.Path).Err(err).Msg("Failed to extract chapter")
		return fmt.Errorf("failed to load chapter: %v", err)
	}
	defer func() {
		if cleanupErr := chapter.Cleanup(); cleanupErr != nil {
			log.Warn().Str("file", options.Path).Err(cleanupErr).Msg("Failed to cleanup temp directory")
		}
	}()

	// Double-check conversion status from extracted metadata
	if chapter.IsConverted {
		log.Info().Str("file", options.Path).Msg("Chapter already converted")
		return nil
	}

	log.Debug().
		Str("file", options.Path).
		Int("pages", len(chapter.Pages)).
		Msg("Chapter extracted successfully")

	// Step 3: Convert pages file-to-file
	var ctx context.Context
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), options.Timeout)
		defer cancel()
		log.Debug().Str("file", options.Path).Dur("timeout", options.Timeout).Msg("Applying timeout")
	} else {
		ctx = context.Background()
	}

	convertedChapter, err := options.ChapterConverter.ConvertChapter(ctx, chapter, options.Quality, options.Split, func(msg string, current uint32, total uint32) {
		if current%10 == 0 || current == total {
			log.Info().Str("file", chapter.FilePath).Uint32("current", current).Uint32("total", total).Msg("Converting")
		} else {
			log.Debug().Str("file", chapter.FilePath).Uint32("current", current).Uint32("total", total).Msg("Converting page")
		}
	})
	if err != nil {
		var pageIgnoredError *errors2.PageIgnoredError
		if errors.As(err, &pageIgnoredError) {
			log.Debug().Str("file", chapter.FilePath).Err(err).Msg("Page conversion error (non-fatal)")
		} else {
			log.Error().Str("file", chapter.FilePath).Err(err).Msg("Chapter conversion failed")
			return fmt.Errorf("failed to convert chapter: %v", err)
		}
	}
	if convertedChapter == nil {
		log.Error().Str("file", chapter.FilePath).Msg("Conversion returned nil chapter")
		return fmt.Errorf("failed to convert chapter")
	}

	log.Debug().
		Str("file", chapter.FilePath).
		Int("converted_pages", len(convertedChapter.Pages)).
		Msg("Chapter conversion completed")

	convertedChapter.SetConverted()

	// Step 4: Determine output path
	outputPath := options.Path
	originalPath := options.Path
	isCbrOverride := false

	if options.Override {
		pathLower := strings.ToLower(options.Path)
		if strings.HasSuffix(pathLower, ".cbr") {
			outputPath = strings.TrimSuffix(options.Path, filepath.Ext(options.Path)) + ".cbz"
			isCbrOverride = true
		}
	} else {
		pathLower := strings.ToLower(options.Path)
		if strings.HasSuffix(pathLower, ".cbz") {
			outputPath = strings.TrimSuffix(options.Path, ".cbz") + "_converted.cbz"
		} else if strings.HasSuffix(pathLower, ".cbr") {
			outputPath = strings.TrimSuffix(options.Path, ".cbr") + "_converted.cbz"
		} else {
			outputPath = options.Path + "_converted.cbz"
		}
	}

	// Step 5: Write converted chapter to CBZ (streaming from disk)
	log.Debug().Str("output_path", outputPath).Msg("Writing converted chapter to CBZ file")
	err = cbz.WriteChapterToCBZ(convertedChapter, outputPath)
	if err != nil {
		log.Error().Str("output_path", outputPath).Err(err).Msg("Failed to write converted chapter")
		return fmt.Errorf("failed to write converted chapter: %v", err)
	}

	// If overriding a CBR file, delete the original
	if isCbrOverride {
		err = os.Remove(originalPath)
		if err != nil {
			log.Warn().Str("file", originalPath).Err(err).Msg("Failed to delete original CBR file")
		} else {
			log.Info().Str("file", originalPath).Msg("Deleted original CBR file")
		}
	}

	log.Info().Str("output", outputPath).Msg("Converted file written")
	return nil
}
