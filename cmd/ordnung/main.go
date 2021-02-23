package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	log "github.com/sirupsen/logrus"

	image "github.com/pkazmierczak/ordnung"
)

func getImages(directory string) ([]*image.Image, error) {
	images := make([]*image.Image, 0)

	jpgRegexp, err := regexp.Compile("^.+\\.(jpg|jpeg|JPG|JPEG)$") // we only support JPG for now
	if err != nil {
		return images, err
	}

	if err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err == nil && jpgRegexp.MatchString(info.Name()) {
			img := image.New(path)
			images = append(images, img)
		}
		return nil
	}); err != nil {
		return images, err
	}

	if len(images) == 0 {
		return images, fmt.Errorf("no files found in %v", directory)
	}

	return images, nil
}

var (
	dryRun    = flag.Bool("dry", true, "Dry run? (only prints a list of pairs: old name -> new name; defaults to true)")
	directory = flag.String("dir", ".", "Directory where to look for images (defaults to pwd)")
	pattern   = flag.String("pattern", "YYYY-MM-DD", "Renaming pattern (defaults to YYYY-MM-DD)")
	loglvl    = flag.String("log-level", "info", "The log level")
)

func main() {
	flag.Parse()

	// setup logging
	logLevel, err := log.ParseLevel(*loglvl)
	if err != nil {
		logLevel = log.InfoLevel
		log.Warnf("invalid log-level %s, set to %v", *loglvl, log.InfoLevel)
	}
	log.SetLevel(logLevel)

	images, err := getImages(*directory)
	if err != nil {
		log.Fatalf("unable to scan files: %v", err)
	}

	// keep track of all the new names we're creating to avoid conflicts
	newNames := map[string]int{}

	for _, img := range images {
		if err := img.ExtractExifDate(); err != nil {
			log.Warnln(err)
		}
		if img.Process {
			if err := img.GenerateNewName(*pattern, &newNames); err != nil {
				log.Fatal(err)
			}
			if !*dryRun {
				if err := img.Rename(); err != nil {
					log.Errorln(err)
				}
			}
		}
		fmt.Printf("%s ⇒ %s\n", img.OriginalName, img.NewName)
	}
}
