// Package discover finds still-image files beneath a directory without
// following symlinks.
package discover

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SkipReason explains why a discovered entry is not an uploadable still image.
type SkipReason int

const (
	// SkipNone marks an entry whose extension is a supported still-image extension.
	SkipNone SkipReason = iota
	// SkipSymlink marks a symlink entry; symlinks are recorded but never followed.
	SkipSymlink
	// SkipUnsupportedExt marks an entry whose extension is not a supported
	// still-image extension, including files with no extension.
	SkipUnsupportedExt
)

// Entry is a single file-system entry discovered beneath a root directory.
type Entry struct {
	// Path is the entry's path as produced by the walk from the root.
	Path string
	// Ext is the entry's lowercased dot extension, empty when the name has none.
	Ext string
	// Skip reports why the entry is not an uploadable still image, or SkipNone.
	Skip SkipReason
}

// SupportedExtensions lists the recognized still-image extensions, lowercase
// with a leading dot.
var SupportedExtensions = []string{".jpg", ".jpeg", ".png"}

var (
	// ErrRootSymlink is returned when the root path is itself a symlink, even
	// when its target is a directory.
	ErrRootSymlink = errors.New("root is a symlink")
	// ErrRootNotDirectory is returned when the root path exists but is not a
	// directory.
	ErrRootNotDirectory = errors.New("root is not a directory")
)

// IsSupported reports whether path's final extension is a supported
// still-image extension, ignoring case.
func IsSupported(path string) bool {
	return hasSupportedExt(strings.ToLower(filepath.Ext(path)))
}

// Directory walks the file tree rooted at root and returns every entry
// directly beneath root and, when recursive is true, every entry beneath its
// descendant directories. Symlinks are recorded with SkipSymlink and never
// followed. The returned entries are sorted by Path. An empty directory
// yields an empty, non-nil slice.
func Directory(root string, recursive bool) ([]Entry, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("discover: root %q: %w", root, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("discover: root %q: %w", root, ErrRootSymlink)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("discover: root %q: %w", root, ErrRootNotDirectory)
	}

	entries := make([]Entry, 0)
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && path != root {
				return fs.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		skip := SkipUnsupportedExt // non-regular entries and unsupported extensions
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			skip = SkipSymlink
		case d.Type().IsRegular() && hasSupportedExt(ext):
			skip = SkipNone
		}
		entries = append(entries, Entry{Path: path, Ext: ext, Skip: skip})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("discover: walk %q: %w", root, walkErr)
	}

	slices.SortFunc(entries, func(a, b Entry) int {
		return strings.Compare(a.Path, b.Path)
	})
	return entries, nil
}

func hasSupportedExt(ext string) bool {
	return slices.Contains(SupportedExtensions, ext)
}
