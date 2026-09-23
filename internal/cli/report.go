// report.go renders deterministic upload-run reports: a stable, versioned
// JSON envelope for machine consumption and concise human text for the
// terminal. Rendering only formats fields already present in upload
// results; it never constructs, rewrites, or augments URLs.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/anjuls/cloudinary-cli/internal/upload"
)

// reportVersion is the schema version of the JSON report envelope.
const reportVersion = 1

// reportEnvelope is the private, versioned JSON document emitted by
// RenderJSON: the per-source results verbatim plus the summary computed
// from them at render time.
type reportEnvelope struct {
	Version int                   `json:"version"`
	Sources []upload.SourceResult `json:"sources"`
	Summary upload.Summary        `json:"summary"`
}

// RenderJSON writes the whole run report to w as exactly one indented
// JSON object followed by a newline: a version-1 envelope holding the
// source results and their upload.Summarize summary. A nil results
// slice is reported as an empty array, never null. HTML escaping is
// disabled so URLs stay verbatim and readable. Writer failures are
// returned wrapped with context.
func RenderJSON(w io.Writer, results []upload.SourceResult) error {
	if results == nil {
		results = []upload.SourceResult{}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reportEnvelope{
		Version: reportVersion,
		Sources: results,
		Summary: upload.Summarize(results),
	}); err != nil {
		return fmt.Errorf("cli: render json: %w", err)
	}
	return nil
}

// RenderSourceText writes one concise text block for a single source: a
// header line "source: status" extended with the source-level error when
// present, then one indented line per variant — uploaded variants show
// format, public ID, and URL; error variants show the failing stage and
// message; skipped variants show the reason. URLs are passed through
// verbatim; none are constructed. Writer failures are returned wrapped
// with context.
func RenderSourceText(w io.Writer, result upload.SourceResult) error {
	var b strings.Builder
	b.WriteString(result.Source)
	b.WriteString(": ")
	b.WriteString(string(result.Status))
	if result.Error != "" {
		b.WriteString(": ")
		b.WriteString(result.Error)
	}
	b.WriteByte('\n')
	for _, variant := range result.Variants {
		b.WriteString("  ")
		b.WriteString(string(variant.Format))
		b.WriteByte(' ')
		b.WriteString(string(variant.Status))
		switch variant.Status {
		case upload.VariantUploaded:
			b.WriteByte(' ')
			b.WriteString(variant.PublicID)
			b.WriteByte(' ')
			b.WriteString(variant.SecureURL)
		case upload.VariantError:
			b.WriteByte(' ')
			if variant.Stage != "" {
				b.WriteString(string(variant.Stage))
				b.WriteString(": ")
			}
			b.WriteString(variant.Error)
		case upload.VariantSkipped:
			b.WriteByte(' ')
			b.WriteString(variant.Reason)
		}
		b.WriteByte('\n')
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("cli: render source text: %w", err)
	}
	return nil
}

// RenderSummaryText writes the final one-line count summary of a run.
// Writer failures are returned wrapped with context.
func RenderSummaryText(w io.Writer, summary upload.Summary) error {
	line := fmt.Sprintf("%d sources: %d ok, %d partial, %d failed, %d skipped; %d variants uploaded",
		summary.Sources, summary.OK, summary.Partial, summary.Failed, summary.Skipped, summary.VariantsUploaded)
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		return fmt.Errorf("cli: render summary text: %w", err)
	}
	return nil
}
