package ordnung

import "fmt"

// ExtractErrorKind classifies why ExtractDate could not read a file's capture
// date cleanly from its embedded metadata. The classification lets the CLI group
// affected files into a single, reviewable report rather than emit a stream of
// interleaved warnings.
type ExtractErrorKind int

const (
	// MissingMetadata means the file carried no usable embedded capture date,
	// so its filesystem creation date was used instead. This is non-fatal: the
	// file is still processed and renamed.
	MissingMetadata ExtractErrorKind = iota
	// Unreadable means the file could not be opened, or its filesystem
	// information could not be read. The file is skipped.
	Unreadable
	// Unsupported means the file's contents could not be decoded, or its type is
	// not one ordnung recognizes. The file is skipped.
	Unsupported
	// TimedOut means reading the metadata exceeded the IO deadline
	// (DefaultIOTimeout). The file is skipped.
	TimedOut
)

// Skipped reports whether a file flagged with this kind is left untouched
// (true), rather than still renamed using a fallback date (false).
func (k ExtractErrorKind) Skipped() bool { return k != MissingMetadata }

// String returns a short, human-readable label for the kind.
func (k ExtractErrorKind) String() string {
	switch k {
	case MissingMetadata:
		return "missing capture metadata"
	case Unreadable:
		return "unreadable"
	case Unsupported:
		return "unsupported or corrupt"
	case TimedOut:
		return "timed out"
	default:
		return "unknown"
	}
}

// ExtractError describes a problem reading a single file's capture date. It
// carries the offending Path and a Kind so callers can present a grouped report,
// and exposes the underlying cause (if any) via Unwrap, so errors.Is/As keep
// working — e.g. a TimedOut error still matches context.DeadlineExceeded.
type ExtractError struct {
	Kind  ExtractErrorKind
	Path  string
	cause error
}

// NewExtractError builds a classified extraction error. ordnung's own producers
// populate the struct directly; this is for callers and tests that need to
// construct one.
func NewExtractError(kind ExtractErrorKind, path string, cause error) *ExtractError {
	return &ExtractError{Kind: kind, Path: path, cause: cause}
}

func (e *ExtractError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Path, e.Kind, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Kind)
}

// Unwrap exposes the underlying cause for errors.Is / errors.As.
func (e *ExtractError) Unwrap() error { return e.cause }
