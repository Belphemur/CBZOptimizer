package manga

// PageFile represents a single page image stored on disk.
// No image data is held in memory — only metadata and a file path.
type PageFile struct {
	// Index of the page in the chapter (original ordering).
	Index uint16
	// Extension of the page image file (e.g., ".webp", ".jpg").
	Extension string
	// FilePath is the absolute path to the image file on disk.
	FilePath string
	// IsSplitted indicates whether this page was split from a larger image.
	IsSplitted bool
	// SplitPartIndex is the part index when the page was split.
	SplitPartIndex uint16
}
