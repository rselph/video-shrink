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

// XattrKey is the extended attribute name used to mark processed (encoded) files.
const XattrKey = "user.com.rselph.vshrink.done"

// RemuxXattrKey is the extended attribute name used to mark remuxed files.
const RemuxXattrKey = "user.com.rselph.vshrink-remux.done"

// ErrXattrNotSupported is returned when the filesystem does not support
// extended attributes.
var ErrXattrNotSupported = errors.New("extended attributes not supported")

func isXattrNotSupported(err error) bool {
	return errors.Is(err, ErrXattrNotSupported) || errors.Is(err, syscall.ENOTSUP)
}

// SetXattr sets the vshrink xattr on path with the given value.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
func SetXattr(path string, value string) error {
	return setXattr(path, XattrKey, value)
}

// GetXattr reads the vshrink xattr from path.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
func GetXattr(path string) (string, error) {
	return getXattr(path, XattrKey)
}

// RemoveXattr removes the vshrink xattr from path.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
// A missing attribute is not considered an error.
func RemoveXattr(path string) error {
	return removeXattr(path, XattrKey)
}

// SetRemuxXattr sets the vshrink remux xattr on path with the given value.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
func SetRemuxXattr(path string, value string) error {
	return setXattr(path, RemuxXattrKey, value)
}

// GetRemuxXattr reads the vshrink remux xattr from path.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
func GetRemuxXattr(path string) (string, error) {
	return getXattr(path, RemuxXattrKey)
}

// RemoveRemuxXattr removes the vshrink remux xattr from path.
// Returns ErrXattrNotSupported if the filesystem does not support xattrs.
// A missing attribute is not considered an error.
func RemoveRemuxXattr(path string) error {
	return removeXattr(path, RemuxXattrKey)
}

// On platforms without xattr support (e.g. Windows), the xattr package's
// operations are no-ops that report success, so check XATTR_SUPPORTED first
// to make callers fall back to marker files.

func setXattr(path, key, value string) error {
	if !xattr.XATTR_SUPPORTED {
		return ErrXattrNotSupported
	}
	err := xattr.Set(path, key, []byte(value))
	if err != nil && errors.Is(err, syscall.ENOTSUP) {
		return ErrXattrNotSupported
	}
	return err
}

func getXattr(path, key string) (string, error) {
	if !xattr.XATTR_SUPPORTED {
		return "", ErrXattrNotSupported
	}
	data, err := xattr.Get(path, key)
	if err != nil {
		if errors.Is(err, syscall.ENOTSUP) {
			return "", ErrXattrNotSupported
		}
		return "", err
	}
	return string(data), nil
}

