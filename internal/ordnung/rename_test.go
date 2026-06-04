package ordnung

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func TestImage_Rename(t *testing.T) {
	t.Run("renames the file and removes the original", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "old.jpg")
		dst := filepath.Join(dir, "new.jpg")
		must.NoError(t, os.WriteFile(src, []byte("photo"), 0o644))

		img := &Image{OriginalName: src, NewName: dst}
		must.NoError(t, img.Rename())

		test.FileNotExists(t, src)
		test.FileContains(t, dst, "photo")
	})

	t.Run("refuses to overwrite a different existing file", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "old.jpg")
		dst := filepath.Join(dir, "new.jpg")
		must.NoError(t, os.WriteFile(src, []byte("source"), 0o644))
		must.NoError(t, os.WriteFile(dst, []byte("precious"), 0o644))

		img := &Image{OriginalName: src, NewName: dst}
		must.Error(t, img.Rename(), must.Sprint("expected Rename to refuse overwriting an existing file"))

		// Both files must be intact.
		test.FileContains(t, src, "source")
		test.FileContains(t, dst, "precious")
	})

	t.Run("no-op when already correctly named", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "same.jpg")
		must.NoError(t, os.WriteFile(src, []byte("photo"), 0o644))

		img := &Image{OriginalName: src, NewName: src}
		must.NoError(t, img.Rename())

		test.FileContains(t, src, "photo")
	})

	t.Run("creates destination subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "old.jpg")
		dst := filepath.Join(dir, "2021", "2021-07-15_abcd-00.jpg")
		must.NoError(t, os.WriteFile(src, []byte("photo"), 0o644))

		img := &Image{OriginalName: src, NewName: dst}
		must.NoError(t, img.Rename())

		test.FileContains(t, dst, "photo")
	})
}
