package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"atomicgo.dev/cursor"
	"github.com/pterm/pterm"

	"github.com/pkazmierczak/ordnung/internal/ordnung"
)

var (
	dryRun     = flag.Bool("dry", true, "Dry run? (only prints a list of pairs: old name -> new name)")
	numWorkers = flag.Int("workers", 4, "How many concurrent workers to spawn?")
	pattern    = flag.String("pattern", "YYYY-MM-DD", `Renaming pattern, can be one of the following:
- YYYY-MM-DD
- YYYY/MM/DD
- YYYY/MM/YYYY-MM-DD
- YYYY/MM-DD`)
	keepLivePhotos = flag.Bool("keep-live-photos", false,
		"Keep Apple Live Photo .mov parts? When false (the default), a .mov is deleted if a same-named HEIC or JPG still sits beside it.")
	includeVideos = flag.Bool("videos", false,
		"Also rename video files (.mov/.mp4/.m4v) using their embedded creation date.")
	assumeYes = flag.Bool("yes", false,
		"Skip the confirmation prompt shown after a scan that reported problems, and proceed anyway.")
	reportOnly = flag.Bool("report-only", false,
		"Scan and print the problem report, then exit without printing a plan or renaming anything (handy with the exit code for auditing a directory in scripts).")
)

// liveLink couples a still with its Apple Live Photo video part. The coupling
// is resolved only after capture dates have been read (see resolveLivePhotos),
// so a still that turns out to be undecodable never orphans its video.
type liveLink struct {
	still   *ordnung.Image
	movPath string         // the video part, deleted via the still's Rename
	movImg  *ordnung.Image // the video's own rename entry, or nil when videos aren't being renamed
}

// findImages runs the directory walk behind a brief live spinner, then resolves
// it into a one-line summary, so scanning a large tree shows activity instead of
// looking like a hang. The work itself is getImages; this only wraps it in
// feedback. With no terminal to animate on it prints a single plain line.
func findImages(dir string) ([]*ordnung.Image, []liveLink, []walkWarning) {
	if !interactiveStderr() {
		images, links, warnings := getImages(dir, *keepLivePhotos, *includeVideos)
		pterm.Info.WithWriter(os.Stderr).Printfln("Found %d file(s) to examine.", len(images))
		return images, links, warnings
	}
	sp, _ := pterm.DefaultSpinner.WithWriter(os.Stderr).Start("Scanning " + dir + " for photos…")
	images, links, warnings := getImages(dir, *keepLivePhotos, *includeVideos)
	sp.Success(fmt.Sprintf("Found %d file(s) to examine.", len(images)))
	return images, links, warnings
}

// getImages scans a directory (recursively) and prepares the files to process.
// Stills are collected with any XMP sidecar; when includeVideos is true, videos
// are collected too (each with its own sidecar). When keepLive is false, each
// still is coupled with its Live Photo .mov part via a liveLink for later
// resolution. Entries the walk can't read are collected as walkWarnings rather
// than logged inline, so the caller can group them with the other problems.
//
// Crucially, a Live Photo .mov is NOT excluded here. Whether it is deleted (its
// still decoded) or renamed like any other video (its still failed) can only be
// known after extraction, so the decision is deferred to resolveLivePhotos.
func getImages(directory string, keepLive, includeVideos bool) ([]*ordnung.Image, []liveLink, []walkWarning) {
	var stills []*ordnung.Image
	var videos []*ordnung.Image
	var videoInfos []os.FileInfo // parallel to videos, for identity matching
	var warnings []walkWarning   // unreadable entries, surfaced with the rest

	if err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Don't abort the whole walk because one entry was unreadable;
			// record it and keep going, so it's grouped into the final report
			// instead of scrolling past the progress bar.
			warnings = append(warnings, walkWarning{path: path, err: err})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		switch {
		case ordnung.IsStill(d.Name()):
			img := ordnung.New(path)
			// Attach an XMP sidecar (same base name) if one sits beside it, so it
			// gets renamed in lockstep with the image.
			img.Sidecar = ordnung.FindSidecar(path)
			stills = append(stills, img)
		case includeVideos && ordnung.IsVideo(d.Name()):
			vid := ordnung.New(path)
			// A standalone video may carry its own XMP sidecar; rename it in
			// lockstep, exactly as for stills.
			vid.Sidecar = ordnung.FindSidecar(path)
			videos = append(videos, vid)
			fi, _ := os.Lstat(path)
			videoInfos = append(videoInfos, fi)
		}
		return nil
	}); err != nil {
		// The walk itself failed (e.g. the root became inaccessible mid-scan).
		// Record it rather than aborting, so anything already found still runs.
		warnings = append(warnings, walkWarning{path: directory, err: err})
	}

	var links []liveLink
	if !keepLive {
		for _, s := range stills {
			v := ordnung.FindLiveVideo(s.OriginalName)
			if v == "" {
				continue
			}
			links = append(links, liveLink{
				still:   s,
				movPath: v,
				movImg:  matchVideo(v, videos, videoInfos),
			})
		}
	}

	return append(stills, videos...), links, warnings
}

