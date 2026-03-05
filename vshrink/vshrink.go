// Package vshrink provides functionality to re-encode video files using HandBrakeCLI.
package vshrink

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	DefaultSuffix    = ".vshrink"
	DefaultPreset    = "vshrink"
	DefaultHandbrake = "HandBrakeCLI"
)

// VideoExtensions is the set of file extensions recognised as video files.
var VideoExtensions = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".m4v":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
	".mpeg": true,
	".mpg":  true,
	".ts":   true,
	".m2ts": true,
	".vob":  true,
}

// IsVideoFile returns true when path has a recognised video file extension.
func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return VideoExtensions[ext]
}

// Config holds the configuration for a video conversion operation.
type Config struct {
	// Input is the path to the input video file.
	Input string
	// Output is the explicit output file path. If empty, it is derived from Input and Suffix.
	Output string
	// Suffix is inserted before the file extension when Output is empty.
	Suffix string
	// Preset is the HandBrake preset name to use.
	Preset string
	// HandbrakePath is the path to the HandBrakeCLI executable.
	HandbrakePath string
	// Verbose enables verbose output when true.
	Verbose bool
	// Progress enables progress output when true.
	Progress bool
	// KeepOnError prevents deletion of the output file when an error occurs.
	KeepOnError bool
}

// OutputPath returns the output file path for the given config.
// If c.Output is set it is returned directly; otherwise the path is derived
// from c.Input by inserting c.Suffix before the file extension.
func OutputPath(c Config) string {
	if c.Output != "" {
		return c.Output
	}
	suffix := c.Suffix
	if suffix == "" {
		suffix = DefaultSuffix
	}
	ext := filepath.Ext(c.Input)
	base := strings.TrimSuffix(c.Input, ext)
	return base + suffix + ext
}

// BuildArgs returns the HandBrakeCLI argument list for the given config.
func BuildArgs(c Config) []string {
	preset := c.Preset
	if preset == "" {
		preset = DefaultPreset
	}
	return []string{
		"--preset-import-gui",
		"--preset", preset,
		"-i", c.Input,
		"-o", OutputPath(c),
	}
}

// Run invokes HandBrakeCLI with the settings in c, streaming its output to
// the process's stdout and stderr.  When c.Input is a directory it recurses
// into it (see runDir).  It returns an error if HandBrakeCLI cannot be started
// or exits with a non-zero status.
func Run(c Config) error {
	info, err := os.Stat(c.Input)
	if err != nil {
		return fmt.Errorf("cannot access input: %w", err)
	}
	if info.IsDir() {
		if c.Output != "" {
			return fmt.Errorf("output file cannot be specified when input is a directory")
		}
		return runDir(c)
	}
	return runFile(c)
}

// runDir walks the directory tree rooted at c.Input, calling runFile for every
// video file that has not already been converted (i.e. whose base name does not
// already end with the configured suffix).
func runDir(c Config) error {
	suffix := c.Suffix
	if suffix == "" {
		suffix = DefaultSuffix
	}
	return filepath.WalkDir(c.Input, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !IsVideoFile(path) {
			return nil
		}
		// Skip files that already carry the suffix (previously converted outputs).
		ext := filepath.Ext(path)
		base := strings.TrimSuffix(path, ext)
		if strings.HasSuffix(base, suffix) {
			return nil
		}
		fileCfg := c
		fileCfg.Input = path
		return runFile(fileCfg)
	})
}

// runFile invokes HandBrakeCLI for a single input file described by c.
func runFile(c Config) error {
	if _, err := os.Stat(OutputPath(c)); err == nil {
		fmt.Printf("skipping %s: output file already exists\n", c.Input)
		return nil
	}

	handbrake := c.HandbrakePath
	if handbrake == "" {
		handbrake = DefaultHandbrake
	}
	cmd := exec.Command(handbrake, BuildArgs(c)...)
	if c.Progress {
		cmd.Stdout = os.Stdout
	}
	if c.Verbose {
		cmd.Stderr = os.Stderr
	}
	fmt.Println(strings.Join(cmd.Args, " "))

	// Set up signal handling before starting the process to avoid a race
	// where a signal arrives before Notify is called.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("HandBrakeCLI failed to start: %w", err)
	}

	go func() {
		if _, ok := <-sigCh; ok {
			cmd.Process.Kill()
		}
	}()

	if err := cmd.Wait(); err != nil {
		if !c.KeepOnError {
			os.Remove(OutputPath(c))
		}
		return fmt.Errorf("HandBrakeCLI failed: %w", err)
	}
	return nil
}
