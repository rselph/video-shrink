// Package vshrink provides functionality to re-encode video files using HandBrakeCLI.
package vshrink

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed vshrink.json
var PresetData []byte

const (
	DefaultSuffix    = ".vshrink"
	DefaultPreset    = "vshrink"
	DefaultHandbrake = "HandBrakeCLI"
	OldMarkerPrefix  = ".vshrink.done."
	MarkerPrefix     = ".vshrink."
	MarkerSuffix     = ".done"
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
	// Suffix is inserted before the file extension to form the output path.
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
	// InPlace replaces the original file with the output when the output is smaller.
	InPlace bool
	// IgnoreSize replaces the original even when the output is larger.
	IgnoreSize bool
	// ContinueOnError continues processing files in a directory even when an error occurs.
	ContinueOnError bool
}

// OutputPath returns the output file path for the given config,
// derived from c.Input by inserting c.Suffix before the file extension.
func OutputPath(c Config) string {
	suffix := c.Suffix
	if suffix == "" {
		suffix = DefaultSuffix
	}
	ext := filepath.Ext(c.Input)
	base := strings.TrimSuffix(c.Input, ext)
	return base + suffix + ext
}

// MarkerPath returns the path of the sentinel file written by swapInPlace after
// a successful in-place replacement.  The marker is an empty file placed in the
// same directory as c.Input, named ".vshrink.done." followed by the input
// file's base name.
func MarkerPath(c Config) string {
	return filepath.Join(filepath.Dir(c.Input), MarkerPrefix+filepath.Base(c.Input)+MarkerSuffix)
}

// LegacyMarkerPath returns the path of the old-format marker file, which has the form
// ".vshrink.done." followed by the input file's base name.  This is checked by IsMarkerFile for
// robustness against changes in the naming scheme.
func LegacyMarkerPath(c Config) string {
	return filepath.Join(filepath.Dir(c.Input), OldMarkerPrefix+filepath.Base(c.Input))
}

// IsMarkerFile returns true when path has the form of a marker file created by swapInPlace. It will
// detect both the current marker format (".vshrink.<basename>.done") and the old format
// (".vshrink.done.<basename>") to be robust against changes in the naming scheme.
func IsMarkerFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, MarkerPrefix) && strings.HasSuffix(base, MarkerSuffix) ||
		strings.HasPrefix(base, OldMarkerPrefix)
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
	var lastErr error
	err := filepath.WalkDir(c.Input, func(path string, d os.DirEntry, err error) error {
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

		// Skip legacy marker files created by in-place replacements.
		if IsMarkerFile(path) {
			return nil
		}

		fileCfg := c
		fileCfg.Input = path
		err = runFile(fileCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error processing %s: %v\n", path, err)
			lastErr = err
			if !c.ContinueOnError {
				return err
			}
		}
		return nil
	})
	if lastErr != nil {
		return lastErr
	}
	return err
}

