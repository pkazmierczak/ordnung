package image

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rwcarlsen/goexif/exif"
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

	x, err := exif.Decode(f)
	if err != nil {
		i.Process = false
		return fmt.Errorf(
			"error processing %v: %v, skipping", i.OriginalName, err,
		)
	}
	tm, err := x.DateTime()
	if err != nil {
		i.Process = false
		return fmt.Errorf(
			"error processing %v: %v, skipping", i.OriginalName, err,
		)
	}

	i.ExifDate = tm

	return nil
}

// GenerateNewName according to the pattern (checks for duplicate names, too)
func (i *Image) GenerateNewName(pattern string, newNames *map[string]int) error {
	path, _ := filepath.Split(i.OriginalName)
	var newName string

	switch pattern {
	case "YYYY-MM-DD":
		newName = i.ExifDate.Format("2006-01-02")
	default:
		return fmt.Errorf("unrecognized renaming pattern: %v", pattern)
	}

	// have we set this filename before already?
	if seen, ok := (*newNames)[newName]; ok {
		i.NewName = fmt.Sprintf(
			"%s%s_%d.jpg", path, newName, seen+1,
		)
		(*newNames)[newName]++
	} else {
		i.NewName = fmt.Sprintf(
			"%s%s.jpg", path, newName,
		)
		(*newNames)[newName] = 0
	}
	return nil
}
