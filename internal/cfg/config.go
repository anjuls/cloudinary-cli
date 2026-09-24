// Package cfg stores the Cloudinary CLI configuration as JSON on disk,
// merges environment overrides, and validates completeness.
package cfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Environment variable names whose non-empty values override the
// corresponding config fields.
const (
	EnvCloudName = "CLOUDINARY_CLOUD_NAME"
	EnvAPIKey    = "CLOUDINARY_API_KEY"    // #nosec G101 -- env var name, not a credential
	EnvAPISecret = "CLOUDINARY_API_SECRET" // #nosec G101 -- env var name, not a credential
)

// RedactedSecret replaces the API secret in redacted output.
const RedactedSecret = "********"

// Config is the on-disk CLI configuration.
type Config struct {
	CloudName string `json:"cloud_name"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}

// ConfigError reports missing required config fields by JSON field name.
// It never contains field values.
type ConfigError struct {
	Missing []string
}

func (e *ConfigError) Error() string {
	return "config: missing required fields: " + strings.Join(e.Missing, ", ")
}

// ErrAPISecretEqualsKey is wrapped by Validate when api_key and api_secret
// are both non-empty and equal. The fix is to copy the API Secret — not the
// API Key — from the Cloudinary console under Settings > API Keys.
var ErrAPISecretEqualsKey = errors.New(
	"api_secret must not equal api_key; copy the API Secret (not the API Key) from the Cloudinary console at Settings > API Keys",
)

// FilePath resolves the config file location. A non-empty override wins;
// otherwise the default is <user config dir>/cloudinary-cli/config.json.
func FilePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "cloudinary-cli", "config.json"), nil
}

// Load reads the config at path. A missing file yields the zero Config and
// no error; malformed JSON and read failures are wrapped with the path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the user-selected config file
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	return c, nil
}

// Save writes c as indented JSON with a trailing newline to path, creating
// parent directories with 0700 and normalizing the file to 0600. It does
// not validate completeness so partial state can be persisted.
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir %q: %w", dir, err)
	}
	data, err := json.MarshalIndent(c, "", "  ") // #nosec G117 -- serializing Config to its own 0600 file is the feature
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure config %q: %w", path, err)
	}
	return nil
}

// MergeEnv returns base with fields overridden by non-empty environment
// variables. Empty variables are ignored.
func MergeEnv(base Config, getenv func(string) string) Config {
	if v := getenv(EnvCloudName); v != "" {
		base.CloudName = v
	}
	if v := getenv(EnvAPIKey); v != "" {
		base.APIKey = v
	}
	if v := getenv(EnvAPISecret); v != "" {
		base.APISecret = v
	}
	return base
}

// Validate returns nil when every field is present and api_key differs from
// api_secret. If any field is missing it returns a *ConfigError listing the
// missing JSON field names in the stable order cloud_name, api_key,
// api_secret; otherwise, if api_key and api_secret are both non-empty and
// equal, it returns an error wrapping ErrAPISecretEqualsKey.
func (c Config) Validate() error {
	var missing []string
	if c.CloudName == "" {
		missing = append(missing, "cloud_name")
	}
	if c.APIKey == "" {
		missing = append(missing, "api_key")
	}
	if c.APISecret == "" {
		missing = append(missing, "api_secret")
	}
	if len(missing) > 0 {
		return &ConfigError{Missing: missing}
	}
	if c.APIKey == c.APISecret {
		return fmt.Errorf("config: %w", ErrAPISecretEqualsKey)
	}
	return nil
}

// Redacted returns a copy of c with the API secret replaced by
// RedactedSecret; every other field is preserved.
func (c Config) Redacted() Config {
	c.APISecret = RedactedSecret
	return c
}
