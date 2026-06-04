package ordnung

import (
	"errors"
	"fmt"
	"io/fs"
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

// GenerateNewName according to the pattern (checks for duplicate names, too)
func (i *Image) GenerateNewName(pattern string, newNames *map[string]int, mutex *sync.Mutex) {
	mutex.Lock()
	defer mutex.Unlock()

	path, _ := filepath.Split(i.OriginalName)
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
	base := fmt.Sprintf("%s%s_%s", path, datePart, timeSuffix(i.Date))

	// Two images can share the same base only if they were taken within the
	// same second (EXIF granularity). Every name gets a zero-padded counter so
	// that same-second images stay correctly ordered: a bare "base.ext" would
	// sort *after* "base-01.ext" (since '.' > '-' in ASCII), so the first image
	// of a group needs its "-00" too. The map returns 0 for an unseen base.
	seen := (*newNames)[base]
	i.NewName = fmt.Sprintf("%s-%02d%s", base, seen, ext)
	(*newNames)[base] = seen + 1
}

func (i *Image) Rename() error {

	// check if we need to create directories in order to rename
	dNew, _ := filepath.Split(i.NewName)
	dOrig, fOrig := filepath.Split(i.OriginalName)

	if dNew != "" {

		// if we're renaming into a directory structure, we want to ensure
		// created directories inherit original files permission modes.
		var dirPermissions fs.FileMode
		if dOrig != "" {
			st, _ := os.Stat(dOrig)
			dirPermissions = st.Mode()
		} else {
			st, _ := os.Stat(fOrig)
			dirPermissions = st.Mode()
		}

		if dirPermissions == 0 {
			return fmt.Errorf("unable to stat original file: %v", i.OriginalName)
		}

		// apply directory bitmasks
		dirPermissions = dirPermissions | 0111 | os.ModeDir

		// make sure new files inherit original files' permissions
		if dNew != "" {
			err := os.MkdirAll(dNew, dirPermissions)
			if err != nil {
				return fmt.Errorf("error creating directory %v: %v", dNew, err)
			}
		}
	}

	if err := os.Rename(i.OriginalName, i.NewName); err != nil {
		return err
	}

	return nil
}
