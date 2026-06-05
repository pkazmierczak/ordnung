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

	"github.com/schollz/progressbar/v3"
	log "github.com/sirupsen/logrus"

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
	loglvl         = flag.String("log-level", "info", "The log level")
	keepLivePhotos = flag.Bool("keep-live-photos", false,
		"Keep Apple Live Photo .mov parts? When false (the default), a .mov is deleted if a same-named HEIC or JPG still sits beside it.")
	includeVideos = flag.Bool("videos", false,
		"Also rename video files (.mov/.mp4/.m4v) using their embedded creation date.")
	assumeYes = flag.Bool("yes", false,
		"Skip the confirmation prompt shown after a scan that reported problems, and proceed anyway.")
)

// liveLink couples a still with its Apple Live Photo video part. The coupling
// is resolved only after capture dates have been read (see resolveLivePhotos),
// so a still that turns out to be undecodable never orphans its video.
type liveLink struct {
	still   *ordnung.Image
	movPath string         // the video part, deleted via the still's Rename
	movImg  *ordnung.Image // the video's own rename entry, or nil when videos aren't being renamed
}

// getImages scans a directory (recursively) and prepares the files to process.
// Stills are collected with any XMP sidecar; when includeVideos is true, videos
// are collected too (each with its own sidecar). When keepLive is false, each
// still is coupled with its Live Photo .mov part via a liveLink for later
// resolution.
//
// Crucially, a Live Photo .mov is NOT excluded here. Whether it is deleted (its
// still decoded) or renamed like any other video (its still failed) can only be
// known after extraction, so the decision is deferred to resolveLivePhotos.
func getImages(directory string, keepLive, includeVideos bool) ([]*ordnung.Image, []liveLink) {
	var stills []*ordnung.Image
	var videos []*ordnung.Image
	var videoInfos []os.FileInfo // parallel to videos, for identity matching

	if err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Don't abort the whole walk because one entry was unreadable;
			// log it and keep going.
			log.Warnf("skipping %s: %v", path, err)
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
		log.Fatal(err)
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

	return append(stills, videos...), links
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
// EXIF-decoding stage, so it runs concurrently. Problems are collected and
// returned (classified as *ordnung.ExtractError) rather than logged inline, so
// the progress bar stays readable and the caller can surface a single grouped
// report once the scan is done.
func extractDates(images []*ordnung.Image, workers int, showBar bool) []*ordnung.ExtractError {
	var bar *progressbar.ProgressBar
	if showBar {
		bar = progressbar.Default(int64(len(images)), "reading metadata")
	} else {
		log.Infof("reading metadata from %d files", len(images))
	}

	var (
		mu     sync.Mutex
		issues []*ordnung.ExtractError
	)
	parallelDo(images, workers, func(img *ordnung.Image) {
		if err := img.ExtractDate(); err != nil {
			var ee *ordnung.ExtractError
			if errors.As(err, &ee) {
				mu.Lock()
				issues = append(issues, ee)
				mu.Unlock()
			} else {
				// Defensive: ExtractDate classifies every failure, but never
				// silently drop an unexpected one.
				log.Warn(err)
			}
		}
		if bar != nil {
			bar.Add(1)
		}
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

// applyRenames performs the renames concurrently. Every target name is unique
// and never equals another image's source, so the renames are independent.
func applyRenames(images []*ordnung.Image, workers int) {
	bar := progressbar.Default(int64(len(images)), "renaming")
	parallelDo(images, workers, func(img *ordnung.Image) {
		if err := img.Rename(); err != nil {
			log.Errorln(err)
		}
		bar.Add(1)
	})
}

func main() {
	flag.Parse()

	// setup logging
	logLevel, err := log.ParseLevel(*loglvl)
	if err != nil {
		logLevel = log.InfoLevel
		log.Warnf("invalid log-level %s, set to %v", *loglvl, log.InfoLevel)
	}
	log.SetLevel(logLevel)

	// custom "help" message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ./ordnung [options] directoryToScan

Options:
`)
		flag.PrintDefaults()
	}

	arg := flag.Args()
	if len(arg) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	imageFiles, liveLinks := getImages(arg[0], *keepLivePhotos, *includeVideos)

	// Phase 1: extract capture dates (parallel, the slow part). Problems are
	// collected rather than logged inline so the progress bar stays readable;
	// they're surfaced together as a report once the scan finishes.
	issues := extractDates(imageFiles, *numWorkers, !*dryRun)

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
		fmt.Fprintln(os.Stderr, "Nothing to rename.")
		return
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
		return
	}

	// A real run that turned up problems pauses for confirmation so the user can
	// review the report above before anything is renamed. The prompt is skipped
	// with -yes, and skipped when stdin isn't a terminal (scripts, CI), so
	// non-interactive use keeps working as before.
	if len(issues) > 0 && !*assumeYes && stdinIsInteractive() {
		if !confirmProceed(os.Stdin, os.Stderr, fmt.Sprintf("Proceed with renaming %d file(s)?", len(toRename))) {
			fmt.Fprintln(os.Stderr, "Aborted; no files were changed.")
			return
		}
	}

	applyRenames(toRename, *numWorkers)
}
