package codec

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/webp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewQuality(t *testing.T) {
	q, err := NewQuality(50)
	require.NoError(t, err)
	assert.Equal(t, Quality(50), q)

	_, err = NewQuality(0)
	assert.ErrorIs(t, err, ErrInvalidQuality)

	_, err = NewQuality(101)
	assert.ErrorIs(t, err, ErrInvalidQuality)

	_, err = NewQuality(-5)
	assert.ErrorIs(t, err, ErrInvalidQuality)
}

func TestPairFromBaseQuality(t *testing.T) {
	tests := []struct {
		base     int
		wantWebP int
		wantAVIF int
		wantErr  bool
	}{
		{80, 80, 60, false},
		{100, 100, 75, false},
		{40, 40, 30, false},
		{1, 1, 1, false},
		{2, 2, 2, false},
		{0, 0, 0, true},
		{101, 0, 0, true},
		{-5, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("base=%d", tt.base), func(t *testing.T) {
			webpQ, avifQ, err := PairFromBaseQuality(tt.base)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Quality(tt.wantWebP), webpQ)
			assert.Equal(t, Quality(tt.wantAVIF), avifQ)
		})
	}
}

func TestDecodeStill_JPEG(t *testing.T) {
	src := generateOpaqueImage(64, 64)
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90})
	require.NoError(t, err)

	decoded, err := DecodeStill(&buf)
	require.NoError(t, err)
	assert.Equal(t, 64, decoded.Bounds().Dx())
	assert.Equal(t, 64, decoded.Bounds().Dy())

	_, isYCbCr := decoded.(*image.YCbCr)
	assert.True(t, isYCbCr, "decoded JPEG should be YCbCr")
}

func TestDecodeStill_PNG(t *testing.T) {
	src := generateAlphaGradientImage(64, 64)
	var buf bytes.Buffer
	err := png.Encode(&buf, src)
	require.NoError(t, err)

	decoded, err := DecodeStill(&buf)
	require.NoError(t, err)
	assert.Equal(t, 64, decoded.Bounds().Dx())
	assert.Equal(t, 64, decoded.Bounds().Dy())
}

func TestDecodeStill_Unsupported(t *testing.T) {
	// GIF magic bytes — must be rejected even if image.Decode has a GIF decoder registered.
	data := []byte("GIF89a")
	_, err := DecodeStill(bytes.NewReader(data))
	assert.ErrorIs(t, err, image.ErrFormat)
}

func TestDecodeStill_TruncatedJPEG(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF} // incomplete JPEG header
	_, err := DecodeStill(bytes.NewReader(data))
	require.Error(t, err)
}

func TestDecodeStill_TruncatedPNG(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00} // incomplete PNG
	_, err := DecodeStill(bytes.NewReader(data))
	require.Error(t, err)
}

func TestDecodeStill_EmptyReader(t *testing.T) {
	_, err := DecodeStill(bytes.NewReader(nil))
	assert.ErrorIs(t, err, image.ErrFormat)
}

func TestEncoders_WebP(t *testing.T) {
	encoders := NewEncoders()
	require.NotNil(t, encoders.WebP)

	q, err := NewQuality(80)
	require.NoError(t, err)

	t.Run("opaque", func(t *testing.T) {
		src := generateOpaqueImage(64, 64)
		var buf bytes.Buffer
		err := encoders.WebP.EncodeWebP(&buf, src, q)
		require.NoError(t, err)
		assert.Greater(t, buf.Len(), 0, "encoded WebP must be non-empty")

		decoded, err := webp.Decode(&buf)
		require.NoError(t, err)
		assert.Equal(t, src.Bounds().Dx(), decoded.Bounds().Dx())
		assert.Equal(t, src.Bounds().Dy(), decoded.Bounds().Dy())
	})

	t.Run("alpha", func(t *testing.T) {
		src := generateAlphaGradientImage(64, 64)
		var buf bytes.Buffer
		err := encoders.WebP.EncodeWebP(&buf, src, q)
		require.NoError(t, err)
		assert.Greater(t, buf.Len(), 0, "encoded WebP must be non-empty")

		decoded, err := webp.Decode(&buf)
		require.NoError(t, err)
		assert.Equal(t, src.Bounds().Dx(), decoded.Bounds().Dx())
		assert.Equal(t, src.Bounds().Dy(), decoded.Bounds().Dy())
		checkAlphaBounds(t, src, decoded)
	})
}

