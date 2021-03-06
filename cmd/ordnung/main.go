package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

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

// getImages scans a directory (recursively) and publishes found files to a
// channel
func getImages(done <-chan interface{}, directory string) <-chan *ordnung.Image {
	images := make(chan *ordnung.Image)
	go func() {
		defer close(images)
		// only JPG and HEIF images are supported for now
		imgRegexp, err := regexp.Compile("^.+\\.(jpg|jpeg|JPG|JPEG|heic|HEIC)$")
		if err != nil {
			log.Fatal(err)
		}

		if err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
			if err == nil && imgRegexp.MatchString(info.Name()) {
				select {
				case <-done:
					return nil
				case images <- ordnung.New(path):
				}
			}
			return nil
		}); err != nil {
			log.Fatal(err)
		}
	}()
	return images
}

// rename images and publish results to a channel
func renameImages(
	done <-chan interface{},
	images <-chan *ordnung.Image,
	newNames map[string]int,
	mutex *sync.Mutex,
) <-chan string {
	results := make(chan string)
	go func() {
		defer close(results)
		for img := range images {
			if err := img.ExtractDate(); err != nil {
				log.Warn(err)
			}
			if img.Process {
				img.GenerateNewName(*pattern, &newNames, mutex)
				if !*dryRun {
					if err := img.Rename(); err != nil {
						log.Errorln(err)
					}
				}
				select {
				case <-done:
					return
				case results <- fmt.Sprintf("%s ⇨ %s", img.OriginalName, img.NewName):
				}
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
	newNames := map[string]int{}

	var mutex sync.Mutex
	done := make(chan interface{})
	defer close(done)

	// find image files
	images := getImages(done, arg[0])

	// rename concurrently
	workers := make([]<-chan string, *numWorkers)
	for i := 0; i < *numWorkers; i++ {
		workers[i] = renameImages(done, images, newNames, &mutex)
	}

	for r := range fanIn(done, workers...) {
		fmt.Println(r)
	}
}
