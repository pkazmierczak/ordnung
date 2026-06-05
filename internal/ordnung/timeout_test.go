package ordnung

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// withTimeout must hand back a fast operation's result untouched.
func TestWithTimeout_CompletesBeforeDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := withTimeout(ctx, func() (int, error) {
		return 42, nil
	})
	must.NoError(t, err)
	test.EqOp(t, 42, got)
}

// withTimeout must surface fn's own error verbatim when fn beats the deadline.
func TestWithTimeout_PropagatesError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sentinel := errors.New("boom")
	_, err := withTimeout(ctx, func() (int, error) {
		return 0, sentinel
	})
	test.True(t, errors.Is(err, sentinel), test.Sprintf("got %v, want the sentinel error", err))
}

// withTimeout must give up with the context's error once the deadline passes,
// returning promptly instead of waiting for the abandoned operation.
func TestWithTimeout_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// The worker blocks until the test ends; the buffered channel inside
	// withTimeout lets it send and exit cleanly once released.
	release := make(chan struct{})
	defer close(release)

	start := time.Now()
	_, err := withTimeout(ctx, func() (int, error) {
		<-release
		return 1, nil
	})
	test.True(t, errors.Is(err, context.DeadlineExceeded), test.Sprintf("got %v, want deadline exceeded", err))
	test.True(t, time.Since(start) < time.Second,
		test.Sprint("withTimeout should return promptly after the deadline, not wait for the worker"))
}

// ExtractDateContext must abandon a read whose context is already past its
// deadline, mark the image unprocessable, and report a timeout error rather
// than blocking.
func TestExtractDateContext_Timeout(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Minute))
	defer cancel()

	img := New("testdata/withexif.jpg")
	err := img.ExtractDateContext(ctx)
	test.True(t, errors.Is(err, context.DeadlineExceeded), test.Sprintf("got %v, want deadline exceeded", err))
	test.False(t, img.Process, test.Sprint("a timed-out image must be skipped"))
}

// RenameContext must report a timeout (and not block) when its context is
// already past its deadline. The source is deliberately absent so the abandoned
// background worker fails fast at its first stat and never mutates the temp dir
// (which would otherwise race t.TempDir's cleanup).
func TestRenameContext_Timeout(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "does-not-exist.jpg")

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Minute))
	defer cancel()

	img := &Image{OriginalName: src, NewName: filepath.Join(dir, "new.jpg")}
	err := img.RenameContext(ctx)
	test.True(t, errors.Is(err, context.DeadlineExceeded), test.Sprintf("got %v, want deadline exceeded", err))
}
