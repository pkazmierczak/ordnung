package ordnung

import (
	"testing"
	"time"

	"github.com/shoenig/test"
)

// sample is a fixed capture instant whose components are easy to recognize in
// expanded patterns: year 2021, month 07, day 15.
var sample = time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)

// TestExpandPattern checks that each token is substituted with the right date
// component, that "/" is preserved (its OS translation happens in the caller),
// and that an empty pattern falls back to the default.
func TestExpandPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"YYYY-MM-DD", "2021-07-15"},
		{"YYYY/MM/DD", "2021/07/15"},
		{"YYYY/MM-DD", "2021/07-15"},
		{"YYYY/YYYY-MM-DD", "2021/2021-07-15"},
		{"YYYY/MM/YYYY-MM-DD", "2021/07/2021-07-15"},
		{"MM-DD", "07-15"},
		{"YYYY", "2021"},
		{"", "2021-07-15"},    // empty falls back to DefaultPattern
		{"   ", "2021-07-15"}, // blank too
	}
	for _, tt := range tests {
		test.Eq(t, tt.want, expandPattern(tt.pattern, sample), test.Sprintf("pattern %q", tt.pattern))
	}
}

// TestValidatePattern_Accepts covers the patterns a user might reasonably enter,
// all of which must validate.
func TestValidatePattern_Accepts(t *testing.T) {
	for _, p := range []string{
		"YYYY-MM-DD",
		"YYYY/MM/DD",
		"YYYY/MM-DD",
		"YYYY/YYYY-MM-DD",
		"YYYY/MM/YYYY-MM-DD",
		"YYYY",
		"MM-DD",
		"  YYYY-MM-DD  ", // surrounding whitespace is trimmed
	} {
		test.NoError(t, ValidatePattern(p), test.Sprintf("pattern %q should be valid", p))
	}
}

// TestValidatePattern_Rejects covers the inputs that must be refused, so the
// wizard and config loader can give a clear error instead of producing unusable
// or unsafe names.
func TestValidatePattern_Rejects(t *testing.T) {
	for _, p := range []string{
		"",                  // empty
		"   ",               // blank
		"photos",            // no date token at all
		`YYYY\MM\DD`,        // backslash separators
		"YYYY:MM:DD",        // illegal character
		"YYYY/MM*",          // illegal character
		"/YYYY/MM/DD",       // leading slash
		"YYYY/MM/DD/",       // trailing slash
		"YYYY//MM",          // empty path segment
		"YYYY/../MM",        // parent traversal
		"YYYY/./MM",         // current-dir segment
		"YYYY-MM-DD\textra", // control character
	} {
		test.Error(t, ValidatePattern(p), test.Sprintf("pattern %q should be rejected", p))
	}
}

// TestPreviewPattern renders an example name for a sample date; it must match
// the real GenerateNewName shape (date part + "_<time>-00" + lowercase ext).
func TestPreviewPattern(t *testing.T) {
	suffix := timeSuffix(sample)
	test.Eq(t, "2021-07-15_"+suffix+"-00.jpg", PreviewPattern("YYYY-MM-DD", sample))
	// Sub-directory separators are rendered with the OS separator, matching the
	// real names; compare against a slash-joined value via filepath below.
	test.StrHasSuffix(t, "_"+suffix+"-00.jpg", PreviewPattern("YYYY/MM/DD", sample))
	test.StrHasPrefix(t, "2021", PreviewPattern("YYYY/MM/DD", sample))
}
