package ordnung

import (
	"errors"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// A valid image lacking EXIF must yield a non-fatal MissingMetadata issue: the
// image stays processable (renamed with a fallback date), and the returned
// error is a classified *ExtractError.
func TestExtractDate_ClassifiesMissingMetadata(t *testing.T) {
	img := New("testdata/noexif.jpg")
	err := img.ExtractDate()

	var ee *ExtractError
	must.True(t, errors.As(err, &ee), must.Sprint("expected a classified *ExtractError"))
	test.EqOp(t, MissingMetadata, ee.Kind)
	test.False(t, ee.Kind.Skipped(), test.Sprint("missing metadata is non-fatal"))
	test.True(t, img.Process, test.Sprint("an image with a fallback date stays processable"))
}

// A file that cannot be opened must be classified Unreadable and skipped.
func TestExtractDate_ClassifiesUnreadable(t *testing.T) {
	img := New("testdata/does-not-exist.jpg")
	err := img.ExtractDate()

	var ee *ExtractError
	must.True(t, errors.As(err, &ee), must.Sprint("expected a classified *ExtractError"))
	test.EqOp(t, Unreadable, ee.Kind)
	test.True(t, ee.Kind.Skipped(), test.Sprint("an unreadable file is skipped"))
	test.False(t, img.Process, test.Sprint("an unreadable file must not be processed"))
}

// Skipped distinguishes the non-fatal fallback kind from the rest.
func TestExtractErrorKind_Skipped(t *testing.T) {
	test.False(t, MissingMetadata.Skipped())
	test.True(t, Unreadable.Skipped())
	test.True(t, Unsupported.Skipped())
	test.True(t, TimedOut.Skipped())
}

// An ExtractError's underlying cause must remain inspectable via errors.Is/As.
func TestExtractError_Unwrap(t *testing.T) {
	sentinel := errors.New("permission denied")
	err := NewExtractError(Unreadable, "/photos/locked.heic", sentinel)
	test.True(t, errors.Is(err, sentinel), test.Sprint("the cause should be reachable via Unwrap"))
}
