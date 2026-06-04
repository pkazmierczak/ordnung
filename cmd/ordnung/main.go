package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	loglvl = flag.String("log-level", "info", "The log level")
)

// getImages scans a directory (recursively) and prepares a list of files to
// process
func getImages(directory string) []*ordnung.Image {
	images := make([]*ordnung.Image, 0)
	// only JPG and HEIF images are supported for now (case-insensitive)
	imgRegexp := regexp.MustCompile(`(?i)\.(jpg|jpeg|heic|heif)$`)

	if err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Don't abort the whole walk because one entry was unreadable;
			// log it and keep going.
			log.Warnf("skipping %s: %v", path, err)
			return nil
		}
		if !d.IsDir() && imgRegexp.MatchString(d.Name()) {
			img := ordnung.New(path)
			// Attach an XMP sidecar (same base name) if one sits beside it, so it
			// gets renamed in lockstep with the image.
			img.Sidecar = ordnung.FindSidecar(path)
			images = append(images, img)
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	return images
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

	imageFiles := getImages(arg[0])

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
		}
		return
	}
	applyRenames(toRename, *numWorkers)
}
