package cbz

import (
	"archive/zip"
	"fmt"
	"github.com/belphemur/CBZOptimizer/manga"
	"github.com/belphemur/CBZOptimizer/utils/errs"
	"os"
	"time"
)

func WriteChapterToCBZ(chapter *manga.Chapter, outputFilePath string) error {
	// Create a new ZIP file
	zipFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create .cbz file: %w", err)
	}
	defer errs.Capture(&err, zipFile.Close, "failed to close .cbz file")

	// Create a new ZIP writer
	zipWriter := zip.NewWriter(zipFile)
	if err != nil {
		return err
	}
	defer errs.Capture(&err, zipWriter.Close, "failed to close .cbz writer")

	// Write each page to the ZIP archive
	for _, page := range chapter.Pages {
		// Construct the file name for the page
		var fileName string
		if page.IsSplitted {
			// Use the format page%03d-%02d for split pages
			fileName = fmt.Sprintf("%04d-%02d%s", page.Index, page.SplitPartIndex, page.Extension)
		} else {
			// Use the format page%03d for non-split pages
			fileName = fmt.Sprintf("%04d%s", page.Index, page.Extension)
		}

		// Create a new file in the ZIP archive
		fileWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
			Name:     fileName,
			Method:   zip.Store,
			Modified: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to create file in .cbz: %w", err)
		}

		// Write the page contents to the file
		_, err = fileWriter.Write(page.Contents.Bytes())
		if err != nil {
			return fmt.Errorf("failed to write page contents: %w", err)
		}
	}

	// Optionally, write the ComicInfo.xml file if present
	if chapter.ComicInfoXml != "" {
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
			return fmt.Errorf("failed to write ComicInfo.xml contents: %w", err)
		}
	}

	if chapter.IsConverted {

		convertedString := fmt.Sprintf("%s\nThis chapter has been converted by CBZOptimizer.", chapter.ConvertedTime)
		err = zipWriter.SetComment(convertedString)
		if err != nil {
			return fmt.Errorf("failed to write comment: %w", err)
		}
	}

	return nil
}
