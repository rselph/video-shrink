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
	DefaultSuffix     = ".vshrink"
	DefaultPreset     = "vshrink"
	DefaultHandbrake  = "HandBrakeCLI"
	DefaultFFmpeg     = "ffmpeg"
	OldMarkerPrefix   = ".vshrink.done."
	MarkerPrefix      = ".vshrink."
	MarkerSuffix      = ".done"
	RemuxMarkerPrefix = ".vshrink-remux."
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
	// FFmpegPath is the path to the ffmpeg executable used for the remux pass.
	FFmpegPath string
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
	// MarkOnly records the file as processed without encoding it, so later runs skip it.
	MarkOnly bool
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

// TempPath returns the path HandBrakeCLI writes to before the ffmpeg remux pass
// produces the final output.  It is derived from c.Input by inserting ".tmp"
// followed by c.Suffix before the file extension, so the base name still ends
// with the suffix and runDir skips any leftover temp files.
func TempPath(c Config) string {
	suffix := c.Suffix
	if suffix == "" {
		suffix = DefaultSuffix
	}
	ext := filepath.Ext(c.Input)
	base := strings.TrimSuffix(c.Input, ext)
	return base + ".tmp" + suffix + ext
}

// MarkerPath returns the path of the sentinel file written by swapInPlace after
// a successful in-place replacement.  The marker is an empty file placed in the
// same directory as c.Input, named ".vshrink.done." followed by the input
// file's base name.
func MarkerPath(c Config) string {
	return filepath.Join(filepath.Dir(c.Input), MarkerPrefix+filepath.Base(c.Input)+MarkerSuffix)
}

// RemuxMarkerPath returns the path of the sentinel file that records a remux
// on filesystems without xattr support.  It is an empty file placed in the same
// directory as c.Input, named ".vshrink-remux." followed by the input file's
// base name and ".done".
func RemuxMarkerPath(c Config) string {
	return filepath.Join(filepath.Dir(c.Input), RemuxMarkerPrefix+filepath.Base(c.Input)+MarkerSuffix)
}

// LegacyMarkerPath returns the path of the old-format marker file, which has the form
// ".vshrink.done." followed by the input file's base name.  This is checked by IsMarkerFile for
// robustness against changes in the naming scheme.
func LegacyMarkerPath(c Config) string {
	return filepath.Join(filepath.Dir(c.Input), OldMarkerPrefix+filepath.Base(c.Input))
}

// IsMarkerFile returns true when path has the form of a marker file created by swapInPlace. It will
// detect the current marker format (".vshrink.<basename>.done"), the remux marker format
// (".vshrink-remux.<basename>.done"), and the old format (".vshrink.done.<basename>") to be robust
// against changes in the naming scheme.
func IsMarkerFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, MarkerPrefix) && strings.HasSuffix(base, MarkerSuffix) ||
		strings.HasPrefix(base, RemuxMarkerPrefix) && strings.HasSuffix(base, MarkerSuffix) ||
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
		"-o", TempPath(c),
	}
}

// BuildRemuxArgs returns the ffmpeg argument list that remuxes input into the
// final output file with fully interleaved tracks.  input is TempPath(c) after
// an encode pass, or c.Input when only the remux pass runs.
func BuildRemuxArgs(c Config, input string) []string {
	return []string{
		"-i", input,
		"-map", "0",
		"-c", "copy",
		"-max_interleave_delta", "0",
		OutputPath(c),
	}
}

// Run invokes HandBrakeCLI with the settings in c, streaming its output to
// the process's stdout and stderr, then remuxes the result with ffmpeg.  When
// c.Input is a directory it recurses into it (see runDir).  It returns an error
// if HandBrakeCLI or ffmpeg cannot be started or exits with a non-zero status.
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

// runFile processes a single input file described by c.  Without -in-place it
// invokes HandBrakeCLI, writing to TempPath(c), then remuxes that file with
// ffmpeg into OutputPath(c).  With -in-place see runInPlace.
func runFile(c Config) error {
	if c.MarkOnly {
		return markOnly(c)
	}
	if c.InPlace {
		return runInPlace(c)
	}

	if IsMarked(c) {
		fmt.Printf("skipping %s: already processed\n", c.Input)
		return nil
	}
	if _, err := os.Stat(OutputPath(c)); err == nil {
		fmt.Printf("skipping %s: output file already exists\n", c.Input)
		return nil
	}

	// Check for ffmpeg up front so a missing executable doesn't waste a long encode.
	ffmpeg, err := lookupFFmpeg(c)
	if err != nil {
		return err
	}
	fmt.Println()
	if err := encode(c); err != nil {
		return err
	}
	return remux(c, ffmpeg, TempPath(c))
}

