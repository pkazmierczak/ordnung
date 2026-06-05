package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shoenig/test"

	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

// reportIssues must group files by kind in a fixed order, sort each group by
// path, append the cause for skipped files, and keep the non-fatal
// missing-metadata list terse.
func TestReportIssues_GroupsAndSorts(t *testing.T) {
	issues := []*ordnung.ExtractError{
		ordnung.NewExtractError(ordnung.MissingMetadata, "/photos/b.jpg", errors.New("no EXIF metadata found")),
		ordnung.NewExtractError(ordnung.MissingMetadata, "/photos/a.jpg", nil),
		ordnung.NewExtractError(ordnung.Unreadable, "/photos/locked.heic", errors.New("permission denied")),
		ordnung.NewExtractError(ordnung.TimedOut, "/photos/slow.mov", context.DeadlineExceeded),
	}

	var buf bytes.Buffer
	reportIssues(&buf, issues)
	out := buf.String()

	contains := func(sub string) {
		t.Helper()
		test.True(t, strings.Contains(out, sub), test.Sprintf("output missing %q:\n%s", sub, out))
	}

	contains("Metadata scan flagged 4 file(s):")
	contains("Missing capture metadata")
	contains("Could not access")
	contains("Timed out")
	// Skipped entries carry their cause; missing-metadata entries do not.
	contains("/photos/locked.heic (permission denied)")
	contains("/photos/a.jpg")
	test.False(t, strings.Contains(out, "/photos/a.jpg ("),
		test.Sprint("missing-metadata entries should not show a cause"))

	// The non-fatal group is rendered first, and its paths are sorted.
	missingAt := strings.Index(out, "Missing capture metadata")
	accessAt := strings.Index(out, "Could not access")
	test.True(t, missingAt >= 0 && accessAt > missingAt,
		test.Sprint("processed files should be reported before skipped ones"))

	aAt := strings.Index(out, "/photos/a.jpg")
	bAt := strings.Index(out, "/photos/b.jpg")
	test.True(t, aAt >= 0 && bAt > aAt, test.Sprint("paths within a group should be sorted"))
}

// confirmProceed defaults to "no" and only proceeds on an explicit yes.
func TestConfirmProceed(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"  yes  \n", true},
		{"\n", false},
		{"n\n", false},
		{"no\n", false},
		{"", false}, // EOF, no input
		{"maybe\n", false},
	}
	for _, c := range cases {
		var out bytes.Buffer
		got := confirmProceed(strings.NewReader(c.input), &out, "Proceed?")
		test.EqOp(t, c.want, got, test.Sprintf("input %q", c.input))
		test.True(t, strings.Contains(out.String(), "Proceed?"),
			test.Sprint("the prompt should be written to the output"))
	}
}

// reportRenameFailures lists every failed rename, sorted by source path.
func TestReportRenameFailures(t *testing.T) {
	failures := []renameFailure{
		{path: "/photos/b.jpg", err: errors.New("permission denied")},
		{path: "/photos/a.jpg", err: errors.New("refusing to overwrite existing file: /photos/x.jpg")},
	}
	var buf bytes.Buffer
	reportRenameFailures(&buf, failures)
	out := buf.String()

	test.True(t, strings.Contains(out, "Renaming finished with 2 error(s):"),
		test.Sprintf("missing header:\n%s", out))
	test.True(t, strings.Contains(out, "/photos/a.jpg: refusing to overwrite existing file: /photos/x.jpg"),
		test.Sprintf("missing entry:\n%s", out))
	test.True(t, strings.Contains(out, "/photos/b.jpg: permission denied"),
		test.Sprintf("missing entry:\n%s", out))

	aAt := strings.Index(out, "/photos/a.jpg")
	bAt := strings.Index(out, "/photos/b.jpg")
	test.True(t, aAt >= 0 && bAt > aAt, test.Sprint("failures should be sorted by source path"))
}

// countSkipped counts only the fatal kinds; the missing-metadata fallback is
// processed, not skipped.
func TestCountSkipped(t *testing.T) {
	issues := []*ordnung.ExtractError{
		ordnung.NewExtractError(ordnung.MissingMetadata, "/a", nil),
		ordnung.NewExtractError(ordnung.Unreadable, "/b", nil),
		ordnung.NewExtractError(ordnung.Unsupported, "/c", nil),
		ordnung.NewExtractError(ordnung.TimedOut, "/d", nil),
	}
	test.EqOp(t, 3, countSkipped(issues))
	test.EqOp(t, 0, countSkipped(nil))
}

// exitCode is non-zero exactly when something needs attention.
func TestExitCode(t *testing.T) {
	test.EqOp(t, 0, exitCode(0, 0))
	test.EqOp(t, 1, exitCode(1, 0))
	test.EqOp(t, 1, exitCode(0, 1))
	test.EqOp(t, 1, exitCode(2, 3))
}
