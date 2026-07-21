package cbz

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
	"github.com/rs/zerolog/log"
)

// resolvePageName picks the final archive entry name for a page.
//
// Order of resolution:
//  1. When page.OriginalName is set (keep-filenames mode), use its stem with
//     the current page.Extension. Split pages append the -NN suffix so all
//     parts of the same source still land in the archive.
//  2. If that final name has already been used in this archive, fall back to
//     the indexed form (stem + "_%04d" + extension) so the zip stays valid.
//  3. Otherwise (OriginalName is empty), use the historical %04d / %04d-NN
//     sequential naming.
//
// usedNames tracks every name already chosen for this archive and is mutated
// in place so the caller can keep a single map across the whole chapter.
func resolvePageName(page *manga.PageFile, usedNames map[string]struct{}) string {
	if page.OriginalName != "" {
		stem := strings.TrimSuffix(page.OriginalName, filepath.Ext(page.OriginalName))
		var candidate string
		if page.IsSplitted {
			candidate = fmt.Sprintf("%s-%02d%s", stem, page.SplitPartIndex, page.Extension)
		} else {
			candidate = stem + page.Extension
		}
		if _, taken := usedNames[candidate]; !taken {
			usedNames[candidate] = struct{}{}
			return candidate
		}
		// Collision: fall back to the indexed form so the entry still gets
		// written, but make it visibly distinct from the preserved one.
		fallback := fmt.Sprintf("%s_%04d%s", stem, page.Index, page.Extension)
		usedNames[fallback] = struct{}{}
		return fallback
	}

	if page.IsSplitted {
		name := fmt.Sprintf("%04d-%02d%s", page.Index, page.SplitPartIndex, page.Extension)
		usedNames[name] = struct{}{}
		return name
	}
	name := fmt.Sprintf("%04d%s", page.Index, page.Extension)
	usedNames[name] = struct{}{}
	return name
}

// WriteChapterToCBZ creates a CBZ file from a Chapter by streaming page files
// from disk directly into the zip archive. No image data is held in memory.
func WriteChapterToCBZ(chapter *manga.Chapter, outputFilePath string) (err error) {
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

	// Write each page to the archive by streaming from disk.
	// Final name resolution: when a page carries an OriginalName (recorded by
	// ExtractChapter when --keep-filenames is on), preserve its stem and only
	// swap the extension to the current page.Extension. Duplicates in the
	// archive fall back to the indexed naming so the output stays a valid zip.
	usedNames := make(map[string]struct{}, len(chapter.Pages))
	for _, page := range chapter.Pages {
		fileName := resolvePageName(page, usedNames)

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
