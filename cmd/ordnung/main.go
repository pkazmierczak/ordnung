package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/schollz/progressbar/v3"
	log "github.com/sirupsen/logrus"

	"github.com/pkazmierczak/ordnung"
)

var (
	dryRun     = flag.Bool("dry", true, "Dry run? (only prints a list of pairs: old name -> new name)")
	numWorkers = flag.Int("workers", 4, "How many concurrent workers to spawn?")
	pattern    = flag.String("pattern", "YYYY-MM-DD", `Renaming pattern, can be one of the following:
- YYYY-MM-DD
- YYYY/MM/DD
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
			images = append(images, ordnung.New(path))
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	return images
}

// rename images and publish results to a channel
func renameImages(
	done <-chan interface{},
	images <-chan *ordnung.Image,
	reserved map[string]struct{},
	mutex *sync.Mutex,
) <-chan string {
	results := make(chan string)
	go func() {
		defer close(results)
		for img := range images {
			var msg string
			if err := img.ExtractDate(); err != nil {
				log.Warn(err)
			}
			if img.Process {
				img.GenerateNewName(*pattern, reserved, mutex)
				if !*dryRun {
					if err := img.Rename(); err != nil {
						log.Errorln(err)
					}
				}
				msg = fmt.Sprintf("%s ⇨ %s", img.OriginalName, img.NewName)
			}
			// Always emit one signal per consumed image (msg is empty for
			// skipped files) so the consumer can keep the progress bar accurate.
			select {
			case <-done:
				return
			case results <- msg:
			}
		}
	}()
	return results
}

func fanIn(done <-chan interface{}, channels ...<-chan string) <-chan string {
	var wg sync.WaitGroup
	multiplexedStream := make(chan string)

	// combine streams from all the channels
	multiplex := func(c <-chan string) {
		defer wg.Done()
		for i := range c {
			select {
			case <-done:
				return
			case multiplexedStream <- i:
			}
		}
	}

	wg.Add(len(channels))
	for _, c := range channels {
		go multiplex(c)
	}

	go func() {
		wg.Wait()
		close(multiplexedStream)
	}()

	return multiplexedStream
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

	// keep track of all the new names we're creating to avoid conflicts
	reserved := map[string]struct{}{}

	var mutex sync.Mutex
	done := make(chan interface{})
	defer close(done)

	// find image files and create a progress bar
	imageFiles := getImages(arg[0])
	bar := progressbar.Default(int64(len(imageFiles)), "renaming")

	// send all the filenames to a channel
	images := make(chan *ordnung.Image)
	go func() {
		defer close(images)
		for _, i := range imageFiles {
			images <- i
		}
	}()

	// rename concurrently
	workers := make([]<-chan string, *numWorkers)
	for i := 0; i < *numWorkers; i++ {
		workers[i] = renameImages(done, images, reserved, &mutex)
	}

	for r := range fanIn(done, workers...) {
		if *dryRun {
			if r != "" {
				fmt.Println(r)
			}
		} else {
			bar.Add(1)
		}
	}
}
