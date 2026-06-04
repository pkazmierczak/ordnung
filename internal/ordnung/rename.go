package ordnung

import (
	"fmt"
	"os"
	"path/filepath"
)

// Rename moves the image from its original path to its computed NewName. It is
// a no-op when the file is already correctly named, refuses to overwrite a
// different file, and creates any destination subdirectories required by the
// pattern.
func (i *Image) Rename() error {
	// Nothing to do if the file is already named correctly. Avoids a needless
	// syscall and keeps repeated runs side-effect free.
	if i.NewName == i.OriginalName {
		return nil
	}

	srcInfo, err := os.Lstat(i.OriginalName)
	if err != nil {
		return fmt.Errorf("unable to stat source file %v: %v", i.OriginalName, err)
	}

	// Final safety net: never overwrite a *different* file that already occupies
	// the target name. GenerateNewName should already guarantee this, but the
	// check guards against races and changes made by other processes. SameFile
	// allows a case-only rename (e.g. .JPG -> .jpg) on case-insensitive volumes.
	if dstInfo, err := os.Lstat(i.NewName); err == nil && !os.SameFile(srcInfo, dstInfo) {
		return fmt.Errorf("refusing to overwrite existing file: %v", i.NewName)
	}

	// If the pattern renames into a directory structure, create it, inheriting
	// the permission bits of the source's parent directory.
	dNew, _ := filepath.Split(i.NewName)
	if dNew != "" {
		dOrig, _ := filepath.Split(i.OriginalName)
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

	if err := os.Rename(i.OriginalName, i.NewName); err != nil {
		return fmt.Errorf("error renaming %v to %v: %v", i.OriginalName, i.NewName, err)
	}

	return nil
}
