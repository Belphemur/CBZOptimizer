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
func Optimize(options *OptimizeOptions) error {
	log.Info().Str("file", options.Path).Msg("Processing file")
	log.Debug().
		Str("file", options.Path).
		Uint8("quality", options.Quality).
		Bool("override", options.Override).
		Bool("split", options.Split).
		Msg("Optimization parameters")

	// Load the chapter
	log.Debug().Str("file", options.Path).Msg("Loading chapter")
	chapter, err := cbz.LoadChapter(options.Path)
	if err != nil {
		log.Error().Str("file", options.Path).Err(err).Msg("Failed to load chapter")
		return fmt.Errorf("failed to load chapter: %v", err)
	}
	log.Debug().
		Str("file", options.Path).
		Int("pages", len(chapter.Pages)).
		Bool("converted", chapter.IsConverted).
		Msg("Chapter loaded successfully")

	if chapter.IsConverted {
		log.Info().Str("file", options.Path).Msg("Chapter already converted")
		return nil
	}

	// Convert the chapter
	log.Debug().
		Str("file", chapter.FilePath).
		Int("pages", len(chapter.Pages)).
		Uint8("quality", options.Quality).
		Bool("split", options.Split).
		Msg("Starting chapter conversion")

	var ctx context.Context
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), options.Timeout)
		defer cancel()
		log.Debug().Str("file", chapter.FilePath).Dur("timeout", options.Timeout).Msg("Applying timeout to chapter conversion")
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

	// Clean up the staging temp folder (if any) used to hold converted page
	// contents on disk once we're done with the chapter, regardless of
	// success or failure below.
	defer func() {
		if cleanupErr := convertedChapter.Cleanup(); cleanupErr != nil {
			log.Warn().Str("file", chapter.FilePath).Err(cleanupErr).Msg("Failed to remove staging temp folder")
		}
	}()

	log.Debug().
		Str("file", chapter.FilePath).
		Int("original_pages", len(chapter.Pages)).
		Int("converted_pages", len(convertedChapter.Pages)).
		Msg("Chapter conversion completed")

	convertedChapter.SetConverted()

	// Determine output path and handle CBR override logic
	log.Debug().
		Str("input_path", options.Path).
		Bool("override", options.Override).
		Msg("Determining output path")

	outputPath := options.Path
	originalPath := options.Path
	isCbrOverride := false

	if options.Override {
		// For override mode, check if it's a CBR file that needs to be converted to CBZ
		pathLower := strings.ToLower(options.Path)
		if strings.HasSuffix(pathLower, ".cbr") {
			// Convert CBR to CBZ: change extension and mark for deletion
			outputPath = strings.TrimSuffix(options.Path, filepath.Ext(options.Path)) + ".cbz"
			isCbrOverride = true
			log.Debug().
				Str("original_path", originalPath).
				Str("output_path", outputPath).
				Msg("CBR to CBZ conversion: will delete original after conversion")
		} else {
			log.Debug().
				Str("original_path", originalPath).
				Str("output_path", outputPath).
				Msg("CBZ override mode: will overwrite original file")
		}
	} else {
		// Handle both .cbz and .cbr files - strip the extension and add _converted.cbz
		pathLower := strings.ToLower(options.Path)
		if strings.HasSuffix(pathLower, ".cbz") {
			outputPath = strings.TrimSuffix(options.Path, ".cbz") + "_converted.cbz"
		} else if strings.HasSuffix(pathLower, ".cbr") {
			outputPath = strings.TrimSuffix(options.Path, ".cbr") + "_converted.cbz"
		} else {
			// Fallback for other extensions - just add _converted.cbz
			outputPath = options.Path + "_converted.cbz"
		}
		log.Debug().
			Str("original_path", originalPath).
			Str("output_path", outputPath).
			Msg("Non-override mode: creating converted file alongside original")
	}

	// Write the converted chapter to CBZ file
	log.Debug().Str("output_path", outputPath).Msg("Writing converted chapter to CBZ file")
	err = cbz.WriteChapterToCBZ(convertedChapter, outputPath)
	if err != nil {
		log.Error().Str("output_path", outputPath).Err(err).Msg("Failed to write converted chapter")
		return fmt.Errorf("failed to write converted chapter: %v", err)
	}
	log.Debug().Str("output_path", outputPath).Msg("Successfully wrote converted chapter")

	// If we're overriding a CBR file, delete the original CBR after successful write
	if isCbrOverride {
		log.Debug().Str("file", originalPath).Msg("Attempting to delete original CBR file")
		err = os.Remove(originalPath)
		if err != nil {
			// Log the error but don't fail the operation since conversion succeeded
			log.Warn().Str("file", originalPath).Err(err).Msg("Failed to delete original CBR file")
		} else {
			log.Info().Str("file", originalPath).Msg("Deleted original CBR file")
		}
	}

	log.Info().Str("output", outputPath).Msg("Converted file written")
	return nil

}
