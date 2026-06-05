package ordnung

import (
	"context"
	"time"
)

// DefaultIOTimeout bounds a single IO-bound operation — reading a file's
// metadata or moving it on disk. A damaged file, an unresponsive network mount,
// or a kernel-level stall can make an otherwise ordinary read or rename block
// forever; rather than let one bad file freeze an entire run, each IO method
// abandons the operation after this long and moves on.
const DefaultIOTimeout = 10 * time.Second

// withTimeout runs fn on its own goroutine and returns its result. If ctx is
// cancelled or its deadline elapses before fn finishes, it instead returns the
// zero value of T together with ctx.Err(), abandoning the goroutine.
//
// The IO ordnung performs (os.Open, ReadAt, os.Lstat, os.Rename, …) is blocking
// and cannot observe a context on its own, so the only way to bound it is to
// run it elsewhere and stop waiting. A timed-out goroutine keeps running until
// its stuck syscall finally returns; the channel is buffered so that abandoned
// goroutine can deliver its late result and exit without leaking.
//
// Because fn may still be running after withTimeout returns, fn must not write
// to state the caller inspects afterwards: have fn compute into locals (or a
// copy) and let the caller commit the result only once withTimeout reports
// success.
func withTimeout[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type outcome struct {
		val T
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		v, err := fn()
		done <- outcome{val: v, err: err}
	}()

	select {
	case o := <-done:
		return o.val, o.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// withTimeoutErr is withTimeout for operations that yield only an error (a move,
// a deletion) rather than a value.
func withTimeoutErr(ctx context.Context, fn func() error) error {
	_, err := withTimeout(ctx, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}