func removeXattr(path, key string) error {
	if !xattr.XATTR_SUPPORTED {
		return ErrXattrNotSupported
	}
	err := xattr.Remove(path, key)
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

// markerKind describes one kind of marker: the xattr that records it and the
// marker file(s) used on filesystems without xattr support.
type markerKind struct {
	key        string
	path       func(Config) string
	legacyPath func(Config) string // nil when there is no legacy format
}

var (
	// encodedMarker records that a file has been processed (re-encoded).
	encodedMarker = markerKind{XattrKey, MarkerPath, LegacyMarkerPath}
	// remuxMarker records that a file has been remuxed.
	remuxMarker = markerKind{RemuxXattrKey, RemuxMarkerPath, nil}
)

// findMarker checks the marker sources for k in priority order and returns
// which source was found along with its effective timestamp.  An xattr whose
// value has no parsable timestamp is still reported, with a zero time.
func findMarker(c Config, k markerKind) (markerSource, time.Time) {
	// 1. xattr on the video file
	value, err := getXattr(c.Input, k.key)
	if err == nil {
		ts, _ := ParseXattrTimestamp(value)
		return markerXattr, ts
	}

	// 2. Current marker file
	info, err := os.Stat(k.path(c))
	if err == nil {
		return markerDotFile, info.ModTime()
	}

	// 3. Legacy marker file
	if k.legacyPath != nil {
		info, err = os.Stat(k.legacyPath(c))
		if err == nil {
			return markerLegacy, info.ModTime()
		}
	}

	return markerNone, time.Time{}
}

// IsMarked returns true if any marker source indicates the file has been
// processed.  An xattr travels with the file's contents, so it is always
// honored, even when the file has been copied and has a newer mtime.  A marker
// file is disregarded when it is stale relative to the video's mtime.  When a
// marker file is found without a corresponding xattr, an upgrade to xattr is
// attempted automatically.
func IsMarked(c Config) bool {
	return isMarked(c, encodedMarker)
}

// IsRemuxed returns true if any remux marker source indicates the file has
// been remuxed.  Like IsMarked, it always honors the xattr, disregards a stale
// marker file, and upgrades a remux marker file to an xattr when possible.
func IsRemuxed(c Config) bool {
	return isMarked(c, remuxMarker)
}

func isMarked(c Config, k markerKind) bool {
	inInfo, err := os.Stat(c.Input)
	if err != nil {
		return false
	}

	source, mtime := findMarker(c, k)
	switch source {
	case markerNone:
		return false
	case markerXattr:
		return true
	}

	// Staleness check: marker files are named after the video rather than
	// attached to it, so disregard one if the video file was modified
	// significantly after it was written.
	if inInfo.ModTime().Sub(mtime) >= 1*time.Minute {
		return false
	}

	// Attempt upgrade from marker file to xattr.
	upgradeMarker(c, k)
	return true
}

// markerContent returns the text recorded in c.Input's marker of kind k, without
// its timestamp, checking the sources in the same order as findMarker.  It
// returns "" if there is no marker.
func markerContent(c Config, k markerKind) string {
	if value, err := getXattr(c.Input, k.key); err == nil {
		if _, err := ParseXattrTimestamp(value); err != nil {
			return value
		}
		_, content, _ := strings.Cut(value, " ")
		return content
	}
	paths := []string{k.path(c)}
	if k.legacyPath != nil {
		paths = append(paths, k.legacyPath(c))
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

// MarkerTime returns the effective timestamp of the marker for c.Input.
// For xattr markers this is the parsed timestamp field; for marker files
// it is the file's mtime.
func MarkerTime(c Config) (time.Time, error) {
	source, mtime := findMarker(c, encodedMarker)
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
	return writeMarkerFile(MarkerPath(c), content)
}

// MarkRemuxed records that c.Input has been remuxed.  info is the remuxed
// file's FileInfo, whose mtime is recorded as the marker's timestamp.  It
// writes an xattr on the file; if xattrs are not supported it falls back to
// creating a remux marker file.
func MarkRemuxed(c Config, info os.FileInfo) error {
	return writeMarker(c, remuxMarker, info.ModTime(), "remuxed")
}

// MarkSkipped records c.Input as both encoded and remuxed without it having
// been processed, so later runs leave it alone.  Like MarkComplete, it falls
// back to marker files if xattrs are not supported.
func MarkSkipped(c Config) error {
	if err := MarkSkippedEncoded(c); err != nil {
		return err
	}
	return MarkSkippedRemux(c)
}

// MarkSkippedEncoded records c.Input as encoded without it having been
// processed, leaving any remux marker as it is.  It falls back to a marker
// file if xattrs are not supported.
func MarkSkippedEncoded(c Config) error {
	info, err := os.Stat(c.Input)
	if err != nil {
		return err
	}
	return writeMarker(c, encodedMarker, info.ModTime(), skippedContent)
}

// MarkSkippedRemux records c.Input as remuxed without it having been
// processed, leaving any encoded marker as it is.  It falls back to a marker
// file if xattrs are not supported.
func MarkSkippedRemux(c Config) error {
	info, err := os.Stat(c.Input)
	if err != nil {
		return err
	}
	return writeMarker(c, remuxMarker, info.ModTime(), skippedContent)
}

// skippedContent is the marker content written for files marked with -mark.
const skippedContent = "marked without processing"

// writeMarker records a marker of kind k on c.Input with the given timestamp
// and content.  It writes an xattr on the file; if xattrs are not supported it
// falls back to creating a marker file.
func writeMarker(c Config, k markerKind, timestamp time.Time, content string) error {
	err := setXattr(c.Input, k.key, FormatXattrValue(timestamp, content))
	if err == nil || !isXattrNotSupported(err) {
		return err
	}
	return writeMarkerFile(k.path(c), content)
}

// UpgradeMarker migrates a marker file to an xattr.  If the xattr is
// written successfully the marker file(s) are deleted.  If xattrs are not
// supported the marker file is left in place.
func UpgradeMarker(c Config) error {
	return upgradeMarker(c, encodedMarker)
}

func upgradeMarker(c Config, k markerKind) error {
	var content []byte
	var mtime time.Time

	markerPath := k.path(c)
	info, err := os.Stat(markerPath)
	if err == nil {
		content, _ = os.ReadFile(markerPath)
		mtime = info.ModTime()
	} else {
		if k.legacyPath == nil {
			return nil // no marker file to upgrade
		}
		legacyPath := k.legacyPath(c)
		info, err = os.Stat(legacyPath)
		if err != nil {
			return nil // no marker file to upgrade
		}
		content, _ = os.ReadFile(legacyPath)
		mtime = info.ModTime()
	}

	value := FormatXattrValue(mtime, strings.TrimSpace(string(content)))
	err = setXattr(c.Input, k.key, value)
	if err != nil {
		if isXattrNotSupported(err) {
			return nil // can't upgrade, leave marker files in place
		}
		return err
	}

	// Clean up marker files.
	os.Remove(markerPath)
	if k.legacyPath != nil {
		os.Remove(k.legacyPath(c))
	}
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

// writeMarkerFile creates a marker file at path with the given content.
func writeMarkerFile(path string, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, content)
	return nil
}
