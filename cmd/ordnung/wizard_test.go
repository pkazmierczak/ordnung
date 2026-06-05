package main

import (
	"bufio"
	"strings"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"

	"github.com/pkazmierczak/ordnung/internal/config"
)

// TestRunWizard_PresetSelections drives the wizard with explicit answers and
// checks the assembled config, including a non-default pattern preset, videos
// on, Live Photos deleted, and a custom worker count.
func TestRunWizard_PresetSelections(t *testing.T) {
	// pattern=2 (YYYY/MM/DD), videos=y, keep-live=n, workers=8
	in := strings.NewReader("2\ny\nn\n8\n")
	var out strings.Builder

	got, err := runWizard(in, &out)
	must.NoError(t, err)

	want := config.Config{
		Pattern:        "YYYY/MM/DD",
		Workers:        8,
		KeepLivePhotos: false,
		Videos:         true,
		DryRun:         true,
	}
	test.Eq(t, want, got)
}

// TestRunWizard_AllDefaults verifies that pressing Enter at every prompt yields
// exactly the built-in default configuration.
func TestRunWizard_AllDefaults(t *testing.T) {
	in := strings.NewReader("\n\n\n\n")
	var out strings.Builder

	got, err := runWizard(in, &out)
	must.NoError(t, err)
	test.Eq(t, config.Default(), got)
}

// TestRunWizard_CustomPatternReprompts checks the custom path: an invalid
// pattern is rejected and re-requested, and the eventual valid one is used.
func TestRunWizard_CustomPatternReprompts(t *testing.T) {
	// pattern=6 (custom), "bogus" (rejected: no token), "YYYY/MM" (accepted),
	// videos=n, keep-live=n, workers default.
	in := strings.NewReader("6\nbogus\nYYYY/MM\nn\nn\n\n")
	var out strings.Builder

	got, err := runWizard(in, &out)
	must.NoError(t, err)
	test.Eq(t, "YYYY/MM", got.Pattern)
	test.Eq(t, 4, got.Workers)
	test.False(t, got.Videos)
	// The rejected pattern's error should have been surfaced to the user.
	test.StrContains(t, out.String(), "at least one of")
}

// TestRunWizard_RepromptsOnBadChoice checks that an out-of-range menu choice is
// rejected and the menu re-asked.
func TestRunWizard_RepromptsOnBadChoice(t *testing.T) {
	// pattern: "9" (out of range) then "1" (YYYY-MM-DD), videos=yes, keep=no, workers=2
	in := strings.NewReader("9\n1\nyes\nno\n2\n")
	var out strings.Builder

	got, err := runWizard(in, &out)
	must.NoError(t, err)
	test.Eq(t, "YYYY-MM-DD", got.Pattern)
	test.True(t, got.Videos)
	test.Eq(t, 2, got.Workers)
}

// TestRunWizard_AbortsOnEOF makes sure an input stream that ends early (e.g.
// Ctrl-D) returns an error rather than looping or saving a half-built config.
func TestRunWizard_AbortsOnEOF(t *testing.T) {
	in := strings.NewReader("") // no answers at all
	var out strings.Builder

	_, err := runWizard(in, &out)
	test.Error(t, err, test.Sprint("an empty input stream must abort the wizard"))
}

// TestPromptInt_RepromptsThenAccepts checks the integer prompt rejects
// non-numbers and values below one before accepting a valid answer.
func TestPromptInt_RepromptsThenAccepts(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("abc\n0\n6\n"))
	var out strings.Builder

	n, err := promptInt(r, &out, "Workers", 4)
	must.NoError(t, err)
	test.Eq(t, 6, n)
}

// TestPromptYesNo_Defaults checks that an empty answer returns the supplied
// default for both polarities.
func TestPromptYesNo_Defaults(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\n"))
	var out strings.Builder
	v, err := promptYesNo(r, &out, "q", true)
	must.NoError(t, err)
	test.True(t, v)

	r = bufio.NewReader(strings.NewReader("\n"))
	out.Reset()
	v, err = promptYesNo(r, &out, "q", false)
	must.NoError(t, err)
	test.False(t, v)
}