// runFile invokes HandBrakeCLI for a single input file described by c.
func runFile(c Config) error {
	outInfo, _ := os.Stat(OutputPath(c))

	if IsMarked(c) {
		fmt.Printf("skipping %s: already processed\n", c.Input)
		return nil
	}

	if outInfo != nil {
		if c.InPlace {
			return swapInPlace(c, OutputPath(c))
		}
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
	fmt.Println()
	fmt.Println(strings.Join(cmd.Args, " "))

	// Set up signal handling before starting the process. If an interrupt is received, kill the
	// HandBrakeCLI process and set a flag.
	killed := false
	sigCh := make(chan os.Signal, 1)
	go func() {
		if _, ok := <-sigCh; ok {
			cmd.Process.Kill()
			killed = true
		}
	}()
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	if err := cmd.Start(); err != nil {
		signal.Stop(sigCh)
		close(sigCh)
		return fmt.Errorf("HandBrakeCLI failed to start: %w", err)
	}

	err := cmd.Wait()
	// Shut down the signal handler before potentially entering swapInPlace,
	// which installs its own handler. Stop then close so the goroutine exits cleanly.
	signal.Stop(sigCh)
	close(sigCh)

	if err != nil {
		if !c.KeepOnError {
			os.Remove(OutputPath(c))
		}
		if !killed {
			return fmt.Errorf("HandBrakeCLI failed: %w", err)
		}
	}

	if killed {
		os.Exit(1)
	}

	if c.InPlace {
		return swapInPlace(c, OutputPath(c))
	}
	return nil
}

// swapInPlace replaces c.Input with outputPath if outputPath is smaller.
// The swap is performed via three steps so the original is never destroyed
// before a valid replacement exists:
//
//  1. Rename original → original.vshrink.orig   (backup)
//  2. Rename output   → original                (promote)
//  3. Remove backup                             (cleanup)
//
// A signal handler is installed before step 1. On interrupt, recovery is
// determined by inspecting which files exist on disk.
//
//   - backup absent: nothing moved yet, or step 3 already finished — nothing to do.
//   - backup present, output present: step 1 done, step 2 not yet.
//     Rename backup → original.
//   - backup present, output absent: step 2 done, step 3 not yet.
//     Rename original → output, then backup → original.
func swapInPlace(c Config, outputPath string) error {
	origInfo, err := os.Stat(c.Input)
	if err != nil {
		return fmt.Errorf("in-place: cannot stat original: %w", err)
	}
	newInfo, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("in-place: cannot stat output: %w", err)
	}
	if !c.IgnoreSize && newInfo.Size() >= origInfo.Size() {
		fmt.Printf("in-place: output is not smaller; discarding %s\n", outputPath)
		MarkComplete(c, origInfo, newInfo)
		os.Remove(outputPath)
		return nil
	}

	backupPath := c.Input + ".vshrink.orig"
	if _, err := os.Stat(backupPath); err == nil {
		return fmt.Errorf("in-place: backup path already exists: %s", backupPath)
	}

	// Register signal handler before any renames so no interrupt can slip through.
	sigCh := make(chan os.Signal, 1)
	go func() {
		if _, ok := <-sigCh; !ok {
			return
		}
		// Determine recovery action from filesystem state.  The three cases are mutually exclusive:
		//
		//   backupPath absent: nothing has moved yet (or step 3 already finished).
		//   backupPath present, outputPath present: step 1 done, step 2 not yet.
		//     → rename backup back to original.
		//   backupPath present, outputPath absent: step 2 done, step 3 not yet.
		//     → rename original back to outputPath, then backup back to original.
		_, backupErr := os.Stat(backupPath)
		_, outputErr := os.Stat(outputPath)
		if backupErr == nil && outputErr == nil {
			// Original is at backupPath; encoded file is still at outputPath.
			os.Rename(backupPath, c.Input)
		} else if backupErr == nil && outputErr != nil {
			// Encoded file has been promoted to c.Input; original is at backupPath.
			os.Rename(c.Input, outputPath)
			os.Rename(backupPath, c.Input)
		}

		// Remove marker/xattr if it exists, since in-place processing did not complete.
		os.Remove(MarkerPath(c))
		RemoveXattr(c.Input)
		os.Exit(1)
	}()
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()

	// Step 1: move original out of the way.
	if err := os.Rename(c.Input, backupPath); err != nil {
		return fmt.Errorf("in-place: cannot move original to backup: %w", err)
	}

	// Step 2: promote the output to the original name.
	if err := os.Rename(outputPath, c.Input); err != nil {
		// Undo step 1 so the original is restored.
		if rerr := os.Rename(backupPath, c.Input); rerr != nil {
			return fmt.Errorf("in-place: rename failed (%w) and could not restore original: %v", err, rerr)
		}
		return fmt.Errorf("in-place: cannot rename output to original: %w", err)
	}

	// Record that this file has been processed.
	if err := MarkComplete(c, origInfo, newInfo); err != nil {
		fmt.Printf("in-place: warning: could not create marker: %v\n", err)
		// Restore original, so we don't double encode on the next run.  This is a best effort
		// attempt, so if it fails we just print a warning and leave the files as they are.
		err2 := os.Rename(backupPath, c.Input)
		if err2 != nil {
			fmt.Printf("in-place: warning: could not restore original after marker creation failure: %v\n", err2)
		}
		return err
	}

	// Step 3: remove the backup.
	if err := os.Remove(backupPath); err != nil {
		fmt.Printf("in-place: warning: could not remove backup %s: %v\n", backupPath, err)
	}

	return nil
}
