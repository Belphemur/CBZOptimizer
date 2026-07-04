package cbz

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
	"github.com/rs/zerolog/log"
)

// WriteChapterToCBZ creates a CBZ file from a Chapter by streaming page files
// from disk directly into the zip archive. No image data is held in memory.
func WriteChapterToCBZ(chapter *manga.Chapter, outputFilePath string) error {
	log.Debug().
		Str("chapter_file", chapter.FilePath).
		Str("output_path", outputFilePath).
		Int("page_count", len(chapter.Pages)).
		Bool("is_converted", chapter.IsConverted).
		Msg("Starting CBZ file creation")

	// Create output file
	zipFile, err := os.Create(outputFilePath)
	if err != nil {
		log.Error().Str("output_path", outputFilePath).Err(err).Msg("Failed to create CBZ file")
		return fmt.Errorf("failed to create .cbz file: %w", err)
	}
	defer errs.Capture(&err, zipFile.Close, "failed to close .cbz file")

	// Create ZIP writer
	zipWriter := zip.NewWriter(zipFile)
	defer errs.Capture(&err, zipWriter.Close, "failed to close .cbz writer")

	// Write each page to the archive by streaming from disk
	for _, page := range chapter.Pages {
		var fileName string
		if page.IsSplitted {
			fileName = fmt.Sprintf("%04d-%02d%s", page.Index, page.SplitPartIndex, page.Extension)
		} else {
			fileName = fmt.Sprintf("%04d%s", page.Index, page.Extension)
		}

		log.Debug().
			Str("output_path", outputFilePath).
			Uint16("page_index", page.Index).
			Str("filename", fileName).
			Str("source", page.FilePath).
			Msg("Writing page to CBZ archive")

		// Create file entry in the zip (Store method = no compression, images are already compressed)
		fileWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
			Name:     fileName,
			Method:   zip.Store,
			Modified: time.Now(),
		})
		if err != nil {
			log.Error().Str("filename", fileName).Err(err).Msg("Failed to create file in CBZ archive")
			return fmt.Errorf("failed to create file in .cbz: %w", err)
		}

		// Stream the page file from disk into the archive
		pageFile, err := os.Open(page.FilePath)
		if err != nil {
			log.Error().Str("filename", fileName).Str("source", page.FilePath).Err(err).Msg("Failed to open page file")
			return fmt.Errorf("failed to open page file: %w", err)
		}

		bytesWritten, err := io.Copy(fileWriter, pageFile)
		closeErr := pageFile.Close()
		if err != nil {
			log.Error().Str("filename", fileName).Err(err).Msg("Failed to write page contents")
			return fmt.Errorf("failed to write page contents: %w", err)
		}
		if closeErr != nil {
			log.Error().Str("filename", fileName).Err(closeErr).Msg("Failed to close page file")
			return fmt.Errorf("failed to close page file: %w", closeErr)
		}

		log.Debug().
			Str("filename", fileName).
			Int64("bytes_written", bytesWritten).
			Msg("Page written successfully")
	}

	// Write ComicInfo.xml if present
	if chapter.ComicInfoXml != "" {
		log.Debug().Str("output_path", outputFilePath).Msg("Writing ComicInfo.xml")
		comicInfoWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
			Name:     "ComicInfo.xml",
			Method:   zip.Deflate,
			Modified: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to create ComicInfo.xml in .cbz: %w", err)
		}

		_, err = comicInfoWriter.Write([]byte(chapter.ComicInfoXml))
		if err != nil {
			return fmt.Errorf("failed to write ComicInfo.xml: %w", err)
		}
	}

	// Set zip comment for converted chapters
	if chapter.IsConverted {
		comment := fmt.Sprintf("%s\nThis chapter has been converted by CBZOptimizer.", chapter.ConvertedTime)
		err = zipWriter.SetComment(comment)
		if err != nil {
			return fmt.Errorf("failed to write comment: %w", err)
		}
	}

	log.Debug().Str("output_path", outputFilePath).Msg("CBZ file creation completed")
	return nil
}
