package converter

import (
	"context"
	"fmt"
	"strings"

	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/webp"
	"github.com/samber/lo"
)

// Converter defines the interface for image format converters.
// All operations are file-to-file: no image data is held in memory in the happy path.
type Converter interface {
	// Format returns the output format of this converter.
	Format() constant.ConversionFormat

	// ConvertChapter converts all pages in a chapter from their source files to
	// the target format. Pages are processed in parallel (bounded by CPU count).
	// On success, chapter.Pages is updated with converted PageFile entries.
	// Returns partial success (non-fatal errors) via errors.PageIgnoredError.
	ConvertChapter(ctx context.Context, chapter *manga.Chapter, quality uint8, split bool, progress func(message string, current uint32, total uint32)) (*manga.Chapter, error)

	// PrepareConverter ensures the external encoder binary is available.
	PrepareConverter() error
}

var converters = map[constant.ConversionFormat]Converter{
	constant.WebP: webp.New(),
}

// Available returns a list of available converters.
func Available() []constant.ConversionFormat {
	return lo.Keys(converters)
}

// Get returns a converter by format name.
// If the converter is not available, an error is returned.
var Get = getConverter

func getConverter(name constant.ConversionFormat) (Converter, error) {
	if converter, ok := converters[name]; ok {
		return converter, nil
	}

	return nil, fmt.Errorf("unknown converter \"%s\", available options are %s", name, strings.Join(lo.Map(Available(), func(item constant.ConversionFormat, index int) string {
		return item.String()
	}), ", "))
}