// runInPlace processes c.Input in place, deciding the encode and remux passes
// separately from the file's markers:
//
//   - not encoded: encode, then remux.  Re-encoding invalidates any earlier remux.
//   - encoded, not remuxed: remux only.
//   - encoded and remuxed: skip.
//
// Only the encode pass compares sizes: when the re-encoded file is not smaller
// (and -ignore-size is not set) it is discarded and the original is remuxed
// instead.  The remux result always replaces the original.
func runInPlace(c Config) error {
	needEncode := !IsMarked(c)
	if !needEncode && IsRemuxed(c) {
		fmt.Printf("skipping %s: already processed\n", c.Input)
		return nil
	}

	// encodedContent is the encoded marker text for an encode pass done by this
	// run.  Empty means the existing encoded marker is carried over.
	encodedContent := ""

	// A leftover output from an interrupted run has already been remuxed.
	if outInfo, err := os.Stat(OutputPath(c)); err == nil {
		if !needEncode {
			return swapInPlace(c, OutputPath(c), "")
		}
		origInfo, err := os.Stat(c.Input)
		if err != nil {
			return fmt.Errorf("in-place: cannot stat original: %w", err)
		}
		encodedContent = formatMarkerContent(origInfo, outInfo)
		if keepEncoded(c, origInfo, outInfo) {
			return swapInPlace(c, OutputPath(c), encodedContent)
		}
		fmt.Printf("in-place: re-encoded file is not smaller; discarding %s\n", OutputPath(c))
		os.Remove(OutputPath(c))
		needEncode = false
	}

	// Check for ffmpeg up front so a missing executable doesn't waste a long encode.
	ffmpeg, err := lookupFFmpeg(c)
	if err != nil {
		return err
	}
	fmt.Println()

	src := c.Input
	if needEncode {
		if err := encode(c); err != nil {
			return err
		}
		origInfo, err := os.Stat(c.Input)
		if err != nil {
			return fmt.Errorf("in-place: cannot stat original: %w", err)
		}
		tmpInfo, err := os.Stat(TempPath(c))
		if err != nil {
			return fmt.Errorf("in-place: cannot stat re-encoded file: %w", err)
		}
		encodedContent = formatMarkerContent(origInfo, tmpInfo)
		if keepEncoded(c, origInfo, tmpInfo) {
			src = TempPath(c)
		} else {
			fmt.Printf("in-place: re-encoded file is not smaller; keeping original\n")
			os.Remove(TempPath(c))
		}
	}

	if err := remux(c, ffmpeg, src); err != nil {
		return err
	}
	return swapInPlace(c, OutputPath(c), encodedContent)
}

// keepEncoded reports whether a re-encoded file should replace the original:
// it must be smaller, unless -ignore-size is set.
func keepEncoded(c Config, origInfo, newInfo os.FileInfo) bool {
	return c.IgnoreSize || newInfo.Size() < origInfo.Size()
}

// lookupFFmpeg returns the ffmpeg executable to use, or an error if it cannot be found.
func lookupFFmpeg(c Config) (string, error) {
	ffmpeg := c.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = DefaultFFmpeg
	}
	if _, err := exec.LookPath(ffmpeg); err != nil {
		return "", fmt.Errorf("ffmpeg not found: %w", err)
	}
	return ffmpeg, nil
}

// encode invokes HandBrakeCLI to re-encode c.Input into TempPath(c).
func encode(c Config) error {
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

	killed, err := runCmd(cmd)
	if err != nil {
		if !c.KeepOnError {
			os.Remove(TempPath(c))
		}
		if !killed {
			return fmt.Errorf("HandBrakeCLI failed: %w", err)
		}
	}
	if killed {
		os.Exit(1)
	}
	return nil
}

