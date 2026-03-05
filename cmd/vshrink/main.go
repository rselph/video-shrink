// Command vshrink re-encodes a video file using HandBrakeCLI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rselph/video-shrink/vshrink"
)

func main() {
	var (
		output      string
		suffix      string
		preset      string
		handbrake   string
		verbose     bool
		noProgress  bool
		keepOnError bool
		inPlace     bool
	)

	flag.StringVar(&output, "output", "", "Set output file name")
	flag.StringVar(&output, "o", "", "Set output file name (shorthand)")
	flag.StringVar(&suffix, "suffix", vshrink.DefaultSuffix, `Suffix inserted before the file extension when -output is not set`)
	flag.StringVar(&suffix, "s", vshrink.DefaultSuffix, `Suffix inserted before the file extension when -output is not set (shorthand)`)
	flag.StringVar(&preset, "preset", vshrink.DefaultPreset, "Name of the HandBrake preset to use")
	flag.StringVar(&preset, "p", vshrink.DefaultPreset, "Name of the HandBrake preset to use (shorthand)")
	flag.StringVar(&handbrake, "handbrake", vshrink.DefaultHandbrake, "Path to the HandBrakeCLI executable")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.BoolVar(&noProgress, "no-progress", false, "disable progress output")
	flag.BoolVar(&keepOnError, "keep", false, "keep output file on error instead of deleting it")
	flag.BoolVar(&inPlace, "in-place", false, "replace original with output when output is smaller")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: vshrink [options] input-file\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	cfg := vshrink.Config{
		Input:         flag.Arg(0),
		Output:        output,
		Suffix:        suffix,
		Preset:        preset,
		HandbrakePath: handbrake,
		Verbose:       verbose,
		Progress:      !noProgress,
		KeepOnError:   keepOnError,
		InPlace:       inPlace,
	}

	if err := vshrink.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "\nvshrink: %v\n", err)
		os.Exit(1)
	}
}
