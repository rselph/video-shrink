package vshrink_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rselph/video-shrink/impl/vshrink"
)

func TestSetGetRemoveXattrRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "testfile")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	value := vshrink.FormatXattrValue(time.Now(), "original size: 100, new size: 50 (50.000000%)")
	if err := vshrink.SetXattr(f, value); err != nil {
		t.Fatalf("SetXattr() error: %v", err)
	}

	got, err := vshrink.GetXattr(f)
	if err != nil {
		t.Fatalf("GetXattr() error: %v", err)
	}
	if got != value {
		t.Errorf("GetXattr() = %q, want %q", got, value)
	}

	if err := vshrink.RemoveXattr(f); err != nil {
		t.Fatalf("RemoveXattr() error: %v", err)
	}

	_, err = vshrink.GetXattr(f)
	if err == nil {
		t.Error("GetXattr() should return error after RemoveXattr()")
	}
}

func TestRemoveXattrMissing(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "testfile")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	// Removing a nonexistent attribute should not return an error.
	if err := vshrink.RemoveXattr(f); err != nil {
		t.Errorf("RemoveXattr() on missing attribute returned error: %v", err)
	}
}

func TestParseXattrTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:  "valid value with content",
			value: "2026-03-25T14:30:00Z original size: 123456, new size: 78901 (64.00%)",
		},
		{
			name:  "timestamp only",
			value: "2026-03-25T14:30:00Z",
		},
		{
			name:    "empty string",
			value:   "",
			wantErr: true,
		},
		{
			name:    "not a timestamp",
			value:   "hello world",
			wantErr: true,
		},
		{
			name:    "partial timestamp",
			value:   "2026-03-25 extra",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := vshrink.ParseXattrTimestamp(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseXattrTimestamp(%q) should return error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseXattrTimestamp(%q) error: %v", tt.value, err)
			}
			if ts.IsZero() {
				t.Errorf("ParseXattrTimestamp(%q) returned zero time", tt.value)
			}
		})
	}
}

func TestParseXattrTimestampRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	value := vshrink.FormatXattrValue(now, "test content")
	got, err := vshrink.ParseXattrTimestamp(value)
	if err != nil {
		t.Fatalf("ParseXattrTimestamp() error: %v", err)
	}
	if !got.Equal(now) {
		t.Errorf("round-trip timestamp = %v, want %v", got, now)
	}
}

func TestUpgradeMarkerFromCurrent(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input}
	markerContent := "original size: 1000, new size: 500 (50.000000%)"
	if err := os.WriteFile(vshrink.MarkerPath(cfg), []byte(markerContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := vshrink.UpgradeMarker(cfg); err != nil {
		t.Fatalf("UpgradeMarker() error: %v", err)
	}

	// xattr should be set.
	value, err := vshrink.GetXattr(input)
	if err != nil {
		t.Fatalf("GetXattr() error after upgrade: %v", err)
	}
	ts, err := vshrink.ParseXattrTimestamp(value)
	if err != nil {
		t.Fatalf("ParseXattrTimestamp() error: %v", err)
	}
	if ts.IsZero() {
		t.Error("xattr timestamp should not be zero")
	}

	// Marker file should be deleted.
	if _, err := os.Stat(vshrink.MarkerPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("current marker file should be removed after upgrade, stat err = %v", err)
	}
}

func TestUpgradeMarkerFromLegacy(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input}
	markerContent := "original size: 2000, new size: 800 (40.000000%)"
	if err := os.WriteFile(vshrink.LegacyMarkerPath(cfg), []byte(markerContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := vshrink.UpgradeMarker(cfg); err != nil {
		t.Fatalf("UpgradeMarker() error: %v", err)
	}

	// xattr should be set.
	value, err := vshrink.GetXattr(input)
	if err != nil {
		t.Fatalf("GetXattr() error after upgrade: %v", err)
	}
	ts, err := vshrink.ParseXattrTimestamp(value)
	if err != nil {
		t.Fatalf("ParseXattrTimestamp() error: %v", err)
	}
	if ts.IsZero() {
		t.Error("xattr timestamp should not be zero")
	}

	// Legacy marker file should be deleted.
	if _, err := os.Stat(vshrink.LegacyMarkerPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("legacy marker file should be removed after upgrade, stat err = %v", err)
	}
}

func TestUpgradeMarkerNoOp(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input}
	// No marker file exists — UpgradeMarker should be a no-op.
	if err := vshrink.UpgradeMarker(cfg); err != nil {
		t.Errorf("UpgradeMarker() with no marker should not error: %v", err)
	}
}

func TestMarkCompleteWritesXattr(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, make([]byte, 1000), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input}
	origInfo, _ := os.Stat(input)

	small := filepath.Join(dir, "small.mp4")
	if err := os.WriteFile(small, make([]byte, 500), 0644); err != nil {
		t.Fatal(err)
	}
	newInfo, _ := os.Stat(small)

	if err := vshrink.MarkComplete(cfg, origInfo, newInfo); err != nil {
		t.Fatalf("MarkComplete() error: %v", err)
	}

	// xattr should be set.
	value, err := vshrink.GetXattr(input)
	if err != nil {
		t.Fatalf("GetXattr() error: %v", err)
	}
	ts, err := vshrink.ParseXattrTimestamp(value)
	if err != nil {
		t.Fatalf("ParseXattrTimestamp() error: %v", err)
	}
	if time.Since(ts) > 5*time.Second {
		t.Errorf("xattr timestamp too old: %v", ts)
	}

	// No marker file should exist.
	if _, err := os.Stat(vshrink.MarkerPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("marker file should not exist when xattr is supported, stat err = %v", err)
	}
}

func TestIsMarkedUpgradesOnRead(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input}
	if err := os.WriteFile(vshrink.MarkerPath(cfg), []byte("original size: 100, new size: 50"), 0644); err != nil {
		t.Fatal(err)
	}

	// IsMarked should return true and trigger an upgrade.
	if !vshrink.IsMarked(cfg) {
		t.Fatal("IsMarked() should return true")
	}

	// After the call, xattr should be set and marker file removed.
	if _, err := vshrink.GetXattr(input); err != nil {
		t.Errorf("xattr should be set after IsMarked upgrade: %v", err)
	}
	if _, err := os.Stat(vshrink.MarkerPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("marker file should be removed after IsMarked upgrade, stat err = %v", err)
	}
}
