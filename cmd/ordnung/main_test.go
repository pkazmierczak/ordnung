package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pterm/pterm"
	"github.com/shoenig/test"
	"github.com/shoenig/test/must"

	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

// TestMain disables pterm styling for the whole package so the report builders
// (reportIssues, reportRenameFailures, reportWalkWarnings) emit plain,
// deterministic text. pterm forces color on by default — even off a TTY — which
// would otherwise wrap styled headers in ANSI escapes.
func TestMain(m *testing.M) {
	pterm.DisableStyling()
	os.Exit(m.Run())
}

func touch(t *testing.T, path string) {
	t.Helper()
	must.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
}

// find returns the queued Image whose OriginalName matches path, or nil.
func find(images []*ordnung.Image, path string) *ordnung.Image {
	for _, img := range images {
		if img.OriginalName == path {
			return img
		}
	}
	return nil
}

// TestGetImages_LivePhotoVideoDeletedWhenStillDecodes is the main path: with a
// decodable still, its Live Photo video is queued for deletion (via the still)
// and kept out of the rename set, while a genuinely standalone video — and its
// own sidecar — are queued for renaming.
func TestGetImages_LivePhotoVideoDeletedWhenStillDecodes(t *testing.T) {
	dir := t.TempDir()

	// A Live Photo: still + video part sharing a base name, plus the single
	// shared sidecar that belongs to the still.
	heic := filepath.Join(dir, "IMG_1234.heic")
	heicXMP := filepath.Join(dir, "IMG_1234.xmp")
	liveMov := filepath.Join(dir, "IMG_1234.mov")
	touch(t, heic)
	touch(t, heicXMP)
	touch(t, liveMov)

	// A genuinely standalone video with its own sidecar.
	standalone := filepath.Join(dir, "VID_5678.mov")
	standaloneXMP := filepath.Join(dir, "VID_5678.xmp")
	touch(t, standalone)
	touch(t, standaloneXMP)

	images, links, _ := getImages(dir, false /* keepLive */, true /* includeVideos */)
	// Stills decode successfully (New sets Process=true), so resolve deletes.
	resolveLivePhotos(links)

	still := find(images, heic)
	must.NotNil(t, still, must.Sprint("the still should be processed"))
	test.Eq(t, liveMov, still.LiveVideo, test.Sprint("the still should delete its live video"))
	test.Eq(t, heicXMP, still.Sidecar, test.Sprint("the still keeps the shared sidecar"))

	liveVid := find(images, liveMov)
	must.NotNil(t, liveVid, must.Sprint("the live video is present as an entry"))
	test.False(t, liveVid.Process, test.Sprint("but is excluded from the rename set, deleted instead"))

	standaloneVid := find(images, standalone)
	must.NotNil(t, standaloneVid, must.Sprint("the standalone video should be processed"))
	test.True(t, standaloneVid.Process)
	test.Eq(t, "", standaloneVid.LiveVideo)
	test.Eq(t, standaloneXMP, standaloneVid.Sidecar, test.Sprint("a standalone video's own sidecar should be attached"))
}

// TestGetImages_LivePhotoVideoRenamedWhenStillFails is the regression test for
// orphaning: if the still can't be decoded, its video must NOT be deleted (the
// still never runs) and must instead be renamed like any other video.
func TestGetImages_LivePhotoVideoRenamedWhenStillFails(t *testing.T) {
	dir := t.TempDir()
	heic := filepath.Join(dir, "IMG_1234.heic")
	liveMov := filepath.Join(dir, "IMG_1234.mov")
	touch(t, heic)
	touch(t, liveMov)

	images, links, _ := getImages(dir, false /* keepLive */, true /* includeVideos */)

	// Simulate the still failing to decode (ExtractDate would set Process=false).
	still := find(images, heic)
	must.NotNil(t, still)
	still.Process = false
	resolveLivePhotos(links)

	test.Eq(t, "", still.LiveVideo, test.Sprint("a failed still must not delete its video"))
	liveVid := find(images, liveMov)
	must.NotNil(t, liveVid)
	test.True(t, liveVid.Process, test.Sprint("the video is renamed, not orphaned, when its still fails"))
}

// TestGetImages_KeepLivePhotosRenamesTheVideo verifies that with the
// keep-live-photos setting enabled no link is created, so the Live Photo .mov
// is renamed like any other video and the still never claims it for deletion.
func TestGetImages_KeepLivePhotosRenamesTheVideo(t *testing.T) {
	dir := t.TempDir()
	heic := filepath.Join(dir, "IMG_1234.heic")
	liveMov := filepath.Join(dir, "IMG_1234.mov")
	touch(t, heic)
	touch(t, liveMov)

	images, links, _ := getImages(dir, true /* keepLive */, true /* includeVideos */)
	test.SliceEmpty(t, links, test.Sprint("keepLive should create no deletion links"))
	resolveLivePhotos(links)

	still := find(images, heic)
	must.NotNil(t, still)
	test.Eq(t, "", still.LiveVideo, test.Sprint("with keepLive the still must not claim the video"))

	video := find(images, liveMov)
	must.NotNil(t, video, must.Sprint("with keepLive+videos the kept video should be renamed"))
	test.True(t, video.Process)
}

// TestGetImages_VideosDisabledStillDeletesLivePhoto confirms that with video
// renaming disabled no video is queued for renaming, while Live Photo deletion
// still happens via the still.
func TestGetImages_VideosDisabledStillDeletesLivePhoto(t *testing.T) {
	dir := t.TempDir()
	heic := filepath.Join(dir, "IMG_1234.heic")
	liveMov := filepath.Join(dir, "IMG_1234.mov")
	standalone := filepath.Join(dir, "VID_5678.mov")
	touch(t, heic)
	touch(t, liveMov)
	touch(t, standalone)

	images, links, _ := getImages(dir, false /* keepLive */, false /* includeVideos */)
	resolveLivePhotos(links)

	test.Nil(t, find(images, liveMov), test.Sprint("no video should be queued for renaming"))
	test.Nil(t, find(images, standalone), test.Sprint("no video should be queued for renaming"))

	still := find(images, heic)
	must.NotNil(t, still, must.Sprint("the still should be processed"))
	test.Eq(t, liveMov, still.LiveVideo, test.Sprint("Live Photo deletion still applies without video renaming"))
}
