package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
	"github.com/anjuls/cloudinary-cli/internal/codec"
	"github.com/anjuls/cloudinary-cli/internal/prompt"
	"github.com/anjuls/cloudinary-cli/internal/upload"
)

// --- fakes ---

type recordingUploader struct {
	uploads []struct {
		FilePath string
		Req      upload.UploadRequest
	}
}

func (r *recordingUploader) Upload(_ context.Context, filePath string, req upload.UploadRequest) (upload.UploadResponse, error) {
	r.uploads = append(r.uploads, struct {
		FilePath string
		Req      upload.UploadRequest
	}{FilePath: filePath, Req: req})
	return upload.UploadResponse{PublicID: req.PublicID, SecureURL: "https://cdn.example/" + req.PublicID}, nil
}

type recordingWebPEncoder struct {
	qualities []codec.Quality
}

func (r *recordingWebPEncoder) EncodeWebP(w io.Writer, _ image.Image, q codec.Quality) error {
	r.qualities = append(r.qualities, q)
	_, err := w.Write([]byte("fake-webp"))
	return err
}

type recordingAVIFEncoder struct {
	qualities []codec.Quality
}

func (r *recordingAVIFEncoder) EncodeAVIF(w io.Writer, _ image.Image, q codec.Quality) error {
	r.qualities = append(r.qualities, q)
	_, err := w.Write([]byte("fake-avif"))
	return err
}

type scriptedPrompter struct {
	values []string
	fields []prompt.Field
}

func (s *scriptedPrompter) Ask(_ context.Context, _ string, fields []prompt.Field) ([]string, error) {
	s.fields = fields
	return s.values, nil
}

// --- helpers ---

func writePNG(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())
	return p
}

func writeConfig(t *testing.T, c cfg.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, cfg.Save(path, c))
	return path
}

func newUploadTestApp(stdout, stderr io.Writer, uploader upload.Uploader, encoders func() (codec.WebPEncoder, codec.AVIFEncoder), prompter prompt.Prompter) *App {
	if prompter == nil {
		prompter = fakePrompter{}
	}
	a := &App{
		Stdout:      stdout,
		Stderr:      stderr,
		Prompt:      prompter,
		NewUploader: func(cfg.Config) (upload.Uploader, error) { return uploader, nil },
		NewEncoders: encoders,
	}
	a.addCommand(newUploadCmd)
	return a
}

// --- tests ---

func Test_upload_happy_path_two_pngs_in_dir(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	writePNG(t, dir, "b.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, dir})

	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), "a.png: ok")
	require.Contains(t, stdout.String(), "b.png: ok")
	require.Contains(t, stdout.String(), "2 sources: 2 ok, 0 partial, 0 failed, 0 skipped; 4 variants uploaded")
	require.Len(t, uploader.uploads, 4)

	require.Equal(t, "a-webp", uploader.uploads[0].Req.PublicID)
	require.Equal(t, "a-avif", uploader.uploads[1].Req.PublicID)
	require.Equal(t, "b-webp", uploader.uploads[2].Req.PublicID)
	require.Equal(t, "b-avif", uploader.uploads[3].Req.PublicID)
}

func Test_upload_quality_90(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	webpEnc := &recordingWebPEncoder{}
	avifEnc := &recordingAVIFEncoder{}
	app := newUploadTestApp(&stdout, &stderr, &recordingUploader{}, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return webpEnc, avifEnc
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--quality", "90", dir})

	require.Equal(t, 0, code)
	require.Len(t, webpEnc.qualities, 1)
	require.Len(t, avifEnc.qualities, 1)
	require.Equal(t, codec.Quality(90), webpEnc.qualities[0])
	require.Equal(t, codec.Quality(68), avifEnc.qualities[0])
}

func Test_upload_quality_invalid(t *testing.T) {
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	tests := []struct {
		name    string
		quality string
	}{
		{"quality 0", "0"},
		{"quality 101", "101"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			uploader := &recordingUploader{}
			app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
				return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
			}, nil)

			code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--quality", tt.quality, t.TempDir()})

			require.Equal(t, 2, code)
			require.Len(t, uploader.uploads, 0)
		})
	}
}

