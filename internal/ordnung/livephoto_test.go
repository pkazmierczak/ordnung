package ordnung

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// TestFindLiveVideo covers locating an Apple Live Photo .mov part that shares
// the image's base name, and returning "" when there is none.
func TestFindLiveVideo(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "IMG_1234.heic")
	mov := filepath.Join(dir, "IMG_1234.mov")
	must.NoError(t, os.WriteFile(img, []byte("photo"), 0o644))

	test.Eq(t, "", FindLiveVideo(img), test.Sprint("no live video should be found yet"))

	must.NoError(t, os.WriteFile(mov, []byte("video"), 0o644))
	test.Eq(t, mov, FindLiveVideo(img), test.Sprint("video sharing the base name should be found"))
}

// TestRename_DeletesLiveVideo is the end-to-end case: renaming an image that
// carries a Live Photo video part must move the still and delete the .mov.
func TestRename_DeletesLiveVideo(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "IMG_1234.heic")
	mov := filepath.Join(dir, "IMG_1234.mov")
	must.NoError(t, os.WriteFile(img, []byte("photo"), 0o644))
	must.NoError(t, os.WriteFile(mov, []byte("video"), 0o644))

	newImg := filepath.Join(dir, "2021-07-15_abcd-00.heic")

	i := &Image{
		OriginalName: img,
		NewName:      newImg,
		LiveVideo:    mov,
	}
	must.NoError(t, i.Rename())

	test.FileNotExists(t, img)
	test.FileNotExists(t, mov, test.Sprint("live photo video should be deleted"))
	test.FileContains(t, newImg, "photo")
}

// TestRename_ToleratesAlreadyDeletedLiveVideo makes sure that a missing video
// part (e.g. already removed by a same-named sibling still) is not an error, so
// the operation stays idempotent.
func TestRename_ToleratesAlreadyDeletedLiveVideo(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "IMG_1234.jpg")
	mov := filepath.Join(dir, "IMG_1234.mov") // never created
	must.NoError(t, os.WriteFile(img, []byte("photo"), 0o644))

	newImg := filepath.Join(dir, "2021-07-15_abcd-00.jpg")

	i := &Image{
		OriginalName: img,
		NewName:      newImg,
		LiveVideo:    mov,
	}
	must.NoError(t, i.Rename())

	test.FileContains(t, newImg, "photo")
}
