package main

import (
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
)

// getImages scans a directory (recursively) and prepares a list of files to
// process. When keepLive is false, each still is also probed for an Apple Live
// Photo .mov part sharing its base name, which is marked for deletion. When
// includeVideos is true, standalone videos are added as renameable files too
// (each with any XMP sidecar of its own) — except a .mov that a still has
// claimed as its Live Photo part, so a still's video is never both renamed and
// deleted.
//
// Claimed parts are matched by file identity (os.SameFile), not by path string:
// FindLiveVideo probes ".mov" before ".MOV", so on a case-insensitive
// filesystem it reports the lowercase candidate while WalkDir yields the real
// name. A string comparison would miss that, letting the Live Photo video into
// the rename set where it would race its own deletion during apply.
func getImages(directory string, keepLive, includeVideos bool) []*ordnung.Image {
	stills := make([]*ordnung.Image, 0)
	videoPaths := make([]string, 0)
	// FileInfos of the .mov parts a still has claimed for deletion, used to
	// exclude them from standalone video processing by identity.
	var claimed []os.FileInfo

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
			// Unless we're keeping Live Photos, attach a same-named .mov video
			// part so it gets deleted when the still is processed.
			if !keepLive {
				if v := ordnung.FindLiveVideo(path); v != "" {
					img.LiveVideo = v
					if fi, err := os.Lstat(v); err == nil {
						claimed = append(claimed, fi)
					}
				}
			}
			stills = append(stills, img)
		case includeVideos && ordnung.IsVideo(d.Name()):
			videoPaths = append(videoPaths, path)
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	images := stills
	for _, v := range videoPaths {
		// Skip a video that a still has claimed as a Live Photo part to delete.
		if claimedForDeletion(v, claimed) {
			continue
		}
		vid := ordnung.New(v)
		// A standalone video may carry its own XMP sidecar; rename it in
		// lockstep, exactly as for stills.
		vid.Sidecar = ordnung.FindSidecar(v)
		images = append(images, vid)
	}

	return images
}

// claimedForDeletion reports whether path refers to the same file as any of the
// claimed Live Photo video parts. It compares by identity (os.SameFile) so a
// case-only filename difference on a case-insensitive filesystem still matches.
func claimedForDeletion(path string, claimed []os.FileInfo) bool {
	if len(claimed) == 0 {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	for _, c := range claimed {
		if os.SameFile(info, c) {
			return true
		}
	}
	return false
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
// EXIF-decoding stage, so it runs concurrently.
func extractDates(images []*ordnung.Image, workers int, showBar bool) {
	var bar *progressbar.ProgressBar
	if showBar {
		bar = progressbar.Default(int64(len(images)), "reading metadata")
	} else {
		log.Infof("reading metadata from %d files", len(images))
	}

	parallelDo(images, workers, func(img *ordnung.Image) {
		if err := img.ExtractDate(); err != nil {
			log.Warn(err)
		}
		if bar != nil {
			bar.Add(1)
		}
	})
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

	imageFiles := getImages(arg[0], *keepLivePhotos, *includeVideos)

	// Phase 1: extract capture dates (parallel, the slow part).
	extractDates(imageFiles, *numWorkers, !*dryRun)

	// Phase 2: assign target names deterministically (single-threaded).
	toRename := assignNames(imageFiles, *pattern)

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
	applyRenames(toRename, *numWorkers)
}
