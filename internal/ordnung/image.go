// Package ordnung contains the core image model and the logic for deriving a
// capture date, computing a collision-free target name, and applying the
// rename on disk.
package ordnung

import (
	"errors"
	"fmt"
	"os"
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

	m, err := imagemeta.Parse(f)
	if err != nil {
		i.Process = false
		return fmt.Errorf("unable to parse file: %v, skipping", i.OriginalName)
	}

	if m.ImageType().IsUnknown() {
		i.Process = false
		return fmt.Errorf("unknown file type: %v, skipping", i.OriginalName)
	}

	// Prefer the EXIF capture date. Fall back to filesystem times if the image
	// has no EXIF block or no usable DateTime (non-fatal: the file is still
	// processed, but the returned error reports which source was used).
	e, _ := m.Exif()
	if e == nil {
		return i.dateFromFile(f.Name(), errors.New("no EXIF metadata found"))
	}

	exifTime, err := e.DateTime(time.Local)
	if err != nil {
		return i.dateFromFile(f.Name(), err)
	}

	i.Date = exifTime
	return nil
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
