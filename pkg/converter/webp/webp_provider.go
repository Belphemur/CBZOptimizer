package webp

import (
	"fmt"
	"strings"
	"sync"

	"github.com/belphemur/go-webpbin/v2"
)

const libwebpVersion = "1.6.0"

var config = webpbin.NewConfig()

var prepareMutex sync.Mutex

func init() {
	config.SetLibVersion(libwebpVersion)
}

func PrepareEncoder() error {
	prepareMutex.Lock()
	defer prepareMutex.Unlock()

	container := webpbin.NewCWebP(config)
	version, err := container.Version()
	if err != nil {
		return err
	}

	if !strings.HasPrefix(version, libwebpVersion) {
		return fmt.Errorf("unexpected webp version: got %s, want %s", version, libwebpVersion)
	}

	return nil
}

// EncodeFile converts an image file directly to WebP using cwebp.
// This is a zero-copy operation: no image data is loaded into Go memory.
func EncodeFile(inputPath string, outputPath string, quality uint) error {
	return webpbin.NewCWebP(config).
		Quality(quality).
		InputFile(inputPath).
		OutputFile(outputPath).
		Run()
}

// EncodeFileWithCrop converts a cropped region of an image file to WebP.
// Uses cwebp's native -crop flag — no Go-side image decode needed.
func EncodeFileWithCrop(inputPath string, outputPath string, quality uint, x, y, width, height int) error {
	return webpbin.NewCWebP(config).
		Quality(quality).
		Crop(x, y, width, height).
		InputFile(inputPath).
		OutputFile(outputPath).
		Run()
}
