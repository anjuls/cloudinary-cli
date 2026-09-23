package upload_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/codec"
	"github.com/anjuls/cloudinary-cli/internal/upload"
)

func TestProcess_happy_path_uploads_both_variants_in_order(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := writePNG(t, tmpDir, "hero.png", generateImage(64, 64))
	up := &fakeUploader{responses: []upload.UploadResponse{
		{PublicID: "hero-webp", SecureURL: "https://cdn.example/hero-webp"},
		{PublicID: "hero-avif", SecureURL: "https://cdn.example/hero-avif"},
	}}
	result := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{}, AVIF: &fakeAVIFEncoder{}, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
	assert.Equal(t, srcPath, result.Source)
	assert.Equal(t, "hero", result.PublicIDBase)
	assert.Equal(t, upload.SourceOK, result.Status)
	require.Len(t, result.Variants, 2)
	assert.Equal(t, upload.FormatWebP, result.Variants[0].Format)
	assert.Equal(t, upload.VariantUploaded, result.Variants[0].Status)
	assert.Equal(t, "hero-webp", result.Variants[0].PublicID)
	assert.Equal(t, "https://cdn.example/hero-webp", result.Variants[0].SecureURL)
	assert.Equal(t, upload.FormatAVIF, result.Variants[1].Format)
	assert.Equal(t, upload.VariantUploaded, result.Variants[1].Status)
	assert.Equal(t, "hero-avif", result.Variants[1].PublicID)
	assert.Equal(t, "https://cdn.example/hero-avif", result.Variants[1].SecureURL)
	require.Len(t, up.uploads, 2)
	assert.True(t, strings.HasSuffix(up.uploads[0].FilePath, ".webp"))
	assert.Equal(t, "hero-webp", up.uploads[0].Req.PublicID)
	assert.Equal(t, "hero-avif", up.uploads[1].Req.PublicID)
}

func TestProcess_garbage_source(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "garbage.png")
	require.NoError(t, os.WriteFile(srcPath, []byte("not an image"), 0o644))
	result := upload.Process(context.Background(), upload.Ports{}, upload.Options{}, srcPath)
	assert.Equal(t, upload.SourceError, result.Status)
	assert.NotEmpty(t, result.Error)
	assert.Nil(t, result.Variants)
}

func TestProcess_encode_failures(t *testing.T) {
	tests := []struct {
		name                           string
		webpErr, avifErr               error
		wantWebP, wantAVIF             upload.VariantStatus
		wantWebPReason, wantAVIFReason string
		wantWebPStage, wantAVIFStage   upload.Stage
		errMsg                         string
	}{
		{"webp encode", errors.New("webp boom"), nil, upload.VariantError, upload.VariantSkipped, "", "pair aborted: webp encode failed", upload.StageEncode, "", "webp boom"},
		{"avif encode", nil, errors.New("avif boom"), upload.VariantSkipped, upload.VariantError, "pair aborted: avif encode failed", "", "", upload.StageEncode, "avif boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			srcPath := writePNG(t, tmpDir, "hero.png", generateImage(64, 64))
			up := &fakeUploader{}
			result := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{err: tt.webpErr}, AVIF: &fakeAVIFEncoder{err: tt.avifErr}, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
			assert.Equal(t, upload.SourceError, result.Status)
			require.Len(t, result.Variants, 2)
			assert.Equal(t, tt.wantWebP, result.Variants[0].Status)
			assert.Equal(t, tt.wantAVIF, result.Variants[1].Status)
			assert.Equal(t, tt.wantWebPReason, result.Variants[0].Reason)
			assert.Equal(t, tt.wantAVIFReason, result.Variants[1].Reason)
			if tt.webpErr != nil {
				assert.Equal(t, tt.errMsg, result.Variants[0].Error)
			}
			if tt.avifErr != nil {
				assert.Equal(t, tt.errMsg, result.Variants[1].Error)
			}
			assert.Empty(t, up.uploads)
		})
	}
}

func TestProcess_webp_upload_failure(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := writePNG(t, tmpDir, "hero.png", generateImage(64, 64))
	up := &fakeUploader{err: errors.New("upload boom"), errIndex: 0}
	result := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{}, AVIF: &fakeAVIFEncoder{}, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
	assert.Equal(t, upload.SourceError, result.Status)
	require.Len(t, result.Variants, 2)
	assert.Equal(t, upload.VariantError, result.Variants[0].Status)
	assert.Equal(t, upload.StageUpload, result.Variants[0].Stage)
	assert.Equal(t, "upload boom", result.Variants[0].Error)
	assert.Equal(t, upload.VariantSkipped, result.Variants[1].Status)
	assert.Equal(t, "pair aborted: webp upload failed", result.Variants[1].Reason)
	require.Len(t, up.uploads, 1)
}

