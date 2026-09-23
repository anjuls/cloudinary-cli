// Package naming derives deterministic base slugs for public IDs from
// local file paths.
package naming

import (
	"path/filepath"
	"strings"
	"unicode"
)

// fallbackSlug is returned when a path yields no slug characters at all.
const fallbackSlug = "asset"

// Base returns the deterministic public-ID base slug for path: the final
// path element with only its final extension stripped, reduced to Unicode
// letters, digits, and '_' with every other run of runes collapsed into a
// single '-', leading and trailing hyphens dropped, and case preserved. It
// returns "asset" when nothing remains.
func Base(path string) string {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	var b strings.Builder
	pendingHyphen := false
	for _, r := range stem {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_':
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteRune(r)
		default:
			pendingHyphen = true
		}
	}
	if b.Len() == 0 {
		return fallbackSlug
	}
	return b.String()
}
