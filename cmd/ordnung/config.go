package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/pterm/pterm"

	"github.com/pkazmierczak/ordnung/internal/config"
)

// configFilePath resolves which config file a run should use and whether the
// user named it explicitly. An explicit -config path is taken as-is; otherwise
// the conventional per-user location is used.
func configFilePath() (path string, explicit bool, err error) {
	if *configPath != "" {
		return *configPath, true, nil
	}
	p, err := config.DefaultPath()
	return p, false, err
}

// resolveConfig returns the effective configuration for a run. It loads an
// existing file, runs the first-run wizard when none exists at the default
// location and we can prompt, and otherwise falls back to built-in defaults so
// non-interactive use (scripts, CI) keeps working without a config file.
func resolveConfig() (config.Config, error) {
	path, explicit, err := configFilePath()
	if err != nil {
		return config.Config{}, err
	}

	switch _, statErr := os.Stat(path); {
	case statErr == nil:
		return config.Load(path)
	case !errors.Is(statErr, fs.ErrNotExist):
		return config.Config{}, fmt.Errorf("reading config %s: %w", path, statErr)
	}

	// No config exists at the resolved path.
	if explicit {
		// The user pointed -config at a missing file; don't silently invent one
		// where they chose — tell them how to create it instead.
		return config.Config{}, fmt.Errorf(
			"config file %s does not exist; create it with \"ordnung -configure -config %s\", or omit -config to use the default",
			path, path)
	}

	// First run at the default location: offer the wizard when we can prompt,
	// but never block unattended runs — fall back to defaults off a terminal.
	if stdinIsInteractive() && interactiveStderr() {
		pterm.Info.WithWriter(os.Stderr).Println("No configuration file found; let's create one.")
		cfg, err := runWizard(os.Stdin, os.Stderr)
		if err != nil {
			return config.Config{}, fmt.Errorf("setup cancelled: %w", err)
		}
		if err := config.Save(path, cfg); err != nil {
			return config.Config{}, err
		}
		pterm.Success.WithWriter(os.Stderr).Printfln("Saved configuration to %s.", path)
		return cfg, nil
	}

	pterm.Info.WithWriter(os.Stderr).Printfln(
		"No configuration file found; using built-in defaults. Run \"ordnung -configure\" to create %s.", path)
	return config.Default(), nil
}

// runConfigure (re)creates the config file through the wizard and returns the
// process exit status. It writes to the -config path when given, otherwise the
// default location, asks before overwriting an existing file, and requires an
// interactive terminal since the wizard must prompt.
func runConfigure() int {
	path, _, err := configFilePath()
	if err != nil {
		pterm.Error.WithWriter(os.Stderr).Println(err.Error())
		return 1
	}
	if !stdinIsInteractive() || !interactiveStderr() {
		pterm.Error.WithWriter(os.Stderr).Println("-configure needs an interactive terminal to ask questions.")
		return 1
	}
	if _, err := os.Stat(path); err == nil {
		if !confirmProceed(os.Stdin, os.Stderr, fmt.Sprintf("%s already exists. Overwrite it?", path)) {
			pterm.Warning.WithWriter(os.Stderr).Println("Left the existing configuration unchanged.")
			return 0
		}
	}
	cfg, err := runWizard(os.Stdin, os.Stderr)
	if err != nil {
		pterm.Error.WithWriter(os.Stderr).Printfln("setup cancelled: %v", err)
		return 1
	}
	if err := config.Save(path, cfg); err != nil {
		pterm.Error.WithWriter(os.Stderr).Println(err.Error())
		return 1
	}
	pterm.Success.WithWriter(os.Stderr).Printfln("Saved configuration to %s.", path)
	return 0
}
