package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

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

	fmt.Fprintf(w, "\nMetadata scan flagged %d file(s):\n\n", len(issues))
	for _, s := range sections {
		entries := groups[s.kind]
		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(a, b int) bool { return entries[a].Path < entries[b].Path })

		fmt.Fprintln(w, s.header)
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
