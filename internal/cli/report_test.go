package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/cli"
	"github.com/anjuls/cloudinary-cli/internal/upload"
)

// goldenResults is the fixed result set pinned by the JSON golden test:
// one ok source, one partial, one failed, one skipped.
var goldenResults = []upload.SourceResult{
	{
		Source:       "hero.jpg",
		PublicIDBase: "hero",
		Status:       upload.SourceOK,
		Variants: []upload.VariantResult{
			{
				Format:    upload.FormatWebP,
				Status:    upload.VariantUploaded,
				PublicID:  "hero.webp",
				SecureURL: "https://res.cloudinary.com/demo/image/upload/hero.webp",
			},
			{
				Format:    upload.FormatAVIF,
				Status:    upload.VariantUploaded,
				PublicID:  "hero.avif",
				SecureURL: "https://res.cloudinary.com/demo/image/upload/hero.avif",
			},
		},
	},
	{
		Source:       "banner.tif",
		PublicIDBase: "banner",
		Status:       upload.SourcePartial,
		Variants: []upload.VariantResult{
			{
				Format:    upload.FormatWebP,
				Status:    upload.VariantUploaded,
				PublicID:  "banner.webp",
				SecureURL: "https://res.cloudinary.com/demo/image/upload/banner.webp",
			},
			{
				Format: upload.FormatAVIF,
				Status: upload.VariantError,
				Stage:  upload.StageEncode,
				Error:  "avif encode failed",
			},
		},
	},
	{
		Source:       "broken.bmp",
		PublicIDBase: "broken",
		Status:       upload.SourceError,
		Error:        "decode failed",
	},
	{
		Source:       "vector.svg",
		PublicIDBase: "vector",
		Status:       upload.SourceSkipped,
	},
}

// goldenJSON is the exact version-1 report envelope for goldenResults.
const goldenJSON = `{
  "version": 1,
  "sources": [
    {
      "source": "hero.jpg",
      "public_id_base": "hero",
      "status": "ok",
      "variants": [
        {
          "format": "webp",
          "status": "uploaded",
          "public_id": "hero.webp",
          "secure_url": "https://res.cloudinary.com/demo/image/upload/hero.webp"
        },
        {
          "format": "avif",
          "status": "uploaded",
          "public_id": "hero.avif",
          "secure_url": "https://res.cloudinary.com/demo/image/upload/hero.avif"
        }
      ]
    },
    {
      "source": "banner.tif",
      "public_id_base": "banner",
      "status": "partial",
      "variants": [
        {
          "format": "webp",
          "status": "uploaded",
          "public_id": "banner.webp",
          "secure_url": "https://res.cloudinary.com/demo/image/upload/banner.webp"
        },
        {
          "format": "avif",
          "status": "error",
          "stage": "encode",
          "public_id": "",
          "secure_url": "",
          "error": "avif encode failed"
        }
      ]
    },
    {
      "source": "broken.bmp",
      "public_id_base": "broken",
      "status": "error",
      "error": "decode failed",
      "variants": null
    },
    {
      "source": "vector.svg",
      "public_id_base": "vector",
      "status": "skipped",
      "variants": null
    }
  ],
  "summary": {
    "sources": 4,
    "ok": 1,
    "partial": 1,
    "failed": 1,
    "skipped": 1,
    "variants_uploaded": 3
  }
}
`

// emptyJSON is the exact envelope for a nil or empty result set: sources
// normalizes to [], never null.
const emptyJSON = `{
  "version": 1,
  "sources": [],
  "summary": {
    "sources": 0,
    "ok": 0,
    "partial": 0,
    "failed": 0,
    "skipped": 0,
    "variants_uploaded": 0
  }
}
`

// forbiddenTransforms are URL substrings the report must never
// introduce: rendering passes URLs through and never constructs them.
var forbiddenTransforms = []string{"f_auto", "f_webp", "f_avif", "q_auto", "?_a="}

// failWriter fails every Write with err, to exercise writer error paths.
type failWriter struct {
	err error
}

func (w failWriter) Write([]byte) (int, error) { return 0, w.err }

func Test_RenderJSON_emits_golden_envelope_when_fixed_results_given(t *testing.T) {
	t.Parallel()

	// When
	var buf bytes.Buffer
	err := cli.RenderJSON(&buf, goldenResults)

	// Then
	require.NoError(t, err)
	assert.Equal(t, goldenJSON, buf.String())
}

func Test_RenderJSON_normalizes_sources_to_empty_array_when_results_nil_or_empty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []upload.SourceResult
	}{
		{"nil slice", nil},
		{"empty slice", []upload.SourceResult{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// When
			var buf bytes.Buffer
			err := cli.RenderJSON(&buf, tt.results)

			// Then
			require.NoError(t, err)
			assert.Equal(t, emptyJSON, buf.String())
		})
	}
}

