// Package codec validates quality, decodes JPEG/PNG stills, and locally encodes
// lossy WebP and AVIF using the pinned pure-Go libraries.
package codec

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
)

// Quality is an encoding-quality value in the range [1,100].
type Quality int

// ErrInvalidQuality is returned when a quality value is outside [1,100].
var ErrInvalidQuality = errors.New("codec: quality must be between 1 and 100")

// NewQuality constructs a Quality after validating the range.
func NewQuality(q int) (Quality, error) {
	if q < 1 || q > 100 {
		return 0, fmt.Errorf("codec: %d: %w", q, ErrInvalidQuality)
	}
	return Quality(q), nil
}

// PairFromBaseQuality converts a single base quality into WebP and AVIF qualities.
// WebP uses the base quality directly; AVIF uses max(1, round(q*0.75)).
func PairFromBaseQuality(q int) (webp Quality, avif Quality, err error) {
	base, err := NewQuality(q)
	if err != nil {
		return 0, 0, err
	}

	avifVal := int(float64(base)*0.75 + 0.5) // round to nearest int
	if avifVal < 1 {
		avifVal = 1
	}
	avifQ, err := NewQuality(avifVal)
	if err != nil {
		return 0, 0, err
	}

	return base, avifQ, nil
}

// DecodeStill decodes a single-frame image from r. Only JPEG and PNG are accepted.
// The function detects the format from magic bytes and explicitly delegates to the
// standard-library decoder. If the data is neither JPEG nor PNG, the error wraps
// image.ErrFormat.
func DecodeStill(r io.Reader) (image.Image, error) {
	const (
		jpegMagic = "\xff\xd8"
		pngMagic  = "\x89PNG\r\n\x1a\n"
	)

	prefix := make([]byte, 8)
	n, err := io.ReadFull(r, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("codec: read header: %w", err)
	}
	prefix = prefix[:n]

	mr := io.MultiReader(bytes.NewReader(prefix), r)

	switch {
	case len(prefix) >= 2 && string(prefix[:2]) == jpegMagic:
		img, err := jpeg.Decode(mr)
		if err != nil {
			return nil, fmt.Errorf("codec: jpeg decode: %w", err)
		}
		return img, nil
	case len(prefix) >= 8 && string(prefix[:8]) == pngMagic:
		img, err := png.Decode(mr)
		if err != nil {
			return nil, fmt.Errorf("codec: png decode: %w", err)
		}
		return img, nil
	default:
		return nil, fmt.Errorf("codec: unsupported image format: %w", image.ErrFormat)
	}
}
