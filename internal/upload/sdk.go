package upload

import (
	"context"
	"fmt"

	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/cloudinary/cloudinary-go/v2/config"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
)

// RejectedError reports that Cloudinary rejected an upload.
type RejectedError struct {
	PublicID string
	Message  string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("upload rejected: public_id=%q message=%q", e.PublicID, e.Message)
}

// sdkUploader adapts the Cloudinary SDK to the Uploader port.
type sdkUploader struct {
	api *uploader.API
}

// NewUploaderFromConfig creates a Uploader from cfg.Config using the Cloudinary SDK.
func NewUploaderFromConfig(c cfg.Config) (Uploader, error) {
	conf, err := newSDKConfig(c, "")
	if err != nil {
		return nil, err
	}

	apiClient, err := uploader.NewWithConfiguration(conf)
	if err != nil {
		return nil, fmt.Errorf("create uploader: %w", err)
	}

	return &sdkUploader{api: apiClient}, nil
}

// newSDKConfig builds a Cloudinary SDK configuration.
// The uploadPrefix parameter is a test seam for httptest servers.
func newSDKConfig(c cfg.Config, uploadPrefix string) (*config.Configuration, error) {
	conf, err := config.NewFromParams(c.CloudName, c.APIKey, c.APISecret)
	if err != nil {
		return nil, fmt.Errorf("build cloudinary config: %w", err)
	}

	conf.URL.Analytics = false
	conf.API.UploadTimeout = 60
	if uploadPrefix != "" {
		conf.API.UploadPrefix = uploadPrefix
	}

	return conf, nil
}

// Upload uploads a file to Cloudinary.
func (u *sdkUploader) Upload(ctx context.Context, filePath string, req UploadRequest) (UploadResponse, error) {
	params := uploader.UploadParams{
		PublicID:     req.PublicID,
		ResourceType: string(api.Image),
		Overwrite:    api.Bool(req.Overwrite),
	}

	switch req.Folder.Kind {
	case FolderNone:
		// No folder fields.
	case FolderAsset:
		params.AssetFolder = req.Folder.Path
	case FolderLegacy:
		params.Folder = req.Folder.Path
	}

	res, err := u.api.Upload(ctx, filePath, params)
	if err != nil {
		return UploadResponse{}, fmt.Errorf("cloudinary upload: %w", err)
	}

	if res == nil {
		return UploadResponse{}, &RejectedError{PublicID: req.PublicID, Message: "empty response"}
	}

	if res.Error.Message != "" {
		return UploadResponse{}, &RejectedError{PublicID: req.PublicID, Message: res.Error.Message}
	}

	if res.SecureURL == "" {
		return UploadResponse{}, &RejectedError{PublicID: req.PublicID, Message: "missing secure_url"}
	}

	return UploadResponse{
		PublicID:  res.PublicID,
		SecureURL: res.SecureURL,
	}, nil
}
