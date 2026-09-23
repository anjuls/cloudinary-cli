package discover

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFixture builds the standard discovery tree beneath a fresh temp dir:
//
//	a.jpg b.jpeg c.PNG d.txt e.webp noext .h.jpg   root-level files
//	sub/f.jpg                                       descendant, recursive only
//	filelink.jpg -> a.jpg                           file symlink
//	dirlink     -> sub                              directory symlink
func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	for _, name := range []string{"a.jpg", "b.jpeg", "c.PNG", "d.txt", "e.webp", "noext", ".h.jpg"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o644))
	}
	require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "f.jpg"), []byte("fixture"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(root, "a.jpg"), filepath.Join(root, "filelink.jpg")))
	require.NoError(t, os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "dirlink")))
	return root
}

// wantEntries returns the expected sorted entries for the standard fixture;
// recursive mode additionally expects sub/f.jpg.
func wantEntries(root string, recursive bool) []Entry {
	entries := []Entry{
		{Path: filepath.Join(root, ".h.jpg"), Ext: ".jpg", Skip: SkipNone},
		{Path: filepath.Join(root, "a.jpg"), Ext: ".jpg", Skip: SkipNone},
		{Path: filepath.Join(root, "b.jpeg"), Ext: ".jpeg", Skip: SkipNone},
		{Path: filepath.Join(root, "c.PNG"), Ext: ".png", Skip: SkipNone},
		{Path: filepath.Join(root, "d.txt"), Ext: ".txt", Skip: SkipUnsupportedExt},
		{Path: filepath.Join(root, "dirlink"), Ext: "", Skip: SkipSymlink},
		{Path: filepath.Join(root, "e.webp"), Ext: ".webp", Skip: SkipUnsupportedExt},
		{Path: filepath.Join(root, "filelink.jpg"), Ext: ".jpg", Skip: SkipSymlink},
		{Path: filepath.Join(root, "noext"), Ext: "", Skip: SkipUnsupportedExt},
	}
	if recursive {
		entries = append(entries, Entry{Path: filepath.Join(root, "sub", "f.jpg"), Ext: ".jpg", Skip: SkipNone})
	}
	return entries
}

func TestDirectory(t *testing.T) {
	t.Run("non-recursive excludes descendants of child directories", func(t *testing.T) {
		// Given the standard fixture tree.
		root := writeFixture(t)

		// When discovering without recursion.
		got, err := Directory(root, false)

		// Then exactly the root-level entries are returned, sorted by path,
		// with the hidden file included, extensions lowercased, unsupported
		// extensions and both symlinks recorded with their skip reasons, and
		// no descent into sub or the dirlink symlink.
		require.NoError(t, err)
		require.Equal(t, wantEntries(root, false), got)
	})

	t.Run("recursive includes descendants of child directories", func(t *testing.T) {
		// Given the standard fixture tree.
		root := writeFixture(t)

		// When discovering with recursion.
		got, err := Directory(root, true)

		// Then the root-level entries plus sub/f.jpg are returned, sorted by
		// path, still without following either symlink.
		require.NoError(t, err)
		require.Equal(t, wantEntries(root, true), got)
	})

	t.Run("empty root returns an empty non-nil slice", func(t *testing.T) {
		// Given an empty directory.
		root := t.TempDir()

		// When discovering it.
		got, err := Directory(root, false)

		// Then the result is an empty, non-nil slice with no error.
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})
}

func TestDirectoryRootValidation(t *testing.T) {
	t.Run("missing root wraps the underlying stat error", func(t *testing.T) {
		// Given a root path that does not exist.
		root := filepath.Join(t.TempDir(), "missing")

		// When discovering it.
		got, err := Directory(root, false)

		// Then the error wraps the cause and no entries are returned.
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, got)
	})

	t.Run("regular-file root returns ErrRootNotDirectory", func(t *testing.T) {
		// Given a regular file as the root.
		root := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(root, []byte("fixture"), 0o644))

		// When discovering it.
		got, err := Directory(root, true)

		// Then the error wraps ErrRootNotDirectory and no entries are returned.
		require.ErrorIs(t, err, ErrRootNotDirectory)
		require.Nil(t, got)
	})

	t.Run("symlink root returns ErrRootSymlink even when the target is a directory", func(t *testing.T) {
		// Given a symlink whose target is a real directory.
		target := t.TempDir()
		root := filepath.Join(t.TempDir(), "rootlink")
		require.NoError(t, os.Symlink(target, root))

		// When discovering the symlink itself.
		got, err := Directory(root, true)

		// Then the error wraps ErrRootSymlink and no entries are returned.
		require.ErrorIs(t, err, ErrRootSymlink)
		require.Nil(t, got)
	})
}

func TestIsSupported(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "lowercase jpg", path: "img/a.jpg", want: true},
		{name: "uppercase JPG", path: "photo.JPG", want: true},
		{name: "mixed-case JpEg", path: "photo.JpEg", want: true},
		{name: "uppercase PNG", path: "photo.PNG", want: true},
		{name: "hidden jpg", path: ".h.jpg", want: true},
		{name: "txt", path: "notes.txt", want: false},
		{name: "webp", path: "pic.webp", want: false},
		{name: "gif", path: "anim.gif", want: false},
		{name: "avif", path: "pic.avif", want: false},
		{name: "no extension", path: "README", want: false},
		{name: "only final extension counts", path: "archive.tar.gz", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a path with the extension described by the case name.
			// When checking support.
			got := IsSupported(tt.path)

			// Then the result matches the table.
			require.Equal(t, tt.want, got)
		})
	}
}
