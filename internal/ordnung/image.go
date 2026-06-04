// Package ordnung contains the core image model and the logic for deriving a
// capture date, computing a collision-free target name, and applying the
// rename on disk.
package ordnung

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
	"gopkg.in/djherbis/times.v1"
)

var (
	// stillExtRegexp matches the still-image extensions ordnung can read EXIF
	// from (case-insensitive).
	stillExtRegexp = regexp.MustCompile(`(?i)\.(jpg|jpeg|heic|heif)$`)
	// videoExtRegexp matches the video extensions ordnung can read a creation
	// date from (case-insensitive).
	videoExtRegexp = regexp.MustCompile(`(?i)\.(mov|mp4|m4v)$`)
)

// IsStill reports whether path names a supported still-image file, judged by
// its extension.
func IsStill(path string) bool { return stillExtRegexp.MatchString(path) }

// IsVideo reports whether path names a supported video file, judged by its
// extension.
func IsVideo(path string) bool { return videoExtRegexp.MatchString(path) }

// Image stores all the information about the image we're about to process (or
// not)
type Image struct {
	OriginalName string
	NewName      string
	Date         time.Time
	Process      bool

	// Sidecar is the path to an associated XMP sidecar file that shares the
	// image's base name (e.g. "IMG_1234.xmp" next to "IMG_1234.jpg"), or "" if
	// the image has none. Sidecars carry no EXIF data of their own; they follow
	// the image, taking the same target name with their own extension.
	Sidecar string
	// SidecarNewName is the sidecar's computed target, set by GenerateNewName
	// alongside NewName whenever Sidecar is non-empty.
	SidecarNewName string

	// LiveVideo is the path to an associated Apple "Live Photo" video part: a
	// .mov sharing the image's base name (e.g. "IMG_1234.mov" next to
	// "IMG_1234.heic"), or "" if the image has none or Live Photos are being
	// kept. Unlike a sidecar, the video is not renamed but deleted when the
	// image is processed, collapsing an exported Live Photo down to its still.
	LiveVideo string
}

// New creates a new Image object
func New(name string) *Image {
	return &Image{OriginalName: name, NewName: name, Process: true}
}

// FindSidecar returns the path to an XMP sidecar file that shares imagePath's
// base name, or "" if none exists. The sidecar is expected to differ from the
// image only in extension (e.g. "IMG_1234.jpg" -> "IMG_1234.xmp"). Both common
// extension casings are probed so the lookup works on case-sensitive
// filesystems too.
func FindSidecar(imagePath string) string {
	base := strings.TrimSuffix(imagePath, filepath.Ext(imagePath))
	for _, ext := range []string{".xmp", ".XMP"} {
		candidate := base + ext
		if candidate == imagePath {
			continue
		}
		if info, err := os.Lstat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// FindLiveVideo returns the path to an Apple "Live Photo" video part that
// shares imagePath's base name (e.g. "IMG_1234.heic" -> "IMG_1234.mov"), or ""
// if none exists. Exporting "unmodified originals" from Apple Photos yields a
// HEIC or JPG still alongside a same-named .mov; this locates that video so it
// can be removed. Both extension casings are probed so the lookup works on
// case-sensitive filesystems too.
func FindLiveVideo(imagePath string) string {
	base := strings.TrimSuffix(imagePath, filepath.Ext(imagePath))
	for _, ext := range []string{".mov", ".MOV"} {
		candidate := base + ext
		if candidate == imagePath {
			continue
		}
		if info, err := os.Lstat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// ExtractDate extracts the capture date from the file, falling back to the file
// creation date if it cannot find one. Stills are read via EXIF; videos are
// read from their QuickTime/ISO-BMFF metadata (see extractVideoDate).
func (i *Image) ExtractDate() error {
	if IsVideo(i.OriginalName) {
		return i.extractVideoDate()
	}

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