func TestProcess_avif_upload_failure_after_webp_success(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := writePNG(t, tmpDir, "hero.png", generateImage(64, 64))
	up := &fakeUploader{responses: []upload.UploadResponse{{PublicID: "hero-webp", SecureURL: "https://cdn.example/hero-webp"}}, err: errors.New("avif upload boom"), errIndex: 1}
	result := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{}, AVIF: &fakeAVIFEncoder{}, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
	assert.Equal(t, upload.SourcePartial, result.Status)
	require.Len(t, result.Variants, 2)
	assert.Equal(t, upload.VariantUploaded, result.Variants[0].Status)
	assert.Equal(t, "hero-webp", result.Variants[0].PublicID)
	assert.Equal(t, upload.VariantError, result.Variants[1].Status)
	assert.Equal(t, upload.StageUpload, result.Variants[1].Stage)
	assert.Equal(t, "avif upload boom", result.Variants[1].Error)
	require.Len(t, up.uploads, 2)
}

func TestProcess_propagates_overwrite_and_folder(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := writePNG(t, tmpDir, "hero.png", generateImage(64, 64))
	up := &fakeUploader{}
	result := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{}, AVIF: &fakeAVIFEncoder{}, Up: up}, upload.Options{Folder: upload.FolderDirective{Kind: upload.FolderAsset, Path: "assets/images"}, Overwrite: true}, srcPath)
	require.Equal(t, upload.SourceOK, result.Status)
	require.Len(t, up.uploads, 2)
	assert.True(t, up.uploads[0].Req.Overwrite)
	assert.Equal(t, upload.FolderAsset, up.uploads[0].Req.Folder.Kind)
	assert.Equal(t, "assets/images", up.uploads[0].Req.Folder.Path)
	assert.True(t, up.uploads[1].Req.Overwrite)
	assert.Equal(t, upload.FolderAsset, up.uploads[1].Req.Folder.Kind)
	assert.Equal(t, "assets/images", up.uploads[1].Req.Folder.Path)
}

func TestProcess_deterministic_public_id_base(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := writePNG(t, tmpDir, "my-file.png", generateImage(10, 10))
	up := &fakeUploader{}
	r1 := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{}, AVIF: &fakeAVIFEncoder{}, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
	r2 := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{}, AVIF: &fakeAVIFEncoder{}, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
	assert.Equal(t, r1.PublicIDBase, r2.PublicIDBase)
	assert.Equal(t, "my-file", r1.PublicIDBase)
}

func TestProcess_temp_files(t *testing.T) {
	tests := []struct {
		name       string
		failAfter  int
		wantStatus upload.SourceStatus
	}{
		{"success", -1, upload.SourceOK},
		{"avif upload failure", 1, upload.SourcePartial},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			srcPath := writePNG(t, tmpDir, "hero.png", generateImage(64, 64))
			up := &checkingUploader{failAfter: tt.failAfter}
			result := upload.Process(context.Background(), upload.Ports{WebP: &fakeWebPEncoder{}, AVIF: &fakeAVIFEncoder{}, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
			require.Equal(t, tt.wantStatus, result.Status)
			assert.True(t, up.webpExisted)
			assert.True(t, up.avifExisted)
			assert.NoFileExists(t, up.webpPath)
			assert.NoFileExists(t, up.avifPath)
		})
	}
}

func TestProcessRealCodecs_produces_decodable_variants(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := writePNG(t, tmpDir, "alpha.png", generateImage(16, 16))
	encoders := codec.NewEncoders()
	up := &realCodecUploader{t: t, w: 16, h: 16}
	result := upload.Process(context.Background(), upload.Ports{WebP: encoders.WebP, AVIF: encoders.AVIF, Up: up}, upload.Options{WebPQuality: 80, AVIFQuality: 60}, srcPath)
	require.Equal(t, upload.SourceOK, result.Status)
	require.Len(t, result.Variants, 2)
	assert.Equal(t, upload.VariantUploaded, result.Variants[0].Status)
	assert.Equal(t, upload.VariantUploaded, result.Variants[1].Status)
}
