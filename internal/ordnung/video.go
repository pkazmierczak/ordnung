package ordnung

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"time"
)

// quickTimeEpoch is the reference instant for QuickTime/ISO-BMFF timestamps:
// midnight, 1 January 1904, UTC. The movie header (mvhd) stores its creation
// time as a count of seconds elapsed since this moment.
var quickTimeEpoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)

// extractVideoDate sets i.Date from a video's embedded creation time. Like the
// EXIF path it is non-fatal: if the file has no usable metadata it falls back
// to the filesystem creation date (and reports which source was used via the
// returned error), so the video is still processed.
func (i *Image) extractVideoDate() error {
	f, err := os.Open(i.OriginalName)
	if err != nil {
		i.Process = false
		return &ExtractError{Kind: Unreadable, Path: i.OriginalName, cause: err}
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		i.Process = false
		return &ExtractError{Kind: Unreadable, Path: i.OriginalName, cause: err}
	}

	t, err := videoCreationTime(f, info.Size())
	if err != nil {
		return i.dateFromFile(f.Name(), err)
	}

	i.Date = t
	return nil
}

// videoCreationTime reads the movie-header creation time from a QuickTime/
// ISO-BMFF stream (.mov, .mp4, .m4v). It locates the top-level "moov" box and,
// within it, the "mvhd" box, returning the time it records.
//
// QuickTime/ISO-BMFF stores this timestamp as wall-clock seconds since the
// 1904 epoch with no timezone. To stay consistent with how the EXIF path treats
// its (equally timezone-less) DateTimeOriginal — as local time — the wall-clock
// components are reinterpreted in time.Local. This keeps a video's computed day
// aligned with a photo taken at the same moment.
func videoCreationTime(r io.ReaderAt, size int64) (time.Time, error) {
	moovStart, moovEnd, ok, err := findBox(r, 0, size, "moov")
	if err != nil {
		return time.Time{}, err
	}
	if !ok {
		return time.Time{}, errors.New("no moov box found")
	}

	mvhdStart, mvhdEnd, ok, err := findBox(r, moovStart, moovEnd, "mvhd")
	if err != nil {
		return time.Time{}, err
	}
	if !ok {
		return time.Time{}, errors.New("no mvhd box found")
	}

	return mvhdCreationTime(r, mvhdStart, mvhdEnd)
}

// mvhdCreationTime decodes the creation time from an mvhd box payload spanning
// [start, end). The box is a FullBox: a 1-byte version and 3 flag bytes precede
// the fields. creation_time is 64-bit when version == 1, else 32-bit.
func mvhdCreationTime(r io.ReaderAt, start, end int64) (time.Time, error) {
	version, err := readU8(r, start)
	if err != nil {
		return time.Time{}, err
	}

	var secs uint64
	switch version {
	case 1:
		if start+12 > end {
			return time.Time{}, errors.New("mvhd box too small")
		}
		secs, err = readU64(r, start+4)
	default:
		if start+8 > end {
			return time.Time{}, errors.New("mvhd box too small")
		}
		var v uint32
		v, err = readU32(r, start+4)
		secs = uint64(v)
	}
	if err != nil {
		return time.Time{}, err
	}
	if secs == 0 {
		return time.Time{}, errors.New("mvhd creation time is zero")
	}

	wall := quickTimeEpoch.Add(time.Duration(secs) * time.Second).UTC()
	return time.Date(
		wall.Year(), wall.Month(), wall.Day(),
		wall.Hour(), wall.Minute(), wall.Second(), 0,
		time.Local,
	), nil
}

// findBox scans the ISO-BMFF boxes contained in [start, end) and returns the
// payload range [payloadStart, payloadEnd) of the first box whose four-character
// type matches typ. ok is false when no such box is found. Each box header is a
// 32-bit big-endian size followed by a 4-byte type; a size of 1 means a 64-bit
// size follows the type, and a size of 0 means the box runs to end.
func findBox(r io.ReaderAt, start, end int64, typ string) (payloadStart, payloadEnd int64, ok bool, err error) {
	for off := start; off+8 <= end; {
		size32, err := readU32(r, off)
		if err != nil {
			return 0, 0, false, err
		}
		boxType, err := readType(r, off+4)
		if err != nil {
			return 0, 0, false, err
		}

		var headerSize, boxEnd int64
		switch size32 {
		case 0:
			// Box extends to the end of the enclosing region.
			headerSize, boxEnd = 8, end
		case 1:
			if off+16 > end {
				return 0, 0, false, nil
			}
			big, err := readU64(r, off+8)
			if err != nil {
				return 0, 0, false, err
			}
			headerSize, boxEnd = 16, off+int64(big)
		default:
			headerSize, boxEnd = 8, off+int64(size32)
		}

		// Guard against malformed sizes that would loop forever or read past
		// the region.
		if boxEnd <= off || boxEnd > end {
			return 0, 0, false, nil
		}
		if boxType == typ {
			return off + headerSize, boxEnd, true, nil
		}
		off = boxEnd
	}
	return 0, 0, false, nil
}

func readU8(r io.ReaderAt, off int64) (byte, error) {
	var b [1]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return b[0], nil
}

func readU32(r io.ReaderAt, off int64) (uint32, error) {
	var b [4]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b[:]), nil
}

func readU64(r io.ReaderAt, off int64) (uint64, error) {
	var b [8]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b[:]), nil
}

func readType(r io.ReaderAt, off int64) (string, error) {
	var b [4]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return "", err
	}
	return string(b[:]), nil
}
