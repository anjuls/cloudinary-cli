package upload

import (
	"context"
	"image"
	"os"

	"github.com/anjuls/cloudinary-cli/internal/codec"
	"github.com/anjuls/cloudinary-cli/internal/naming"
)

// Ports holds the external dependencies for Process.
type Ports struct {
	WebP codec.WebPEncoder
	AVIF codec.AVIFEncoder
	Up   Uploader
}

// Options configures the encoding and upload behavior for one source.
type Options struct {
	WebPQuality codec.Quality
	AVIFQuality codec.Quality
	Folder      FolderDirective
	Overwrite   bool
}

// Process encodes a source image to WebP and AVIF and uploads both variants.
func Process(ctx context.Context, ports Ports, opts Options, sourcePath string) SourceResult {
	result := SourceResult{
		Source:       sourcePath,
		PublicIDBase: naming.Base(sourcePath),
		Status:       SourceError,
	}

	img, err := openAndDecode(sourcePath)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	var webpPath, avifPath string
	defer func() {
		removeTemp(webpPath)
		removeTemp(avifPath)
	}()

	webpPath, err = encodeWebPToTemp(ports.WebP, img, opts.WebPQuality)
	if err != nil {
		result.Variants = []VariantResult{
			{Format: FormatWebP, Status: VariantError, Stage: StageEncode, Error: err.Error()},
			{Format: FormatAVIF, Status: VariantSkipped, Reason: "pair aborted: webp encode failed"},
		}
		return result
	}

	avifPath, err = encodeAVIFToTemp(ports.AVIF, img, opts.AVIFQuality)
	if err != nil {
		result.Variants = []VariantResult{
			{Format: FormatWebP, Status: VariantSkipped, Reason: "pair aborted: avif encode failed"},
			{Format: FormatAVIF, Status: VariantError, Stage: StageEncode, Error: err.Error()},
		}
		return result
	}

	webpReq := UploadRequest{
		PublicID:  result.PublicIDBase + "-webp",
		Folder:    opts.Folder,
		Overwrite: opts.Overwrite,
	}
	webpRes, err := ports.Up.Upload(ctx, webpPath, webpReq)
	if err != nil {
		result.Variants = []VariantResult{
			{Format: FormatWebP, Status: VariantError, Stage: StageUpload, Error: err.Error()},
			{Format: FormatAVIF, Status: VariantSkipped, Reason: "pair aborted: webp upload failed"},
		}
		return result
	}

	avifReq := UploadRequest{
		PublicID:  result.PublicIDBase + "-avif",
		Folder:    opts.Folder,
		Overwrite: opts.Overwrite,
	}
	avifRes, err := ports.Up.Upload(ctx, avifPath, avifReq)
	if err != nil {
		result.Status = SourcePartial
		result.Variants = []VariantResult{
			{Format: FormatWebP, Status: VariantUploaded, PublicID: webpRes.PublicID, SecureURL: webpRes.SecureURL},
			{Format: FormatAVIF, Status: VariantError, Stage: StageUpload, Error: err.Error()},
		}
		return result
	}

	result.Status = SourceOK
	result.Variants = []VariantResult{
		{Format: FormatWebP, Status: VariantUploaded, PublicID: webpRes.PublicID, SecureURL: webpRes.SecureURL},
		{Format: FormatAVIF, Status: VariantUploaded, PublicID: avifRes.PublicID, SecureURL: avifRes.SecureURL},
	}
	return result
}

func openAndDecode(path string) (img image.Image, err error) {
	f, err := os.Open(path) // #nosec G304 -- path is the source image selected by the user
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	img, err = codec.DecodeStill(f)
	return img, err
}

func encodeWebPToTemp(enc codec.WebPEncoder, img image.Image, q codec.Quality) (path string, err error) {
	f, err := os.CreateTemp("", "cloudinary-cli-*.webp")
	if err != nil {
		return "", err
	}
	path = f.Name()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			removeTemp(path)
			path = ""
		}
	}()
	err = enc.EncodeWebP(f, img, q)
	return path, err
}

func encodeAVIFToTemp(enc codec.AVIFEncoder, img image.Image, q codec.Quality) (path string, err error) {
	f, err := os.CreateTemp("", "cloudinary-cli-*.avif")
	if err != nil {
		return "", err
	}
	path = f.Name()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			removeTemp(path)
			path = ""
		}
	}()
	err = enc.EncodeAVIF(f, img, q)
	return path, err
}

func removeTemp(path string) {
	if path == "" {
		return
	}
	// Best-effort cleanup; errors (including not-exist) never fail the pipeline.
	_ = os.Remove(path)
}
