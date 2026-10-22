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

func TestMarkSkippedWritesXattr(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input}

	if err := vshrink.MarkSkipped(cfg); err != nil {
		t.Fatalf("MarkSkipped() error: %v", err)
	}

	value, err := vshrink.GetXattr(input)
	if err != nil {
		t.Fatalf("GetXattr() error: %v", err)
	}
	if _, err := vshrink.ParseXattrTimestamp(value); err != nil {
		t.Fatalf("ParseXattrTimestamp() error: %v", err)
	}
	if !vshrink.IsMarked(cfg) {
		t.Error("IsMarked() should return true after MarkSkipped")
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("IsRemuxed() should return true after MarkSkipped")
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

func TestSetGetRemoveRemuxXattrRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "testfile")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	value := vshrink.FormatXattrValue(time.Now(), "remuxed")
	if err := vshrink.SetRemuxXattr(f, value); err != nil {
		t.Fatalf("SetRemuxXattr() error: %v", err)
	}
	got, err := vshrink.GetRemuxXattr(f)
	if err != nil {
		t.Fatalf("GetRemuxXattr() error: %v", err)
	}
	if got != value {
		t.Errorf("GetRemuxXattr() = %q, want %q", got, value)
	}
	// The remux xattr is independent of the encoded xattr.
	if _, err := vshrink.GetXattr(f); err == nil {
		t.Error("GetXattr() should not see the remux xattr")
	}

	if err := vshrink.RemoveRemuxXattr(f); err != nil {
		t.Fatalf("RemoveRemuxXattr() error: %v", err)
	}
	if _, err := vshrink.GetRemuxXattr(f); err == nil {
		t.Error("GetRemuxXattr() should return error after RemoveRemuxXattr()")
	}
	if err := vshrink.RemoveRemuxXattr(f); err != nil {
		t.Errorf("RemoveRemuxXattr() on missing attribute returned error: %v", err)
	}
}

func TestMarkRemuxedWritesXattr(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input}
	if err := vshrink.MarkRemuxed(cfg, info); err != nil {
		t.Fatalf("MarkRemuxed() error: %v", err)
	}
	if _, err := vshrink.GetRemuxXattr(input); err != nil {
		t.Errorf("remux xattr should be set: %v", err)
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("IsRemuxed() should return true after MarkRemuxed()")
	}
	// Remuxing alone does not mark the file as encoded.
	if vshrink.IsMarked(cfg) {
		t.Error("IsMarked() should return false after only MarkRemuxed()")
	}
}

func TestIsRemuxed(t *testing.T) {
	setup := func(t *testing.T) (string, vshrink.Config) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		return input, vshrink.Config{Input: input}
	}

	t.Run("no marker", func(t *testing.T) {
		_, cfg := setup(t)
		if vshrink.IsRemuxed(cfg) {
			t.Error("IsRemuxed() should return false when no marker exists")
		}
	})

	t.Run("legacy encoded marker only", func(t *testing.T) {
		input, cfg := setup(t)
		if err := vshrink.SetXattr(input, vshrink.FormatXattrValue(time.Now(), "test")); err != nil {
			t.Fatal(err)
		}
		if !vshrink.IsMarked(cfg) || vshrink.IsRemuxed(cfg) {
			t.Error("a file with only the encoded marker should be marked but not remuxed")
		}
	})

	t.Run("stale xattr", func(t *testing.T) {
		input, cfg := setup(t)
		if err := vshrink.SetRemuxXattr(input, vshrink.FormatXattrValue(time.Now().Add(-2*time.Hour), "remuxed")); err != nil {
			t.Fatal(err)
		}
		if vshrink.IsRemuxed(cfg) {
			t.Error("IsRemuxed() should return false when the marker is stale")
		}
	})

	t.Run("marker file upgrades to xattr", func(t *testing.T) {
		input, cfg := setup(t)
		if err := os.WriteFile(vshrink.RemuxMarkerPath(cfg), []byte("remuxed"), 0644); err != nil {
			t.Fatal(err)
		}
		if !vshrink.IsRemuxed(cfg) {
			t.Fatal("IsRemuxed() should return true when the remux marker file exists")
		}
		if _, err := vshrink.GetRemuxXattr(input); err != nil {
			t.Errorf("remux xattr should be set after upgrade: %v", err)
		}
		if _, err := os.Stat(vshrink.RemuxMarkerPath(cfg)); !os.IsNotExist(err) {
			t.Errorf("remux marker file should be removed after upgrade, stat err = %v", err)
		}
		if vshrink.IsMarked(cfg) {
			t.Error("IsMarked() should not be affected by the remux marker")
		}
	})
}
