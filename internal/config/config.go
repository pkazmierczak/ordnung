// Package config defines ordnung's persistent, user-editable settings and the
// reading/writing of the TOML file that stores them. It keeps the on-disk
// format (and its documentation comments) in one place so both the CLI and the
// first-run wizard agree on defaults, validation, and layout.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

// Config holds the settings ordnung persists between runs. These used to be
// command-line flags; moving them into a file lets the renaming pattern be an
// arbitrary YYYY/MM/DD convention and trims the flag surface to per-run choices.
//
// The toml tags are the on-disk key names; keep them in sync with the template
// written by Save.
type Config struct {
	// Pattern is the renaming convention, built from the YYYY, MM, and DD
	// tokens (see ordnung.ValidatePattern). "/" introduces sub-directories.
	Pattern string `toml:"pattern"`
	// Workers is how many files are read and renamed concurrently.
	Workers int `toml:"workers"`
	// KeepLivePhotos keeps Apple Live Photo .mov parts instead of deleting them
	// when a same-named still sits beside them.
	KeepLivePhotos bool `toml:"keep_live_photos"`
	// Videos enables renaming standalone video files by their embedded date.
	Videos bool `toml:"videos"`
	// DryRun is the default for previewing (versus applying) a run. The -dry
	// flag overrides it per invocation.
	DryRun bool `toml:"dry_run"`
}

// Default returns the built-in configuration used when no file exists yet and
// as the base that a loaded file is layered onto, so any key a user omits keeps
// a sensible value. The defaults match ordnung's historical flag defaults.
func Default() Config {
	return Config{
		Pattern:        ordnung.DefaultPattern,
		Workers:        4,
		KeepLivePhotos: false,
		Videos:         false,
		DryRun:         true,
	}
}

// DefaultPath returns the conventional location of the config file:
// $XDG_CONFIG_HOME/ordnung/config.toml, falling back to ~/.config/ordnung/
// config.toml. This XDG-style path is predictable and easy to hand-edit, which
// matters for a file users are expected to tweak.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "ordnung", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "ordnung", "config.toml"), nil
}

// Load reads and validates the config file at path. It starts from Default() so
// any key omitted from the file keeps its default, then overlays the file's
// values. A parse error or an invalid setting (e.g. a malformed pattern) is
// returned with the path for context.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks that the configuration is internally usable, returning a
// user-facing error otherwise. It is called on load and by the wizard before a
// config is saved.
func (c Config) Validate() error {
	if err := ordnung.ValidatePattern(c.Pattern); err != nil {
		return fmt.Errorf("pattern %q: %w", c.Pattern, err)
	}
	if c.Workers < 1 {
		return fmt.Errorf("workers must be at least 1, got %d", c.Workers)
	}
	return nil
}

// Save writes cfg to path as a commented TOML file, creating the parent
// directory if needed. The file is hand-formatted (rather than machine-encoded)
// so each setting carries documentation a user can read and edit later; Validate
// guards against writing something that couldn't be loaded back.
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating config directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(render(cfg)), 0o644); err != nil {
		return fmt.Errorf("writing config %s: %w", path, err)
	}
	return nil
}

// render produces the commented TOML text for cfg. The comments document each
// setting (and the pattern tokens) so the file is self-explanatory when opened
// in an editor. Patterns are restricted by ValidatePattern to characters that
// are safe inside a TOML basic string, so %q quoting is sufficient.
func render(cfg Config) string {
	return fmt.Sprintf(`# ordnung configuration.
#
# Renaming pattern. Tokens are replaced with the photo's capture date:
#   YYYY  four-digit year   (2021)
#   MM    two-digit month   (07)
#   DD    two-digit day     (15)
# Use "/" to create sub-directories. A "_<time>-NN" suffix and the original
# extension are always appended, so a photo from 15 July 2021 becomes:
#   "YYYY-MM-DD"          -> 2021-07-15_29730-00.jpg
#   "YYYY/MM/DD"          -> 2021/07/15_29730-00.jpg
#   "YYYY/MM/YYYY-MM-DD"  -> 2021/07/2021-07-15_29730-00.jpg
pattern = %q

# How many files to read and rename concurrently.
workers = %d

# Keep Apple Live Photo .mov parts. When false, a .mov is deleted if a
# same-named HEIC or JPG still sits beside it.
keep_live_photos = %t

# Also rename video files (.mov/.mp4/.m4v) using their embedded creation date.
videos = %t

# Default to a dry run (preview only). Override per-run with -dry=false.
dry_run = %t
`, cfg.Pattern, cfg.Workers, cfg.KeepLivePhotos, cfg.Videos, cfg.DryRun)
}
