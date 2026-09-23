// Package prompt provides a small injectable boundary for interactive
// terminal prompts, backed by github.com/charmbracelet/huh.
//
// All terminal-specific code is confined to the huh-backed implementation
// behind the Prompter interface; the rest of the CLI depends only on this
// package's abstraction.
package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// Field describes one input field of a prompt form.
type Field struct {
	// Label is the text shown above the input.
	Label string

	// Secret masks the typed value (password echo) when true.
	Secret bool
}

// Prompter asks the user for ordered input values on the terminal.
type Prompter interface {
	// Ask presents the fields as a single form, in order, under the given
	// title, and returns the entered values in the same order. Secret
	// fields are masked; their values are never echoed to the terminal.
	Ask(ctx context.Context, title string, fields []Field) ([]string, error)
}

// huhPrompter is the huh-backed Prompter implementation.
type huhPrompter struct{}

// compile-time check that huhPrompter satisfies Prompter.
var _ Prompter = huhPrompter{}

// New returns the default huh-backed Prompter.
func New() Prompter {
	return huhPrompter{}
}

// Ask implements Prompter.
func (huhPrompter) Ask(ctx context.Context, title string, fields []Field) ([]string, error) {
	if len(fields) == 0 {
		return nil, errors.New("prompt: no fields provided")
	}

	values := make([]string, len(fields))
	inputs := make([]huh.Field, len(fields))
	for i, field := range fields {
		input := huh.NewInput().
			Title(field.Label).
			Value(&values[i]).
			Validate(nonEmpty)
		if field.Secret {
			input = input.EchoMode(huh.EchoModePassword)
		}
		inputs[i] = input
	}

	form := huh.NewForm(huh.NewGroup(inputs...).Title(title))
	if err := form.RunWithContext(ctx); err != nil {
		// wrapErr keeps the huh error chain (including
		// huh.ErrUserAborted) discoverable and never includes values.
		return nil, wrapErr(err)
	}
	return values, nil
}

// wrapErr adds prompt-package context to a huh error. It is a named seam so
// the error contract (prompt: prefix, %w chain) stays unit-testable without
// driving a real terminal.
func wrapErr(err error) error {
	return fmt.Errorf("prompt: %w", err)
}

// nonEmpty rejects blank input so every field must be filled in.
func nonEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("value must not be empty")
	}
	return nil
}
