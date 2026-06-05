package main

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// interactiveStderr reports whether the UI stream — stderr, where progress and
// status are drawn — is a real terminal. When it isn't (a pipe, a file
// redirect, /dev/null, CI, a cron job) ordnung prints plain, uncolored status
// instead of animated progress, so logs stay readable and grep-able and nothing
// tries to redraw a line that can't be redrawn.
//
// A genuine TTY check (not merely "is a character device") is used so that
// /dev/null — a character device — is correctly treated as non-interactive,
// which keeps unattended runs from ever stalling on a prompt.
func interactiveStderr() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// progress is a thin wrapper over a pterm progress bar that reports a parallel
// pass live: a single, continuously-updating line showing how far we've got and
// which file is being touched right now, instead of thousands of scrolling log
// lines. When stderr isn't a terminal it degrades to one plain summary line.
//
// Its methods are safe to call from many worker goroutines at once: every call
// into the underlying bar is serialized by mu. Elapsed-time rendering is turned
// off so the bar runs no background redraw goroutine of its own, which makes mu
// the only thing that ever touches it — no data races, no torn output.
type progress struct {
	mu  sync.Mutex
	bar *pterm.ProgressbarPrinter // nil when non-interactive or there's nothing to do
}

// newProgress starts a live bar titled label for total items. With nothing to
// do, or with no terminal to animate on, it starts no bar: in the latter case
// it prints a single plain line so non-interactive runs still say what's
// happening.
func newProgress(label string, total int) *progress {
	if total <= 0 {
		return &progress{}
	}
	if !interactiveStderr() {
		pterm.Info.WithWriter(os.Stderr).Printfln("%s %d file(s)…", label, total)
		return &progress{}
	}
	bar, _ := pterm.DefaultProgressbar.
		WithTotal(total).
		WithTitle(label).
		WithShowElapsedTime(false). // no background re-render goroutine
		WithWriter(os.Stderr).
		Start()
	return &progress{bar: bar}
}

// item announces, on the live line, that name is the file now being processed.
func (p *progress) item(name string) {
	if p.bar == nil {
		return
	}
	p.mu.Lock()
	p.bar.UpdateTitle(shortName(name))
	p.mu.Unlock()
}

// done marks one item finished, advancing the bar by one.
func (p *progress) done() {
	if p.bar == nil {
		return
	}
	p.mu.Lock()
	p.bar.Increment()
	p.mu.Unlock()
}

// stop tears the bar down, leaving the cursor on a fresh line.
func (p *progress) stop() {
	if p.bar == nil {
		return
	}
	p.mu.Lock()
	p.bar.Stop()
	p.mu.Unlock()
}

// shortName is the file's base name, clipped so a long path can't overflow the
// progress line and wrap into a scrolling mess.
func shortName(path string) string {
	const max = 48
	name := filepath.Base(path)
	if r := []rune(name); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return name
}
