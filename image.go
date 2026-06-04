package ordnung

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evanoberholster/imagemeta"
	"gopkg.in/djherbis/times.v1"
)

// Image stores all the information about the image we're about to process (or
// not)
type Image struct {
	OriginalName string
	NewName      string
	Date         time.Time
	Process      bool
}

// New creates a new Image object
func New(name string) *Image {
	return &Image{OriginalName: name, NewName: name, Process: true}
}

// ExtractDate extracts EXIF data from images or gets the file creation date if
// it cannot find one
func (i *Image) ExtractDate() error {
	f, err := os.Open(i.OriginalName)
	if err != nil {
		return err
	}
	defer f.Close()

	e, err := imagemeta.Decode(f)
	if err != nil {
		// No EXIF block at all: not fatal, fall back to filesystem times.
		if errors.Is(err, imagemeta.ErrNoExif) {
			return i.dateFromFile(f.Name(), err)
		}
		// Unrecognized image type: nothing we can do, skip it.
		if errors.Is(err, imagemeta.ErrImageTypeNotFound) {
			i.Process = false
			return fmt.Errorf("unknown file type: %v, skipping", i.OriginalName)
		}
		i.Process = false
		return fmt.Errorf("unable to parse file: %v, skipping", i.OriginalName)
	}

	if e.ImageType.IsUnknown() {
		i.Process = false
		return fmt.Errorf("unknown file type: %v, skipping", i.OriginalName)
	}

	// Prefer the EXIF capture date (DateTimeOriginal, then CreateDate, then
	// ModifyDate). If none is present, fall back to the filesystem times.
	if d := e.SelectedDate(); !d.IsZero() {
		i.Date = d
		return nil
	}

	return i.dateFromFile(f.Name(), errors.New("no EXIF date found"))
}

// dateFromFile sets i.Date from the filesystem birth (or change) time. It is
// used as a non-fatal fallback: the image is still processed, but the returned
// error informs the caller which date source was used. cause describes why the
// EXIF date was unavailable.
func (i *Image) dateFromFile(name string, cause error) error {
	t, err := times.Stat(name)
	if err != nil {
		i.Process = false
		return fmt.Errorf("unable to stat file: %v, skipping", i.OriginalName)
	}
	if t.HasBirthTime() {
		i.Date = t.BirthTime()
	} else {
		i.Date = t.ChangeTime()
	}
	return fmt.Errorf("error processing exif data for %v: %v, using file creation date instead",
		i.OriginalName, cause,
	)
}

// timeSuffix encodes the time of day at which an image was taken as a short,
// fixed-width token that sorts lexicographically in chronological order.
//
// It works by counting the seconds elapsed since midnight (0..86399) and
// formatting that number in base 36 (digits 0-9 followed by letters a-z),
// left-padded with zeros to a fixed width of 4 characters. Because base 36's
// alphabet is itself in ascending ASCII order and the width is fixed, a larger
// number always yields a lexicographically larger string: an image taken
// earlier in the day therefore always sorts before one taken later, without
// leaking the actual timestamp into the file name.
func timeSuffix(t time.Time) string {
	secondsSinceMidnight := t.Hour()*3600 + t.Minute()*60 + t.Second()
	s := strconv.FormatInt(int64(secondsSinceMidnight), 36)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

// GenerateNewName computes a collision-free target name for the image.
//
// reserved is the shared set of names already claimed during this run; mutex
// guards both the claim-and-reserve step and the on-disk existence checks so
// that concurrent workers can never select the same destination.
//
// The chosen name is guaranteed not to clobber anything: it is skipped if it
// is already reserved by another image or if a *different* file already exists
// at that path on disk. If the only file occupying the target is the image
// itself (i.e. it is already named correctly, or differs only in extension
// case on a case-insensitive filesystem), the name is claimed as-is, which
// makes repeated runs idempotent and order-independent.
func (i *Image) GenerateNewName(pattern string, reserved map[string]struct{}, mutex *sync.Mutex) {
	mutex.Lock()
	defer mutex.Unlock()

	dir, _ := filepath.Split(i.OriginalName)
	ext := strings.ToLower(filepath.Ext(i.OriginalName)) // all extensions lowercase for consistency
	var datePart string

	switch pattern {
	case "YYYY-MM-DD":
		datePart = i.Date.Format("2006-01-02")
	case "YYYY/MM/DD":
		datePart = i.Date.Format("2006/01/02")
	case "YYYY/MM-DD":
		datePart = i.Date.Format("2006/01-02")
	case "YYYY/YYYY-MM-DD":
		datePart = fmt.Sprintf("%s/%s", i.Date.Format("2006"), i.Date.Format("2006-01-02"))
	default:
		datePart = i.Date.Format("2006-01-02")
	}

	// The base name encodes the date (from the pattern) plus a time-derived
	// suffix that keeps same-day images ordered. base sorts chronologically.
	base := fmt.Sprintf("%s%s_%s", dir, datePart, timeSuffix(i.Date))

	// Identify the source on disk so we can recognize when a candidate name
	// already refers to this very file (the idempotent / case-only-rename case).
	srcInfo, _ := os.Lstat(i.OriginalName)

	// Pick the lowest zero-padded counter that is free. Same-second images get
	// consecutive counters; the fixed-width "-NN" keeps them lexicographically
	// ordered (a bare "base.ext" would otherwise sort after "base-01.ext").
	for n := 0; ; n++ {
		candidate := fmt.Sprintf("%s-%02d%s", base, n, ext)

		if _, taken := reserved[candidate]; taken {
			continue
		}

		if dstInfo, err := os.Lstat(candidate); err == nil {
			// A file already occupies this name. Only claim it if it is the
			// source itself; otherwise move on so we never overwrite a photo.
			if srcInfo == nil || !os.SameFile(srcInfo, dstInfo) {
				continue
			}
		}

		reserved[candidate] = struct{}{}
		i.NewName = candidate
		return
	}
}

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
