// Package upload holds the pure result and domain model for the upload
// pipeline: typed statuses, folder directives, per-source and per-variant
// results, run summaries, and the narrow Uploader port implemented by the
// Cloudinary SDK adapter. The package deliberately contains no processing,
// encoding, or SDK logic.
package upload

import "context"

// VariantFormat identifies one encoded image variant produced per source.
type VariantFormat string

// Variant formats, listed in pipeline processing order.
const (
	FormatWebP VariantFormat = "webp"
	FormatAVIF VariantFormat = "avif"
)

// SourceStatus rolls up the outcome of one source file after the pipeline ran.
type SourceStatus string

// Source statuses. Partial means at least one variant uploaded and at least
// one failed; error means the source itself failed before any variant
// uploaded.
const (
	SourceOK      SourceStatus = "ok"
	SourcePartial SourceStatus = "partial"
	SourceError   SourceStatus = "error"
	SourceSkipped SourceStatus = "skipped"
)

// VariantStatus is the outcome of a single variant of one source.
type VariantStatus string

// Variant statuses.
const (
	VariantUploaded VariantStatus = "uploaded"
	VariantError    VariantStatus = "error"
	VariantSkipped  VariantStatus = "skipped"
)

// Stage names the pipeline step at which a variant error occurred.
type Stage string

// Pipeline stages a variant can fail at.
const (
	StageEncode Stage = "encode"
	StageUpload Stage = "upload"
)

// FolderKind selects how an upload is placed into Cloudinary folders.
type FolderKind string

// Folder kinds. FolderNone is the zero value of FolderKind: no folder is
// applied, so a zero FolderDirective is valid and means "no folder".
const (
	FolderNone   FolderKind = ""
	FolderAsset  FolderKind = "asset"  // dynamic asset folders
	FolderLegacy FolderKind = "legacy" // fixed folder path
)

// FolderDirective tells the uploader how to place one asset into folders.
type FolderDirective struct {
	// Kind selects the folder mode; FolderNone means no folder is applied.
	Kind FolderKind
	// Path is the folder path associated with Kind, empty for FolderNone.
	Path string
}

// VariantResult reports the outcome of one encoded variant of a source.
// Stage, Error, and Reason are omitted from JSON when empty; PublicID and
// SecureURL are always present, empty for variants that were not uploaded.
type VariantResult struct {
	Format    VariantFormat `json:"format"`
	Status    VariantStatus `json:"status"`
	Stage     Stage         `json:"stage,omitempty"`
	PublicID  string        `json:"public_id"`
	SecureURL string        `json:"secure_url"`
	Error     string        `json:"error,omitempty"`
	Reason    string        `json:"reason,omitempty"`
}

// SourceResult reports the outcome of one source file. The variants key is
// always present in JSON: a nil slice renders as null, an empty slice as [].
type SourceResult struct {
	Source       string          `json:"source"`
	PublicIDBase string          `json:"public_id_base"`
	Status       SourceStatus    `json:"status"`
	Error        string          `json:"error,omitempty"`
	Variants     []VariantResult `json:"variants"`
}

// Summary aggregates one pipeline run over a set of source results.
type Summary struct {
	Sources          int `json:"sources"`
	OK               int `json:"ok"`
	Partial          int `json:"partial"`
	Failed           int `json:"failed"`
	Skipped          int `json:"skipped"`
	VariantsUploaded int `json:"variants_uploaded"`
}

// Summarize rolls up per-source results into aggregate counts. Sources counts
// every result, including skipped and failed ones; VariantsUploaded counts
// every variant with status uploaded, including variants of partial sources.
func Summarize(results []SourceResult) Summary {
	summary := Summary{Sources: len(results)}
	for _, result := range results {
		switch result.Status {
		case SourceOK:
			summary.OK++
		case SourcePartial:
			summary.Partial++
		case SourceError:
			summary.Failed++
		case SourceSkipped:
			summary.Skipped++
		}
		for _, variant := range result.Variants {
			if variant.Status == VariantUploaded {
				summary.VariantsUploaded++
			}
		}
	}
	return summary
}

// AnyFailed reports whether any source ended partial or failed, i.e. whether
// the run as a whole needs attention.
func AnyFailed(summary Summary) bool {
	return summary.Partial > 0 || summary.Failed > 0
}

// UploadRequest describes one asset upload: the desired public ID, the
// folder directive, and whether an existing asset with the same public ID
// may be replaced.
//
//nolint:revive // name mirrors the Cloudinary SDK's UploadParams domain term.
type UploadRequest struct {
	PublicID  string
	Folder    FolderDirective
	Overwrite bool
}

// UploadResponse is the successful outcome of one upload.
//
//nolint:revive // name mirrors the Cloudinary SDK's UploadResult domain term.
type UploadResponse struct {
	PublicID  string
	SecureURL string
}

// Uploader is the narrow port the pipeline uses to upload one encoded file.
// The Cloudinary SDK adapter implements it; filePath is a local file path.
type Uploader interface {
	Upload(ctx context.Context, filePath string, req UploadRequest) (UploadResponse, error)
}
