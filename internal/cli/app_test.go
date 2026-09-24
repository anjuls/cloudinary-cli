package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
	"github.com/anjuls/cloudinary-cli/internal/codec"
	"github.com/anjuls/cloudinary-cli/internal/prompt"
	"github.com/anjuls/cloudinary-cli/internal/upload"
)

// fakePrompter is a Prompter stand-in that fails loudly if a test invokes it.
type fakePrompter struct{}

func (fakePrompter) Ask(context.Context, string, []prompt.Field) ([]string, error) {
	return nil, errors.New("prompt: unexpected call in test")
}

func (fakePrompter) Select(context.Context, string, []prompt.Choice, string) (string, error) {
	return "", errors.New("prompt: unexpected Select call in test")
}

// fakeUploader is an Uploader stand-in that fails loudly if a test invokes it.
type fakeUploader struct{}

func (fakeUploader) Upload(context.Context, string, upload.UploadRequest) (upload.UploadResponse, error) {
	return upload.UploadResponse{}, errors.New("upload: unexpected call in test")
}

// fakeWebPEncoder is a WebPEncoder stand-in that fails loudly if invoked.
type fakeWebPEncoder struct{}

func (fakeWebPEncoder) EncodeWebP(io.Writer, image.Image, codec.Quality) error {
	return errors.New("codec: unexpected WebP encode in test")
}

// fakeAVIFEncoder is an AVIFEncoder stand-in that fails loudly if invoked.
type fakeAVIFEncoder struct{}

func (fakeAVIFEncoder) EncodeAVIF(io.Writer, image.Image, codec.Quality) error {
	return errors.New("codec: unexpected AVIF encode in test")
}

// newTestApp returns an App wired to fail-loudly fakes and the given writers.
func newTestApp(stdout, stderr io.Writer) *App {
	return &App{
		Stdout:     stdout,
		Stderr:     stderr,
		Prompt:     fakePrompter{},
		IsTerminal: func() bool { return false },
		NewUploader: func(cfg.Config) (upload.Uploader, error) {
			return fakeUploader{}, nil
		},
		NewEncoders: func() (codec.WebPEncoder, codec.AVIFEncoder) {
			return fakeWebPEncoder{}, fakeAVIFEncoder{}
		},
	}
}

// flagString returns the named string flag value, failing the test when the
// flag is absent.
func flagString(t *testing.T, cmd *cobra.Command, name string) string {
	t.Helper()

	value, err := cmd.Flags().GetString(name)
	require.NoError(t, err)

	return value
}

func Test_Execute_maps_exit_code_when_child_returns_error(t *testing.T) {
	tests := []struct {
		name             string
		runE             func(cmd *cobra.Command, args []string) error
		wantCode         int
		wantErrSubstring string
	}{
		{
			name:     "nil error maps to zero",
			runE:     func(*cobra.Command, []string) error { return nil },
			wantCode: 0,
		},
		{
			name:             "ordinary error maps to one",
			runE:             func(*cobra.Command, []string) error { return errors.New("upload failed") },
			wantCode:         1,
			wantErrSubstring: "cloudinary-cli: upload failed",
		},
		{
			name: "usage error maps to two",
			runE: func(*cobra.Command, []string) error {
				return &UsageError{Err: errors.New("flag --a conflicts with --b")}
			},
			wantCode:         2,
			wantErrSubstring: "cloudinary-cli: usage: flag --a conflicts with --b",
		},
		{
			name: "wrapped usage error maps to two",
			runE: func(*cobra.Command, []string) error {
				return fmt.Errorf("work: %w", &UsageError{Err: errors.New("quality out of range")})
			},
			wantCode:         2,
			wantErrSubstring: "cloudinary-cli: work: usage: quality out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := newTestApp(&stdout, &stderr)
			app.addCommand(func(*App) *cobra.Command {
				return &cobra.Command{Use: "work", RunE: tt.runE}
			})

			code := app.Execute(context.Background(), []string{"work"})

			require.Equal(t, tt.wantCode, code)
			if tt.wantErrSubstring == "" {
				require.Empty(t, stderr.String())
			} else {
				require.Contains(t, stderr.String(), tt.wantErrSubstring)
				require.Equal(t, 1, strings.Count(stderr.String(), tt.wantErrSubstring),
					"error must be printed exactly once")
			}
		})
	}
}

func Test_Execute_returns_one_when_command_unknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)
	app.addCommand(func(*App) *cobra.Command {
		return &cobra.Command{Use: "work", RunE: func(*cobra.Command, []string) error { return nil }}
	})

	code := app.Execute(context.Background(), []string{"nope"})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), `unknown command "nope"`)
}

