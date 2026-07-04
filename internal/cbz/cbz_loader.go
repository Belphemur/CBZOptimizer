package cbz

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
	"github.com/mholt/archives"
	"github.com/rs/zerolog/log"
)

// supportedImageExtensions contains file extensions considered valid image pages.
var supportedImageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
	".tiff": true,
	".tif":  true,
}

// parseConvertedComment checks if a zip comment's first line is a parseable date,
// indicating the archive was already converted. Returns true and the parsed time if so.
func parseConvertedComment(comment string) bool {
	if comment == "" {
		return false
	}
	scanner := bufio.NewScanner(strings.NewReader(comment))
	if scanner.Scan() {
		_, err := dateparse.ParseAny(scanner.Text())
		return err == nil
	}
	return false
}

// parseConvertedCommentTime parses the converted timestamp from a zip comment.
// Returns the time and true if the comment indicates conversion, otherwise zero time and false.
func parseConvertedCommentTime(comment string) (time.Time, bool) {
	if comment == "" {
		return time.Time{}, false
	}
	scanner := bufio.NewScanner(strings.NewReader(comment))
	if scanner.Scan() {
		t, err := dateparse.ParseAny(scanner.Text())
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// IsAlreadyConverted performs a fast check to see if the archive is already
// converted without extracting any image data. It reads only the zip comment
// and metadata files (converted.txt) to determine conversion status.
func IsAlreadyConverted(ctx context.Context, filePath string) (converted bool, err error) {
	log.Debug().Str("file_path", filePath).Msg("Checking if already converted")

	pathLower := strings.ToLower(filepath.Ext(filePath))

	if pathLower == ".cbz" {
		r, err := zip.OpenReader(filePath)
		if err != nil {
			return false, fmt.Errorf("failed to open CBZ for conversion check: %w", err)
		}
		defer errs.Capture(&err, r.Close, "failed to close zip reader")

		// Check zip comment
		if parseConvertedComment(r.Comment) {
			log.Debug().Str("file_path", filePath).Msg("Already converted (zip comment)")
			return true, nil
		}

		// Check for converted.txt inside the archive
		for _, f := range r.File {
			if strings.ToLower(filepath.Base(f.Name)) == "converted.txt" {
				rc, err := f.Open()
				if err != nil {
					continue
				}
				scanner := bufio.NewScanner(rc)
				if scanner.Scan() {
					_, parseErr := dateparse.ParseAny(scanner.Text())
					_ = rc.Close()
					if parseErr == nil {
						log.Debug().Str("file_path", filePath).Msg("Already converted (converted.txt)")
						return true, nil
					}
				} else {
					_ = rc.Close()
				}
			}
		}
	}

	// For CBR files, we need to use the archives library to check
	if pathLower == ".cbr" {
		fsys, err := archives.FileSystem(ctx, filePath, nil)
		if err != nil {
			return false, fmt.Errorf("failed to open archive: %w", err)
		}

		var converted bool
		_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if strings.ToLower(filepath.Base(path)) == "converted.txt" {
				file, err := fsys.Open(path)
				if err != nil {
					return nil
				}
				defer func() { _ = file.Close() }()
				scanner := bufio.NewScanner(file)
				if scanner.Scan() {
					_, err := dateparse.ParseAny(scanner.Text())
					if err == nil {
						converted = true
						return fs.SkipAll
					}
				}
			}
			return nil
		})
		return converted, nil
	}

	return false, nil
}

