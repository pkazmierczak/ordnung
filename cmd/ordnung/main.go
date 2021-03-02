package main

import (
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/pkazmierczak/ordnung"
)

var (
	dryRun  = flag.Bool("dry", true, "Dry run? (only prints a list of pairs: old name -> new name)")
	pattern = flag.String("pattern", "YYYY-MM-DD", "Renaming pattern")
	loglvl  = flag.String("log-level", "info", "The log level")
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

	images, err := ordnung.GetImages(arg[0])
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
			img.GenerateNewName(*pattern, &newNames)
			if !*dryRun {
				if err := img.Rename(); err != nil {
					log.Errorln(err)
				}
			}
		}
		fmt.Printf("%s ⇒ %s\n", img.OriginalName, img.NewName)
	}
}
