package codec

import (
	"image"
	"io"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/webp"
)

// WebPEncoder encodes a still image to lossy WebP.
type WebPEncoder interface {
	EncodeWebP(w io.Writer, m image.Image, q Quality) error
}

// AVIFEncoder encodes a still image to lossy AVIF.
type AVIFEncoder interface {
	EncodeAVIF(w io.Writer, m image.Image, q Quality) error
}

// webpAdapter wraps github.com/gen2brain/webp to satisfy WebPEncoder.
type webpAdapter struct{}

func (webpAdapter) EncodeWebP(w io.Writer, m image.Image, q Quality) error {
	return webp.Encode(w, m, webp.Options{
		Quality: int(q),
		Method:  4,
	})
}

// avifAdapter wraps github.com/gen2brain/avif to satisfy AVIFEncoder.
type avifAdapter struct{}

func (avifAdapter) EncodeAVIF(w io.Writer, m image.Image, q Quality) error {
	return avif.Encode(w, m, avif.Options{
		Quality:      int(q),
		QualityAlpha: int(q),
		Speed:        6,
	})
}

// Encoders holds the concrete encoder implementations used by the application.
type Encoders struct {
	WebP WebPEncoder
	AVIF AVIFEncoder
}

// NewEncoders returns encoders backed by the pure-Go gen2brain libraries.
func NewEncoders() *Encoders {
	return &Encoders{
		WebP: webpAdapter{},
		AVIF: avifAdapter{},
	}
}
