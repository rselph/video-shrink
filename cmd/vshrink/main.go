// Command vshrink re-encodes a video file using HandBrakeCLI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rselph/video-shrink/impl/vshrink"
)

func main() {
	var (
		suffix          string
		preset          string
		handbrake       string
		ffmpeg          string
		verbose         bool
		noProgress      bool
		keepOnError     bool
		inPlace         bool
		ignoreSize      bool
		continueOnError bool
		printPreset     bool
		markOnly        bool
		encodeOnly      bool
		muxOnly         bool
	)

	flag.StringVar(&suffix, "suffix", vshrink.DefaultSuffix, "Suffix inserted before the file extension")
	flag.StringVar(&suffix, "s", vshrink.DefaultSuffix, "Suffix inserted before the file extension (shorthand)")
	flag.StringVar(&preset, "preset", vshrink.DefaultPreset, "Name of the HandBrake preset to use")
	flag.StringVar(&preset, "p", vshrink.DefaultPreset, "Name of the HandBrake preset to use (shorthand)")
	flag.StringVar(&handbrake, "handbrake", vshrink.DefaultHandbrake, "Path to the HandBrakeCLI executable")
	flag.StringVar(&ffmpeg, "ffmpeg", vshrink.DefaultFFmpeg, "Path to the ffmpeg executable")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.BoolVar(&noProgress, "no-progress", false, "disable progress output")
	flag.BoolVar(&keepOnError, "keep", false, "keep output file on error instead of deleting it")
	flag.BoolVar(&inPlace, "in-place", false, "replace original with the re-encoded (if smaller) and remuxed file")
	flag.BoolVar(&ignoreSize, "ignore-size", false, "with -in-place, keep the re-encoded file even if it is larger")
	flag.BoolVar(&continueOnError, "continue", false, "continue processing other files on error")
	flag.BoolVar(&markOnly, "mark", false, "mark files as processed without encoding them")
	flag.BoolVar(&encodeOnly, "encode-only", false, "only re-encode with HandBrakeCLI; skip the ffmpeg remux")
	flag.BoolVar(&muxOnly, "mux-only", false, "only remux with ffmpeg; skip the HandBrakeCLI re-encode")
	flag.BoolVar(&printPreset, "print-preset", false, "print the suggested HandBrake preset JSON and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: vshrink [options] input-file [input-file...]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if printPreset {
		fmt.Println(string(vshrink.PresetData))
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	exitVal := 0
	for _, arg := range flag.Args() {
		cfg := vshrink.Config{
			Input:           arg,
			Suffix:          suffix,
			Preset:          preset,
			HandbrakePath:   handbrake,
			FFmpegPath:      ffmpeg,
			Verbose:         verbose,
			Progress:        !noProgress,
			KeepOnError:     keepOnError,
			InPlace:         inPlace,
			IgnoreSize:      ignoreSize,
			ContinueOnError: continueOnError,
			MarkOnly:        markOnly,
			EncodeOnly:      encodeOnly,
			MuxOnly:         muxOnly,
		}

		if err := vshrink.Run(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "\nvshrink: %v\n", err)
			exitVal = 1
			if !continueOnError {
				break
			}
		}
	}
	os.Exit(exitVal)
}
