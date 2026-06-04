package ordnung

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// TestFindSidecar covers locating an XMP sidecar that shares the image's base
// name, and returning "" when there is none.
func TestFindSidecar(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "IMG_1234.jpg")
	xmp := filepath.Join(dir, "IMG_1234.xmp")
	must.NoError(t, os.WriteFile(img, []byte("photo"), 0o644))

	test.Eq(t, "", FindSidecar(img), test.Sprint("no sidecar should be found yet"))

	must.NoError(t, os.WriteFile(xmp, []byte("edits"), 0o644))
	test.Eq(t, xmp, FindSidecar(img), test.Sprint("sidecar sharing the base name should be found"))
}

// TestGenerateNewName_SetsSidecarTarget verifies that an image carrying a
// sidecar gets a SidecarNewName that mirrors NewName but keeps the (lowercased)
// sidecar extension.
func TestGenerateNewName_SetsSidecarTarget(t *testing.T) {
	d := time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)
	dir := t.TempDir()
	suffix := timeSuffix(d)

	src := filepath.Join(dir, "IMG_1234.jpg")
	must.NoError(t, os.WriteFile(src, []byte("photo"), 0o644))

	img := &Image{
		OriginalName: src,
		NewName:      src,
		Date:         d,
		Process:      true,
		Sidecar:      filepath.Join(dir, "IMG_1234.xmp"),
	}
	img.GenerateNewName("YYYY-MM-DD", map[string]struct{}{})

	wantImg := filepath.Join(dir, "2021-07-15_"+suffix+"-00.jpg")
	wantSidecar := filepath.Join(dir, "2021-07-15_"+suffix+"-00.xmp")
	test.Eq(t, wantImg, img.NewName)
	test.Eq(t, wantSidecar, img.SidecarNewName, test.Sprint("sidecar should share the image's base name and counter"))
}

// TestGenerateNewName_NoSidecarLeavesTargetEmpty makes sure we don't invent a
// sidecar target for images that have none.
func TestGenerateNewName_NoSidecarLeavesTargetEmpty(t *testing.T) {
	d := time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)
	dir := t.TempDir()
	src := filepath.Join(dir, "IMG_1234.jpg")
	must.NoError(t, os.WriteFile(src, []byte("photo"), 0o644))

	img := &Image{OriginalName: src, NewName: src, Date: d, Process: true}
	img.GenerateNewName("YYYY-MM-DD", map[string]struct{}{})

	test.Eq(t, "", img.SidecarNewName)
}

// TestRename_MovesSidecarAlongsideImage is the end-to-end case: renaming an
// image with a sidecar must move both files to their new names.
func TestRename_MovesSidecarAlongsideImage(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "IMG_1234.jpg")
	xmp := filepath.Join(dir, "IMG_1234.xmp")
	must.NoError(t, os.WriteFile(img, []byte("photo"), 0o644))
	must.NoError(t, os.WriteFile(xmp, []byte("edits"), 0o644))

	newImg := filepath.Join(dir, "2021-07-15_abcd-00.jpg")
	newXMP := filepath.Join(dir, "2021-07-15_abcd-00.xmp")

	i := &Image{
		OriginalName:   img,
		NewName:        newImg,
		Sidecar:        xmp,
		SidecarNewName: newXMP,
	}
	must.NoError(t, i.Rename())

	test.FileNotExists(t, img)
	test.FileNotExists(t, xmp)
	test.FileContains(t, newImg, "photo")
	test.FileContains(t, newXMP, "edits")
}
