package prompt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
)

// Given the default constructor
// When New is called
// Then it returns a non-nil Prompter.
func TestNew_returns_nonnil_prompter(t *testing.T) {
	if got := New(); got == nil {
		t.Fatal("New() = nil, want non-nil Prompter")
	}
}

// Given the zero value of Field
// When its fields are read
// Then Label is empty and Secret is false (fields are visible by default).
func TestField_zero_value(t *testing.T) {
	var field Field
	if field.Label != "" {
		t.Errorf("zero Field.Label = %q, want empty", field.Label)
	}
	if field.Secret {
		t.Error("zero Field.Secret = true, want false")
	}
}

// Given the field list for config init (cloud name, API key, hidden API secret)
// When the list is built with Field literals
// Then exactly the API secret field is marked Secret, in the given order.
func TestField_marks_only_secret_fields(t *testing.T) {
	fields := []Field{
		{Label: "Cloud name"},
		{Label: "API key"},
		{Label: "API secret", Secret: true},
	}

	for i, field := range fields {
		wantSecret := i == len(fields)-1
		if field.Secret != wantSecret {
			t.Errorf("fields[%d] (%q).Secret = %v, want %v", i, field.Label, field.Secret, wantSecret)
		}
	}
}

// Given a Prompter and an empty field list
// When Ask is called
// Then it fails fast without opening a terminal form and returns no values.
func TestAsk_fails_fast_when_no_fields(t *testing.T) {
	tests := []struct {
		name   string
		fields []Field
	}{
		{"nil field list", nil},
		{"empty field list", []Field{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := New().Ask(context.Background(), "title", tt.fields)
			if err == nil {
				t.Fatal("Ask() err = nil, want error for empty field list")
			}
			if values != nil {
				t.Errorf("Ask() values = %v, want nil on error", values)
			}
		})
	}
}

// Given a huh error, including user abort
// When wrapErr adds prompt context
// Then the message carries the prompt: prefix and the original error stays
// discoverable through errors.Is.
func Test_wrapErr_preserves_huh_error_chain(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"user aborted", huh.ErrUserAborted},
		{"program failure", errors.New("could not open a new TTY")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapErr(tt.err)

			if !strings.HasPrefix(got.Error(), "prompt:") {
				t.Errorf("wrapErr().Error() = %q, want prompt: prefix", got.Error())
			}
			if !errors.Is(got, tt.err) {
				t.Errorf("errors.Is(wrapErr(%v), original) = false, want true", tt.err)
			}
		})
	}
}
