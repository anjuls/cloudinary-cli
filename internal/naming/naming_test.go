package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Base_returns_expected_slug_when_path_given(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{"plain filename strips final extension", "cat.jpg", "cat"},
		{"spaces and parens collapse to single hyphens", "My Photo (2).png", "My-Photo-2"},
		{"unicode letters and case are preserved", "Ünïcode.JPEG", "Ünïcode"},
		{"punctuation-only stem falls back to asset", "...jpg", "asset"},
		{"surrounding spaces are trimmed away", "  x  .jpg", "x"},
		{"repeated hyphens collapse to one", "a--b.png", "a-b"},
		{"leading hyphen is trimmed", "-lead.png", "lead"},
		{"underscores and cyrillic letters are preserved", "та_pet.bmp", "та_pet"},
		{"absolute path yields same base as bare filename", "/tmp/uploads/cat.jpg", "cat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := Base(tt.path)

			// Then
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_Base_returns_same_slug_when_absolute_or_relative_path_given(t *testing.T) {
	t.Parallel()

	// Given
	relative := "My Photo (2).png"
	absolute := "/Users/anjul/Pictures/My Photo (2).png"

	// When
	fromRelative := Base(relative)
	fromAbsolute := Base(absolute)

	// Then
	require.Equal(t, fromRelative, fromAbsolute)
	require.Equal(t, "My-Photo-2", fromAbsolute)
}
