package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
	"github.com/anjuls/cloudinary-cli/internal/prompt"
)

func newConfigTestApp(stdout, stderr io.Writer) *App {
	app := newTestApp(stdout, stderr)
	app.addCommand(newConfigCmd)
	return app
}

type recordingPrompter struct {
	title  string
	fields []prompt.Field
	values []string
	err    error
}

func (r *recordingPrompter) Ask(_ context.Context, title string, fields []prompt.Field) ([]string, error) {
	r.title = title
	r.fields = fields
	if r.err != nil {
		return nil, r.err
	}
	return r.values, nil
}

func (r *recordingPrompter) Select(context.Context, string, []prompt.Choice, string) (string, error) {
	return "", errors.New("prompt: unexpected Select call in test")
}

func Test_config_init_writes_valid_config(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	var stdout, stderr bytes.Buffer
	app := newConfigTestApp(&stdout, &stderr)
	p := &recordingPrompter{values: []string{"mycloud", "mykey", "mysecret"}}
	app.Prompt = p

	code := app.Execute(context.Background(), []string{"config", "init", "--config", path})

	require.Equal(t, 0, code)
	require.Equal(t, fmt.Sprintf("config written: %s\n", path), stdout.String())
	require.Empty(t, stderr.String())
	require.NotContains(t, stdout.String(), "mysecret")

	require.Equal(t, "Cloudinary configuration", p.title)
	require.Len(t, p.fields, 3)
	require.Equal(t, "Cloud name", p.fields[0].Label)
	require.False(t, p.fields[0].Secret)
	require.Equal(t, "API key", p.fields[1].Label)
	require.False(t, p.fields[1].Secret)
	require.Equal(t, "API secret", p.fields[2].Label)
	require.True(t, p.fields[2].Secret)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var c cfg.Config
	require.NoError(t, json.Unmarshal(data, &c))
	require.Equal(t, "mycloud", c.CloudName)
	require.Equal(t, "mykey", c.APIKey)
	require.Equal(t, "mysecret", c.APISecret)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func Test_config_init_extra_args_returns_usage_error(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	var stdout, stderr bytes.Buffer
	app := newConfigTestApp(&stdout, &stderr)

	code := app.Execute(context.Background(), []string{"config", "init", "--config", path, "extra"})

	require.Equal(t, 2, code)
	require.Contains(t, stderr.String(), "usage:")
}

func Test_config_init_prompt_error_returns_one(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	var stdout, stderr bytes.Buffer
	app := newConfigTestApp(&stdout, &stderr)
	app.Prompt = &recordingPrompter{err: errors.New("prompt failed")}

	code := app.Execute(context.Background(), []string{"config", "init", "--config", path})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "prompt failed")
}

func Test_config_set_updates_field_preserving_others(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"cloud-name", "newcloud"},
		{"api-key", "newkey"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			require.NoError(t, cfg.Save(path, cfg.Config{CloudName: "c", APIKey: "k", APISecret: "s"}))

			var stdout, stderr bytes.Buffer
			app := newConfigTestApp(&stdout, &stderr)

			code := app.Execute(context.Background(), []string{"config", "set", tt.key, tt.value, "--config", path})

			require.Equal(t, 0, code)
			require.Equal(t, fmt.Sprintf("config %s set: %s\n", tt.key, path), stdout.String())
			require.Empty(t, stderr.String())

			c, err := cfg.Load(path)
			require.NoError(t, err)

			switch tt.key {
			case "cloud-name":
				require.Equal(t, tt.value, c.CloudName)
				require.Equal(t, "k", c.APIKey)
				require.Equal(t, "s", c.APISecret)
			case "api-key":
				require.Equal(t, "c", c.CloudName)
				require.Equal(t, tt.value, c.APIKey)
				require.Equal(t, "s", c.APISecret)
			}
		})
	}
}

func Test_config_set_usage_errors(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantCode    int
		wantContain string
	}{
		{
			name:        "empty value",
			args:        []string{"config", "set", "cloud-name", "", "--config", "/dev/null"},
			wantCode:    2,
			wantContain: "usage:",
		},
		{
			name:        "missing value",
			args:        []string{"config", "set", "api-key", "--config", "/dev/null"},
			wantCode:    2,
			wantContain: "usage:",
		},
		{
			name:        "unknown key",
			args:        []string{"config", "set", "unknown", "--config", "/dev/null"},
			wantCode:    2,
			wantContain: "usage:",
		},
		{
			name:        "secret in argv",
			args:        []string{"config", "set", "api-secret", "leak", "--config", "/dev/null"},
			wantCode:    2,
			wantContain: "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := newConfigTestApp(&stdout, &stderr)

			code := app.Execute(context.Background(), tt.args)

			require.Equal(t, tt.wantCode, code)
			require.Contains(t, stderr.String(), tt.wantContain)
		})
	}
}

func Test_config_set_api_secret_prompts_and_saves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, cfg.Save(path, cfg.Config{CloudName: "c", APIKey: "k", APISecret: "old"}))

	var stdout, stderr bytes.Buffer
	app := newConfigTestApp(&stdout, &stderr)
	p := &recordingPrompter{values: []string{"newsecret"}}
	app.Prompt = p

	code := app.Execute(context.Background(), []string{"config", "set", "api-secret", "--config", path})

	require.Equal(t, 0, code)
	require.Equal(t, fmt.Sprintf("config api-secret set: %s\n", path), stdout.String())
	require.Empty(t, stderr.String())

	require.Len(t, p.fields, 1)
	require.Equal(t, "API secret", p.fields[0].Label)
	require.True(t, p.fields[0].Secret)

	c, err := cfg.Load(path)
	require.NoError(t, err)
	require.Equal(t, "c", c.CloudName)
	require.Equal(t, "k", c.APIKey)
	require.Equal(t, "newsecret", c.APISecret)
}

func Test_config_show_prints_redacted_config(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, cfg.Save(path, cfg.Config{CloudName: "mycloud", APIKey: "mykey", APISecret: "mysecret"}))

	t.Setenv(cfg.EnvCloudName, "envcloud")

	var stdout, stderr bytes.Buffer
	app := newConfigTestApp(&stdout, &stderr)

	code := app.Execute(context.Background(), []string{"config", "show", "--config", path})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())

	out := stdout.String()
	require.Contains(t, out, fmt.Sprintf("path: %s", path))
	require.Contains(t, out, "cloud_name: envcloud")
	require.Contains(t, out, "api_key: mykey")
	require.Contains(t, out, "api_secret: ********")
	require.NotContains(t, out, "mysecret")
}

func Test_config_show_load_error_returns_one(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	app := newConfigTestApp(&stdout, &stderr)

	code := app.Execute(context.Background(), []string{"config", "show", "--config", dir})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "load config")
}

func Test_config_help_lists_subcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newConfigTestApp(&stdout, &stderr)

	code := app.Execute(context.Background(), []string{"config", "--help"})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	out := stdout.String()
	require.Contains(t, out, "init")
	require.Contains(t, out, "set")
	require.Contains(t, out, "show")
}