func Test_upload_dynamic_folder_nested_recursive(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "b.png")
	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	writePNG(t, subDir, "a.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--recursive", dir})

	require.Equal(t, 0, code)
	require.Len(t, uploader.uploads, 4)

	require.Equal(t, "b-webp", uploader.uploads[0].Req.PublicID)
	require.Equal(t, upload.FolderNone, uploader.uploads[0].Req.Folder.Kind)
	require.Equal(t, "", uploader.uploads[0].Req.Folder.Path)

	require.Equal(t, "b-avif", uploader.uploads[1].Req.PublicID)
	require.Equal(t, upload.FolderNone, uploader.uploads[1].Req.Folder.Kind)
	require.Equal(t, "", uploader.uploads[1].Req.Folder.Path)

	require.Equal(t, "a-webp", uploader.uploads[2].Req.PublicID)
	require.Equal(t, upload.FolderAsset, uploader.uploads[2].Req.Folder.Kind)
	require.Equal(t, "sub", uploader.uploads[2].Req.Folder.Path)

	require.Equal(t, "a-avif", uploader.uploads[3].Req.PublicID)
	require.Equal(t, upload.FolderAsset, uploader.uploads[3].Req.Folder.Kind)
	require.Equal(t, "sub", uploader.uploads[3].Req.Folder.Path)
}

func Test_upload_fixed_folder_mode(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	t.Run("without folder flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		app := newUploadTestApp(&stdout, &stderr, &recordingUploader{}, func() (codec.WebPEncoder, codec.AVIFEncoder) {
			return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
		}, nil)

		code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--folder-mode", "fixed", dir})

		require.Equal(t, 2, code)
	})

	t.Run("with folder flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		uploader := &recordingUploader{}
		app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
			return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
		}, nil)

		code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--folder-mode", "fixed", "--folder", "pics", dir})

		require.Equal(t, 0, code)
		require.Len(t, uploader.uploads, 2)
		for _, u := range uploader.uploads {
			require.Equal(t, upload.FolderLegacy, u.Req.Folder.Kind)
			require.Equal(t, "pics", u.Req.Folder.Path)
		}
	})
}

func Test_upload_overwrite(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--overwrite", dir})

	require.Equal(t, 0, code)
	require.Len(t, uploader.uploads, 2)
	for _, u := range uploader.uploads {
		require.True(t, u.Req.Overwrite)
	}
}

func Test_upload_unsupported_txt_in_dir(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("text"), 0o644))
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, dir})

	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), "b.txt: skipped")
	require.Len(t, uploader.uploads, 2) // only a.png
}

func Test_upload_corrupt_png_and_valid_png(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "valid.png")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corrupt.png"), []byte("not a png"), 0o644))
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, dir})

	require.Equal(t, 1, code)
	require.Contains(t, stdout.String(), "corrupt.png: error")
	require.Contains(t, stdout.String(), "valid.png: ok")
	require.Contains(t, stdout.String(), "2 sources: 1 ok, 0 partial, 1 failed, 0 skipped; 2 variants uploaded")
}

func Test_upload_output_json(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--output", "json", dir})

	require.Equal(t, 0, code)

	var envelope reportEnvelope
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
	require.Equal(t, 1, envelope.Version)
	require.Len(t, envelope.Sources, 1)
	require.Contains(t, envelope.Sources[0].Source, "a.png")
	require.Equal(t, 2, envelope.Summary.VariantsUploaded)

	for _, src := range envelope.Sources {
		for _, v := range src.Variants {
			require.NotContains(t, v.SecureURL, "f_auto")
			require.NotContains(t, v.SecureURL, "f_webp")
			require.NotContains(t, v.SecureURL, "f_avif")
			require.NotContains(t, v.SecureURL, "q_auto")
			require.NotContains(t, v.SecureURL, "?_a=")
		}
	}
}

