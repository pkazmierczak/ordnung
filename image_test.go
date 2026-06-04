package ordnung

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestExtractDate_FromEXIF guards against silent decoder regressions: a JPEG
// with EXIF must yield its DateTimeOriginal capture time. The fixture's
// DateTimeOriginal is "2016:10:11 16:59:50"; ExtractDate interprets it in the
// local zone, so we assert the wall-clock components rather than an absolute
// instant to stay timezone-independent.
func TestExtractDate_FromEXIF(t *testing.T) {
	img := New("testdata/withexif.jpg")
	if err := img.ExtractDate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !img.Process {
		t.Fatal("expected image to be processable")
	}
	y, mo, d := img.Date.Date()
	h, mi, s := img.Date.Clock()
	if y != 2016 || mo != time.October || d != 11 || h != 16 || mi != 59 || s != 50 {
		t.Errorf("expected 2016-10-11 16:59:50, got %v", img.Date)
	}
}

// TestExtractDate_FallbackNoEXIF verifies that a valid image without EXIF is
// still processed, using the filesystem timestamp, and reports a non-fatal
// error describing the fallback.
func TestExtractDate_FallbackNoEXIF(t *testing.T) {
	img := New("testdata/noexif.jpg")
	err := img.ExtractDate()
	if err == nil {
		t.Fatal("expected a non-fatal error reporting the filesystem fallback")
	}
	if !img.Process {
		t.Error("expected image to remain processable on fallback")
	}
	if img.Date.IsZero() {
		t.Error("expected a filesystem date to be set on fallback")
	}
}

func TestImage_GenerateNewName(t *testing.T) {
	// Use a fixed date so the time-derived suffix is deterministic. Work inside
	// a temp dir so the on-disk collision checks have a clean slate.
	d := time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

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
			if img.NewName != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, img.NewName)
			}
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
	if err := os.WriteFile(occupied, []byte("existing photo"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "IMG_1234.jpg")
	if err := os.WriteFile(src, []byte("new photo"), 0o644); err != nil {
		t.Fatal(err)
	}

	img := &Image{OriginalName: src, NewName: src, Date: d, Process: true}
	img.GenerateNewName("YYYY-MM-DD", map[string]struct{}{})

	want := filepath.Join(dir, fmt.Sprintf("2022-03-04_%s-01.jpg", suffix))
	if img.NewName != want {
		t.Fatalf("expected generator to skip the occupied slot and pick %v, got %v", want, img.NewName)
	}
}

// TestGenerateNewName_AlreadyCorrectlyNamed verifies idempotency: a file that
// already carries its correct name keeps it (claims its own slot) rather than
// being bumped to a higher counter.
func TestGenerateNewName_AlreadyCorrectlyNamed(t *testing.T) {
	d := time.Date(2022, 3, 4, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	suffix := timeSuffix(d)

	correct := filepath.Join(dir, fmt.Sprintf("2022-03-04_%s-00.jpg", suffix))
	if err := os.WriteFile(correct, []byte("photo"), 0o644); err != nil {
		t.Fatal(err)
	}

	img := &Image{OriginalName: correct, NewName: correct, Date: d, Process: true}
	img.GenerateNewName("YYYY-MM-DD", map[string]struct{}{})

	if img.NewName != correct {
		t.Fatalf("expected already-correct name to be kept (%v), got %v", correct, img.NewName)
	}
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
	writeFile(t, a, "a")
	writeFile(t, b, "b")

	reserved := map[string]struct{}{}
	imgA := &Image{OriginalName: a, NewName: a, Date: d, Process: true}
	imgB := &Image{OriginalName: b, NewName: b, Date: d, Process: true}
	imgA.GenerateNewName("YYYY-MM-DD", reserved)
	imgB.GenerateNewName("YYYY-MM-DD", reserved)

	wantA := filepath.Join(dir, fmt.Sprintf("2023-05-06_%s-00.jpg", suffix))
	wantB := filepath.Join(dir, fmt.Sprintf("2023-05-06_%s-01.jpg", suffix))
	if imgA.NewName != wantA {
		t.Errorf("imgA: expected %v, got %v", wantA, imgA.NewName)
	}
	if imgB.NewName != wantB {
		t.Errorf("imgB: expected %v, got %v", wantB, imgB.NewName)
	}
	if imgA.NewName == imgB.NewName {
		t.Fatal("two images were assigned the same name")
	}
}

// TestTimeSuffix_LexicographicOrder verifies the core property:
// earlier in the day must produce lexicographically smaller suffixes than
// images taken later, regardless of the order they're processed in.
func TestTimeSuffix_LexicographicOrder(t *testing.T) {
	day := func(h, m, s int) time.Time {
		return time.Date(2021, 7, 15, h, m, s, 0, time.UTC)
	}

	times := []time.Time{
		day(0, 0, 0),
		day(8, 15, 30),
		day(12, 0, 0),
		day(18, 42, 10),
		day(23, 59, 59),
	}

	for i := 1; i < len(times); i++ {
		prev := timeSuffix(times[i-1])
		curr := timeSuffix(times[i])
		if !(prev < curr) {
			t.Errorf(
				"expected suffix for %v (%q) to sort before %v (%q)",
				times[i-1].Format("15:04:05"), prev,
				times[i].Format("15:04:05"), curr,
			)
		}
		if len(curr) != 4 {
			t.Errorf("expected fixed-width suffix of 4 chars, got %q", curr)
		}
	}
}

func TestImage_Rename(t *testing.T) {
	t.Run("renames the file and removes the original", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "old.jpg")
		dst := filepath.Join(dir, "new.jpg")
		writeFile(t, src, "photo")

		img := &Image{OriginalName: src, NewName: dst}
		if err := img.Rename(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fileExists(src) {
			t.Errorf("original %v should no longer exist", src)
		}
		if got := readFile(t, dst); got != "photo" {
			t.Errorf("expected renamed file to contain %q, got %q", "photo", got)
		}
	})

	t.Run("refuses to overwrite a different existing file", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "old.jpg")
		dst := filepath.Join(dir, "new.jpg")
		writeFile(t, src, "source")
		writeFile(t, dst, "precious")

		img := &Image{OriginalName: src, NewName: dst}
		if err := img.Rename(); err == nil {
			t.Fatal("expected Rename to refuse overwriting an existing file")
		}
		// Both files must be intact.
		if got := readFile(t, src); got != "source" {
			t.Errorf("source corrupted: got %q", got)
		}
		if got := readFile(t, dst); got != "precious" {
			t.Errorf("destination was overwritten: got %q", got)
		}
	})

	t.Run("no-op when already correctly named", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "same.jpg")
		writeFile(t, src, "photo")

		img := &Image{OriginalName: src, NewName: src}
		if err := img.Rename(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := readFile(t, src); got != "photo" {
			t.Errorf("file should be untouched, got %q", got)
		}
	})

	t.Run("creates destination subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "old.jpg")
		dst := filepath.Join(dir, "2021", "2021-07-15_abcd-00.jpg")
		writeFile(t, src, "photo")

		img := &Image{OriginalName: src, NewName: dst}
		if err := img.Rename(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := readFile(t, dst); got != "photo" {
			t.Errorf("expected file in new subdir to contain %q, got %q", "photo", got)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
