package ordnung

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// DefaultPattern is the renaming pattern used when none is configured (or when
// an empty pattern reaches name generation). It mirrors the historical default.
const DefaultPattern = "YYYY-MM-DD"

// patternTokens are the placeholders a pattern may contain, each expanded to a
// component of the capture date. They are deliberately distinct substrings (no
// token is a substring of another) so a single non-overlapping pass can expand
// them unambiguously. Keep this list, expandPattern, and ValidatePattern in
// agreement when adding new tokens.
var patternTokens = []string{"YYYY", "MM", "DD"}

// expandPattern turns a renaming pattern into the date-derived part of a target
// name by substituting each token with the corresponding component of t:
//
//	YYYY -> four-digit year   (2006)
//	MM   -> two-digit month   (01)
//	DD   -> two-digit day     (02)
//
// Substitution is a single non-overlapping pass, so tokens never cascade into
// one another. An empty pattern falls back to DefaultPattern so name generation
// always has something usable. "/" is preserved here and translated to the OS
// path separator by the caller.
func expandPattern(pattern string, t time.Time) string {
	if strings.TrimSpace(pattern) == "" {
		pattern = DefaultPattern
	}
	return strings.NewReplacer(
		"YYYY", t.Format("2006"),
		"MM", t.Format("01"),
		"DD", t.Format("02"),
	).Replace(pattern)
}

// ValidatePattern reports whether pattern is a usable renaming pattern,
// returning a descriptive error if it is not. It is used both when loading a
// config file and by the interactive wizard, so the message is meant to be
// shown to the user.
//
// A pattern is valid when it: is non-empty; contains at least one date token
// (so files don't all collapse onto one name base); uses "/" — not "\" — for
// sub-directories without leading, trailing, or empty segments; never escapes
// the scan directory via "." or ".." segments; and contains no characters that
// are illegal in filenames on common filesystems.
func ValidatePattern(pattern string) error {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return fmt.Errorf("pattern must not be empty")
	}

	if !containsToken(p) {
		return fmt.Errorf("pattern must contain at least one of %s", strings.Join(patternTokens, ", "))
	}

	if strings.Contains(p, `\`) {
		return fmt.Errorf(`use "/" (not "\") to separate sub-directories`)
	}
	// Characters that are illegal (or deeply troublesome) in filenames on
	// macOS, Linux, or Windows. Rejecting them up front keeps generated names
	// portable and prevents surprises like ":" showing up as "/" in Finder.
	if i := strings.IndexAny(p, `:*?"<>|`); i >= 0 {
		return fmt.Errorf("pattern must not contain the character %q", string(p[i]))
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("pattern must not contain control characters")
		}
	}

	if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return fmt.Errorf(`pattern must not start or end with "/"`)
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "":
			return fmt.Errorf(`pattern must not contain empty path segments ("//")`)
		case ".", "..":
			return fmt.Errorf(`pattern must not contain "." or ".." path segments`)
		}
	}

	return nil
}

// containsToken reports whether p contains at least one recognized date token.
func containsToken(p string) bool {
	for _, tok := range patternTokens {
		if strings.Contains(p, tok) {
			return true
		}
	}
	return false
}

// PreviewPattern renders an example target name for a photo captured at t, so a
// user choosing a pattern (in the wizard, or reading the docs) can see what it
// produces. The "_<time>-00" tail and lowercase extension match the real names
// GenerateNewName builds; only the date-derived part comes from the pattern.
func PreviewPattern(pattern string, t time.Time) string {
	datePart := filepath.FromSlash(expandPattern(pattern, t))
	return fmt.Sprintf("%s_%s-00.jpg", datePart, timeSuffix(t))
}
