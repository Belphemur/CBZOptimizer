package manga

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog/log"
)

type Page struct {
	// Index of the page in the chapter.
	Index uint16 `json:"index" jsonschema:"description=Index of the page in the chapter."`
	// Extension of the page image.
	Extension string `json:"extension" jsonschema:"description=Extension of the page image."`
	// Size of the page in bytes
	Size uint64 `json:"-"`
	// Contents of the page. Nil when the page contents have been staged to
	// disk (see TempFilePath) to bound memory usage.
	Contents *bytes.Buffer `json:"-"`
	// TempFilePath, when non-empty, points to a file on disk (in a staging
	// temp folder) holding the page contents instead of keeping them fully
	// in memory. Use Open() to transparently read the page contents
	// regardless of where they are stored.
	TempFilePath string `json:"-"`
	// IsSplitted tell us if the page was cropped to multiple pieces
	IsSplitted bool `json:"is_cropped" jsonschema:"description=Was this page cropped."`
	// SplitPartIndex represent the index of the crop if the page was cropped
	SplitPartIndex uint16 `json:"crop_part_index" jsonschema:"description=Index of the crop if the image was cropped."`
}

// Open returns a reader for the page contents, transparently handling
// whether the contents are held in memory (Contents) or staged on disk
// (TempFilePath). The caller is responsible for closing the returned
// reader.
func (page *Page) Open() (io.ReadCloser, error) {
	if page.TempFilePath != "" {
		file, err := os.Open(page.TempFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open staged page contents: %w", err)
		}
		return file, nil
	}
	if page.Contents == nil {
		return nil, fmt.Errorf("page has no contents: neither TempFilePath nor Contents is set")
	}
	return io.NopCloser(bytes.NewReader(page.Contents.Bytes())), nil
}

// Stage writes the given content to a file in tempDir instead of keeping it
// in memory, updating Extension, Size and TempFilePath accordingly and
// clearing Contents. This is used after converting a page so that only the
// pages currently being processed are held in memory, bounding memory usage
// for chapters with many/large pages.
func (page *Page) Stage(tempDir string, content *bytes.Buffer, extension string) error {
	file, err := os.CreateTemp(tempDir, "page-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create staging file: %w", err)
	}
	fileName := file.Name()

	written, writeErr := file.Write(content.Bytes())
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil && written == content.Len() {
		page.TempFilePath = fileName
		page.Extension = extension
		page.Size = uint64(written)
		page.Contents = nil
		return nil
	}

	// Something went wrong: remove the incomplete/partial staging file
	// rather than leaving corrupted data behind on disk.
	if removeErr := os.Remove(fileName); removeErr != nil {
		log.Warn().Str("file", fileName).Err(removeErr).Msg("Failed to remove incomplete staging file")
	}

	if writeErr != nil {
		return fmt.Errorf("failed to write staging file: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close staging file: %w", closeErr)
	}
	return fmt.Errorf("short write to staging file: wrote %d of %d bytes", written, content.Len())
}
