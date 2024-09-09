package cbz

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"github.com/araddon/dateparse"
	"github.com/belphemur/CBZOptimizer/manga"
	"github.com/belphemur/CBZOptimizer/utils/errs"
	"io"
	"path/filepath"
	"strings"
)

func LoadChapter(filePath string) (*manga.Chapter, error) {
	// Open the .cbz file
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open .cbz file: %w", err)
	}
	defer errs.Capture(&err, r.Close, "failed to close opened .cbz file")

	chapter := &manga.Chapter{
		FilePath: filePath,
	}
	// Check for comment
	if r.Comment != "" {
		scanner := bufio.NewScanner(strings.NewReader(r.Comment))
		if scanner.Scan() {
			convertedTime := scanner.Text()
			chapter.ConvertedTime, err = dateparse.ParseAny(convertedTime)
			if err == nil {
				chapter.IsConverted = true
			}
		}
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		err := func() error {
			// Open the file inside the zip
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("failed to open file inside .cbz: %w", err)
			}

			defer errs.Capture(&err, rc.Close, "failed to close file inside .cbz")

			// Determine the file extension
			ext := strings.ToLower(filepath.Ext(f.Name))

			if ext == ".xml" && strings.ToLower(filepath.Base(f.Name)) == "comicinfo.xml" {
				// Read the ComicInfo.xml file content
				xmlContent, err := io.ReadAll(rc)
				if err != nil {
					return fmt.Errorf("failed to read ComicInfo.xml content: %w", err)
				}
				chapter.ComicInfoXml = string(xmlContent)
			} else if !chapter.IsConverted && ext == ".txt" && strings.ToLower(filepath.Base(f.Name)) == "converted.txt" {
				textContent, err := io.ReadAll(rc)
				if err != nil {
					return fmt.Errorf("failed to read Converted.xml content: %w", err)
				}
				scanner := bufio.NewScanner(bytes.NewReader(textContent))
				if scanner.Scan() {
					convertedTime := scanner.Text()
					chapter.ConvertedTime, err = dateparse.ParseAny(convertedTime)
					if err != nil {
						return fmt.Errorf("failed to parse converted time: %w", err)
					}
					chapter.IsConverted = true
				}
			} else {
				// Read the file contents for page
				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, rc)
				if err != nil {
					return fmt.Errorf("failed to read file contents: %w", err)
				}

				// Create a new Page object
				page := &manga.Page{
					Index:      uint16(len(chapter.Pages)), // Simple index based on order
					Extension:  ext,
					Size:       uint64(buf.Len()),
					Contents:   buf,
					IsSplitted: false,
				}

				// Add the page to the chapter
				chapter.Pages = append(chapter.Pages, page)
			}
			return nil
		}()
		if err != nil {
			return nil, err
		}
	}

	return chapter, nil
}
