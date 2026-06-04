package ordnung

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// box assembles an ISO-BMFF box (32-bit size + 4-byte type + payload).
func box(typ string, payload []byte) []byte {
	if len(typ) != 4 {
		panic("box type must be 4 bytes")
	}
	b := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(b[:4], uint32(len(b)))
	copy(b[4:8], typ)
	copy(b[8:], payload)
	return b
}

// mvhdPayloadV0 builds a version-0 mvhd payload carrying the given creation
// time (truncated after creation_time; the parser only reads that far).
func mvhdPayloadV0(creation uint32) []byte {
	p := make([]byte, 8)
	// p[0] = version 0, p[1:4] = flags (all zero).
	binary.BigEndian.PutUint32(p[4:8], creation)
	return p
}

// quickTimeSeconds expresses a wall-clock time as seconds since the QuickTime
// 1904 epoch, matching how a file stores it.
func quickTimeSeconds(t time.Time) uint32 {
	return uint32(t.Sub(quickTimeEpoch).Seconds())
}

func TestVideoCreationTime_ReadsMvhd(t *testing.T) {
	// A wall-clock capture time; the parser should hand it back in local time.
	wall := time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)
	mvhd := box("mvhd", mvhdPayloadV0(quickTimeSeconds(wall)))
	moov := box("moov", mvhd)
	// A leading ftyp box exercises the scan skipping past unrelated boxes.
	data := append(box("ftyp", []byte("qt  \x00\x00\x00\x00")), moov...)

	got, err := videoCreationTime(bytes.NewReader(data), int64(len(data)))
	must.NoError(t, err)

	want := time.Date(2021, 7, 15, 8, 15, 30, 0, time.Local)
	test.True(t, got.Equal(want), test.Sprintf("got %v, want %v", got, want))
}

func TestVideoCreationTime_NoMoov(t *testing.T) {
	data := box("ftyp", []byte("qt  \x00\x00\x00\x00"))
	_, err := videoCreationTime(bytes.NewReader(data), int64(len(data)))
	test.Error(t, err, test.Sprint("a stream without a moov box should error"))
}

// TestExtractDate_Video is the end-to-end path: ExtractDate must route a .mov
// through the QuickTime parser and set Date from its mvhd.
func TestExtractDate_Video(t *testing.T) {
	wall := time.Date(2019, 3, 2, 17, 45, 0, 0, time.UTC)
	mvhd := box("mvhd", mvhdPayloadV0(quickTimeSeconds(wall)))
	data := append(box("ftyp", []byte("qt  \x00\x00\x00\x00")), box("moov", mvhd)...)

	dir := t.TempDir()
	path := filepath.Join(dir, "VID_0001.mov")
	must.NoError(t, os.WriteFile(path, data, 0o644))

	img := New(path)
	must.NoError(t, img.ExtractDate())

	want := time.Date(2019, 3, 2, 17, 45, 0, 0, time.Local)
	test.True(t, img.Date.Equal(want), test.Sprintf("got %v, want %v", img.Date, want))
	test.True(t, img.Process, test.Sprint("a readable video should remain processable"))
}

func TestIsVideoAndIsStill(t *testing.T) {
	test.True(t, IsStill("IMG_1234.HEIC"))
	test.True(t, IsStill("photo.jpg"))
	test.False(t, IsStill("clip.mov"))

	test.True(t, IsVideo("clip.MOV"))
	test.True(t, IsVideo("movie.mp4"))
	test.False(t, IsVideo("photo.jpg"))
}