func TestEncoders_AVIF(t *testing.T) {
	encoders := NewEncoders()
	require.NotNil(t, encoders.AVIF)

	q, err := NewQuality(80)
	require.NoError(t, err)

	t.Run("opaque", func(t *testing.T) {
		src := generateOpaqueImage(64, 64)
		var buf bytes.Buffer
		err := encoders.AVIF.EncodeAVIF(&buf, src, q)
		require.NoError(t, err)
		assert.Greater(t, buf.Len(), 0, "encoded AVIF must be non-empty")

		decoded, err := avif.Decode(&buf)
		require.NoError(t, err)
		assert.Equal(t, src.Bounds().Dx(), decoded.Bounds().Dx())
		assert.Equal(t, src.Bounds().Dy(), decoded.Bounds().Dy())
	})

	t.Run("alpha", func(t *testing.T) {
		src := generateAlphaGradientImage(64, 64)
		var buf bytes.Buffer
		err := encoders.AVIF.EncodeAVIF(&buf, src, q)
		require.NoError(t, err)
		assert.Greater(t, buf.Len(), 0, "encoded AVIF must be non-empty")

		decoded, err := avif.Decode(&buf)
		require.NoError(t, err)
		assert.Equal(t, src.Bounds().Dx(), decoded.Bounds().Dx())
		assert.Equal(t, src.Bounds().Dy(), decoded.Bounds().Dy())
		checkAlphaBounds(t, src, decoded)
	})
}

func TestEncoders_AcceptYCbCr(t *testing.T) {
	// Produce a JPEG, decode it to obtain a YCbCr image, then feed it to both encoders.
	src := generateOpaqueImage(64, 64)
	var jpegBuf bytes.Buffer
	err := jpeg.Encode(&jpegBuf, src, &jpeg.Options{Quality: 90})
	require.NoError(t, err)

	decoded, err := jpeg.Decode(&jpegBuf)
	require.NoError(t, err)
	_, isYCbCr := decoded.(*image.YCbCr)
	require.True(t, isYCbCr, "expected YCbCr image from jpeg.Decode")

	encoders := NewEncoders()
	q, err := NewQuality(80)
	require.NoError(t, err)

	var webpBuf bytes.Buffer
	err = encoders.WebP.EncodeWebP(&webpBuf, decoded, q)
	require.NoError(t, err)
	assert.Greater(t, webpBuf.Len(), 0)

	var avifBuf bytes.Buffer
	err = encoders.AVIF.EncodeAVIF(&avifBuf, decoded, q)
	require.NoError(t, err)
	assert.Greater(t, avifBuf.Len(), 0)
}

// generateOpaqueImage returns a small RGBA image with no transparency.
func generateOpaqueImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 255 / w),
				G: uint8(y * 255 / h),
				B: 128,
				A: 255,
			})
		}
	}
	return img
}

// generateAlphaGradientImage returns a small RGBA image with a horizontal alpha gradient.
func generateAlphaGradientImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: 128,
				G: 128,
				B: 128,
				A: uint8(x * 255 / w),
			})
		}
	}
	return img
}

// checkAlphaBounds verifies that fully-transparent source pixels decode to <30 alpha
// and fully-opaque source pixels decode to >220 alpha.
func checkAlphaBounds(t *testing.T, _ image.Image, decoded image.Image) {
	t.Helper()
	bounds := decoded.Bounds()

	// Leftmost column was fully transparent in the source.
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		_, _, _, a := decoded.At(bounds.Min.X, y).RGBA()
		alpha8 := uint8(a >> 8)
		assert.Less(t, alpha8, uint8(30),
			"pixel at (%d,%d) should remain nearly transparent", bounds.Min.X, y)
	}

	// Rightmost column was fully opaque in the source.
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		_, _, _, a := decoded.At(bounds.Max.X-1, y).RGBA()
		alpha8 := uint8(a >> 8)
		assert.Greater(t, alpha8, uint8(220),
			"pixel at (%d,%d) should remain nearly opaque", bounds.Max.X-1, y)
	}
}

// compile-time interface checks.
var (
	_ WebPEncoder = webpAdapter{}
	_ AVIFEncoder = avifAdapter{}
)

// FuzzDecodeStill exercises header detection with random prefixes.
func FuzzDecodeStill(f *testing.F) {
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE0})       // JPEG-like
	f.Add([]byte{0x89, 0x50, 0x4E, 0x47})       // PNG-like
	f.Add([]byte("GIF89a"))                     // GIF (unsupported)
	f.Add([]byte{})                             // empty
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00}) // unknown

	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := DecodeStill(bytes.NewReader(data))
		if err == nil {
			// Decoding success is only expected for well-formed JPEG/PNG data,
			// which is unlikely from random bytes. We accept either outcome.
			return
		}
		// Every error must either wrap image.ErrFormat or be a decode/read error.
		if !errors.Is(err, image.ErrFormat) {
			// jpeg.Decode and png.Decode may return other errors for malformed data;
			// ensure we wrapped them.
			require.NotEqual(t, io.EOF, err)
		}
	})
}
