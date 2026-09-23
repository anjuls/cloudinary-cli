package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/anjuls/cloudinary-cli/internal/cfg"
	"github.com/anjuls/cloudinary-cli/internal/codec"
	"github.com/anjuls/cloudinary-cli/internal/discover"
	"github.com/anjuls/cloudinary-cli/internal/naming"
	"github.com/anjuls/cloudinary-cli/internal/prompt"
	"github.com/anjuls/cloudinary-cli/internal/upload"
)

type sourceCandidate struct {
	Path string
	Root string
}

func newUploadCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload <path>",
		Short: "Upload images to Cloudinary",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return &UsageError{Err: fmt.Errorf("accepts exactly one positional arg, received %d", len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpload(cmd.Context(), app, cmd, args[0])
		},
	}

	cmd.Flags().Int("quality", 80, "encoding quality (1-100)")
	cmd.Flags().Bool("overwrite", false, "overwrite existing assets")
	cmd.Flags().String("folder-mode", "dynamic", "folder mode: dynamic|fixed")
	cmd.Flags().String("folder", "", "folder path prefix")
	cmd.Flags().Bool("recursive", false, "recursively upload directory contents")
	cmd.Flags().String("output", "text", "output format: text|json")

	return cmd
}

func runUpload(ctx context.Context, app *App, cmd *cobra.Command, arg string) error {
	quality, err := cmd.Flags().GetInt("quality")
	if err != nil {
		return err
	}
	webpQ, avifQ, err := codec.PairFromBaseQuality(quality)
	if err != nil {
		return &UsageError{Err: fmt.Errorf("invalid quality: %w", err)}
	}

	overwrite, _ := cmd.Flags().GetBool("overwrite")
	folderMode, _ := cmd.Flags().GetString("folder-mode")
	if folderMode != "dynamic" && folderMode != "fixed" {
		return &UsageError{Err: fmt.Errorf("invalid folder-mode %q, expected dynamic or fixed", folderMode)}
	}
	folderFlag, _ := cmd.Flags().GetString("folder")
	if folderMode == "fixed" && folderFlag == "" {
		return &UsageError{Err: errors.New("fixed folder mode requires --folder")}
	}
	recursive, _ := cmd.Flags().GetBool("recursive")
	output, _ := cmd.Flags().GetString("output")
	if output != "text" && output != "json" {
		return &UsageError{Err: fmt.Errorf("invalid output %q, expected text or json", output)}
	}

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
	if err := c.Validate(); err != nil {
		var cfgErr *cfg.ConfigError
		if errors.As(err, &cfgErr) {
			fields := make([]prompt.Field, 0, len(cfgErr.Missing))
			for _, f := range cfgErr.Missing {
				switch f {
				case "cloud_name":
					fields = append(fields, prompt.Field{Label: "Cloud name"})
				case "api_key":
					fields = append(fields, prompt.Field{Label: "API key"})
				case "api_secret":
					fields = append(fields, prompt.Field{Label: "API secret", Secret: true})
				}
			}
			values, perr := app.Prompt.Ask(ctx, "Cloudinary credentials", fields)
			if perr != nil {
				return fmt.Errorf("prompt: %w", perr)
			}
			for i, f := range cfgErr.Missing {
				switch f {
				case "cloud_name":
					c.CloudName = values[i]
				case "api_key":
					c.APIKey = values[i]
				case "api_secret":
					c.APISecret = values[i]
				}
			}
			if err := c.Validate(); err != nil {
				return fmt.Errorf("config still invalid after prompt: %w", err)
			}
		} else {
			return fmt.Errorf("validate config: %w", err)
		}
	}

	up, err := app.NewUploader(c)
	if err != nil {
		return fmt.Errorf("create uploader: %w", err)
	}

	webpEnc, avifEnc := app.NewEncoders()
	ports := upload.Ports{WebP: webpEnc, AVIF: avifEnc, Up: up}
	baseOpts := upload.Options{WebPQuality: webpQ, AVIFQuality: avifQ, Overwrite: overwrite}

	candidates, results, err := buildSources(arg, recursive)
	if err != nil {
		return err
	}

	if output == "text" {
		for _, r := range results {
			if err := RenderSourceText(app.Stdout, r); err != nil {
				return fmt.Errorf("render source: %w", err)
			}
		}
	}

	for _, cand := range candidates {
		opts := baseOpts
		opts.Folder = computeFolder(folderMode, folderFlag, cand.Path, cand.Root)
		result := upload.Process(ctx, ports, opts, cand.Path)
		results = append(results, result)
		if output == "text" {
			if err := RenderSourceText(app.Stdout, result); err != nil {
				return fmt.Errorf("render source: %w", err)
			}
		}
	}

	summary := upload.Summarize(results)
	if output == "text" {
		if err := RenderSummaryText(app.Stdout, summary); err != nil {
			return fmt.Errorf("render summary: %w", err)
		}
	} else {
		if err := RenderJSON(app.Stdout, results); err != nil {
			return fmt.Errorf("render json: %w", err)
		}
	}

	if upload.AnyFailed(summary) {
		return errors.New("upload: one or more sources failed")
	}
	return nil
}

func buildSources(arg string, recursive bool) ([]sourceCandidate, []upload.SourceResult, error) {
	info, err := os.Lstat(arg)
	if err != nil {
		return nil, nil, fmt.Errorf("discover: %q: %w", arg, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, []upload.SourceResult{{
			Source:       arg,
			PublicIDBase: naming.Base(arg),
			Status:       upload.SourceSkipped,
			Variants:     nil,
		}}, nil
	}

	if info.Mode().IsRegular() {
		if !discover.IsSupported(arg) {
			return nil, []upload.SourceResult{{
				Source:       arg,
				PublicIDBase: naming.Base(arg),
				Status:       upload.SourceSkipped,
				Variants:     nil,
			}}, nil
		}
		return []sourceCandidate{{Path: arg, Root: filepath.Dir(arg)}}, nil, nil
	}

	if info.IsDir() {
		entries, err := discover.Directory(arg, recursive)
		if err != nil {
			return nil, nil, fmt.Errorf("discover directory: %w", err)
		}
		var candidates []sourceCandidate
		var results []upload.SourceResult
		for _, entry := range entries {
			if entry.Skip != discover.SkipNone {
				results = append(results, upload.SourceResult{
					Source:       entry.Path,
					PublicIDBase: naming.Base(entry.Path),
					Status:       upload.SourceSkipped,
					Variants:     nil,
				})
				continue
			}
			candidates = append(candidates, sourceCandidate{Path: entry.Path, Root: arg})
		}
		return candidates, results, nil
	}

	return nil, nil, fmt.Errorf("discover: %q: unsupported file type", arg)
}

func computeFolder(folderMode, folderFlag, path, root string) upload.FolderDirective {
	if folderMode == "fixed" {
		return upload.FolderDirective{Kind: upload.FolderLegacy, Path: folderFlag}
	}

	dir := filepath.Dir(path)
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." || rel == "" {
		rel = ""
	}

	var folder string
	if rel == "" {
		folder = folderFlag
	} else if folderFlag == "" {
		folder = filepath.ToSlash(rel)
	} else {
		folder = filepath.ToSlash(filepath.Join(folderFlag, rel))
	}

	if folder == "" {
		return upload.FolderDirective{Kind: upload.FolderNone}
	}
	return upload.FolderDirective{Kind: upload.FolderAsset, Path: folder}
}
