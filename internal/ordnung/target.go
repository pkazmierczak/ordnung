package ordnung

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// timeSuffix encodes the time of day at which an image was taken as a short,
// fixed-width token that sorts in chronological order.
//
// It counts the seconds elapsed since midnight (0..86399) and formats that as a
// zero-padded, fixed-width decimal number ("00000".."86399"). A larger number
// therefore always yields a sorting-greater string, so an image taken earlier
// in the day always sorts before one taken later.
//
// The token is deliberately all-decimal rather than a denser base-36 encoding.
// macOS Finder (and many file managers) sort "naturally": runs of digits are
// compared as numbers, not byte by byte. A base-36 token mixes digits and
// letters, so the length of its leading digit-run varies between values (e.g.
// "09zz" vs "1abc"), and Finder would compare those runs numerically as 9 vs 1
// — inverting the order. A fixed-width all-decimal token has a single digit-run
// of constant width, so natural and byte-wise ordering agree.
func timeSuffix(t time.Time) string {
	secondsSinceMidnight := t.Hour()*3600 + t.Minute()*60 + t.Second()
	return fmt.Sprintf("%05d", secondsSinceMidnight)
}

// GenerateNewName computes a collision-free target name for the image.
//
// reserved is the set of names already claimed during this run. It is not
// safe for concurrent use: callers must invoke GenerateNewName from a single
// goroutine (see the deterministic phase-2 pass in cmd/ordnung). Assigning
// names single-threaded over a sorted input is what makes the dry-run plan
// match the real run exactly.
//
// The chosen name is guaranteed not to clobber anything: it is skipped if it
// is already reserved by another image or if a *different* file already exists
// at that path on disk. If the only file occupying the target is the image
// itself (i.e. it is already named correctly, or differs only in extension
// case on a case-insensitive filesystem), the name is claimed as-is, which
// makes repeated runs idempotent and order-independent.
func (i *Image) GenerateNewName(pattern string, reserved map[string]struct{}) {
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
	case "YYYY/MM/YYYY-MM-DD":
		datePart = fmt.Sprintf("%s/%s/%s", i.Date.Format("2006"), i.Date.Format("01"), i.Date.Format("2006-01-02"))
	default:
		datePart = i.Date.Format("2006-01-02")
	}

	// Patterns express subdirectories with "/"; translate those to the OS path
	// separator so generated names use native separators (e.g. backslashes on
	// Windows). Without this, names would mix separators and the separator-
	// sensitive collision checks below could miss an already-claimed slot.
	datePart = filepath.FromSlash(datePart)

	// The base name encodes the date (from the pattern) plus a time-derived
	// suffix that keeps same-day images ordered. base sorts chronologically.
	base := fmt.Sprintf("%s%s_%s", dir, datePart, timeSuffix(i.Date))

	// Identify the sources on disk so we can recognize when a candidate name
	// already refers to this very file (the idempotent / case-only-rename case).
	srcInfo, _ := os.Lstat(i.OriginalName)
	var sidecarExt string
	var sidecarSrcInfo os.FileInfo
	if i.Sidecar != "" {
		sidecarExt = strings.ToLower(filepath.Ext(i.Sidecar))
		sidecarSrcInfo, _ = os.Lstat(i.Sidecar)
	}

	// Pick the lowest zero-padded counter at which both the image name and (if
	// present) its sidecar name are free. Same-second images get consecutive
	// counters; the fixed-width "-NN" keeps them lexicographically ordered (a
	// bare "base.ext" would otherwise sort after "base-01.ext").
	//
	// The sidecar must be checked too, not just the image: two files that differ
	// only in extension (e.g. a Live Photo's "IMG.heic" and "IMG.mov", or a still
	// and a video taken in the same second) would otherwise be handed the same
	// counter and collide on a shared "base-NN.xmp" target, since the image names
	// themselves don't clash. Reserving both keeps every target unique.
	for n := 0; ; n++ {
		candidate := fmt.Sprintf("%s-%02d%s", base, n, ext)
		if _, taken := reserved[candidate]; taken {
			continue
		}

		var sidecarCandidate string
		if i.Sidecar != "" {
			sidecarCandidate = strings.TrimSuffix(candidate, ext) + sidecarExt
			if _, taken := reserved[sidecarCandidate]; taken {
				continue
			}
		}

		// A file already occupies a target name. Only claim it if it is that
		// target's own source; otherwise move on so we never overwrite a file.
		if dstInfo, err := os.Lstat(candidate); err == nil {
			if srcInfo == nil || !os.SameFile(srcInfo, dstInfo) {
				continue
			}
		}
		if sidecarCandidate != "" {
			if dstInfo, err := os.Lstat(sidecarCandidate); err == nil {
				if sidecarSrcInfo == nil || !os.SameFile(sidecarSrcInfo, dstInfo) {
					continue
				}
			}
		}

		reserved[candidate] = struct{}{}
		i.NewName = candidate
		if sidecarCandidate != "" {
			reserved[sidecarCandidate] = struct{}{}
			i.SidecarNewName = sidecarCandidate
		}
		return
	}
}
