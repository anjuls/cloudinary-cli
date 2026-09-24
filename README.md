# cloudinary-cli

A command-line tool that optimizes still images locally to **WebP** and **AVIF**, uploads both variants to [Cloudinary](https://cloudinary.com), and reports the direct delivery URLs.

Optimization happens entirely on your machine before upload. The tool does **not** apply Cloudinary delivery-time transformations (`f_auto`, `q_auto`, and similar), so your Cloudinary account incurs no extra transformation compute.

## Features

- **Local optimization** — pure-Go WebP and AVIF encoding; no external binaries (ImageMagick, cwebp, etc.) required.
- **Two variants per source** — every accepted image is encoded and uploaded as both a WebP and an AVIF asset.
- **Batch uploads** — point it at a file or a directory; discovery is deterministic (sorted) and non-recursive by default.
- **Folder placement** — preserve source directory structure (`--folder-mode dynamic`) or force a fixed Cloudinary folder (`--folder-mode fixed`).
- **Human and machine output** — concise terminal text or a versioned JSON report for scripting.
- **Safe defaults** — no overwrites unless you pass `--overwrite`; secrets are never accepted as command-line arguments.

## Requirements

- A Cloudinary account (cloud name, API key, API secret).
- Go 1.25 or newer — only when building from source (not needed for Homebrew or `go install`).

## Installation

### Homebrew (macOS / Linux)

```bash
brew tap anjuls/tap
brew install cloudinary-cli
```

New Homebrew versions require trusting the tap first:

```bash
brew trust anjuls/tap   # or: brew trust --formula anjuls/tap/cloudinary-cli
brew install cloudinary-cli
```

### Go

```bash
go install github.com/anjuls/cloudinary-cli@latest
```

### From source

Requires Go 1.25+:

```bash
go build -o cloudinary-cli .
```

Then either invoke it as `./cloudinary-cli`, or put it on your `PATH`:

```bash
go install .          # installs to $(go env GOPATH)/bin
# ensure GOPATH/bin is on PATH, then:
cloudinary-cli --help
```

Check the installed version:

```bash
cloudinary-cli --version
```

## Quick start

```bash
# 1. Store your credentials (interactive prompt; the secret is masked)
cloudinary-cli config init

# 2. Upload a single file (encodes WebP + AVIF, uploads both)
cloudinary-cli upload hero.jpg

# 3. Upload a directory
cloudinary-cli upload ./photos

# 4. Recursively upload a directory tree, preserving structure
cloudinary-cli upload ./site-assets --recursive
```

## Configuration

Credentials live in a JSON file at:

| Platform | Path |
|----------|------|
| Linux    | `$XDG_CONFIG_HOME/cloudinary-cli/config.json` (default `~/.config/cloudinary-cli/config.json`) |
| macOS    | `~/Library/Application Support/cloudinary-cli/config.json` |
| Windows  | `%AppData%\cloudinary-cli\config.json` |

The file is created with mode `0600` and its parent directory with `0700`. You can point the tool at a different file with the global `--config` flag on any command.

### Commands

#### `config init`

Interactive setup. Prompts for cloud name, API key, and API secret (the secret is entered with masked echo) and writes the config file.

```bash
cloudinary-cli config init
cloudinary-cli config init --config ./my-config.json
```

#### `config set <key> [<value>]`

Set one value. `cloud-name` and `api-key` take a value argument; `api-secret` deliberately does **not** — it prompts interactively so the secret never appears in your shell history or process list.

```bash
cloudinary-cli config set cloud-name my-cloud
cloudinary-cli config set api-key 123456789012345
cloudinary-cli config set api-secret        # prompts for the secret
```

#### `config show`

Print the effective configuration with the API secret redacted. Environment overrides (see below) are applied first.

```bash
$ cloudinary-cli config show
path: /home/you/.config/cloudinary-cli/config.json
cloud_name: my-cloud
api_key: 123456789012345
api_secret: ********
```

### Environment variables

Non-empty environment variables override values from the config file:

| Variable | Field |
|----------|-------|
| `CLOUDINARY_CLOUD_NAME` | Cloud name |
| `CLOUDINARY_API_KEY` | API key |
| `CLOUDINARY_API_SECRET` | API secret |

Useful for CI:

```bash
export CLOUDINARY_CLOUD_NAME=my-cloud
export CLOUDINARY_API_KEY=...
export CLOUDINARY_API_SECRET=...
cloudinary-cli upload ./dist --output json
```

If credentials are missing entirely, `upload` falls back to an interactive prompt for just the missing fields (secret fields are masked). In non-interactive environments (CI, pipes), set the environment variables instead — the prompt will fail without a TTY.

## Usage

### `upload <path>`

Encode every accepted source image to WebP and AVIF, upload both variants, and print a report. `<path>` is either a single file or a directory.

```bash
cloudinary-cli upload <path> [flags]
```

#### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--quality <0-1]>` | `0.5` | Encoding quality as a fraction. WebP uses `round(quality × 100)`; AVIF uses `max(1, round(webpQuality × 0.75))` (so the default 0.5 → WebP 50, AVIF 38). |
| `--size <0-1]>` | `1` | Linear scale for width and height before encoding. `1` = no change; `0.5` = half dimensions per side. |
| `--overwrite` | off | Allow replacing an existing Cloudinary asset with the same public ID. Without it, a name collision fails that source. |
| `--folder-mode <dynamic\|fixed>` | `dynamic` | How assets are placed in folders (see below). |
| `--folder <path>` | — | Folder path. Required when `--folder-mode fixed`. In dynamic mode it acts as a prefix for the derived structure. |
| `--recursive` | off | Walk subdirectories. Without it, only the top level of a directory is considered. |
| `--output <text\|json>` | `text` | Report format. |
| `--format <webp\|avif\|both>` | `both` (prompts when interactive) | Output format(s) to encode and upload. When stdin is a TTY and the flag is omitted, an interactive prompt asks which format to use. In non-interactive environments the default is `both`. |
| `--config <file>` | platform default | Alternate config file (global flag, works on every command). |

#### Supported input formats

`.jpg`, `.jpeg`, `.png` (case-insensitive). Anything else in a directory — including symlinks — is reported as **skipped** and does not fail the run. Animated GIFs and other formats are not supported in this version.

#### Public IDs and naming

Each source file gets a public-ID base derived from its filename: the final path element with only its final extension stripped, reduced to Unicode letters, digits, and `_`, with other runs collapsed to `-`, leading/trailing hyphens dropped, and **case preserved**. The two uploaded assets use:

- `<base>-webp`
- `<base>-avif`

Example: `My Photo.JPG` → `My-Photo-webp` and `My-Photo-avif`. A name with no usable characters falls back to `asset`.

Filenames are not used directly for the upload name (`UseFilename` is off), and no random suffix is appended — re-uploading the same file targets the same public IDs (subject to `--overwrite`).

#### Folder modes

**Dynamic (default)** — the source file's directory relative to the upload root becomes an [asset folder](https://cloudinary.com/docs/resource_image_delivery#asset_folders):

```bash
# photos/2024/cat.jpg uploaded from ./photos
# → asset_folder "2024", public ID base "cat"
cloudinary-cli upload ./photos --recursive
```

A `--folder` value is prepended as a prefix when given. Files at the root of the scan get no folder unless `--folder` is set.

**Fixed** — every asset goes to one folder using the legacy `folder` parameter. `--folder` is required:

```bash
cloudinary-cli upload ./photos --folder-mode fixed --folder blog-images
```

#### Output

**Text** (default) — one block per source, then a summary line:

```
photos/hero.jpg: ok
  webp uploaded hero-webp https://res.cloudinary.com/demo/image/upload/v1/hero-webp.webp
  avif uploaded hero-avif https://res.cloudinary.com/demo/image/upload/v1/hero-avif.webp
photos/broken.png: error: codec: unsupported image format: image: unknown format
2 sources: 1 ok, 0 partial, 1 failed, 0 skipped; 2 variants uploaded
```

**JSON** (`--output json`) — a single versioned envelope on stdout:

```json
{
  "version": 1,
  "sources": [
    {
      "source": "photos/hero.jpg",
      "public_id_base": "hero",
      "status": "ok",
      "variants": [
        {
          "format": "webp",
          "status": "uploaded",
          "public_id": "hero-webp",
          "secure_url": "https://res.cloudinary.com/demo/image/upload/v1/hero-webp.webp"
        },
        {
          "format": "avif",
          "status": "uploaded",
          "public_id": "hero-avif",
          "secure_url": "https://res.cloudinary.com/demo/image/upload/v1/hero-avif.webp"
        }
      ]
    }
  ],
  "summary": {
    "sources": 1,
    "ok": 1,
    "partial": 0,
    "failed": 0,
    "skipped": 0,
    "variants_uploaded": 2
  }
}
```

Field notes:

- `version` — report schema version; currently always `1`.
- `sources[].status` — `ok` (both variants uploaded), `partial` (one uploaded, one failed), `error` (failed before any upload), or `skipped` (unsupported/symlink).
- `sources[].variants[].status` — `uploaded`, `error`, or `skipped`. On error, `stage` is `encode` or `upload`; on skip, `reason` explains why.
- `secure_url` is passed through from Cloudinary verbatim — the tool never rewrites it and never appends transformation parameters.
- An empty run serializes `"sources": []` (never `null`).

#### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success (all sources ok or skipped). |
| `1` | Operational failure — one or more sources were `partial` or `error`, or the command itself failed (bad credentials, unwritable config, etc.). |
| `2` | Usage error — unknown flag, invalid flag value, wrong number of arguments. |

#### Failure semantics

- Selected variants are encoded to temporary files **before** any network call. If either encode fails, nothing is uploaded for that source.
- WebP uploads first, then AVIF. If the WebP upload fails, the AVIF upload is not attempted for that source.
- If WebP succeeded but AVIF fails, the source is reported as `partial` — the already-uploaded WebP asset is left in place (no automatic cleanup in this version). `partial` is only possible when both formats are selected; with a single format, a failure is reported as `error`.
- Each source is processed independently; one bad file does not stop the rest of the batch.
- Temporary encode files are always removed, including on failure.

### Examples

```bash
# Higher quality, overwrite existing assets, machine-readable report
cloudinary-cli upload ./img --quality 0.9 --overwrite --output json

# Resize to 50% and lower quality
cloudinary-cli upload ./img --size 0.5 --quality 0.4

# Upload only AVIF variants
cloudinary-cli upload ./img --format avif

# Fixed folder for a flat batch
cloudinary-cli upload ./press-kit --folder-mode fixed --folder press

# Dynamic folders with a common prefix, recursive
cloudinary-cli upload ./site --recursive --folder my-site

# Non-interactive CI upload
CLOUDINARY_CLOUD_NAME=... CLOUDINARY_API_KEY=... CLOUDINARY_API_SECRET=... \
  cloudinary-cli upload ./dist --output json > report.json

# Alternate config file
cloudinary-cli --config ./ci-config.json upload ./dist
```

### Shell completion

Cobra generates completion scripts for Bash, Zsh, Fish, and PowerShell:

```bash
cloudinary-cli completion bash
cloudinary-cli completion zsh
```

## How it works

For each source file:

1. **Decode** — the file is read and validated as JPEG or PNG (detected by magic bytes).
2. **Resize (optional)** — if `--size` is set to a value other than `1`, the image is linearly scaled on both axes using Catmull-Rom resampling before encoding.
3. **Encode locally** — WebP and AVIF are written to temporary files using pure-Go encoders (`gen2brain/webp`, `gen2brain/avif`). Lossy only; WebP uses the rounded base quality directly, AVIF uses `max(1, round(q × 0.75))`. Encoder settings: WebP `Method: 4`, AVIF `Speed: 6`.
4. **Upload** — both files are uploaded via the official Cloudinary Go SDK as signed `multipart` requests to `resource_type=image`, with the folder directive and overwrite policy applied.
5. **Report** — results are collected and rendered as text or JSON; `SecureURL` from Cloudinary is passed through untouched.

There is no concurrency in this version: files are processed sequentially in sorted order, which keeps output deterministic and rate limits predictable.

## Design choices (v1)

- **No delivery-time transformations** — optimization is done once at upload, not on every request. Direct `secure_url` values are reported as stored.
- **No Admin API** — only the upload endpoint is used; no listing or deletion of remote assets.
- **Optional local resize** — `--size` scales dimensions before encoding; no Cloudinary-side transforms are applied.
- **No keychain integration** — credentials live in the config file (0600) or environment variables.
- **Secrets stay off the command line** — `config set api-secret` prompts instead of accepting an argument.
- **No `.env` loading** — environment variables must be exported by the shell or CI system.

## Development

```bash
# Run the full test suite with the race detector
go test -race -shuffle=on -count=1 ./...

# Vet
go vet ./...

# Lint (golangci-lint v2 config in .golangci.yml)
golangci-lint run

# Format
gofumpt -l .
```

### Project layout

```
main.go                    # signal-aware entrypoint
internal/
  cli/                     # Cobra commands, report rendering, app shell
    app.go                 # dependency-injected App, exit-code mapping
    config_cmd.go          # config init | set | show
    upload_cmd.go          # upload command
    report.go              # text/JSON report renderers
  cfg/                     # config file load/save, env merge, validation, redaction
  codec/                   # quality math, JPEG/PNG decode, WebP/AVIF encode
  discover/                # symlink-safe directory walking, extension allowlist
  naming/                  # filename → public-ID base slug
  prompt/                  # injectable interactive prompt (huh-backed)
  upload/                  # domain model, pair pipeline, Cloudinary SDK adapter
```

## License

No license file has been added to this repository yet.
