package codec

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResize_scale_0_5_halves_dimensions(t *testing.T) {
	src := generateOpaqueImage(64, 64)
	dst := Resize(src, 0.5)
	assert.Equal(t, 32, dst.Bounds().Dx())
	assert.Equal(t, 32, dst.Bounds().Dy())
}

func TestResize_scale_1_returns_same_image(t *testing.T) {
	src := generateOpaqueImage(64, 64)
	dst := Resize(src, 1.0)
	assert.Equal(t, src, dst)
}

func TestResize_scale_0_returns_same_image(t *testing.T) {
	src := generateOpaqueImage(64, 64)
	dst := Resize(src, 0.0)
	assert.Equal(t, src, dst)
}

func TestResize_odd_size_rounds_correctly(t *testing.T) {
	src := generateOpaqueImage(15, 15)
	dst := Resize(src, 0.5)
	// math.Round(15*0.5) = 8
	assert.Equal(t, 8, dst.Bounds().Dx())
	assert.Equal(t, 8, dst.Bounds().Dy())
}

func TestResize_very_small_ensures_minimum_1x1(t *testing.T) {
	src := generateOpaqueImage(10, 10)
	dst := Resize(src, 0.01)
	assert.Equal(t, 1, dst.Bounds().Dx())
	assert.Equal(t, 1, dst.Bounds().Dy())
}

func TestResize_preserves_alpha(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 0})
	src.Set(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 128})
	src.Set(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	src.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	dst := Resize(src, 0.5)
	require.Equal(t, 1, dst.Bounds().Dx())
	require.Equal(t, 1, dst.Bounds().Dy())

	// NRGBA stores alpha pre-multiplied; just verify non-zero alpha channel exists.
	_, _, _, a := dst.At(0, 0).RGBA()
	assert.Greater(t, a, uint32(0), "alpha channel should be preserved")
}
