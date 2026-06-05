// Package ordnung contains the core image model and the logic for deriving a
// capture date, computing a collision-free target name, and applying the
// rename on disk.
package ordnung

import (
	"context"
	"errors"
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
	return findSibling(imagePath, []string{".xmp", ".XMP"})
}

// FindLiveVideo returns the path to an Apple "Live Photo" video part that
// shares imagePath's base name (e.g. "IMG_1234.heic" -> "IMG_1234.mov"), or ""
// if none exists. Exporting "unmodified originals" from Apple Photos yields a
// HEIC or JPG still alongside a same-named .mov; this locates that video so it
// can be removed. Both extension casings are probed so the lookup works on
// case-sensitive filesystems too.
func FindLiveVideo(imagePath string) string {
	return findSibling(imagePath, []string{".mov", ".MOV"})
}

// findSibling returns the path of a file that shares imagePath's base name but
// uses one of exts (probed in order), or "" if none exists. The stats are
// bounded by DefaultIOTimeout: a lookup that cannot complete in time degrades
// to "" — the same, already-handled outcome as a genuine absence — so a stuck
// stat on a flaky filesystem can't wedge the scan.
func findSibling(imagePath string, exts []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultIOTimeout)
	defer cancel()

	found, err := withTimeout(ctx, func() (string, error) {
		base := strings.TrimSuffix(imagePath, filepath.Ext(imagePath))
		for _, ext := range exts {
			candidate := base + ext
			if candidate == imagePath {
				continue
			}
			if info, err := os.Lstat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
		return "", nil
	})
	if err != nil {
		return ""
	}
	return found
}

// ExtractDate extracts the capture date from the file, falling back to the file
// creation date if it cannot find one. Stills are read via EXIF; videos are
// read from their QuickTime/ISO-BMFF metadata (see extractVideoDate).
//
// The read is bounded by DefaultIOTimeout: a file that cannot be read in time —
// a corrupt image, an unresponsive mount — is skipped (Process becomes false)
// rather than hanging the run. Use ExtractDateContext to supply your own
// deadline.
func (i *Image) ExtractDate() error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultIOTimeout)
	defer cancel()
	return i.ExtractDateContext(ctx)
}

// ExtractDateContext is ExtractDate with a caller-supplied context governing the
// underlying IO. If ctx is cancelled or its deadline passes before the metadata
// has been read, the image is marked unprocessable and a timeout error is
// returned.
func (i *Image) ExtractDateContext(ctx context.Context) error {
	// Read into a snapshot taken before the worker starts. If the read overruns
	// ctx and its goroutine is abandoned, a late finish then writes only to the
	// snapshot and can't race the caller's later use of i. The result is
	// committed to i below, and only when the read beat its deadline.
	snapshot := *i
	res, err := withTimeout(ctx, func() (Image, error) {
		e := snapshot.extractDate()
		return snapshot, e
	})
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		i.Process = false
		return &ExtractError{Kind: TimedOut, Path: i.OriginalName, cause: err}
	}

	i.Date = res.Date
	i.Process = res.Process
	return err
}

// extractDate performs the actual metadata read — EXIF for stills, QuickTime/
// ISO-BMFF for videos — setting i.Date and i.Process. It is the blocking worker
// ExtractDateContext runs under a deadline.
func (i *Image) extractDate() error {
	if IsVideo(i.OriginalName) {
		return i.extractVideoDate()
	}

	f, err := os.Open(i.OriginalName)
	if err != nil {
		i.Process = false
		return &ExtractError{Kind: Unreadable, Path: i.OriginalName, cause: err}
	}
	defer f.Close()

	m, err := imagemeta.Parse(f)
	if err != nil {
		i.Process = false
		return &ExtractError{Kind: Unsupported, Path: i.OriginalName, cause: err}
	}

	if m.ImageType().IsUnknown() {
		i.Process = false
		return &ExtractError{Kind: Unsupported, Path: i.OriginalName, cause: errors.New("unknown file type")}
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
// error (an *ExtractError of kind MissingMetadata) records which date source was
// used. cause describes why the EXIF/embedded date was unavailable.
func (i *Image) dateFromFile(name string, cause error) error {
	t, err := times.Stat(name)
	if err != nil {
		i.Process = false
		return &ExtractError{Kind: Unreadable, Path: i.OriginalName, cause: err}
	}
	if t.HasBirthTime() {
		i.Date = t.BirthTime()
	} else {
		i.Date = t.ChangeTime()
	}
	return &ExtractError{Kind: MissingMetadata, Path: i.OriginalName, cause: cause}
}
