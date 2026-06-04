package ordnung

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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

	// Identify the source on disk so we can recognize when a candidate name
	// already refers to this very file (the idempotent / case-only-rename case).
	srcInfo, _ := os.Lstat(i.OriginalName)

	// Pick the lowest zero-padded counter that is free. Same-second images get
	// consecutive counters; the fixed-width "-NN" keeps them lexicographically
	// ordered (a bare "base.ext" would otherwise sort after "base-01.ext").
	for n := 0; ; n++ {
		candidate := fmt.Sprintf("%s-%02d%s", base, n, ext)

		if _, taken := reserved[candidate]; taken {
			continue
		}

		if dstInfo, err := os.Lstat(candidate); err == nil {
			// A file already occupies this name. Only claim it if it is the
			// source itself; otherwise move on so we never overwrite a photo.
			if srcInfo == nil || !os.SameFile(srcInfo, dstInfo) {
				continue
			}
		}

		reserved[candidate] = struct{}{}
		i.NewName = candidate
		return
	}
}