func Test_Execute_returns_two_when_flag_unknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)
	app.addCommand(func(*App) *cobra.Command {
		return &cobra.Command{Use: "work", RunE: func(*cobra.Command, []string) error { return nil }}
	})

	code := app.Execute(context.Background(), []string{"work", "--nope"})

	require.Equal(t, 2, code)
	require.Contains(t, stderr.String(), "unknown flag")
	require.Equal(t, 1, strings.Count(stderr.String(), "unknown flag"),
		"usage error must be printed exactly once")
}

func Test_Execute_exposes_config_flag_when_child_runs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)
	var gotConfig string
	app.addCommand(func(*App) *cobra.Command {
		return &cobra.Command{
			Use: "work",
			RunE: func(cmd *cobra.Command, _ []string) error {
				gotConfig = flagString(t, cmd, "config")
				return nil
			},
		}
	})

	code := app.Execute(context.Background(), []string{"work", "--config", "custom.json"})

	require.Equal(t, 0, code)
	require.Equal(t, "custom.json", gotConfig)
}

func Test_Execute_routes_output_when_child_writes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)
	app.addCommand(func(*App) *cobra.Command {
		return &cobra.Command{
			Use: "work",
			RunE: func(cmd *cobra.Command, _ []string) error {
				fmt.Fprintln(cmd.OutOrStdout(), "stdout-line")
				fmt.Fprintln(cmd.ErrOrStderr(), "stderr-line")
				return nil
			},
		}
	})

	code := app.Execute(context.Background(), []string{"work"})

	require.Equal(t, 0, code)
	require.Equal(t, "stdout-line\n", stdout.String())
	require.Equal(t, "stderr-line\n", stderr.String())
}

func Test_Execute_builds_fresh_flag_state_when_called_twice(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)
	type capture struct {
		config string
		tag    string
	}
	var calls []capture
	app.addCommand(func(*App) *cobra.Command {
		cmd := &cobra.Command{
			Use: "work",
			RunE: func(cmd *cobra.Command, _ []string) error {
				calls = append(calls, capture{config: flagString(t, cmd, "config"), tag: flagString(t, cmd, "tag")})
				return nil
			},
		}
		cmd.Flags().String("tag", "", "arbitrary child-local flag")
		return cmd
	})

	code1 := app.Execute(context.Background(), []string{"work", "--config", "first.json", "--tag", "one"})
	code2 := app.Execute(context.Background(), []string{"work"})

	require.Equal(t, 0, code1)
	require.Equal(t, 0, code2)
	require.Equal(t, []capture{{config: "first.json", tag: "one"}, {config: "", tag: ""}}, calls)
}

func Test_Execute_hides_secret_text_when_printing_errors(t *testing.T) {
	const secret = "super-secret-api-secret-value"
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)
	app.NewUploader = func(cfg.Config) (upload.Uploader, error) {
		return nil, errors.New("upload: construct client: invalid credentials")
	}
	app.addCommand(func(a *App) *cobra.Command {
		return &cobra.Command{
			Use: "work",
			RunE: func(*cobra.Command, []string) error {
				_, err := a.NewUploader(cfg.Config{CloudName: "demo", APIKey: "key123", APISecret: secret})
				return err
			},
		}
	})

	code := app.Execute(context.Background(), []string{"work"})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "construct client")
	require.NotContains(t, stderr.String(), secret)
	require.NotContains(t, stdout.String(), secret)
}

func Test_NewApp_wires_default_dependencies(t *testing.T) {
	app := NewApp()

	require.Equal(t, os.Stdout, app.Stdout)
	require.Equal(t, os.Stderr, app.Stderr)
	require.Equal(t, Version, app.Version)
	require.NotNil(t, app.Prompt)
	require.NotNil(t, app.NewUploader)
	webp, avif := app.NewEncoders()
	require.NotNil(t, webp)
	require.NotNil(t, avif)
}

func Test_version_flag_prints_app_version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)
	app.Version = "v9.9.9"

	code := app.Execute(context.Background(), []string{"--version"})

	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), "cloudinary-cli version v9.9.9")
	require.Empty(t, stderr.String())
}

func Test_version_flag_defaults_to_dev_when_unset(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newTestApp(&stdout, &stderr)

	code := app.Execute(context.Background(), []string{"--version"})

	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), "cloudinary-cli version dev")
}

func Test_UsageError_wraps_inner_error(t *testing.T) {
	inner := errors.New("bad flag combination")
	err := &UsageError{Err: inner}

	require.Equal(t, "usage: bad flag combination", err.Error())
	require.ErrorIs(t, err, inner)
}
