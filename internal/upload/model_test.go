package upload_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/upload"
)

// unmarshalKeys decodes a marshaled JSON object into its key set so tests can
// assert key presence and omission directly.
func unmarshalKeys(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	keys := make(map[string]json.RawMessage)
	require.NoError(t, json.Unmarshal(data, &keys))
	return keys
}

func Test_enum_wire_values_match_contract(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"FormatWebP is webp", string(upload.FormatWebP), "webp"},
		{"FormatAVIF is avif", string(upload.FormatAVIF), "avif"},
		{"SourceOK is ok", string(upload.SourceOK), "ok"},
		{"SourcePartial is partial", string(upload.SourcePartial), "partial"},
		{"SourceError is error", string(upload.SourceError), "error"},
		{"SourceSkipped is skipped", string(upload.SourceSkipped), "skipped"},
		{"VariantUploaded is uploaded", string(upload.VariantUploaded), "uploaded"},
		{"VariantError is error", string(upload.VariantError), "error"},
		{"VariantSkipped is skipped", string(upload.VariantSkipped), "skipped"},
		{"StageEncode is encode", string(upload.StageEncode), "encode"},
		{"StageUpload is upload", string(upload.StageUpload), "upload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

func Test_Summarize_counts_exact_totals_when_results_mixed(t *testing.T) {
	// Given — five sources: two ok, one partial, one error, one skipped;
	// six variants: four uploaded, one error, one skipped.
	results := []upload.SourceResult{
		{
			Source:       "hero.jpg",
			PublicIDBase: "hero",
			Status:       upload.SourceOK,
			Variants: []upload.VariantResult{
				{Format: upload.FormatWebP, Status: upload.VariantUploaded, PublicID: "hero.webp"},
				{Format: upload.FormatAVIF, Status: upload.VariantUploaded, PublicID: "hero.avif"},
			},
		},
		{
			Source:       "icon.png",
			PublicIDBase: "icon",
			Status:       upload.SourceOK,
			Variants: []upload.VariantResult{
				{Format: upload.FormatWebP, Status: upload.VariantUploaded, PublicID: "icon.webp"},
				{Format: upload.FormatAVIF, Status: upload.VariantSkipped, Reason: "avif disabled"},
			},
		},
		{
			Source:       "banner.tif",
			PublicIDBase: "banner",
			Status:       upload.SourcePartial,
			Variants: []upload.VariantResult{
				{Format: upload.FormatWebP, Status: upload.VariantUploaded, PublicID: "banner.webp"},
				{Format: upload.FormatAVIF, Status: upload.VariantError, Stage: upload.StageEncode, Error: "avif encode failed"},
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

	// When
	got := upload.Summarize(results)

	// Then
	want := upload.Summary{Sources: 5, OK: 2, Partial: 1, Failed: 1, Skipped: 1, VariantsUploaded: 4}
	require.Equal(t, want, got)
}

func Test_Summarize_returns_zero_counts_when_no_results(t *testing.T) {
	tests := []struct {
		name    string
		results []upload.SourceResult
	}{
		{"nil slice", nil},
		{"empty slice", []upload.SourceResult{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := upload.Summarize(tt.results)

			// Then
			assert.Equal(t, upload.Summary{}, got)
		})
	}
}

func Test_AnyFailed_truth_table(t *testing.T) {
	tests := []struct {
		name string
		s    upload.Summary
		want bool
	}{
		{"false when zero summary", upload.Summary{}, false},
		{"false when only ok", upload.Summary{Sources: 3, OK: 3}, false},
		{"false when only skipped", upload.Summary{Sources: 2, Skipped: 2}, false},
		{"false when variants uploaded without source failures", upload.Summary{Sources: 1, OK: 1, VariantsUploaded: 2}, false},
		{"true when partial only", upload.Summary{Sources: 1, Partial: 1}, true},
		{"true when failed only", upload.Summary{Sources: 1, Failed: 1}, true},
		{"true when partial and failed", upload.Summary{Sources: 2, Partial: 1, Failed: 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, upload.AnyFailed(tt.s))
		})
	}
}

func Test_VariantResult_JSON_key_presence_and_omitempty(t *testing.T) {
	// Given — the three variant outcomes: uploaded, error at a stage, skipped.
	uploaded := upload.VariantResult{
		Format:    upload.FormatWebP,
		Status:    upload.VariantUploaded,
		PublicID:  "hero.webp",
		SecureURL: "https://res.cloudinary.com/demo/hero.webp",
	}
	encodeFailed := upload.VariantResult{
		Format: upload.FormatAVIF,
		Status: upload.VariantError,
		Stage:  upload.StageEncode,
		Error:  "avif encode failed",
	}
	skipped := upload.VariantResult{
		Format: upload.FormatAVIF,
		Status: upload.VariantSkipped,
		Reason: "avif disabled",
	}
	tests := []struct {
		name        string
		variant     upload.VariantResult
		wantPresent []string
		wantAbsent  []string
	}{
		{"uploaded omits empty optional fields", uploaded, []string{"format", "status", "public_id", "secure_url"}, []string{"stage", "error", "reason"}},
		{"error at stage keeps required keys even when empty", encodeFailed, []string{"format", "status", "stage", "error", "public_id", "secure_url"}, []string{"reason"}},
		{"skipped includes reason", skipped, []string{"format", "status", "reason", "public_id", "secure_url"}, []string{"stage", "error"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			data, err := json.Marshal(tt.variant)
			require.NoError(t, err)
			keys := unmarshalKeys(t, data)

			// Then
			for _, key := range tt.wantPresent {
				assert.Contains(t, keys, key)
			}
			for _, key := range tt.wantAbsent {
				assert.NotContains(t, keys, key)
			}
		})
	}
}

func Test_SourceResult_JSON_keys_when_ok_with_variants(t *testing.T) {
	// Given — a successful source with one uploaded variant.
	result := upload.SourceResult{
		Source:       "hero.jpg",
		PublicIDBase: "hero",
		Status:       upload.SourceOK,
		Variants: []upload.VariantResult{
			{Format: upload.FormatWebP, Status: upload.VariantUploaded, PublicID: "hero.webp"},
		},
	}

	// When
	data, err := json.Marshal(result)
	require.NoError(t, err)
	keys := unmarshalKeys(t, data)

	// Then — error is omitted when empty; every other key is present.
	for _, key := range []string{"source", "public_id_base", "status", "variants"} {
		assert.Contains(t, keys, key)
	}
	assert.NotContains(t, keys, "error")
}

func Test_SourceResult_JSON_keeps_variants_key_when_nil_or_empty(t *testing.T) {
	tests := []struct {
		name     string
		variants []upload.VariantResult
		wantRaw  string
	}{
		{"nil variants render as null", nil, "null"},
		{"empty variants render as []", []upload.VariantResult{}, "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given — a failed source with no variant results.
			result := upload.SourceResult{
				Source:       "broken.bmp",
				PublicIDBase: "broken",
				Status:       upload.SourceError,
				Error:        "decode failed",
				Variants:     tt.variants,
			}

			// When
			data, err := json.Marshal(result)
			require.NoError(t, err)
			keys := unmarshalKeys(t, data)

			// Then — the variants key stays present with the chosen rendering.
			assert.Contains(t, keys, "variants")
			assert.Equal(t, tt.wantRaw, string(keys["variants"]))
			assert.Contains(t, keys, "error")
		})
	}
}

func Test_SourceResult_JSON_preserves_webp_then_avif_order(t *testing.T) {
	// Given — variants supplied in pipeline order: WebP first, then AVIF.
	result := upload.SourceResult{
		Source:       "hero.jpg",
		PublicIDBase: "hero",
		Status:       upload.SourceOK,
		Variants: []upload.VariantResult{
			{Format: upload.FormatWebP, Status: upload.VariantUploaded, PublicID: "hero.webp"},
			{Format: upload.FormatAVIF, Status: upload.VariantUploaded, PublicID: "hero.avif"},
		},
	}

	// When
	data, err := json.Marshal(result)
	require.NoError(t, err)

	// Then — WebP precedes AVIF in the marshaled array...
	webpAt := bytes.Index(data, []byte(`"webp"`))
	avifAt := bytes.Index(data, []byte(`"avif"`))
	require.GreaterOrEqual(t, webpAt, 0)
	require.Greater(t, avifAt, webpAt)

	// ...and the order survives a JSON round-trip.
	var back upload.SourceResult
	require.NoError(t, json.Unmarshal(data, &back))
	require.Len(t, back.Variants, 2)
	assert.Equal(t, upload.FormatWebP, back.Variants[0].Format)
	assert.Equal(t, upload.FormatAVIF, back.Variants[1].Format)
}

func Test_Summary_JSON_keys_present(t *testing.T) {
	// Given
	summary := upload.Summary{Sources: 5, OK: 2, Partial: 1, Failed: 1, Skipped: 1, VariantsUploaded: 4}

	// When
	data, err := json.Marshal(summary)
	require.NoError(t, err)
	keys := unmarshalKeys(t, data)

	// Then — every summary field is always present.
	for _, key := range []string{"sources", "ok", "partial", "failed", "skipped", "variants_uploaded"} {
		assert.Contains(t, keys, key)
	}
}
