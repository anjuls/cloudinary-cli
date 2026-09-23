package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
	"github.com/anjuls/cloudinary-cli/internal/prompt"
)

func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Cloudinary CLI configuration",
	}

	cmd.AddCommand(newConfigInitCmd(app))
	cmd.AddCommand(newConfigSetCmd(app))
	cmd.AddCommand(newConfigShowCmd(app))

	return cmd
}

func configPathFlag(cmd *cobra.Command) (string, error) {
	v, err := cmd.Flags().GetString("config")
	if err != nil {
		return "", fmt.Errorf("get config flag: %w", err)
	}
	return v, nil
}

func newConfigInitCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration interactively",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return &UsageError{Err: fmt.Errorf("accepts no positional args, received %d", len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			flag, err := configPathFlag(cmd)
			if err != nil {
				return err
			}
			path, err := cfg.FilePath(flag)
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}

			values, err := app.Prompt.Ask(cmd.Context(), "Cloudinary configuration", []prompt.Field{
				{Label: "Cloud name", Secret: false},
				{Label: "API key", Secret: false},
				{Label: "API secret", Secret: true},
			})
			if err != nil {
				return fmt.Errorf("prompt: %w", err)
			}

			if err := cfg.Save(path, cfg.Config{
				CloudName: values[0],
				APIKey:    values[1],
				APISecret: values[2],
			}); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Fprintf(app.Stdout, "config written: %s\n", path)
			return nil
		},
	}
}

func newConfigSetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> [<value>]",
		Short: "Set a configuration value",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &UsageError{Err: fmt.Errorf("missing key")}
			}
			switch args[0] {
			case "cloud-name", "api-key":
				if len(args) != 2 {
					return &UsageError{Err: fmt.Errorf("missing value for %s", args[0])}
				}
				if args[1] == "" {
					return &UsageError{Err: fmt.Errorf("empty value for %s", args[0])}
				}
			case "api-secret":
				if len(args) != 1 {
					return &UsageError{Err: fmt.Errorf("secret value must not be passed on the command line; use 'config set api-secret' to prompt interactively")}
				}
			default:
				return &UsageError{Err: fmt.Errorf("unknown key %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			flag, err := configPathFlag(cmd)
			if err != nil {
				return err
			}
			path, err := cfg.FilePath(flag)
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}

			c, err := cfg.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			switch args[0] {
			case "cloud-name":
				c.CloudName = args[1]
			case "api-key":
				c.APIKey = args[1]
			case "api-secret":
				values, err := app.Prompt.Ask(cmd.Context(), "Cloudinary configuration", []prompt.Field{
					{Label: "API secret", Secret: true},
				})
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				c.APISecret = values[0]
			}

			if err := cfg.Save(path, c); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Fprintf(app.Stdout, "config %s set: %s\n", args[0], path)
			return nil
		},
	}
}

func newConfigShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return &UsageError{Err: fmt.Errorf("accepts no positional args, received %d", len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			flag, err := configPathFlag(cmd)
			if err != nil {
				return err
			}
			path, err := cfg.FilePath(flag)
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}

			c, err := cfg.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			c = cfg.MergeEnv(c, os.Getenv)
			c = c.Redacted()

			fmt.Fprintf(app.Stdout, "path: %s\n", path)
			fmt.Fprintf(app.Stdout, "cloud_name: %s\n", c.CloudName)
			fmt.Fprintf(app.Stdout, "api_key: %s\n", c.APIKey)
			fmt.Fprintf(app.Stdout, "api_secret: %s\n", c.APISecret)
			return nil
		},
	}
}
