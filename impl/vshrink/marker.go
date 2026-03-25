package vshrink

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/xattr"
)

// XattrKey is the extended attribute name used to mark processed files.
const XattrKey = "user.com.rselph.vshrink.done"

// ErrXattrNotSupported is returned when the filesystem does not support
// extended attributes.
var ErrXattrNotSupported = errors.New("extended attributes not supported")

func isXattrNotSupported(err error) bool {
	return errors.Is(err, ErrXattrNotSupported) || errors.Is(err, syscall.ENOTSUP)
}

// SetXattr sets the vshrink xattr on path with the given value.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
func SetXattr(path string, value string) error {
	err := xattr.Set(path, XattrKey, []byte(value))
	if err != nil && errors.Is(err, syscall.ENOTSUP) {
		return ErrXattrNotSupported
	}
	return err
}

// GetXattr reads the vshrink xattr from path.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
func GetXattr(path string) (string, error) {
	data, err := xattr.Get(path, XattrKey)
	if err != nil {
		if errors.Is(err, syscall.ENOTSUP) {
			return "", ErrXattrNotSupported
		}
		return "", err
	}
	return string(data), nil
}

// RemoveXattr removes the vshrink xattr from path.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
// A missing attribute is not considered an error.
func RemoveXattr(path string) error {
	err := xattr.Remove(path, XattrKey)
	if err != nil {
		if errors.Is(err, syscall.ENOTSUP) {
			return ErrXattrNotSupported
		}
		if errors.Is(err, xattr.ENOATTR) {
			return nil
		}
	}
	return err
}

// ParseXattrTimestamp extracts the RFC 3339 timestamp from the beginning of
// an xattr value string.  The expected format is "<RFC3339> <content>".
func ParseXattrTimestamp(value string) (time.Time, error) {
	tsStr := value
	if idx := strings.IndexByte(value, ' '); idx >= 0 {
		tsStr = value[:idx]
	}
	t, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid xattr timestamp: %w", err)
	}
	return t, nil
}

// FormatXattrValue creates an xattr value string from a timestamp and content.
func FormatXattrValue(timestamp time.Time, content string) string {
	return timestamp.UTC().Format(time.RFC3339) + " " + content
}

// markerSource identifies which source provided the marker information.
type markerSource int

const (
	markerNone    markerSource = iota
	markerXattr                // extended attribute on the video file
	markerDotFile              // .vshrink.<basename>.done file
	markerLegacy               // .vshrink.done.<basename> file
)

// findMarker checks all three marker sources in priority order and returns
// which source was found along with its effective timestamp.
func findMarker(c Config) (markerSource, time.Time) {
	// 1. xattr on the video file
	value, err := GetXattr(c.Input)
	if err == nil {
		ts, err := ParseXattrTimestamp(value)
		if err == nil {
			return markerXattr, ts
		}
	}

	// 2. Current marker file
	info, err := os.Stat(MarkerPath(c))
	if err == nil {
		return markerDotFile, info.ModTime()
	}

	// 3. Legacy marker file
	info, err = os.Stat(LegacyMarkerPath(c))
	if err == nil {
		return markerLegacy, info.ModTime()
	}

	return markerNone, time.Time{}
}

// IsMarked returns true if any marker source indicates the file has been
// processed and the marker is not stale relative to the video's mtime.
// When a marker file is found without a corresponding xattr, an upgrade
// to xattr is attempted automatically.
func IsMarked(c Config) bool {
	inInfo, err := os.Stat(c.Input)
	if err != nil {
		return false
	}

	source, mtime := findMarker(c)
	if source == markerNone {
		return false
	}

	// Staleness check: disregard the marker if the video file was modified
	// significantly after it was written.
	if inInfo.ModTime().Sub(mtime) >= 1*time.Minute {
		return false
	}

	// Attempt upgrade from marker file to xattr.
	if source != markerXattr {
		UpgradeMarker(c)
	}

	return true
}

// MarkerTime returns the effective timestamp of the marker for c.Input.
// For xattr markers this is the parsed timestamp field; for marker files
// it is the file's mtime.
func MarkerTime(c Config) (time.Time, error) {
	source, mtime := findMarker(c)
	if source == markerNone {
		return time.Time{}, fmt.Errorf("no marker found for %s", c.Input)
	}
	return mtime, nil
}

// MarkComplete records that in-place processing has completed for c.Input.
// It writes an xattr on the file; if xattrs are not supported it falls
// back to creating a marker file.
func MarkComplete(c Config, origInfo, newInfo os.FileInfo) error {
	content := formatMarkerContent(origInfo, newInfo)
	value := FormatXattrValue(newInfo.ModTime(), content)

	err := SetXattr(c.Input, value)
	if err == nil {
		return nil
	}
	if !isXattrNotSupported(err) {
		return err
	}

	// Fall back to marker file on filesystems without xattr support.
	return writeMarkerFile(c, content)
}

// UpgradeMarker migrates a marker file to an xattr.  If the xattr is
// written successfully the marker file(s) are deleted.  If xattrs are not
// supported the marker file is left in place.
func UpgradeMarker(c Config) error {
	var content []byte
	var mtime time.Time

	markerPath := MarkerPath(c)
	info, err := os.Stat(markerPath)
	if err == nil {
		content, _ = os.ReadFile(markerPath)
		mtime = info.ModTime()
	} else {
		legacyPath := LegacyMarkerPath(c)
		info, err = os.Stat(legacyPath)
		if err != nil {
			return nil // no marker file to upgrade
		}
		content, _ = os.ReadFile(legacyPath)
		mtime = info.ModTime()
	}

	value := FormatXattrValue(mtime, strings.TrimSpace(string(content)))
	err = SetXattr(c.Input, value)
	if err != nil {
		if isXattrNotSupported(err) {
			return nil // can't upgrade, leave marker files in place
		}
		return err
	}

	// Clean up marker files.
	os.Remove(MarkerPath(c))
	os.Remove(LegacyMarkerPath(c))
	return nil
}

// formatMarkerContent produces the human-readable size summary written into
// both xattr values and marker files.
func formatMarkerContent(origInfo, newInfo os.FileInfo) string {
	s := fmt.Sprintf("original size: %d, new size: %d (%02f%%)",
		origInfo.Size(), newInfo.Size(), float64(newInfo.Size())/float64(origInfo.Size())*100)
	if newInfo.Size() >= origInfo.Size() {
		s += " (not replaced)"
	}
	return s
}

// writeMarkerFile creates a marker file with the given content.
func writeMarkerFile(c Config, content string) error {
	f, err := os.Create(MarkerPath(c))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, content)
	return nil
}