// ExtractChapter extracts an archive (CBZ/CBR) to a temp directory on disk.
// Pages are streamed directly to files — no image data is held in memory.
// Returns a Chapter with PageFile entries pointing to extracted files.
func ExtractChapter(ctx context.Context, filePath string) (*manga.Chapter, error) {
	log.Debug().Str("file_path", filePath).Msg("Extracting chapter to disk")

	// Create temp directory for extraction
	tempDir, err := os.MkdirTemp("", "cbzoptimizer-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	inputDir := filepath.Join(tempDir, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to create input directory: %w", err)
	}

	chapter := &manga.Chapter{
		FilePath: filePath,
		TempDir:  tempDir,
	}

	// For CBZ files, read metadata from zip comment
	pathLower := strings.ToLower(filepath.Ext(filePath))
	if pathLower == ".cbz" {
		r, err := zip.OpenReader(filePath)
		if err == nil {
			if t, ok := parseConvertedCommentTime(r.Comment); ok {
				chapter.IsConverted = true
				chapter.ConvertedTime = t
			}
			_ = r.Close()
		}
	}

	// Extract files using the archives library (supports both CBZ and CBR)
	fsys, err := archives.FileSystem(ctx, filePath, nil)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}

	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		// Check for context cancellation during extraction
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ext := strings.ToLower(filepath.Ext(path))
		fileName := strings.ToLower(filepath.Base(path))

		// Skip OS-specific metadata files and junk
		if isJunkFile(path) {
			log.Debug().Str("file_path", filePath).Str("skipped", path).Msg("Skipping junk file")
			return nil
		}

		// Handle ComicInfo.xml
		if ext == ".xml" && fileName == "comicinfo.xml" {
			file, err := fsys.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open ComicInfo.xml: %w", err)
			}
			defer func() { _ = file.Close() }()
			xmlContent, err := io.ReadAll(file)
			if err != nil {
				return fmt.Errorf("failed to read ComicInfo.xml: %w", err)
			}
			chapter.ComicInfoXml = string(xmlContent)
			log.Debug().Str("file_path", filePath).Int("xml_size", len(xmlContent)).Msg("ComicInfo.xml loaded")
			return nil
		}

		// Handle converted.txt (check conversion status)
		if !chapter.IsConverted && ext == ".txt" && fileName == "converted.txt" {
			file, err := fsys.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open converted.txt: %w", err)
			}
			defer func() { _ = file.Close() }()
			scanner := bufio.NewScanner(file)
			if scanner.Scan() {
				t, err := dateparse.ParseAny(scanner.Text())
				if err == nil {
					chapter.IsConverted = true
					chapter.ConvertedTime = t
				}
			}
			return nil
		}

		// Only extract supported image files
		if !supportedImageExtensions[ext] {
			log.Debug().Str("file_path", filePath).Str("skipped", path).Str("ext", ext).Msg("Skipping non-image file")
			return nil
		}

		// Extract image file to disk
		file, err := fsys.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer func() { _ = file.Close() }()

		// Create output file with sequential naming
		pageIndex := uint16(len(chapter.Pages))
		outputName := fmt.Sprintf("%04d%s", pageIndex, ext)
		outputPath := filepath.Join(inputDir, outputName)

		outFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file %s: %w", outputPath, err)
		}

		_, err = io.Copy(outFile, file)
		closeErr := outFile.Close()
		if err != nil {
			_ = os.Remove(outputPath)
			return fmt.Errorf("failed to write file %s: %w", outputPath, err)
		}
		if closeErr != nil {
			_ = os.Remove(outputPath)
			return fmt.Errorf("failed to close file %s: %w", outputPath, closeErr)
		}

		page := &manga.PageFile{
			Index:     pageIndex,
			Extension: ext,
			FilePath:  outputPath,
		}
		chapter.Pages = append(chapter.Pages, page)

		log.Debug().
			Str("file_path", filePath).
			Str("archive_file", path).
			Uint16("page_index", pageIndex).
			Msg("Page extracted to disk")

		return nil
	})

	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to extract archive: %w", err)
	}

	log.Debug().
		Str("file_path", filePath).
		Int("pages_extracted", len(chapter.Pages)).
		Bool("is_converted", chapter.IsConverted).
		Msg("Chapter extraction completed")

	return chapter, nil
}

// isJunkFile returns true for known OS/tool metadata files that should not be
// treated as pages (e.g., __MACOSX/, Thumbs.db, .DS_Store).
func isJunkFile(path string) bool {
	// __MACOSX resource fork directories
	if strings.Contains(path, "__MACOSX") {
		return true
	}
	baseLower := strings.ToLower(filepath.Base(path))
	switch baseLower {
	case "thumbs.db", ".ds_store", "desktop.ini":
		return true
	}
	return false
}

// LoadChapter extracts the chapter from a CBZ/CBR file to disk.
// It delegates to ExtractChapter and always extracts all pages.
// Use IsAlreadyConverted for a fast conversion status check without extraction.
func LoadChapter(filePath string) (*manga.Chapter, error) {
	return ExtractChapter(context.Background(), filePath)
}
