package ordnung

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Rename moves the image to its computed NewName and, if the image has an XMP
// sidecar, moves that to SidecarNewName too. If the image has an associated
// Live Photo video part (LiveVideo), that video is deleted. Each move is a
// no-op when the file is already correctly named, refuses to overwrite a
// different file, and creates any destination subdirectories required by the
// pattern.
//
// The moves are bounded by DefaultIOTimeout so an unresponsive filesystem can't
// hang the run on a single file. Use RenameContext to supply your own deadline.
func (i *Image) Rename() error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultIOTimeout)
	defer cancel()
	return i.RenameContext(ctx)
}

// RenameContext is Rename with a caller-supplied context governing the moves. A
// move that has not completed when ctx is cancelled or times out is abandoned
// and a timeout error is returned; note that, because the underlying os.Rename
// can't itself be cancelled, an abandoned move may still complete in the
// background.
func (i *Image) RenameContext(ctx context.Context) error {
	err := withTimeoutErr(ctx, i.renameFiles)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("timed out renaming %v: %w", i.OriginalName, err)
	}
	return err
}

// renameFiles performs the image move, the optional sidecar move, and the
// optional Live Photo removal. It is the blocking worker RenameContext runs
// under a deadline.
func (i *Image) renameFiles() error {
	if err := moveFile(i.OriginalName, i.NewName); err != nil {
		return err
	}
	if i.Sidecar != "" {
		if err := moveFile(i.Sidecar, i.SidecarNewName); err != nil {
			return err
		}
	}
	if i.LiveVideo != "" {
		// Drop the Live Photo video part. Tolerate an already-absent file so
		// the rare case of two stills (e.g. a HEIC and a JPG) sharing one .mov
		// stays idempotent: the first image deletes it, the second no-ops.
		if err := os.Remove(i.LiveVideo); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("error removing live photo video %v: %v", i.LiveVideo, err)
		}
	}
	return nil
}

// moveFile relocates src to dst. It is a no-op when dst equals src, refuses to
// overwrite a *different* existing file, and creates any parent directories
// required by dst (inheriting the source parent's permission bits).
func moveFile(src, dst string) error {
	// Nothing to do if the file is already named correctly. Avoids a needless
	// syscall and keeps repeated runs side-effect free.
	if dst == src {
		return nil
	}

	srcInfo, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("unable to stat source file %v: %v", src, err)
	}

	// Final safety net: never overwrite a *different* file that already occupies
	// the target name. GenerateNewName should already guarantee this, but the
	// check guards against races and changes made by other processes. SameFile
	// allows a case-only rename (e.g. .JPG -> .jpg) on case-insensitive volumes.
	if dstInfo, err := os.Lstat(dst); err == nil && !os.SameFile(srcInfo, dstInfo) {
		return fmt.Errorf("refusing to overwrite existing file: %v", dst)
	}

	// If the pattern renames into a directory structure, create it, inheriting
	// the permission bits of the source's parent directory.
	dNew, _ := filepath.Split(dst)
	if dNew != "" {
		dOrig, _ := filepath.Split(src)
		if dOrig == "" {
			dOrig = "."
		}
		parent, err := os.Stat(dOrig)
		if err != nil {
			return fmt.Errorf("unable to stat source directory %v: %v", dOrig, err)
		}
		perm := parent.Mode().Perm() | 0o111 // ensure traversable
		if err := os.MkdirAll(dNew, perm); err != nil {
			return fmt.Errorf("error creating directory %v: %v", dNew, err)
		}
	}

	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("error renaming %v to %v: %v", src, dst, err)
	}

	return nil
}
