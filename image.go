package ordnung

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/imagetype"
	"gopkg.in/djherbis/times.v1"
)

func dateFromFS(filename string) time.Time {
	t, _ := times.Stat(filename)

	if t.HasBirthTime() {
		return t.BirthTime()
	}

	return t.ChangeTime()
}

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

	exifError := func(err error) error {
		return fmt.Errorf("error processing exif data for %v: %v, using file creation date instead",
			i.OriginalName,
			err,
		)
	}

	x, err := imagemeta.ScanExif(f)
	if err != nil {
		i.ExifDate = dateFromFS(i.OriginalName)
		return exifError(err)
	}
	tm, err := x.DateTime()
	if err != nil {
		i.ExifDate = dateFromFS(i.OriginalName)
		return exifError(err)
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

// GetImages scans a directory (recursively) and returns an array of Image type
// for the images found there. This function will only look for jpg and heic
// files as these are the only file types we support for now.
func GetImages(directory string) ([]*Image, error) {
	images := make([]*Image, 0)

	jpgRegexp, err := regexp.Compile("^.+\\.(jpg|jpeg|JPG|JPEG|heic|HEIC)$")
	if err != nil {
		return images, err
	}

	if err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err == nil && jpgRegexp.MatchString(info.Name()) {
			img := New(path)
			images = append(images, img)
		}
		return nil
	}); err != nil {
		return images, err
	}

	if len(images) == 0 {
		return images, fmt.Errorf("no files found in %v", directory)
	}

	return images, nil
}