// matchVideo returns the renameable video entry that refers to the same file as
// path, or nil. It compares by identity (os.SameFile) so a ".mov" vs ".MOV"
// difference on a case-insensitive filesystem still matches.
func matchVideo(path string, videos []*ordnung.Image, infos []os.FileInfo) *ordnung.Image {
	target, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	for idx, info := range infos {
		if info != nil && os.SameFile(info, target) {
			return videos[idx]
		}
	}
	return nil
}

// resolveLivePhotos decides, now that capture dates have been read, what becomes
// of each Live Photo video part. When its still was successfully processed the
// video is deleted alongside the still (and kept out of the rename set); when
// the still could not be processed the video is left untouched, so it is renamed
// like any other video rather than orphaned.
func resolveLivePhotos(links []liveLink) {
	for _, link := range links {
		if !link.still.Process {
			continue
		}
		link.still.LiveVideo = link.movPath
		if link.movImg != nil {
			link.movImg.Process = false
		}
	}
}

// parallelDo runs fn over every image using the given number of workers. It
// blocks until all images have been processed.
func parallelDo(images []*ordnung.Image, workers int, fn func(*ordnung.Image)) {
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan *ordnung.Image)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for img := range jobs {
				fn(img)
			}
		}()
	}

	for _, img := range images {
		jobs <- img
	}
	close(jobs)
	wg.Wait()
}

// extractDates reads the capture date of every image. This is the expensive,
// EXIF-decoding stage, so it runs concurrently. Progress is reported live — a
// single updating line naming the file currently being read — while problems
// are collected and returned (classified as *ordnung.ExtractError) rather than
// printed inline, so the caller can surface a single grouped report once the
// scan is done.
func extractDates(images []*ordnung.Image, workers int) []*ordnung.ExtractError {
	prog := newProgress("Reading metadata", len(images))
	defer prog.stop()

	var (
		mu     sync.Mutex
		issues []*ordnung.ExtractError
	)
	parallelDo(images, workers, func(img *ordnung.Image) {
		prog.item(img.OriginalName)
		if err := img.ExtractDate(); err != nil {
			var ee *ordnung.ExtractError
			if !errors.As(err, &ee) {
				// ExtractDate classifies every failure, but never silently drop
				// an unexpected one: fold it into the report as unsupported.
				ee = ordnung.NewExtractError(ordnung.Unsupported, img.OriginalName, err)
			}
			mu.Lock()
			issues = append(issues, ee)
			mu.Unlock()
		}
		prog.done()
	})
	return issues
}

// assignNames computes the target name for every processable image. It runs
// single-threaded over a stably sorted slice so the assignment (and therefore
// the entire plan a dry run prints) is fully deterministic and independent of
// worker scheduling. It returns the images that should actually be renamed.
func assignNames(images []*ordnung.Image, pattern string) []*ordnung.Image {
	// Order by capture date, with the original path as a stable tie-break.
	// Only images taken within the same second share a name base, so this
	// fixes their counter assignment ("-00", "-01", ...) deterministically.
	sort.SliceStable(images, func(a, b int) bool {
		ia, ib := images[a], images[b]
		if !ia.Date.Equal(ib.Date) {
			return ia.Date.Before(ib.Date)
		}
		return ia.OriginalName < ib.OriginalName
	})

	reserved := make(map[string]struct{}, len(images))
	toRename := make([]*ordnung.Image, 0, len(images))
	for _, img := range images {
		if !img.Process {
			continue
		}
		img.GenerateNewName(pattern, reserved)
		toRename = append(toRename, img)
	}
	return toRename
}

// applyRenames performs the renames concurrently, reporting progress live. Every
// target name is unique and never equals another image's source, so the renames
// are independent. Failures are collected and returned (rather than printed
// inline) so they can be reported together once the bar is done.
func applyRenames(images []*ordnung.Image, workers int) []renameFailure {
	prog := newProgress("Renaming", len(images))
	defer prog.stop()

	var (
		mu       sync.Mutex
		failures []renameFailure
	)
	parallelDo(images, workers, func(img *ordnung.Image) {
		prog.item(img.OriginalName)
		if err := img.Rename(); err != nil {
			mu.Lock()
			failures = append(failures, renameFailure{path: img.OriginalName, err: err})
			mu.Unlock()
		}
		prog.done()
	})
	return failures
}

// countSkipped returns how many issues left their file unprocessed (i.e. not the
// non-fatal missing-metadata fallback, which is still renamed).
func countSkipped(issues []*ordnung.ExtractError) int {
	n := 0
	for _, e := range issues {
		if e.Kind.Skipped() {
			n++
		}
	}
	return n
}

