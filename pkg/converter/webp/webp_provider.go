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

var prepareMutex sync.Mutex

func PrepareEncoder() error {
	prepareMutex.Lock()
	defer prepareMutex.Unlock()
	container := webpbin.NewCWebP(webpbin.SetLibVersion(libwebpVersion))
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
	return webpbin.NewCWebP(webpbin.SetLibVersion(libwebpVersion)).
		Quality(quality).
		InputImage(m).
		Output(w).
		Run()
}
