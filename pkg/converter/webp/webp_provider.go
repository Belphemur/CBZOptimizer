package webp

import (
	"image"
	"io"

	"github.com/HugoSmits86/nativewebp"
)

// maxQuality is the maximum value accepted for the quality parameter (0-100),
// kept for backward compatibility with the CLI/config quality flag.
const maxQuality = 100

// PrepareEncoder used to download and configure the libwebp binary used by the
// previous cwebp-based encoder. The encoder is now a pure Go implementation
// (github.com/HugoSmits86/nativewebp) that requires no external binary or
// setup step, so this is kept only for backward compatibility with the
// Converter interface and is a no-op.
func PrepareEncoder() error {
	return nil
}

// qualityToCompressionLevel maps the 0-100 quality value used throughout the
// application to the 0-6 CompressionLevel understood by nativewebp.
//
// Note: nativewebp only encodes lossless WebP (VP8L). There is no lossy
// quality trade-off like the previous libwebp based encoder offered; the
// "quality" value instead controls how much effort the encoder spends trying
// to compress the image (higher effort can yield smaller files at the cost of
// more CPU time), matching nativewebp.CompressionLevel semantics.
func qualityToCompressionLevel(quality uint) nativewebp.CompressionLevel {
	if quality > maxQuality {
		quality = maxQuality
	}
	level := int(nativewebp.BestCompression) * int(quality) / maxQuality
	return nativewebp.CompressionLevel(level)
}

// Encode encodes the given image as a lossless WebP image into w. The
// quality parameter (0-100) is mapped to nativewebp's compression effort
// level (0-6).
func Encode(w io.Writer, m image.Image, quality uint) error {
	return nativewebp.Encode(w, m, &nativewebp.Options{
		CompressionLevel: qualityToCompressionLevel(quality),
	})
}
