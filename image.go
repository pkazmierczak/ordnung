package ordnung

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

	m, err := imagemeta.Parse(f)
	if err != nil {
		i.Process = false
		return fmt.Errorf("unable to parse file: %v, skipping", i.OriginalName)
	}

	if m.ImageType().IsUnknown() {
		i.Process = false
		return fmt.Errorf("unknown file type: %v, skipping", i.OriginalName)
	}

	// attempt to access exif metadata
	e, _ := m.Exif()
	if e != nil {
		exifTime, err := e.DateTime(time.Local)
		if err == nil {
			i.Date = exifTime
		}
	} else {
		t, err := times.Stat(f.Name())
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
			i.OriginalName, err,
		)
	}

	return nil
}

// GenerateNewName according to the pattern (checks for duplicate names, too)
func (i *Image) GenerateNewName(pattern string, newNames *map[string]int, mutex *sync.Mutex) {
	mutex.Lock()
	defer mutex.Unlock()

	path, _ := filepath.Split(i.OriginalName)
	ext := strings.ToLower(filepath.Ext(i.OriginalName)) // all extensions lowercase for consistency
	var newName string

	switch pattern {
	case "YYYY-MM-DD":
		newName = i.Date.Format("2006-01-02")
	case "YYYY/MM/DD":
		newName = i.Date.Format("2006/01/02")
	case "YYYY/MM-DD":
		newName = i.Date.Format("2006/01-02")
	case "YYYY/YYYY-MM-DD":
		newName = fmt.Sprintf("%s/%s", i.Date.Format("2006"), i.Date.Format("2006-01-02"))
	default:
		newName = i.Date.Format("2006-01-02")
	}

	// have we set this filename before already?
	if seen, ok := (*newNames)[newName]; ok {
		paddedSeen := fmt.Sprintf("%04d", seen+1)
		i.NewName = fmt.Sprintf(
			"%s%s_%s%s", path, newName, paddedSeen, ext,
		)
		(*newNames)[newName]++
	} else {
		i.NewName = fmt.Sprintf(
			"%s%s_%s%s", path, newName, "0000", ext,
		)
		(*newNames)[newName] = 0
	}
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
