package webp

import (
	"github.com/belphemur/go-webpbin/v2"
	"image"
	"io"
	"sync"
)

const libwebpVersion = "1.6.0"

var prepareMutex sync.Mutex

func PrepareEncoder() error {
	prepareMutex.Lock()
	defer prepareMutex.Unlock()
	webpbin.SetLibVersion(libwebpVersion)
	container := webpbin.NewCWebP()
	return container.BinWrapper.Run()
}
func Encode(w io.Writer, m image.Image, quality uint) error {
	return webpbin.NewCWebP().
		Quality(quality).
		InputImage(m).
		Output(w).
		Run()
}