func Test_RenderJSON_passes_uploaded_urls_verbatim_when_variants_uploaded(t *testing.T) {
	t.Parallel()

	// When
	var buf bytes.Buffer
	require.NoError(t, cli.RenderJSON(&buf, goldenResults))
	out := buf.String()

	// Then — every uploaded URL appears verbatim...
	for _, url := range []string{
		"https://res.cloudinary.com/demo/image/upload/hero.webp",
		"https://res.cloudinary.com/demo/image/upload/hero.avif",
		"https://res.cloudinary.com/demo/image/upload/banner.webp",
	} {
		assert.Contains(t, out, url)
	}

	// ...and no auto-transformation markers are introduced.
	for _, forbidden := range forbiddenTransforms {
		assert.NotContains(t, out, forbidden)
	}
}

func Test_RenderSourceText_lists_uploaded_variants_when_source_ok(t *testing.T) {
	t.Parallel()

	// Given — hero.jpg uploaded both variants.
	result := goldenResults[0]

	// When
	var buf bytes.Buffer
	err := cli.RenderSourceText(&buf, result)

	// Then
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "hero.jpg: ok")
	assert.Contains(t, out, "hero.webp")
	assert.Contains(t, out, "https://res.cloudinary.com/demo/image/upload/hero.webp")
	assert.Contains(t, out, "hero.avif")
	assert.Contains(t, out, "https://res.cloudinary.com/demo/image/upload/hero.avif")
	for _, forbidden := range forbiddenTransforms {
		assert.NotContains(t, out, forbidden)
	}
}

func Test_RenderSourceText_reports_variant_error_when_source_partial(t *testing.T) {
	t.Parallel()

	// Given — banner.tif uploaded webp; avif failed at encode.
	result := goldenResults[1]

	// When
	var buf bytes.Buffer
	err := cli.RenderSourceText(&buf, result)

	// Then
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "banner.tif: partial")
	assert.Contains(t, out, "banner.webp")
	assert.Contains(t, out, "https://res.cloudinary.com/demo/image/upload/banner.webp")
	assert.Contains(t, out, "encode: avif encode failed")
}

func Test_RenderSourceText_reports_source_error_when_source_failed(t *testing.T) {
	t.Parallel()

	// Given — broken.bmp failed before any variant uploaded.
	result := goldenResults[2]

	// When
	var buf bytes.Buffer
	err := cli.RenderSourceText(&buf, result)

	// Then
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "broken.bmp: error")
	assert.Contains(t, out, "decode failed")
}

func Test_RenderSourceText_reports_skip_reason_when_variant_skipped(t *testing.T) {
	t.Parallel()

	// Given — icon.png uploaded webp; avif was skipped with a reason.
	result := upload.SourceResult{
		Source:       "icon.png",
		PublicIDBase: "icon",
		Status:       upload.SourceOK,
		Variants: []upload.VariantResult{
			{
				Format:    upload.FormatWebP,
				Status:    upload.VariantUploaded,
				PublicID:  "icon.webp",
				SecureURL: "https://res.cloudinary.com/demo/image/upload/icon.webp",
			},
			{
				Format: upload.FormatAVIF,
				Status: upload.VariantSkipped,
				Reason: "avif disabled",
			},
		},
	}

	// When
	var buf bytes.Buffer
	err := cli.RenderSourceText(&buf, result)

	// Then
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "icon.png: ok")
	assert.Contains(t, out, "avif skipped avif disabled")
}

func Test_RenderSourceText_reports_skipped_status_when_source_skipped(t *testing.T) {
	t.Parallel()

	// Given — vector.svg was skipped entirely.
	result := goldenResults[3]

	// When
	var buf bytes.Buffer
	err := cli.RenderSourceText(&buf, result)

	// Then
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "vector.svg: skipped")
}

func Test_RenderSummaryText_reports_counts_when_summary_given(t *testing.T) {
	t.Parallel()

	// Given — the summary rolled up from the golden result set.
	summary := upload.Summarize(goldenResults)

	// When
	var buf bytes.Buffer
	err := cli.RenderSummaryText(&buf, summary)

	// Then — one final line carrying every count.
	require.NoError(t, err)
	out := buf.String()
	for _, want := range []string{
		"4 sources", "1 ok", "1 partial", "1 failed", "1 skipped", "3 variants uploaded",
	} {
		assert.Contains(t, out, want)
	}
	assert.Equal(t, 1, strings.Count(out, "\n"))
}

func Test_render_functions_wrap_writer_errors_when_write_fails(t *testing.T) {
	t.Parallel()

	// Given — a writer that fails every write.
	boom := errors.New("writer boom")
	w := failWriter{err: boom}

	tests := []struct {
		name   string
		render func() error
	}{
		{"RenderJSON", func() error { return cli.RenderJSON(w, goldenResults) }},
		{"RenderSourceText", func() error { return cli.RenderSourceText(w, goldenResults[0]) }},
		{"RenderSummaryText", func() error { return cli.RenderSummaryText(w, upload.Summarize(goldenResults)) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := tt.render()

			// Then — the writer error survives, wrapped with cli context.
			require.ErrorIs(t, err, boom)
			assert.Contains(t, err.Error(), "cli:")
		})
	}
}
