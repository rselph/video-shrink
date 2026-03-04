// Package vshrink provides functionality to re-encode video files using HandBrakeCLI.
package vshrink

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	DefaultSuffix    = ".vshrink"
	DefaultPreset    = "vshrink"
	DefaultHandbrake = "HandBrakeCLI"
)

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
// the process's stdout and stderr. It returns an error if HandBrakeCLI cannot
// be started or exits with a non-zero status.
func Run(c Config) error {
	handbrake := c.HandbrakePath
	if handbrake == "" {
		handbrake = DefaultHandbrake
	}
	cmd := exec.Command(handbrake, BuildArgs(c)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("HandBrakeCLI failed: %w", err)
	}
	return nil
}