// remux invokes ffmpeg to remux src into OutputPath(c) so the audio and video
// tracks are fully interleaved, then removes TempPath(c).
func remux(c Config, ffmpeg, src string) error {
	cmd := exec.Command(ffmpeg, BuildRemuxArgs(c, src)...)
	if c.Verbose {
		cmd.Stderr = os.Stderr
	}
	fmt.Println(strings.Join(cmd.Args, " "))

	killed, err := runCmd(cmd)
	if err != nil {
		if !c.KeepOnError {
			os.Remove(OutputPath(c))
			os.Remove(TempPath(c))
		}
		if !killed {
			return fmt.Errorf("ffmpeg remux failed: %w", err)
		}
	}
	if killed {
		os.Exit(1)
	}
	os.Remove(TempPath(c))
	return nil
}

// markOnly records c.Input as encoded and remuxed without processing it.  A
// file that is already marked as encoded (e.g. by an older version of vshrink)
// but not as remuxed only gets the missing remux marker.
func markOnly(c Config) error {
	if !IsMarked(c) {
		fmt.Printf("marking %s as processed\n", c.Input)
		return MarkSkipped(c)
	}
	if IsRemuxed(c) {
		fmt.Printf("skipping %s: already processed\n", c.Input)
		return nil
	}
	fmt.Printf("marking %s as remuxed\n", c.Input)
	return MarkSkippedRemux(c)
}

// runCmd starts cmd and waits for it to finish.  While it runs, an interrupt
// or SIGTERM kills the process and killed is reported as true.  The signal
// handler is shut down before returning, so callers such as swapInPlace can
// install their own.
func runCmd(cmd *exec.Cmd) (killed bool, err error) {
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, ok := <-sigCh; ok {
			cmd.Process.Kill()
			killed = true
		}
	}()
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	if err := cmd.Start(); err != nil {
		signal.Stop(sigCh)
		close(sigCh)
		<-done
		return false, fmt.Errorf("failed to start: %w", err)
	}

	err = cmd.Wait()
	// Stop then close so the goroutine exits cleanly.
	signal.Stop(sigCh)
	close(sigCh)
	<-done
	return killed, err
}

// swapInPlace replaces c.Input with outputPath and records both the encoded
// and remux markers on the result.  encodedContent is the encoded marker text;
// when empty, the text of c.Input's existing encoded marker is carried over.
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
//     Rename original → output, then backup → original, and remove any marker
//     files this swap created.
func swapInPlace(c Config, outputPath, encodedContent string) error {
	if _, err := os.Stat(c.Input); err != nil {
		return fmt.Errorf("in-place: cannot stat original: %w", err)
	}
	newInfo, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("in-place: cannot stat output: %w", err)
	}
	if encodedContent == "" {
		encodedContent = markerContent(c, encodedMarker)
	}

	backupPath := c.Input + ".vshrink.orig"
	if _, err := os.Stat(backupPath); err == nil {
		return fmt.Errorf("in-place: backup path already exists: %s", backupPath)
	}

	// Note which marker files already belong to the original, so recovery only
	// removes the ones this swap creates.
	_, err = os.Stat(MarkerPath(c))
	hadMarkerFile := err == nil
	_, err = os.Stat(RemuxMarkerPath(c))
	hadRemuxMarkerFile := err == nil

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

			// Markers are only written after step 2.  Xattrs moved back to outputPath with the
			// promoted file, but marker files are named after the original, so remove the ones
			// this swap created.
			if !hadMarkerFile {
				os.Remove(MarkerPath(c))
			}
			if !hadRemuxMarkerFile {
				os.Remove(RemuxMarkerPath(c))
			}
		}
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

	// Record that this file has been processed.  The promoted file is a new file, so the marker
	// is written afresh, timestamped with its mtime.
	if err := writeMarker(c, encodedMarker, newInfo.ModTime(), encodedContent); err != nil {
		fmt.Printf("in-place: warning: could not create marker: %v\n", err)
		// Restore original, so we don't double encode on the next run.  This is a best effort
		// attempt, so if it fails we just print a warning and leave the files as they are.
		err2 := os.Rename(backupPath, c.Input)
		if err2 != nil {
			fmt.Printf("in-place: warning: could not restore original after marker creation failure: %v\n", err2)
		}
		return err
	}

	// Record that this file has been remuxed.  A missing remux marker only means a later run
	// repeats a lossless step, so a failure here is just a warning.
	if err := MarkRemuxed(c, newInfo); err != nil {
		fmt.Printf("in-place: warning: could not create remux marker: %v\n", err)
	}

	// Step 3: remove the backup.
	if err := os.Remove(backupPath); err != nil {
		fmt.Printf("in-place: warning: could not remove backup %s: %v\n", backupPath, err)
	}

	return nil
}
