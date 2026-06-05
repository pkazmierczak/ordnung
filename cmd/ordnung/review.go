package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/pterm/pterm"

	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

// reportIssues prints the problems found during the metadata scan to w, grouped
// by kind and sorted by path so the output is stable and scannable. Files that
// were still processed (missing metadata, renamed with a fallback date) are
// listed separately from those that were skipped.
func reportIssues(w io.Writer, issues []*ordnung.ExtractError) {
	groups := make(map[ordnung.ExtractErrorKind][]*ordnung.ExtractError)
	for _, e := range issues {
		groups[e.Kind] = append(groups[e.Kind], e)
	}

	// A fixed render order, with the non-fatal "still processed" group first.
	sections := []struct {
		kind   ordnung.ExtractErrorKind
		header string
	}{
		{ordnung.MissingMetadata, "Missing capture metadata — will use the file's creation date:"},
		{ordnung.Unreadable, "Could not access — skipped:"},
		{ordnung.Unsupported, "Unsupported or corrupt — skipped:"},
		{ordnung.TimedOut, "Timed out — skipped:"},
	}

	fmt.Fprint(w, pterm.FgCyan.Sprintf("\nMetadata scan flagged %d file(s):\n\n", len(issues)))
	for _, s := range sections {
		entries := groups[s.kind]
		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(a, b int) bool { return entries[a].Path < entries[b].Path })

		fmt.Fprintln(w, sectionStyle(s.kind).Sprint(s.header))
		for _, e := range entries {
			// Show the underlying cause for skipped files (e.g. "permission
			// denied"); the missing-metadata list stays terse, just paths.
			if cause := errors.Unwrap(e); cause != nil && s.kind.Skipped() {
				fmt.Fprintf(w, "  %s (%v)\n", e.Path, cause)
			} else {
				fmt.Fprintf(w, "  %s\n", e.Path)
			}
		}
		fmt.Fprintln(w)
	}
}

// sectionStyle picks a color for a report section: yellow for the non-fatal
// missing-metadata group (those files are still renamed) and red for the
// skipped groups. With styling disabled (a pipe, a file, tests) these resolve
// to plain text, so the output is identical to an uncolored print.
func sectionStyle(k ordnung.ExtractErrorKind) pterm.Color {
	if k.Skipped() {
		return pterm.FgRed
	}
	return pterm.FgYellow
}

// confirmProceed asks the user, via out, whether to continue, reading their
// answer from in. The default (an empty line, or EOF) is "no". Only "y"/"yes"
// (case-insensitive) proceeds.
func confirmProceed(in io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprintf(out, "%s [y/N] ", prompt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// stdinIsInteractive reports whether stdin is a terminal we can prompt on. When
// it isn't — a pipe, a file redirect, CI, a cron job — ordnung must not block
// waiting for input, preserving its non-interactive behavior.
func stdinIsInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// renameFailure records a file whose Rename failed, so failures can be reported
// together after the rename pass instead of scrolling past the progress bar.
type renameFailure struct {
	path string
	err  error
}

// walkWarning records a directory entry the scan couldn't read — an unreadable
// sub-directory, a file that vanished mid-walk — so these are reported together
// after the scan rather than logged inline as the walk stumbles on them.
type walkWarning struct {
	path string
	err  error
}

// reportWalkWarnings prints the entries the directory walk could not read to w,
// sorted by path for stable output, grouped with the other post-scan problems.
func reportWalkWarnings(w io.Writer, warnings []walkWarning) {
	sort.Slice(warnings, func(a, b int) bool { return warnings[a].path < warnings[b].path })
	fmt.Fprintln(w, pterm.FgYellow.Sprintf("\nCould not read %d item(s) while scanning:", len(warnings)))
	for _, x := range warnings {
		fmt.Fprintf(w, "  %s (%v)\n", x.path, x.err)
	}
}

// reportRenameFailures prints the files that could not be renamed to w, sorted
// by source path for stable output.
func reportRenameFailures(w io.Writer, failures []renameFailure) {
	sort.Slice(failures, func(a, b int) bool { return failures[a].path < failures[b].path })
	fmt.Fprintln(w, pterm.FgRed.Sprintf("\nRenaming finished with %d error(s):", len(failures)))
	for _, f := range failures {
		fmt.Fprintf(w, "  %s: %v\n", f.path, f.err)
	}
}
