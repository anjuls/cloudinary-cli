// Package cli provides the Cloudinary CLI application shell: a
// dependency-injected Cobra root command with stable exit-code mapping.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
	"github.com/anjuls/cloudinary-cli/internal/codec"
	"github.com/anjuls/cloudinary-cli/internal/prompt"
	"github.com/anjuls/cloudinary-cli/internal/upload"
)

// App is the dependency-injected CLI application shell. Every Execute call
// builds a fresh Cobra command tree from the registered subcommand
// constructors, routes output through the injected writers, and maps
// execution errors to stable exit codes.
type App struct {
	// Version is the release version reported by `cloudinary-cli --version`.
	// It is overridable at build time via
	// -ldflags "-X github.com/anjuls/cloudinary-cli/internal/cli.Version=vX.Y.Z"
	// and defaults to "dev".
	Version string

	// Stdout receives regular command output such as help and results.
	Stdout io.Writer

	// Stderr receives error diagnostics.
	Stderr io.Writer

	// Prompt asks the user for interactive input.
	Prompt prompt.Prompter

	// IsTerminal reports whether stdin is a TTY. When nil or false, the
	// CLI skips interactive prompts and uses safe non-interactive defaults.
	IsTerminal func() bool

	// NewUploader builds an Uploader from the resolved configuration.
	NewUploader func(cfg.Config) (upload.Uploader, error)

	// NewEncoders builds the local WebP and AVIF encoders.
	NewEncoders func() (codec.WebPEncoder, codec.AVIFEncoder)

	// commands holds subcommand constructors; Execute invokes each one once
	// per run so every call operates on a fresh command tree.
	commands []func(*App) *cobra.Command
}

// UsageError marks an error caused by invalid command-line usage. Execute
// maps it to exit code 2 instead of 1.
type UsageError struct {
	// Err is the underlying cause; it is never nil.
	Err error
}

// Error returns the underlying message with a usage: prefix.
func (e *UsageError) Error() string {
	return "usage: " + e.Err.Error()
}

// Unwrap returns the underlying cause so errors.Is and errors.As traverse it.
func (e *UsageError) Unwrap() error {
	return e.Err
}

// Version is the default version string when App.Version is empty.
// It is a var (not const) so release builds can override it with
// -ldflags "-X github.com/anjuls/cloudinary-cli/internal/cli.Version=vX.Y.Z".
var Version = "dev"

// NewApp returns an App wired to the production dependencies: the process
// streams, the huh-backed prompter, the SDK uploader constructor, and the
// gen2brain-backed encoders.
func NewApp() *App {
	a := &App{
		Version: Version,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Prompt:  prompt.New(),
		IsTerminal: func() bool {
			return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
		},
		NewUploader: upload.NewUploaderFromConfig,
		NewEncoders: func() (codec.WebPEncoder, codec.AVIFEncoder) {
			encoders := codec.NewEncoders()
			return encoders.WebP, encoders.AVIF
		},
	}
	a.addCommand(newConfigCmd, newUploadCmd)
	return a
}

// addCommand registers subcommand constructors used by every subsequent
// Execute call.
func (a *App) addCommand(newCommands ...func(*App) *cobra.Command) {
	a.commands = append(a.commands, newCommands...)
}

// Execute runs the CLI with args and returns the process exit code: 0 on
// success, 2 on usage errors, and 1 on any other error. The returned error
// is printed to Stderr exactly once with a concise prefix.
func (a *App) Execute(ctx context.Context, args []string) int {
	root := a.newRootCommand()
	root.SetArgs(args)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}

	code := 1
	var usage *UsageError
	if errors.As(err, &usage) {
		code = 2
	}
	fmt.Fprintf(a.Stderr, "cloudinary-cli: %v\n", err)

	return code
}

// newRootCommand builds a fresh root command with the persistent --config
// flag, the injected output writers, and the registered subcommands.
func (a *App) newRootCommand() *cobra.Command {
	version := a.Version
	if version == "" {
		version = Version
	}
	root := &cobra.Command{
		Use:           "cloudinary-cli",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(a.Stdout)
	root.SetErr(a.Stderr)
	root.PersistentFlags().String("config", "", "config file path")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Err: err}
	})
	for _, newCommand := range a.commands {
		root.AddCommand(newCommand(a))
	}

	return root
}
