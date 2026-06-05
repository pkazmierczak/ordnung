package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"

	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

// TestDefault checks the built-in configuration is itself valid and matches
// ordnung's historical flag defaults.
func TestDefault(t *testing.T) {
	cfg := Default()
	must.NoError(t, cfg.Validate())
	test.Eq(t, ordnung.DefaultPattern, cfg.Pattern)
	test.Eq(t, 4, cfg.Workers)
	test.False(t, cfg.KeepLivePhotos)
	test.False(t, cfg.Videos)
	test.True(t, cfg.DryRun)
}

// TestSaveLoadRoundTrip is the central guarantee: a saved config parses back to
// exactly the same values. It also guards the hand-written TOML template
// against drift, since any mismatch between the template and the struct tags
// would surface here.
func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	want := Config{
		Pattern:        "YYYY/MM/YYYY-MM-DD",
		Workers:        8,
		KeepLivePhotos: true,
		Videos:         true,
		DryRun:         false,
	}
	must.NoError(t, Save(path, want))

	got, err := Load(path)
	must.NoError(t, err)
	test.Eq(t, want, got)
}

// TestLoadPartialUsesDefaults verifies that keys omitted from the file keep
// their default values, so a minimal hand-written config still works.
func TestLoadPartialUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	must.NoError(t, os.WriteFile(path, []byte("pattern = \"YYYY/MM/DD\"\n"), 0o644))

	got, err := Load(path)
	must.NoError(t, err)
	test.Eq(t, "YYYY/MM/DD", got.Pattern)
	// Everything else falls back to the defaults.
	test.Eq(t, 4, got.Workers)
	test.False(t, got.Videos)
	test.False(t, got.KeepLivePhotos)
	test.True(t, got.DryRun)
}

// TestLoadRejectsInvalidPattern ensures a hand-edited, unusable pattern is
// reported on load rather than silently producing bad names later.
func TestLoadRejectsInvalidPattern(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	must.NoError(t, os.WriteFile(path, []byte("pattern = \"nope\"\n"), 0o644))

	_, err := Load(path)
	test.Error(t, err, test.Sprint("a pattern with no date token must be rejected"))
}

// TestLoadRejectsBadWorkers ensures a non-positive worker count is rejected.
func TestLoadRejectsBadWorkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	must.NoError(t, os.WriteFile(path, []byte("workers = 0\n"), 0o644))

	_, err := Load(path)
	test.Error(t, err, test.Sprint("workers must be at least 1"))
}

// TestLoadMissingFile surfaces a not-exist error so the CLI can distinguish a
// first run (offer the wizard) from a malformed file (report it).
func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	test.True(t, os.IsNotExist(err), test.Sprint("a missing file should report as not-exist"))
}

// TestSaveRefusesInvalid makes sure Save never writes a config that couldn't be
// loaded back.
func TestSaveRefusesInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	err := Save(path, Config{Pattern: "bogus", Workers: 4, DryRun: true})
	test.Error(t, err)
	_, statErr := os.Stat(path)
	test.True(t, os.IsNotExist(statErr), test.Sprint("nothing should be written for an invalid config"))
}

// TestDefaultPathHonorsXDG checks the documented location logic: XDG_CONFIG_HOME
// wins when set, falling back to ~/.config otherwise.
func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	p, err := DefaultPath()
	must.NoError(t, err)
	test.Eq(t, filepath.Join("/tmp/xdg", "ordnung", "config.toml"), p)

	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	must.NoError(t, err)
	p, err = DefaultPath()
	must.NoError(t, err)
	test.Eq(t, filepath.Join(home, ".config", "ordnung", "config.toml"), p)
}
