package codec

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

const resizeEpsilon = 1e-9

// Resize scales img by scale on both axes. scale <= 0 or scale within epsilon
// of 1 returns img unchanged. Otherwise the image is resampled with
// draw.CatmullRom into a new image.NRGBA of size max(1, round(original*scale)).
func Resize(img image.Image, scale float64) image.Image {
	if scale <= 0 || math.Abs(scale-1.0) <= resizeEpsilon {
		return img
	}

	b := img.Bounds()
	nw := int(math.Round(float64(b.Dx()) * scale))
	if nw < 1 {
		nw = 1
	}
	nh := int(math.Round(float64(b.Dy()) * scale))
	if nh < 1 {
		nh = 1
	}
	if nw == b.Dx() && nh == b.Dy() {
		return img
	}

	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
