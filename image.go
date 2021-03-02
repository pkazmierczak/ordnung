package image

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/imagetype"
)

// Image stores all the information about the image we're about to process (or
// not)
type Image struct {
	OriginalName string
	NewName      string
	ExifDate     time.Time
	Process      bool
}

// New creates a new Image object
func New(name string) *Image {
	return &Image{OriginalName: name, NewName: name, Process: true}
}

// ExtractExifDate extracts EXIF data from images
func (i *Image) ExtractExifDate() error {
	f, err := os.Open(i.OriginalName)
	if err != nil {
		return err
	}

	// determines the file type, currently we support JPG and HEIC
	t, err := imagetype.Scan(f)
	if err != nil {
		return err
	}

	if t.IsUnknown() {
		i.Process = false
		return fmt.Errorf("unknown file type: %v, skipping", i.OriginalName)
	}

	x, err := imagemeta.ScanExif(f)
	if err != nil {
		i.Process = false
		return fmt.Errorf("error processing %v: %v, skipping", i.OriginalName, err)
	}
	tm, err := x.DateTime()
	if err != nil {
		i.Process = false
		return fmt.Errorf("error processing %v: %v, skipping", i.OriginalName, err)
	}

	i.ExifDate = tm

	return nil
}

// GenerateNewName according to the pattern (checks for duplicate names, too)
func (i *Image) GenerateNewName(pattern string, newNames *map[string]int) {
	path, _ := filepath.Split(i.OriginalName)
	ext := strings.ToLower(filepath.Ext(i.OriginalName)) // all extensions lowercase for consistency
	var newName string

	switch pattern {
	case "YYYY-MM-DD":
		newName = i.ExifDate.Format("2006-01-02")
	case "YYYY/MM/DD":
		newName = i.ExifDate.Format("2006/01/02")
	case "YYYY/MM-DD":
		newName = i.ExifDate.Format("2006/01-02")
	default:
		newName = i.ExifDate.Format("2006-01-02")
	}

	// have we set this filename before already?
	if seen, ok := (*newNames)[newName]; ok {
		i.NewName = fmt.Sprintf(
			"%s%s_%d%s", path, newName, seen+1, ext,
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
