package cfg_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
)

func Test_FilePath_returns_override_when_nonempty(t *testing.T) {
	got, err := cfg.FilePath("/custom/config.json")

	require.NoError(t, err)
	require.Equal(t, "/custom/config.json", got)
}

func Test_FilePath_returns_user_config_default_when_override_empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	userDir, err := os.UserConfigDir()
	require.NoError(t, err)

	got, err := cfg.FilePath("")

	require.NoError(t, err)
	require.Equal(t, filepath.Join(userDir, "cloudinary-cli", "config.json"), got)
}

func Test_FilePath_wraps_error_when_user_config_dir_unresolvable(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := cfg.FilePath("")

	require.Error(t, err)
}

func Test_Load_returns_zero_config_when_file_missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	got, err := cfg.Load(path)

	require.NoError(t, err)
	require.Equal(t, cfg.Config{}, got)
}

func Test_Load_wraps_syntax_error_when_json_malformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"cloud_name": `), 0o600))

	_, err := cfg.Load(path)

	require.Error(t, err)
	require.ErrorContains(t, err, path)
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)
}

func Test_Load_wraps_read_error_when_path_is_directory(t *testing.T) {
	path := t.TempDir()

	_, err := cfg.Load(path)

	require.Error(t, err)
	require.ErrorContains(t, err, path)
}

func Test_Save_then_Load_roundtrips_config(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := cfg.Config{CloudName: "demo", APIKey: "key123", APISecret: "secret123"}

	require.NoError(t, cfg.Save(path, want))

	got, err := cfg.Load(path)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func Test_Save_writes_indented_json_with_trailing_newline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := cfg.Config{CloudName: "demo", APIKey: "key123", APISecret: "secret123"}

	require.NoError(t, cfg.Save(path, c))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t,
		"{\n  \"cloud_name\": \"demo\",\n  \"api_key\": \"key123\",\n  \"api_secret\": \"secret123\"\n}\n",
		string(data))
}

func Test_Save_sets_file_0600_and_config_dir_0700(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "cloudinary-cli")
	path := filepath.Join(configDir, "config.json")

	require.NoError(t, cfg.Save(path, cfg.Config{CloudName: "demo"}))

	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), fileInfo.Mode().Perm())

	dirInfo, err := os.Stat(configDir)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o700), dirInfo.Mode().Perm())
}

func Test_Save_creates_nested_parent_dirs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "config.json")

	require.NoError(t, cfg.Save(path, cfg.Config{CloudName: "demo"}))

	for _, dir := range []string{filepath.Join(root, "a"), filepath.Join(root, "a", "b")} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		require.True(t, info.IsDir())
		assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm())
	}
}

func Test_Save_normalizes_preexisting_0644_file_to_0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o600))
	require.NoError(t, os.Chmod(path, 0o644))

	require.NoError(t, cfg.Save(path, cfg.Config{CloudName: "demo"}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
}

func Test_Save_persists_partial_config_without_validation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	require.NoError(t, cfg.Save(path, cfg.Config{CloudName: "demo"}))

	got, err := cfg.Load(path)
	require.NoError(t, err)
	require.Equal(t, cfg.Config{CloudName: "demo"}, got)
}

func Test_MergeEnv_overrides_fields_when_env_vars_nonempty(t *testing.T) {
	base := cfg.Config{CloudName: "file-cloud", APIKey: "file-key", APISecret: "file-secret"}
	env := map[string]string{
		cfg.EnvCloudName: "env-cloud",
		cfg.EnvAPIKey:    "env-key",
		cfg.EnvAPISecret: "env-secret",
	}

	got := cfg.MergeEnv(base, func(name string) string { return env[name] })

	require.Equal(t, cfg.Config{CloudName: "env-cloud", APIKey: "env-key", APISecret: "env-secret"}, got)
}

func Test_MergeEnv_keeps_base_fields_when_env_vars_empty(t *testing.T) {
	base := cfg.Config{CloudName: "file-cloud", APIKey: "file-key", APISecret: "file-secret"}
	env := map[string]string{
		cfg.EnvCloudName: "",
		cfg.EnvAPIKey:    "",
		cfg.EnvAPISecret: "",
	}

	got := cfg.MergeEnv(base, func(name string) string { return env[name] })

	require.Equal(t, base, got)
}

func Test_MergeEnv_overrides_only_corresponding_fields(t *testing.T) {
	base := cfg.Config{CloudName: "file-cloud", APIKey: "file-key", APISecret: "file-secret"}
	env := map[string]string{cfg.EnvAPIKey: "env-key"}

	got := cfg.MergeEnv(base, func(name string) string { return env[name] })

	require.Equal(t, cfg.Config{CloudName: "file-cloud", APIKey: "env-key", APISecret: "file-secret"}, got)
}

func Test_Validate_returns_nil_when_all_fields_present(t *testing.T) {
	c := cfg.Config{CloudName: "demo", APIKey: "key123", APISecret: "secret123"}

	require.NoError(t, c.Validate())
}

func Test_Validate_reports_missing_fields_in_stable_order(t *testing.T) {
	tests := []struct {
		name string
		in   cfg.Config
		want []string
	}{
		{"missing cloud_name only", cfg.Config{APIKey: "key", APISecret: "secret"}, []string{"cloud_name"}},
		{"missing api_key only", cfg.Config{CloudName: "demo", APISecret: "secret"}, []string{"api_key"}},
		{"missing api_secret only", cfg.Config{CloudName: "demo", APIKey: "key"}, []string{"api_secret"}},
		{"missing cloud_name and api_secret", cfg.Config{APIKey: "key"}, []string{"cloud_name", "api_secret"}},
		{"missing all fields", cfg.Config{}, []string{"cloud_name", "api_key", "api_secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()

			var cfgErr *cfg.ConfigError
			require.ErrorAs(t, err, &cfgErr)
			assert.Equal(t, tt.want, cfgErr.Missing)
		})
	}
}

func Test_Validate_error_omits_field_values(t *testing.T) {
	c := cfg.Config{CloudName: "demo-cloud", APIKey: "leaked-key"}

	err := c.Validate()

	require.Error(t, err)
	var cfgErr *cfg.ConfigError
	require.ErrorAs(t, err, &cfgErr)
	require.NotContains(t, cfgErr.Error(), "demo-cloud")
	require.NotContains(t, cfgErr.Error(), "leaked-key")
}

func Test_Redacted_replaces_only_api_secret(t *testing.T) {
	in := cfg.Config{CloudName: "demo", APIKey: "key123", APISecret: "secret123"}

	got := in.Redacted()

	require.Equal(t, cfg.Config{CloudName: "demo", APIKey: "key123", APISecret: "********"}, got)
}
