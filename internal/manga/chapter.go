package manga

import (
	"fmt"
	"os"
	"time"
)

// Chapter represents a comic book chapter with pages stored on disk.
type Chapter struct {
	// FilePath is the path to the original archive file.
	FilePath string
	// Pages is a slice of page files on disk.
	Pages []*PageFile
	// ComicInfoXml holds the ComicInfo.xml content (small, kept in memory).
	ComicInfoXml string
	// IsConverted indicates whether the chapter has already been converted.
	IsConverted bool
	// ConvertedTime is when the chapter was converted.
	ConvertedTime time.Time
	// TempDir is the root temp directory for this chapter's extracted/converted files.
	// Cleanup removes this entire directory.
	TempDir string
}

// SetConverted marks the chapter as converted with the current timestamp.
func (chapter *Chapter) SetConverted() {
	chapter.IsConverted = true
	chapter.ConvertedTime = time.Now()
}

// Cleanup removes the chapter's temp directory and all extracted/converted files.
func (chapter *Chapter) Cleanup() error {
	if chapter.TempDir == "" {
		return nil
	}
	if err := os.RemoveAll(chapter.TempDir); err != nil {
		return fmt.Errorf("failed to remove temp directory: %w", err)
	}
	chapter.TempDir = ""
	return nil
}
