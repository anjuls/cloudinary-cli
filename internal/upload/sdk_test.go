package upload

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
)

// capturedUpload holds data extracted from an upload request inside the handler.
type capturedUpload struct {
	Path   string
	Values url.Values
	Files  map[string][]*multipart.FileHeader
}

func newTestUploader(t *testing.T, srv *httptest.Server) Uploader {
	t.Helper()
	conf, err := newSDKConfig(cfg.Config{
		CloudName: "testcloud",
		APIKey:    "testkey",
		APISecret: "testsecret",
	}, srv.URL)
	require.NoError(t, err)
	apiClient, err := uploader.NewWithConfiguration(conf)
	require.NoError(t, err)
	return &sdkUploader{api: apiClient}
}

func writeTempFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return p
}

func verifySignature(t *testing.T, values url.Values, secret string) {
	t.Helper()
	sigParams := make(url.Values)
	for k, v := range values {
		switch k {
		case "api_key", "signature", "resource_type", "cloud_name":
			// omit
		default:
			sigParams[k] = v
		}
	}
	computedSig, err := api.SignParameters(sigParams, secret)
	require.NoError(t, err)
	require.Equal(t, values.Get("signature"), computedSig)
}

func TestSDK_Upload_posts_to_expected_endpoint_and_returns_response_verbatim(t *testing.T) {
	// Given — an httptest server that captures the request and returns a fixed response.
	var got capturedUpload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)
		got.Values = url.Values(r.MultipartForm.Value)
		got.Files = r.MultipartForm.File

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"public_id":  "cat-webp",
			"secure_url": "https://res.cloudinary.com/testcloud/image/upload/cat-webp.webp",
		})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	filePath := writeTempFile(t, tmpDir, "test.webp", []byte("fake image data"))

	u := newTestUploader(t, srv)

	// When
	res, err := u.Upload(context.Background(), filePath, UploadRequest{
		PublicID:  "cat-webp",
		Overwrite: false,
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, "cat-webp", res.PublicID)
	require.Equal(t, "https://res.cloudinary.com/testcloud/image/upload/cat-webp.webp", res.SecureURL)

	require.Equal(t, "/v1_1/testcloud/auto/upload", got.Path)
	require.Equal(t, "image", got.Values.Get("resource_type"))
	require.Equal(t, "cat-webp", got.Values.Get("public_id"))
	require.Equal(t, "false", got.Values.Get("overwrite"))
	require.Equal(t, "testkey", got.Values.Get("api_key"))
	require.NotEmpty(t, got.Values.Get("timestamp"))
	require.NotEmpty(t, got.Values.Get("signature"))

	fileHeaders := got.Files["file"]
	require.Len(t, fileHeaders, 1)
	fh := fileHeaders[0]
	require.True(t, strings.HasSuffix(fh.Filename, ".webp"))
	f, err := fh.Open()
	require.NoError(t, err)
	defer f.Close()
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Equal(t, []byte("fake image data"), content)

	verifySignature(t, got.Values, "testsecret")
}

func TestSDK_Upload_overwrite_true(t *testing.T) {
	// Given
	var got capturedUpload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)
		got.Values = url.Values(r.MultipartForm.Value)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"public_id":  "cat-webp",
			"secure_url": "https://res.cloudinary.com/testcloud/image/upload/cat-webp.webp",
		})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	filePath := writeTempFile(t, tmpDir, "test.webp", []byte("x"))

	u := newTestUploader(t, srv)

	// When
	_, err := u.Upload(context.Background(), filePath, UploadRequest{
		PublicID:  "cat-webp",
		Overwrite: true,
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, "/v1_1/testcloud/auto/upload", got.Path)
	require.Equal(t, "true", got.Values.Get("overwrite"))
}

func TestSDK_Upload_folder_directives(t *testing.T) {
	tests := []struct {
		name        string
		folder      FolderDirective
		wantKey     string
		wantValue   string
		wantAbsent1 string
		wantAbsent2 string
	}{
		{
			name:        "FolderNone emits neither",
			folder:      FolderDirective{Kind: FolderNone},
			wantAbsent1: "folder",
			wantAbsent2: "asset_folder",
		},
		{
			name:        "FolderAsset emits only asset_folder",
			folder:      FolderDirective{Kind: FolderAsset, Path: "assets/images"},
			wantKey:     "asset_folder",
			wantValue:   "assets/images",
			wantAbsent1: "folder",
		},
		{
			name:        "FolderLegacy emits only folder",
			folder:      FolderDirective{Kind: FolderLegacy, Path: "legacy/path"},
			wantKey:     "folder",
			wantValue:   "legacy/path",
			wantAbsent1: "asset_folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			var got capturedUpload
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got.Path = r.URL.Path
				err := r.ParseMultipartForm(10 << 20)
				require.NoError(t, err)
				got.Values = url.Values(r.MultipartForm.Value)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"public_id":  "cat-webp",
					"secure_url": "https://res.cloudinary.com/testcloud/image/upload/cat-webp.webp",
				})
			}))
			defer srv.Close()

			tmpDir := t.TempDir()
			filePath := writeTempFile(t, tmpDir, "test.webp", []byte("x"))

			u := newTestUploader(t, srv)

			// When
			_, err := u.Upload(context.Background(), filePath, UploadRequest{
				PublicID:  "cat-webp",
				Folder:    tt.folder,
				Overwrite: false,
			})

			// Then
			require.NoError(t, err)
			require.Equal(t, "/v1_1/testcloud/auto/upload", got.Path)
			if tt.wantKey != "" {
				require.Equal(t, tt.wantValue, got.Values.Get(tt.wantKey))
			}
			require.Empty(t, got.Values.Get(tt.wantAbsent1), "expected %s to be absent", tt.wantAbsent1)
			require.Empty(t, got.Values.Get(tt.wantAbsent2), "expected %s to be absent", tt.wantAbsent2)
		})
	}
}

func TestSDK_Upload_rejected_error_on_api_error_message(t *testing.T) {
	// Given — server returns 400 with an error message in the JSON body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Resource already exists",
			},
		})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	filePath := writeTempFile(t, tmpDir, "test.webp", []byte("x"))

	u := newTestUploader(t, srv)

	// When
	_, err := u.Upload(context.Background(), filePath, UploadRequest{
		PublicID: "cat-webp",
	})

	// Then
	var rejected *RejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, "cat-webp", rejected.PublicID)
	require.Equal(t, "Resource already exists", rejected.Message)
}

func TestSDK_Upload_rejected_error_on_empty_secure_url(t *testing.T) {
	// Given — server returns 200 but with an empty secure_url.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"public_id":  "cat-webp",
			"secure_url": "",
		})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	filePath := writeTempFile(t, tmpDir, "test.webp", []byte("x"))

	u := newTestUploader(t, srv)

	// When
	_, err := u.Upload(context.Background(), filePath, UploadRequest{
		PublicID: "cat-webp",
	})

	// Then
	var rejected *RejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, "cat-webp", rejected.PublicID)
}

func TestSDK_Upload_transport_error_not_rejected_error(t *testing.T) {
	// Given — a closed server so the request fails at the transport layer.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	tmpDir := t.TempDir()
	filePath := writeTempFile(t, tmpDir, "test.webp", []byte("x"))

	u := newTestUploader(t, srv)

	// When
	_, err := u.Upload(context.Background(), filePath, UploadRequest{
		PublicID: "cat-webp",
	})

	// Then
	require.Error(t, err)
	var rejected *RejectedError
	require.False(t, errors.As(err, &rejected), "expected transport error, not RejectedError")
}
