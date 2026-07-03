package manga

import (
	"fmt"
	"os"
	"time"
)

type Chapter struct {
	// FilePath is the path to the chapter's directory.
	FilePath string
	// Pages is a slice of pointers to Page objects.
	Pages []*Page
	// ComicInfo is a string containing information about the chapter.
	ComicInfoXml string
	// IsConverted is a boolean that indicates whether the chapter has been converted.
	IsConverted bool
	// ConvertedTime is a pointer to a time.Time object that indicates when the chapter was converted. Nil mean not converted.
	ConvertedTime time.Time
	// TempDir, when non-empty, is a staging temp folder holding converted
	// page contents on disk (see Page.TempFilePath) rather than fully in
	// memory. It should be removed via Cleanup once the chapter has been
	// written out (or is no longer needed).
	TempDir string
}

// SetConverted sets the IsConverted field to true and sets the ConvertedTime field to the current time.
func (chapter *Chapter) SetConverted() {
	chapter.IsConverted = true
	chapter.ConvertedTime = time.Now()
}

// Cleanup removes the chapter's staging temp folder (if any), releasing any
// page contents that were staged to disk during conversion.
func (chapter *Chapter) Cleanup() error {
	if chapter.TempDir == "" {
		return nil
	}
	if err := os.RemoveAll(chapter.TempDir); err != nil {
		return fmt.Errorf("failed to remove staging temp folder: %w", err)
	}
	chapter.TempDir = ""
	return nil
}
