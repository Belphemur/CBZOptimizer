package webp

import (
	"fmt"
	"image"
	"io"
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

func Encode(w io.Writer, m image.Image, quality uint) error {
	return webpbin.NewCWebP(config).
		Quality(quality).
		InputImage(m).
		Output(w).
		Run()
}
