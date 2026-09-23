package upload_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/webp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/codec"
	"github.com/anjuls/cloudinary-cli/internal/upload"
)

type fakeWebPEncoder struct{ err error }

func (f *fakeWebPEncoder) EncodeWebP(w io.Writer, _ image.Image, _ codec.Quality) error {
	if f.err != nil {
		return f.err
	}
	_, err := w.Write([]byte("fake-webp"))
	return err
}

type fakeAVIFEncoder struct{ err error }

func (f *fakeAVIFEncoder) EncodeAVIF(w io.Writer, _ image.Image, _ codec.Quality) error {
	if f.err != nil {
		return f.err
	}
	_, err := w.Write([]byte("fake-avif"))
	return err
}

type fakeUploader struct {
	uploads []struct {
		FilePath string
		Req      upload.UploadRequest
	}
	responses []upload.UploadResponse
	err       error
	errIndex  int
}

func (f *fakeUploader) Upload(_ context.Context, filePath string, req upload.UploadRequest) (upload.UploadResponse, error) {
	idx := len(f.uploads)
	f.uploads = append(f.uploads, struct {
		FilePath string
		Req      upload.UploadRequest
	}{FilePath: filePath, Req: req})
	if f.err != nil && idx == f.errIndex {
		return upload.UploadResponse{}, f.err
	}
	if idx < len(f.responses) {
		return f.responses[idx], nil
	}
	return upload.UploadResponse{PublicID: req.PublicID, SecureURL: "https://cdn.example/" + req.PublicID}, nil
}

type checkingUploader struct {
	webpPath, avifPath       string
	webpExisted, avifExisted bool
	failAfter                int
	calls                    int
}

func (c *checkingUploader) Upload(_ context.Context, filePath string, req upload.UploadRequest) (upload.UploadResponse, error) {
	idx := c.calls
	c.calls++
	switch {
	case strings.HasSuffix(filePath, ".webp"):
		c.webpPath = filePath
		_, err := os.Stat(filePath)
		c.webpExisted = err == nil
	case strings.HasSuffix(filePath, ".avif"):
		c.avifPath = filePath
		_, err := os.Stat(filePath)
		c.avifExisted = err == nil
	}
	if c.failAfter >= 0 && idx == c.failAfter {
		return upload.UploadResponse{}, errors.New("upload boom")
	}
	return upload.UploadResponse{PublicID: req.PublicID, SecureURL: "https://cdn.example/" + req.PublicID}, nil
}

func writePNG(t *testing.T, dir, name string, img image.Image) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())
	return p
}

func generateImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	return img
}

type realCodecUploader struct {
	t    *testing.T
	w, h int
}

func (r *realCodecUploader) Upload(_ context.Context, filePath string, req upload.UploadRequest) (upload.UploadResponse, error) {
	data, err := os.ReadFile(filePath)
	require.NoError(r.t, err)
	var img image.Image
	if strings.HasSuffix(filePath, ".webp") {
		img, err = webp.Decode(bytes.NewReader(data))
	} else {
		img, err = avif.Decode(bytes.NewReader(data))
	}
	require.NoError(r.t, err)
	assert.Equal(r.t, r.w, img.Bounds().Dx())
	assert.Equal(r.t, r.h, img.Bounds().Dy())
	return upload.UploadResponse{PublicID: req.PublicID, SecureURL: "https://cdn.example/" + req.PublicID}, nil
}
