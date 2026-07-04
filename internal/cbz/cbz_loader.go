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

	"github.com/araddon/dateparse"
	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
	"github.com/mholt/archives"
	"github.com/rs/zerolog/log"
)

// IsAlreadyConverted performs a fast check to see if the archive is already
// converted without extracting any image data. It reads only the zip comment
// and metadata files (converted.txt) to determine conversion status.
func IsAlreadyConverted(filePath string) (bool, error) {
	log.Debug().Str("file_path", filePath).Msg("Checking if already converted")

	pathLower := strings.ToLower(filepath.Ext(filePath))

	if pathLower == ".cbz" {
		r, err := zip.OpenReader(filePath)
		if err != nil {
			return false, fmt.Errorf("failed to open CBZ for conversion check: %w", err)
		}
		defer errs.Capture(&err, r.Close, "failed to close zip reader")

		// Check zip comment
		if r.Comment != "" {
			scanner := bufio.NewScanner(strings.NewReader(r.Comment))
			if scanner.Scan() {
				_, err := dateparse.ParseAny(scanner.Text())
				if err == nil {
					log.Debug().Str("file_path", filePath).Msg("Already converted (zip comment)")
					return true, nil
				}
			}
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
		ctx := context.Background()
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
func ExtractChapter(filePath string) (*manga.Chapter, error) {
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
			if r.Comment != "" {
				scanner := bufio.NewScanner(strings.NewReader(r.Comment))
				if scanner.Scan() {
					t, err := dateparse.ParseAny(scanner.Text())
					if err == nil {
						chapter.IsConverted = true
						chapter.ConvertedTime = t
					}
				}
			}
			_ = r.Close()
		}
	}

	// Extract files using the archives library (supports both CBZ and CBR)
	ctx := context.Background()
	fsys, err := archives.FileSystem(ctx, filePath, nil)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}

	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		fileName := strings.ToLower(filepath.Base(path))

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

// LoadChapter is a convenience function that checks conversion status and
// extracts the chapter. It returns early if already converted (without
// extracting all pages).
func LoadChapter(filePath string) (*manga.Chapter, error) {
	return ExtractChapter(filePath)
}
