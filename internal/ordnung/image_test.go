package ordnung

import (
	"testing"
	"time"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// TestExtractDate_FromEXIF guards against silent decoder regressions: a JPEG
// with EXIF must yield its DateTimeOriginal capture time. The fixture's
// DateTimeOriginal is "2016:10:11 16:59:50"; ExtractDate interprets it in the
// local zone, so we assert the wall-clock components rather than an absolute
// instant to stay timezone-independent.
func TestExtractDate_FromEXIF(t *testing.T) {
	img := New("testdata/withexif.jpg")
	must.NoError(t, img.ExtractDate())
	must.True(t, img.Process, must.Sprint("expected image to be processable"))

	y, mo, d := img.Date.Date()
	h, mi, s := img.Date.Clock()
	test.EqOp(t, 2016, y)
	test.EqOp(t, time.October, mo)
	test.EqOp(t, 11, d)
	test.EqOp(t, 16, h)
	test.EqOp(t, 59, mi)
	test.EqOp(t, 50, s)
}

// TestExtractDate_FallbackNoEXIF verifies that a valid image without EXIF is
// still processed, using the filesystem timestamp, and reports a non-fatal
// error describing the fallback.
func TestExtractDate_FallbackNoEXIF(t *testing.T) {
	img := New("testdata/noexif.jpg")

	must.Error(t, img.ExtractDate(), must.Sprint("expected a non-fatal error reporting the filesystem fallback"))
	test.True(t, img.Process, test.Sprint("expected image to remain processable on fallback"))
	test.False(t, img.Date.IsZero(), test.Sprint("expected a filesystem date to be set on fallback"))
}
