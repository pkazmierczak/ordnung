package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"

	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

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

// TestGetImages_SeparatesLivePhotosFromStandaloneVideos is the regression test
// for the apply-phase race: a Live Photo .mov must be queued only for deletion
// (via its still), never also as a standalone rename — otherwise one worker
// would move it while another deletes it. A genuinely standalone video must
// still be queued, along with its own sidecar.
func TestGetImages_SeparatesLivePhotosFromStandaloneVideos(t *testing.T) {
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

	images := getImages(dir, false /* keepLive */, true /* includeVideos */)

	// The Live Photo video must not appear as a standalone rename target.
	test.Nil(t, find(images, liveMov), test.Sprint("live photo video must not be queued for renaming"))

	still := find(images, heic)
	must.NotNil(t, still, must.Sprint("the still should be processed"))
	test.Eq(t, liveMov, still.LiveVideo, test.Sprint("the still should claim its live video for deletion"))
	test.Eq(t, heicXMP, still.Sidecar, test.Sprint("the still keeps the shared sidecar"))

	video := find(images, standalone)
	must.NotNil(t, video, must.Sprint("the standalone video should be processed"))
	test.Eq(t, "", video.LiveVideo, test.Sprint("a standalone video has no live-photo part to delete"))
	test.Eq(t, standaloneXMP, video.Sidecar, test.Sprint("a standalone video's own sidecar should be attached"))
}

// TestGetImages_KeepLivePhotosRenamesTheVideo verifies that with -keep-live-photos
// the Live Photo .mov is not deleted but, when -videos is set, is renamed like
// any other video (and the still no longer claims it for deletion).
func TestGetImages_KeepLivePhotosRenamesTheVideo(t *testing.T) {
	dir := t.TempDir()
	heic := filepath.Join(dir, "IMG_1234.heic")
	liveMov := filepath.Join(dir, "IMG_1234.mov")
	touch(t, heic)
	touch(t, liveMov)

	images := getImages(dir, true /* keepLive */, true /* includeVideos */)

	still := find(images, heic)
	must.NotNil(t, still, must.Sprint("the still should be processed"))
	test.Eq(t, "", still.LiveVideo, test.Sprint("with keepLive the still must not claim the video for deletion"))

	video := find(images, liveMov)
	must.NotNil(t, video, must.Sprint("with keepLive+videos the kept video should be renamed"))
}

// TestGetImages_VideosDisabledLeavesVideosAlone confirms that without -videos no
// video is queued for renaming, while Live Photo deletion still happens.
func TestGetImages_VideosDisabledLeavesVideosAlone(t *testing.T) {
	dir := t.TempDir()
	heic := filepath.Join(dir, "IMG_1234.heic")
	liveMov := filepath.Join(dir, "IMG_1234.mov")
	standalone := filepath.Join(dir, "VID_5678.mov")
	touch(t, heic)
	touch(t, liveMov)
	touch(t, standalone)

	images := getImages(dir, false /* keepLive */, false /* includeVideos */)

	test.Nil(t, find(images, liveMov), test.Sprint("no video should be queued for renaming"))
	test.Nil(t, find(images, standalone), test.Sprint("no video should be queued for renaming"))

	still := find(images, heic)
	must.NotNil(t, still, must.Sprint("the still should be processed"))
	test.Eq(t, liveMov, still.LiveVideo, test.Sprint("Live Photo deletion still applies without -videos"))
}
