package ordnung

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/imagetype"
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

	// determines the file type, currently we support JPG and HEIC
	t, err := imagetype.Scan(f)
	if err != nil {
		return err
	}

	if t.IsUnknown() {
		i.Process = false
		return fmt.Errorf("unknown file type: %v, skipping", i.OriginalName)
	}

	exifError := func(err error) error {
		return fmt.Errorf("error processing exif data for %v: %v, using file creation date instead",
			i.OriginalName,
			err,
		)
	}

	dateFromFS := func(filename string) time.Time {
		t, _ := times.Stat(filename)

		if t.HasBirthTime() {
			return t.BirthTime()
		}

		return t.ChangeTime()
	}

	x, err := imagemeta.ScanExif(f)
	if err != nil {
		i.Date = dateFromFS(i.OriginalName)
		return exifError(err)
	}
	tm, err := x.DateTime()
	if err != nil {
		i.Date = dateFromFS(i.OriginalName)
		return exifError(err)
	}

	i.Date = tm

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
			"%s%s%s", path, newName, ext,
		)
		(*newNames)[newName] = 0
	}
}

func (i *Image) Rename() error {
	if err := os.Rename(i.OriginalName, i.NewName); os.IsNotExist(err) {
		dNew, _ := filepath.Split(i.NewName)
		dOrig, _ := filepath.Split(i.OriginalName)
		dOrigPerm, err := os.Stat(dOrig)
		if err != nil {
			return err
		}
		// make sure new files inherit original files' permissions
		err = os.MkdirAll(dNew, dOrigPerm.Mode())
		if err != nil {
			return err
		}
	}
	return nil
}
