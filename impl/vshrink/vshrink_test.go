package vshrink_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rselph/video-shrink/impl/vshrink"
)

func TestOutputPath(t *testing.T) {
	tests := []struct {
		name   string
		config vshrink.Config
		want   string
	}{
		{
			name:   "default suffix inserted before extension",
			config: vshrink.Config{Input: "/path/to/video.mp4"},
			want:   "/path/to/video.vshrink.mp4",
		},
		{
			name:   "custom suffix inserted before extension",
			config: vshrink.Config{Input: "/path/to/video.mp4", Suffix: ".small"},
			want:   "/path/to/video.small.mp4",
		},
		{
			name:   "file without extension",
			config: vshrink.Config{Input: "/path/to/video"},
			want:   "/path/to/video.vshrink",
		},
		{
			name:   "file in current directory",
			config: vshrink.Config{Input: "movie.mkv"},
			want:   "movie.vshrink.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vshrink.OutputPath(tt.config)
			if got != tt.want {
				t.Errorf("OutputPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name   string
		config vshrink.Config
		want   []string
	}{
		{
			name:   "defaults used when fields are empty",
			config: vshrink.Config{Input: "video.mp4"},
			want: []string{
				"--preset-import-gui",
				"--preset", vshrink.DefaultPreset,
				"-i", "video.mp4",
				"-o", "video.vshrink.mp4",
			},
		},
		{
			name:   "custom preset applied",
			config: vshrink.Config{Input: "video.mp4", Preset: "HQ 1080p30 Surround"},
			want: []string{
				"--preset-import-gui",
				"--preset", "HQ 1080p30 Surround",
				"-i", "video.mp4",
				"-o", "video.vshrink.mp4",
			},
		},
		{
			name:   "custom suffix applied",
			config: vshrink.Config{Input: "/movies/film.avi", Suffix: ".enc"},
			want: []string{
				"--preset-import-gui",
				"--preset", vshrink.DefaultPreset,
				"-i", "/movies/film.avi",
				"-o", "/movies/film.enc.avi",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vshrink.BuildArgs(tt.config)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Run("returns error when executable does not exist", func(t *testing.T) {
		cfg := vshrink.Config{
			Input:         "video.mp4",
			HandbrakePath: "/nonexistent/HandBrakeCLI",
		}
		err := vshrink.Run(cfg)
		if err == nil {
			t.Fatal("expected error for missing executable, got nil")
		}
	})

	t.Run("succeeds when executable exits zero", func(t *testing.T) {
		// Create a temporary file so os.Stat succeeds.
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		// Use the system 'true' command as a stand-in for HandBrakeCLI.
		cfg := vshrink.Config{
			Input:         input,
			HandbrakePath: "true",
		}
		if err := vshrink.Run(cfg); err != nil {
			t.Errorf("Run() returned unexpected error: %v", err)
		}
	})

	t.Run("returns error when input path does not exist", func(t *testing.T) {
		cfg := vshrink.Config{
			Input:         "/nonexistent/path/video.mp4",
			HandbrakePath: "true",
		}
		err := vshrink.Run(cfg)
		if err == nil {
			t.Fatal("expected error for missing input, got nil")
		}
	})

	t.Run("wraps error message on non-zero exit", func(t *testing.T) {
		// Create a temporary file so os.Stat succeeds.
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		// Use the system 'false' command to simulate a HandBrakeCLI failure.
		cfg := vshrink.Config{
			Input:         input,
			HandbrakePath: "false",
		}
		err := vshrink.Run(cfg)
		if err == nil {
			t.Fatal("expected error for non-zero exit, got nil")
		}
		if !strings.Contains(err.Error(), "HandBrakeCLI failed") {
			t.Errorf("error message %q does not contain %q", err.Error(), "HandBrakeCLI failed")
		}
	})

	t.Run("directory mode processes video files without error", func(t *testing.T) {
		dir := t.TempDir()
		subDir := filepath.Join(dir, "sub")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create a mix of video and non-video files in root and subdirectory.
		for _, name := range []string{"movie.mp4", "readme.txt"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(subDir, "clip.mkv"), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}

		// Use 'true' as a stand-in so all video files succeed without real encoding.
		cfg := vshrink.Config{
			Input:         dir,
			HandbrakePath: "true",
		}
		if err := vshrink.Run(cfg); err != nil {
			t.Errorf("Run() on directory returned unexpected error: %v", err)
		}
	})

	t.Run("directory mode skips already-converted files", func(t *testing.T) {
		dir := t.TempDir()
		// Only place a previously-converted file in the directory.
		// If it were not skipped it would be passed to 'false' (HandBrakeCLI
		// stand-in) which would cause Run to return an error.
		if err := os.WriteFile(filepath.Join(dir, "movie.vshrink.mp4"), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}

		cfg := vshrink.Config{
			Input:         dir,
			HandbrakePath: "false",
		}
		if err := vshrink.Run(cfg); err != nil {
			t.Errorf("Run() should skip already-converted file but got error: %v", err)
		}
	})
}

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"movie.mp4", true},
		{"clip.mkv", true},
		{"film.AVI", true}, // case-insensitive
		{"film.MOV", true},
		{"doc.pdf", false},
		{"readme.txt", false},
		{"archive.zip", false},
		{"noextension", false},
		{"", false},                   // empty string
		{"video.backup.mp4", true},    // multiple dots
		{".hidden.mp4", true},         // hidden file with video ext
		{"/path/to/movie.m2ts", true}, // full path
		{"movie.MP4", true},           // all caps
		{"/path/to/movie.webm", true}, // all recognized extensions
		{"movie.flv", true},
		{"movie.m4v", true},
		{"movie.wmv", true},
		{"movie.mpeg", true},
		{"movie.mpg", true},
		{"movie.ts", true},
		{"movie.vob", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := vshrink.IsVideoFile(tt.path)
			if got != tt.want {
				t.Errorf("IsVideoFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMarkerPath(t *testing.T) {
	tests := []struct {
		name   string
		config vshrink.Config
		want   string
	}{
		{
			name:   "basic file",
			config: vshrink.Config{Input: "/path/to/video.mp4"},
			want:   "/path/to/" + vshrink.MarkerPrefix + "video.mp4" + vshrink.MarkerSuffix,
		},
		{
			name:   "file in current directory",
			config: vshrink.Config{Input: "movie.mkv"},
			want:   vshrink.MarkerPrefix + "movie.mkv" + vshrink.MarkerSuffix,
		},
		{
			name:   "file without extension",
			config: vshrink.Config{Input: "/dir/video"},
			want:   "/dir/" + vshrink.MarkerPrefix + "video" + vshrink.MarkerSuffix,
		},
		{
			name:   "nested path",
			config: vshrink.Config{Input: "/a/b/c/clip.avi"},
			want:   "/a/b/c/" + vshrink.MarkerPrefix + "clip.avi" + vshrink.MarkerSuffix,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vshrink.MarkerPath(tt.config)
			if got != tt.want {
				t.Errorf("MarkerPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLegacyMarkerPath(t *testing.T) {
	tests := []struct {
		name   string
		config vshrink.Config
		want   string
	}{
		{
			name:   "basic file",
			config: vshrink.Config{Input: "/path/to/video.mp4"},
			want:   "/path/to/" + vshrink.OldMarkerPrefix + "video.mp4",
		},
		{
			name:   "file in current directory",
			config: vshrink.Config{Input: "movie.mkv"},
			want:   vshrink.OldMarkerPrefix + "movie.mkv",
		},
		{
			name:   "nested path",
			config: vshrink.Config{Input: "/a/b/c/clip.avi"},
			want:   "/a/b/c/" + vshrink.OldMarkerPrefix + "clip.avi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vshrink.LegacyMarkerPath(tt.config)
			if got != tt.want {
				t.Errorf("LegacyMarkerPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsMarkerFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// New format: .vshrink.<basename>.done
		{".vshrink.video.mp4.done", true},
		{"/path/to/.vshrink.clip.mkv.done", true},
		{".vshrink.file.done", true},
		// Old format: .vshrink.done.<basename>
		{".vshrink.done.video.mp4", true},
		{"/path/to/.vshrink.done.clip.mkv", true},
		{".vshrink.done.file", true},
		// Old format with video extension (key backward compat case)
		{".vshrink.done.video.ts", true},
		// Not a marker
		{"video.mp4", false},
		{".vshrink.video.mp4", false}, // prefix but no .done suffix
		{"something.done", false},     // suffix but no .vshrink. prefix
		{"movie.vshrink.mp4", false},  // output file, not a marker
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := vshrink.IsMarkerFile(tt.path)
			if got != tt.want {
				t.Errorf("IsMarkerFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsMarkedAndMarkerInfo(t *testing.T) {
	t.Run("returns true for current marker", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		if err := os.WriteFile(vshrink.MarkerPath(cfg), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		if !vshrink.IsMarked(cfg) {
			t.Error("IsMarked() should return true when current marker exists")
		}
		info, err := vshrink.MarkerInfo(cfg)
		if err != nil {
			t.Errorf("MarkerInfo() returned unexpected error: %v", err)
		}
		if info == nil {
			t.Error("MarkerInfo() returned nil FileInfo")
		}
	})

	t.Run("returns true for legacy marker", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		if err := os.WriteFile(vshrink.LegacyMarkerPath(cfg), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		if !vshrink.IsMarked(cfg) {
			t.Error("IsMarked() should return true when legacy marker exists")
		}
		info, err := vshrink.MarkerInfo(cfg)
		if err != nil {
			t.Errorf("MarkerInfo() returned unexpected error: %v", err)
		}
		if info == nil {
			t.Error("MarkerInfo() returned nil FileInfo")
		}
	})

	t.Run("returns false when no marker exists", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		if vshrink.IsMarked(cfg) {
			t.Error("IsMarked() should return false when no marker exists")
		}
		_, err := vshrink.MarkerInfo(cfg)
		if err == nil {
			t.Error("MarkerInfo() should return error when no marker exists")
		}
	})

	t.Run("prefers current marker over legacy", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		// Create both markers.
		if err := os.WriteFile(vshrink.MarkerPath(cfg), []byte("current"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vshrink.LegacyMarkerPath(cfg), []byte("legacy"), 0644); err != nil {
			t.Fatal(err)
		}
		info, err := vshrink.MarkerInfo(cfg)
		if err != nil {
			t.Fatalf("MarkerInfo() returned unexpected error: %v", err)
		}
		// The current marker should be returned (it has the new-format name).
		if info.Name() != filepath.Base(vshrink.MarkerPath(cfg)) {
			t.Errorf("MarkerInfo() returned %q, expected current marker %q", info.Name(), filepath.Base(vshrink.MarkerPath(cfg)))
		}
	})
}

func TestRunSkipsWhenLegacyMarkerExists(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create only the legacy marker file.
	cfg := vshrink.Config{Input: input}
	legacyMarker := vshrink.LegacyMarkerPath(cfg)
	if err := os.WriteFile(legacyMarker, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	// Use 'false' as the HandBrakeCLI stand-in. If the file is not skipped
	// 'false' will cause Run to return an error.
	cfg.HandbrakePath = "false"
	if err := vshrink.Run(cfg); err != nil {
		t.Errorf("Run() should skip file with legacy marker, got error: %v", err)
	}
}

func TestPresetDataIsValidJSON(t *testing.T) {
	if len(vshrink.PresetData) == 0 {
		t.Fatal("PresetData is empty")
	}
	var parsed interface{}
	if err := json.Unmarshal(vshrink.PresetData, &parsed); err != nil {
		t.Errorf("PresetData is not valid JSON: %v", err)
	}
}

func TestRunSkipsWhenMarkerExists(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create the marker file.
	cfg := vshrink.Config{Input: input}
	marker := vshrink.MarkerPath(cfg)
	if err := os.WriteFile(marker, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	// Use 'false' as the HandBrakeCLI stand-in. If the file is not skipped
	// 'false' will cause Run to return an error.
	cfg.HandbrakePath = "false"
	if err := vshrink.Run(cfg); err != nil {
		t.Errorf("Run() should skip file with marker, got error: %v", err)
	}
}

func TestRunSkipsWhenOutputExists(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-create the output file.
	cfg := vshrink.Config{Input: input}
	output := vshrink.OutputPath(cfg)
	if err := os.WriteFile(output, []byte("encoded"), 0644); err != nil {
		t.Fatal(err)
	}
	// Use 'false' — if skipping works, it won't be invoked.
	cfg.HandbrakePath = "false"
	if err := vshrink.Run(cfg); err != nil {
		t.Errorf("Run() should skip file with existing output, got error: %v", err)
	}
}

func TestRunInPlaceSwapsWhenOutputSmaller(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	// Write a large original.
	if err := os.WriteFile(input, make([]byte, 1000), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input, InPlace: true}
	output := vshrink.OutputPath(cfg)
	// Write a smaller encoded file.
	smallContent := []byte("small")
	if err := os.WriteFile(output, smallContent, 0644); err != nil {
		t.Fatal(err)
	}
	// Since output already exists and InPlace is set, Run goes directly to swapInPlace.
	cfg.HandbrakePath = "true"
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	// The input file should now contain the smaller encoded content.
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("cannot read input after swap: %v", err)
	}
	if !reflect.DeepEqual(data, smallContent) {
		t.Errorf("input content after swap = %q, want %q", data, smallContent)
	}
	// Output file should no longer exist at its original path.
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output file should be gone after swap, stat err = %v", err)
	}
	// Marker file should exist.
	if _, err := os.Stat(vshrink.MarkerPath(cfg)); err != nil {
		t.Errorf("marker file should exist after swap: %v", err)
	}
}

func TestRunInPlaceDiscardsWhenOutputLarger(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	origContent := []byte("original")
	if err := os.WriteFile(input, origContent, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input, InPlace: true}
	output := vshrink.OutputPath(cfg)
	// Write a larger encoded file.
	if err := os.WriteFile(output, make([]byte, 1000), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.HandbrakePath = "true"
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	// Original should be untouched.
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("cannot read input: %v", err)
	}
	if !reflect.DeepEqual(data, origContent) {
		t.Errorf("original content changed unexpectedly")
	}
	// Output should have been removed.
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("larger output should be removed, stat err = %v", err)
	}
	// Marker file should still be created (to prevent reprocessing).
	if _, err := os.Stat(vshrink.MarkerPath(cfg)); err != nil {
		t.Errorf("marker file should exist even when output was discarded: %v", err)
	}
}

func TestRunInPlaceIgnoreSize(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("small"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input, InPlace: true, IgnoreSize: true}
	output := vshrink.OutputPath(cfg)
	// Write a larger encoded file — normally this would be discarded.
	largeContent := make([]byte, 1000)
	if err := os.WriteFile(output, largeContent, 0644); err != nil {
		t.Fatal(err)
	}
	cfg.HandbrakePath = "true"
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	// With IgnoreSize the swap should still happen.
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("cannot read input after swap: %v", err)
	}
	if !reflect.DeepEqual(data, largeContent) {
		t.Errorf("input should contain larger encoded content when IgnoreSize is set")
	}
}

func TestRunCleansUpOutputOnError(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	// Script creates the output file ($7) then exits 1 to simulate a failed encode.
	script := filepath.Join(dir, "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \"$7\"\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{
		Input:         input,
		HandbrakePath: script,
		KeepOnError:   false,
	}
	output := vshrink.OutputPath(cfg)
	err := vshrink.Run(cfg)
	if err == nil {
		t.Fatal("expected error from failing HandBrakeCLI")
	}
	// Output should be cleaned up.
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Errorf("output file should be removed on error, stat err = %v", statErr)
	}
}

func TestRunKeepOnErrorRetainsOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	// Script creates the output file ($7) then exits 1.
	script := filepath.Join(dir, "fail-with-output.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \"$7\"\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{
		Input:         input,
		HandbrakePath: script,
		KeepOnError:   true,
	}
	output := vshrink.OutputPath(cfg)
	err := vshrink.Run(cfg)
	if err == nil {
		t.Fatal("expected error from failing HandBrakeCLI")
	}
	// With KeepOnError set, output should still exist.
	if _, statErr := os.Stat(output); os.IsNotExist(statErr) {
		t.Errorf("output file should be retained when KeepOnError is set")
	}
}

func TestRunDirSkipsMarkerFiles(t *testing.T) {
	t.Run("skips new-format marker with video extension", func(t *testing.T) {
		dir := t.TempDir()
		// New-format markers end in .done so they won't pass IsVideoFile,
		// but verify they are also caught by IsMarkerFile.
		markerFile := filepath.Join(dir, vshrink.MarkerPrefix+"video.ts"+vshrink.MarkerSuffix)
		if err := os.WriteFile(markerFile, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{
			Input:         dir,
			HandbrakePath: "false",
		}
		if err := vshrink.Run(cfg); err != nil {
			t.Errorf("Run() should skip new-format marker files, got error: %v", err)
		}
	})

	t.Run("skips legacy marker with video extension", func(t *testing.T) {
		dir := t.TempDir()
		// Legacy markers like .vshrink.done.video.ts end in .ts which is a video extension,
		// so IsMarkerFile must catch them before they reach HandBrakeCLI.
		markerFile := filepath.Join(dir, vshrink.OldMarkerPrefix+"video.ts")
		if err := os.WriteFile(markerFile, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{
			Input:         dir,
			HandbrakePath: "false",
		}
		if err := vshrink.Run(cfg); err != nil {
			t.Errorf("Run() should skip legacy marker files, got error: %v", err)
		}
	})
}

func TestRunInPlaceBackupAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, make([]byte, 1000), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input, InPlace: true}
	output := vshrink.OutputPath(cfg)
	if err := os.WriteFile(output, []byte("small"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-create the backup path to trigger the error.
	backupPath := input + ".vshrink.orig"
	if err := os.WriteFile(backupPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	cfg.HandbrakePath = "true"
	err := vshrink.Run(cfg)
	if err == nil {
		t.Fatal("expected error when backup path already exists")
	}
	if !strings.Contains(err.Error(), "backup path already exists") {
		t.Errorf("error message %q should mention backup path", err.Error())
	}
}

func TestRunInPlaceEqualSizeDiscards(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	content := []byte("exactly the same size!!")
	if err := os.WriteFile(input, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input, InPlace: true}
	output := vshrink.OutputPath(cfg)
	// Same size should also be discarded (condition is >=).
	if err := os.WriteFile(output, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg.HandbrakePath = "true"
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	// Original should be untouched.
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("cannot read input: %v", err)
	}
	if !reflect.DeepEqual(data, content) {
		t.Errorf("original content should be unchanged for equal-size output")
	}
	// Output should have been removed.
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("equal-size output should be removed, stat err = %v", err)
	}
}

func TestOutputPathDefaultSuffix(t *testing.T) {
	// Verify that the default suffix matches the constant.
	c := vshrink.Config{Input: "test.mp4"}
	got := vshrink.OutputPath(c)
	if !strings.Contains(got, vshrink.DefaultSuffix) {
		t.Errorf("OutputPath with empty suffix should use DefaultSuffix %q, got %q", vshrink.DefaultSuffix, got)
	}
}

func TestBuildArgsDefaultPreset(t *testing.T) {
	// Verify that the default preset matches the constant.
	c := vshrink.Config{Input: "test.mp4"}
	args := vshrink.BuildArgs(c)
	found := false
	for i, a := range args {
		if a == "--preset" && i+1 < len(args) && args[i+1] == vshrink.DefaultPreset {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("BuildArgs with empty preset should use DefaultPreset %q, got %v", vshrink.DefaultPreset, args)
	}
}