func Test_upload_recursive(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "top.png")
	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	writePNG(t, subDir, "nested.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	t.Run("non-recursive ignores nested", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		uploader := &recordingUploader{}
		app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
			return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
		}, nil)

		code := app.Execute(context.Background(), []string{"upload", "--config", configPath, dir})

		require.Equal(t, 0, code)
		require.Len(t, uploader.uploads, 2) // only top.png
	})

	t.Run("recursive includes nested", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		uploader := &recordingUploader{}
		app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
			return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
		}, nil)

		code := app.Execute(context.Background(), []string{"upload", "--config", configPath, "--recursive", dir})

		require.Equal(t, 0, code)
		require.Len(t, uploader.uploads, 4) // top.png + nested.png
	})
}

func Test_upload_symlink_in_dir(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	require.NoError(t, os.Symlink(filepath.Join(dir, "a.png"), filepath.Join(dir, "link.png")))
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, nil)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, dir})

	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), "link.png: skipped")
	require.Len(t, uploader.uploads, 2) // only a.png
}

func Test_upload_missing_credentials_prompt(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	configPath := writeConfig(t, cfg.Config{})

	prompter := &scriptedPrompter{values: []string{"demo", "key", "secret"}}
	var stdout, stderr bytes.Buffer
	uploader := &recordingUploader{}
	app := newUploadTestApp(&stdout, &stderr, uploader, func() (codec.WebPEncoder, codec.AVIFEncoder) {
		return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
	}, prompter)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, dir})

	require.Equal(t, 0, code)
	require.Len(t, uploader.uploads, 2)

	var hasSecret bool
	for _, f := range prompter.fields {
		if f.Label == "API secret" {
			require.True(t, f.Secret)
			hasSecret = true
		}
	}
	require.True(t, hasSecret)
}

func Test_upload_new_uploader_error(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "a.png")
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	var stdout, stderr bytes.Buffer
	app := &App{
		Stdout:      &stdout,
		Stderr:      &stderr,
		Prompt:      fakePrompter{},
		NewUploader: func(cfg.Config) (upload.Uploader, error) { return nil, errors.New("boom") },
		NewEncoders: func() (codec.WebPEncoder, codec.AVIFEncoder) {
			return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
		},
	}
	app.addCommand(newUploadCmd)

	code := app.Execute(context.Background(), []string{"upload", "--config", configPath, dir})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "boom")
}

func Test_upload_args_count(t *testing.T) {
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	tests := []struct {
		name string
		args []string
	}{
		{"zero args", []string{"upload", "--config", configPath}},
		{"two args", []string{"upload", "--config", configPath, "a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := newUploadTestApp(&stdout, &stderr, &recordingUploader{}, func() (codec.WebPEncoder, codec.AVIFEncoder) {
				return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
			}, nil)

			code := app.Execute(context.Background(), tt.args)

			require.Equal(t, 2, code)
		})
	}
}

func Test_upload_bad_flag_values(t *testing.T) {
	configPath := writeConfig(t, cfg.Config{CloudName: "demo", APIKey: "key", APISecret: "secret"})

	tests := []struct {
		name string
		args []string
	}{
		{"bad folder-mode", []string{"upload", "--config", configPath, "--folder-mode", "foo", "dir"}},
		{"bad output", []string{"upload", "--config", configPath, "--output", "xml", "dir"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := newUploadTestApp(&stdout, &stderr, &recordingUploader{}, func() (codec.WebPEncoder, codec.AVIFEncoder) {
				return &recordingWebPEncoder{}, &recordingAVIFEncoder{}
			}, nil)

			code := app.Execute(context.Background(), tt.args)

			require.Equal(t, 2, code)
		})
	}
}
