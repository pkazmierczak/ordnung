package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/pterm/pterm"

	"github.com/pkazmierczak/ordnung/internal/config"
	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

// patternPresets are the ready-made renaming conventions the wizard offers
// before the "custom" escape hatch, covering the common flat and dated-folder
// layouts. Any string built from the YYYY/MM/DD tokens is valid; these are just
// a head start.
var patternPresets = []string{
	"YYYY-MM-DD",
	"YYYY/MM/DD",
	"YYYY/MM-DD",
	"YYYY/YYYY-MM-DD",
	"YYYY/MM/YYYY-MM-DD",
}

// wizardSampleDate is the capture date used to render pattern previews: 15 July
// 2021, 08:15:30, whose time-of-day suffix is 29730. It only has to be a fixed,
// recognizable instant so examples are stable and easy to read.
var wizardSampleDate = time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)

// runWizard interactively assembles a Config, reading answers from in and
// writing prompts to out. Every question offers a default that is accepted by
// pressing Enter, so a user can build a sensible config by holding the key down.
// It returns an error only if the input stream ends early (e.g. Ctrl-D), so the
// caller can abort without saving a half-built config.
func runWizard(in io.Reader, out io.Writer) (config.Config, error) {
	r := bufio.NewReader(in)
	cfg := config.Default()

	fmt.Fprintln(out, pterm.FgCyan.Sprint("\nLet's set up ordnung. Press Enter to accept the [default].\n"))

	pattern, err := promptPattern(r, out)
	if err != nil {
		return config.Config{}, err
	}
	cfg.Pattern = pattern

	if cfg.Videos, err = promptYesNo(r, out,
		"Also rename video files (.mov/.mp4/.m4v) by their embedded date?", cfg.Videos); err != nil {
		return config.Config{}, err
	}

	if cfg.KeepLivePhotos, err = promptYesNo(r, out,
		"Keep Apple Live Photo .mov parts (instead of deleting them)?", cfg.KeepLivePhotos); err != nil {
		return config.Config{}, err
	}

	if cfg.Workers, err = promptInt(r, out,
		"How many files to process concurrently?", cfg.Workers); err != nil {
		return config.Config{}, err
	}

	// Guard against a hand-confusing combination slipping through; the prompts
	// already constrain every field, so this should never fire.
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// promptPattern lets the user pick one of patternPresets or enter a custom
// pattern, each shown with a live preview of the name it would produce. A custom
// pattern is re-requested until it validates, so the wizard can't yield an
// unusable convention.
func promptPattern(r *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "Choose a renaming pattern:")
	for i, p := range patternPresets {
		fmt.Fprintf(out, "  %d) %-20s e.g. %s\n", i+1, p, ordnung.PreviewPattern(p, wizardSampleDate))
	}
	custom := len(patternPresets) + 1
	fmt.Fprintf(out, "  %d) Custom…\n", custom)

	for {
		line, err := promptLine(r, out, "Pattern", "1")
		if err != nil {
			return "", err
		}
		choice, err := strconv.Atoi(line)
		if err != nil || choice < 1 || choice > custom {
			fmt.Fprintln(out, pterm.FgRed.Sprintf("Please enter a number between 1 and %d.", custom))
			continue
		}
		if choice <= len(patternPresets) {
			return patternPresets[choice-1], nil
		}
		return promptCustomPattern(r, out)
	}
}

// promptCustomPattern reads a free-form pattern, re-prompting (with the
// validation error) until it is acceptable, then echoes an example of the name
// it produces.
func promptCustomPattern(r *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, `Enter a pattern using YYYY, MM, DD ("/" makes sub-directories), e.g. "YYYY/MM/DD".`)
	for {
		p, err := promptLine(r, out, "Custom pattern", ordnung.DefaultPattern)
		if err != nil {
			return "", err
		}
		if err := ordnung.ValidatePattern(p); err != nil {
			fmt.Fprintln(out, pterm.FgRed.Sprint(err.Error()))
			continue
		}
		fmt.Fprintf(out, "  Example: %s\n", ordnung.PreviewPattern(p, wizardSampleDate))
		return p, nil
	}
}

// promptYesNo asks a yes/no question whose default (shown capitalized in the
// hint) is taken when the user just presses Enter. It re-asks on anything other
// than y/yes/n/no.
func promptYesNo(r *bufio.Reader, out io.Writer, label string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	for {
		line, err := readLine(r, out, fmt.Sprintf("%s %s ", label, hint))
		if err != nil {
			return def, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, pterm.FgRed.Sprint(`Please answer "y" or "n".`))
		}
	}
}

// promptInt asks for a positive integer, defaulting to def on an empty line and
// re-asking until the answer parses and is at least 1.
func promptInt(r *bufio.Reader, out io.Writer, label string, def int) (int, error) {
	for {
		line, err := promptLine(r, out, label, strconv.Itoa(def))
		if err != nil {
			return def, err
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 {
			fmt.Fprintln(out, pterm.FgRed.Sprint("Please enter a whole number of 1 or more."))
			continue
		}
		return n, nil
	}
}

// promptLine prints "label [def]: ", reads one line, and returns it trimmed —
// substituting def when the user just presses Enter.
func promptLine(r *bufio.Reader, out io.Writer, label, def string) (string, error) {
	line, err := readLine(r, out, fmt.Sprintf("%s [%s]: ", label, def))
	if err != nil {
		return "", err
	}
	if trimmed := strings.TrimSpace(line); trimmed != "" {
		return trimmed, nil
	}
	return def, nil
}

// readLine writes prompt to out and reads a single line from r, stripping the
// trailing newline. A read that ends the stream with no content returns the
// error (so callers abort instead of looping forever), while a final line
// lacking a trailing newline is still returned.
func readLine(r *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := r.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if err != nil {
		if err == io.EOF && line != "" {
			return line, nil
		}
		return "", err
	}
	return line, nil
}
