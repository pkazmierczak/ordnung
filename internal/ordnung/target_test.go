package ordnung

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func TestImage_GenerateNewName(t *testing.T) {
	// Use a fixed date so the time-derived suffix is deterministic. Work inside
	// a temp dir so the on-disk collision checks have a clean slate.
	d := time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.jpg")
	must.NoError(t, os.WriteFile(src, []byte("x"), 0o644))

	suffix := timeSuffix(d)

	tests := []struct {
		name     string
		pattern  string
		reserved map[string]struct{}
		expected string
	}{
		{
			"YYYY-MM-DD unseen",
			"YYYY-MM-DD",
			map[string]struct{}{},
			filepath.Join(dir, fmt.Sprintf("2021-07-15_%s-00.jpg", suffix)),
		},
		{
			"YYYY/MM-DD collision skips to next counter",
			"YYYY/MM-DD",
			map[string]struct{}{
				filepath.Join(dir, "2021/07-15_"+suffix+"-00.jpg"): {},
			},
			filepath.Join(dir, fmt.Sprintf("2021/07-15_%s-01.jpg", suffix)),
		},
		{
			"unsupported pattern falls back to YYYY-MM-DD",
			"MM-DD",
			map[string]struct{}{},
			filepath.Join(dir, fmt.Sprintf("2021-07-15_%s-00.jpg", suffix)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := &Image{OriginalName: src, NewName: src, Date: d, Process: true}
			img.GenerateNewName(tt.pattern, tt.reserved)
			test.Eq(t, tt.expected, img.NewName)
		})
	}
}

// TestGenerateNewName_DoesNotClobberExistingFile is the safety-critical case:
// when the natural target name is already occupied by a *different* file on
// disk, the generator must move to the next counter instead of selecting a
// name that Rename would later overwrite.
func TestGenerateNewName_DoesNotClobberExistingFile(t *testing.T) {
	d := time.Date(2022, 3, 4, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	suffix := timeSuffix(d)

	// A pre-existing photo already occupies the "-00" slot.
	occupied := filepath.Join(dir, fmt.Sprintf("2022-03-04_%s-00.jpg", suffix))
	must.NoError(t, os.WriteFile(occupied, []byte("existing photo"), 0o644))

	src := filepath.Join(dir, "IMG_1234.jpg")
	must.NoError(t, os.WriteFile(src, []byte("new photo"), 0o644))

	img := &Image{OriginalName: src, NewName: src, Date: d, Process: true}
	img.GenerateNewName("YYYY-MM-DD", map[string]struct{}{})

	want := filepath.Join(dir, fmt.Sprintf("2022-03-04_%s-01.jpg", suffix))
	must.Eq(t, want, img.NewName, must.Sprint("generator must skip the occupied slot"))
}

// TestGenerateNewName_AlreadyCorrectlyNamed verifies idempotency: a file that
// already carries its correct name keeps it (claims its own slot) rather than
// being bumped to a higher counter.
func TestGenerateNewName_AlreadyCorrectlyNamed(t *testing.T) {
	d := time.Date(2022, 3, 4, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	suffix := timeSuffix(d)

	correct := filepath.Join(dir, fmt.Sprintf("2022-03-04_%s-00.jpg", suffix))
	must.NoError(t, os.WriteFile(correct, []byte("photo"), 0o644))

	img := &Image{OriginalName: correct, NewName: correct, Date: d, Process: true}
	img.GenerateNewName("YYYY-MM-DD", map[string]struct{}{})

	must.Eq(t, correct, img.NewName, must.Sprint("already-correct name should be kept"))
}

// TestGenerateNewName_SequentialAssignmentIsCollisionFree mirrors the phase-2
// pass: assigning names single-threaded through one shared reserved set gives
// same-second images distinct, consecutively ordered counters.
func TestGenerateNewName_SequentialAssignmentIsCollisionFree(t *testing.T) {
	d := time.Date(2023, 5, 6, 9, 30, 0, 0, time.UTC)
	dir := t.TempDir()
	suffix := timeSuffix(d)

	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jpg")
	must.NoError(t, os.WriteFile(a, []byte("a"), 0o644))
	must.NoError(t, os.WriteFile(b, []byte("b"), 0o644))

	reserved := map[string]struct{}{}
	imgA := &Image{OriginalName: a, NewName: a, Date: d, Process: true}
	imgB := &Image{OriginalName: b, NewName: b, Date: d, Process: true}
	imgA.GenerateNewName("YYYY-MM-DD", reserved)
	imgB.GenerateNewName("YYYY-MM-DD", reserved)

	wantA := filepath.Join(dir, fmt.Sprintf("2023-05-06_%s-00.jpg", suffix))
	wantB := filepath.Join(dir, fmt.Sprintf("2023-05-06_%s-01.jpg", suffix))
	test.Eq(t, wantA, imgA.NewName)
	test.Eq(t, wantB, imgB.NewName)
	test.NotEq(t, imgA.NewName, imgB.NewName, test.Sprint("two images were assigned the same name"))
}

// TestTimeSuffix_LexicographicOrder verifies the core property:
// earlier in the day must produce lexicographically smaller suffixes than
// images taken later, regardless of the order they're processed in.
func TestTimeSuffix_LexicographicOrder(t *testing.T) {
	day := func(h, m, s int) time.Time {
		return time.Date(2021, 7, 15, h, m, s, 0, time.UTC)
	}

	stamps := []time.Time{
		day(0, 0, 0),
		day(8, 15, 30),
		day(12, 0, 0),
		day(18, 42, 10),
		day(23, 59, 59),
	}

	// Suffixes derived from ascending times must themselves be in strictly
	// ascending lexicographic order, all at the fixed width of 4 characters.
	suffixes := make([]string, len(stamps))
	for i, ts := range stamps {
		suffixes[i] = timeSuffix(ts)
		test.EqOp(t, 4, len(suffixes[i]), test.Sprintf("suffix %q is not 4 chars wide", suffixes[i]))
	}
	test.AscendingFunc(t, suffixes, func(a, b string) bool { return a < b })
}