// exitCode maps the outcome of a run to a process status: non-zero when some
// files were skipped during the scan or some renames failed, so scripts can
// detect that a directory needs attention.
func exitCode(skipped, renameFailures int) int {
	if skipped > 0 || renameFailures > 0 {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run())
}

// run is the program body, returning the process exit status. It is split out
// from main so the exit code flows from one place (and main stays a one-liner).
func run() int {
	// Set Usage before Parse so -h shows the full help, including the exit status.
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ./ordnung [options] directoryToScan

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Exit status:
  0  success; every file was handled
  1  some files were skipped, or some renames failed
`)
	}
	flag.Parse()

	arg := flag.Args()
	if len(arg) != 1 {
		flag.Usage()
		return 1
	}
	dir := arg[0]

	// Decide once how to present ourselves. The live UI is drawn on stderr, so
	// it keys off whether stderr is a terminal — independently of stdout, which
	// carries the machine-readable dry-run plan.
	if interactiveStderr() {
		// The progress bar hides/shows the terminal cursor; point that at stderr
		// too, so a redirected stdout (e.g. `ordnung dir > plan.txt`) stays clean.
		cursor.SetTarget(os.Stderr)
	} else {
		// No terminal to animate on: drop colors and spinners entirely so piped
		// output and CI logs stay plain and grep-able.
		pterm.DisableStyling()
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		pterm.Error.WithWriter(os.Stderr).Printfln("cannot scan %q: not a readable directory", dir)
		return 1
	}

	imageFiles, liveLinks, warnings := findImages(dir)

	// Phase 1: extract capture dates (parallel, the slow part), reporting live.
	// Problems are collected rather than printed inline so they can be surfaced
	// together as a report once the scan finishes.
	issues := extractDates(imageFiles, *numWorkers)
	skipped := countSkipped(issues)
	// Unreadable directory entries from the walk need attention just like
	// skipped files, so they count toward the exit status too.
	needsAttention := skipped + len(warnings)

	// Surface the walk's unreadable entries alongside the per-file problems,
	// rather than letting them scroll past the progress bar.
	if len(warnings) > 0 {
		reportWalkWarnings(os.Stderr, warnings)
	}

	// Report-only: scan, report, and signal via the exit code. No plan, no
	// renames, no Live Photo bookkeeping.
	if *reportOnly {
		if len(issues) > 0 {
			reportIssues(os.Stderr, issues)
		} else if len(warnings) == 0 {
			pterm.Success.WithWriter(os.Stderr).Println("No problems found.")
		}
		return exitCode(needsAttention, 0)
	}

	// Phase 1b: now that dates are known, decide each Live Photo video's fate
	// (delete with its still, or rename it if the still couldn't be processed).
	resolveLivePhotos(liveLinks)

	// Phase 2: assign target names deterministically (single-threaded).
	toRename := assignNames(imageFiles, *pattern)

	// Surface everything the scan flagged as one grouped report, instead of a
	// stream of warnings interleaved with the progress bar.
	if len(issues) > 0 {
		reportIssues(os.Stderr, issues)
	}

	if len(toRename) == 0 {
		pterm.Info.WithWriter(os.Stderr).Println("Nothing to rename.")
		return exitCode(needsAttention, 0)
	}

	// Phase 3: report the plan (dry run) or apply it (parallel).
	if *dryRun {
		for _, img := range toRename {
			fmt.Printf("%s ⇨ %s\n", img.OriginalName, img.NewName)
			if img.Sidecar != "" {
				fmt.Printf("%s ⇨ %s\n", img.Sidecar, img.SidecarNewName)
			}
			if img.LiveVideo != "" {
				fmt.Printf("%s ✗ (live photo video, deleted)\n", img.LiveVideo)
			}
		}
		pterm.Info.WithWriter(os.Stderr).Printfln(
			"Dry run: %d file(s) would be renamed. Re-run with -dry=false to apply.", len(toRename))
		return exitCode(needsAttention, 0)
	}

	// A real run that turned up problems pauses for confirmation so the user can
	// review the report above before anything is renamed. The prompt is skipped
	// with -yes, and skipped when stdin isn't a terminal (scripts, CI), so
	// non-interactive use keeps working as before.
	if len(issues) > 0 && !*assumeYes && stdinIsInteractive() {
		if !confirmProceed(os.Stdin, os.Stderr, fmt.Sprintf("Proceed with renaming %d file(s)?", len(toRename))) {
			pterm.Warning.WithWriter(os.Stderr).Println("Aborted; no files were changed.")
			return 1
		}
	}

	failures := applyRenames(toRename, *numWorkers)
	if len(failures) > 0 {
		reportRenameFailures(os.Stderr, failures)
		pterm.Warning.WithWriter(os.Stderr).Printfln(
			"Renamed %d of %d file(s); %d failed.", len(toRename)-len(failures), len(toRename), len(failures))
	} else {
		pterm.Success.WithWriter(os.Stderr).Printfln("Renamed %d file(s).", len(toRename))
	}

	return exitCode(needsAttention, len(failures))
}
