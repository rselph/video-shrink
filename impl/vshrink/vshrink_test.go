package vshrink_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
				"-o", "video.tmp.vshrink.mp4",
			},
		},
		{
			name:   "custom preset applied",
			config: vshrink.Config{Input: "video.mp4", Preset: "HQ 1080p30 Surround"},
			want: []string{
				"--preset-import-gui",
				"--preset", "HQ 1080p30 Surround",
				"-i", "video.mp4",
				"-o", "video.tmp.vshrink.mp4",
			},
		},
		{
			name:   "custom suffix applied",
			config: vshrink.Config{Input: "/movies/film.avi", Suffix: ".enc"},
			want: []string{
				"--preset-import-gui",
				"--preset", vshrink.DefaultPreset,
				"-i", "/movies/film.avi",
				"-o", "/movies/film.tmp.enc.avi",
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
			FFmpegPath:    "true",
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
		// Use the system 'true' command as a stand-in for HandBrakeCLI and ffmpeg.
		cfg := vshrink.Config{
			Input:         input,
			HandbrakePath: "true",
			FFmpegPath:    "true",
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
			FFmpegPath:    "true",
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
			FFmpegPath:    "true",
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

func TestRemuxMarkerPath(t *testing.T) {
	tests := []struct {
		name   string
		config vshrink.Config
		want   string
	}{
		{
			name:   "basic file",
			config: vshrink.Config{Input: "/path/to/video.mp4"},
			want:   "/path/to/" + vshrink.RemuxMarkerPrefix + "video.mp4" + vshrink.MarkerSuffix,
		},
		{
			name:   "file in current directory",
			config: vshrink.Config{Input: "movie.mkv"},
			want:   vshrink.RemuxMarkerPrefix + "movie.mkv" + vshrink.MarkerSuffix,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vshrink.RemuxMarkerPath(tt.config)
			if got != tt.want {
				t.Errorf("RemuxMarkerPath() = %q, want %q", got, tt.want)
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
		// Remux marker: .vshrink-remux.<basename>.done
		{".vshrink-remux.video.mp4.done", true},
		{"/path/to/.vshrink-remux.clip.mkv.done", true},
		{".vshrink-remux.video.mp4", false}, // prefix but no .done suffix
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

func TestIsMarkedAndMarkerTime(t *testing.T) {
	t.Run("returns true for xattr", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		if err := vshrink.SetXattr(input, vshrink.FormatXattrValue(time.Now(), "test")); err != nil {
			t.Fatal(err)
		}
		if !vshrink.IsMarked(cfg) {
			t.Error("IsMarked() should return true when xattr exists")
		}
		mt, err := vshrink.MarkerTime(cfg)
		if err != nil {
			t.Errorf("MarkerTime() returned unexpected error: %v", err)
		}
		if mt.IsZero() {
			t.Error("MarkerTime() returned zero time")
		}
	})

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
		mt, err := vshrink.MarkerTime(cfg)
		if err != nil {
			t.Errorf("MarkerTime() returned unexpected error: %v", err)
		}
		if mt.IsZero() {
			t.Error("MarkerTime() returned zero time")
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
		mt, err := vshrink.MarkerTime(cfg)
		if err != nil {
			t.Errorf("MarkerTime() returned unexpected error: %v", err)
		}
		if mt.IsZero() {
			t.Error("MarkerTime() returned zero time")
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
		_, err := vshrink.MarkerTime(cfg)
		if err == nil {
			t.Error("MarkerTime() should return error when no marker exists")
		}
	})

	t.Run("xattr preferred over marker files", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		// Create both an xattr and a marker file with different timestamps.
		xattrTime := time.Now().Add(-30 * time.Second)
		if err := vshrink.SetXattr(input, vshrink.FormatXattrValue(xattrTime, "test")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vshrink.MarkerPath(cfg), []byte("current"), 0644); err != nil {
			t.Fatal(err)
		}
		mt, err := vshrink.MarkerTime(cfg)
		if err != nil {
			t.Fatalf("MarkerTime() returned unexpected error: %v", err)
		}
		// MarkerTime should return the xattr timestamp, not the marker file mtime.
		if mt.Sub(xattrTime).Abs() > time.Second {
			t.Errorf("MarkerTime() = %v, expected close to xattr time %v", mt, xattrTime)
		}
	})

	t.Run("stale marker returns false", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		// Set the video file mtime far into the future.
		future := time.Now().Add(10 * time.Minute)
		if err := os.Chtimes(input, future, future); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		// Create a marker file with the default (current) mtime.
		if err := os.WriteFile(vshrink.MarkerPath(cfg), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		if vshrink.IsMarked(cfg) {
			t.Error("IsMarked() should return false when marker is stale")
		}
	})

	t.Run("old xattr timestamp is still marked", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		// Set the video file mtime far into the future.
		future := time.Now().Add(10 * time.Minute)
		if err := os.Chtimes(input, future, future); err != nil {
			t.Fatal(err)
		}
		cfg := vshrink.Config{Input: input}
		// Set an xattr with the current (non-future) time.
		if err := vshrink.SetXattr(input, vshrink.FormatXattrValue(time.Now(), "test")); err != nil {
			t.Fatal(err)
		}
		// An xattr travels with the file, so its timestamp never makes it stale.
		if !vshrink.IsMarked(cfg) {
			t.Error("IsMarked() should return true for an xattr with an old timestamp")
		}
	})

	t.Run("xattr without timestamp is marked", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "video.mp4")
		if err := os.WriteFile(input, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := vshrink.SetXattr(input, "garbage"); err != nil {
			t.Fatal(err)
		}
		if !vshrink.IsMarked(vshrink.Config{Input: input}) {
			t.Error("IsMarked() should return true when the xattr exists")
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
	// File should be marked (xattr or marker file).
	if !vshrink.IsMarked(cfg) {
		t.Error("file should be marked after swap")
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked remuxed after swap")
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
	cfg.HandbrakePath = "false"
	// ffmpeg stand-in remuxes the original after the larger output is discarded.
	cfg.FFmpegPath = writeScript(t, dir, "ff.sh", "cp \"$2\" \"$9\"\n")
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	// The input should hold the remuxed original.
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("cannot read input: %v", err)
	}
	if !reflect.DeepEqual(data, origContent) {
		t.Errorf("input content = %q, want remuxed original %q", data, origContent)
	}
	// Output should have been removed.
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("larger output should be removed, stat err = %v", err)
	}
	// File should be marked both encoded and remuxed.
	if !vshrink.IsMarked(cfg) {
		t.Error("file should be marked even when output was discarded")
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked remuxed after the original was remuxed")
	}
	if got := encodedMarkerText(t, input); !strings.Contains(got, "(not replaced)") {
		t.Errorf("encoded marker text = %q, want it to note the original was kept", got)
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
	if !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked remuxed after swap")
	}
}

func TestRunCleansUpOutputOnError(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	// Script creates the temp output file ($7) then exits 1 to simulate a failed encode.
	script := filepath.Join(dir, "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \"$7\"\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{
		Input:         input,
		HandbrakePath: script,
		FFmpegPath:    "true",
		KeepOnError:   false,
	}
	output := vshrink.TempPath(cfg)
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
	// Script creates the temp output file ($7) then exits 1.
	script := filepath.Join(dir, "fail-with-output.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \"$7\"\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{
		Input:         input,
		HandbrakePath: script,
		FFmpegPath:    "true",
		KeepOnError:   true,
	}
	output := vshrink.TempPath(cfg)
	err := vshrink.Run(cfg)
	if err == nil {
		t.Fatal("expected error from failing HandBrakeCLI")
	}
	// With KeepOnError set, output should still exist.
	if _, statErr := os.Stat(output); os.IsNotExist(statErr) {
		t.Errorf("output file should be retained when KeepOnError is set")
	}
}

func TestRunMarkOnly(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	// Use 'false' — if HandBrakeCLI is invoked, Run returns an error.
	cfg := vshrink.Config{Input: input, HandbrakePath: "false", MarkOnly: true}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() with MarkOnly error: %v", err)
	}
	if !vshrink.IsMarked(cfg) {
		t.Error("file should be marked after Run() with MarkOnly")
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked remuxed after Run() with MarkOnly")
	}
	if _, err := os.Stat(vshrink.OutputPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("output file should not exist, stat err = %v", err)
	}
	if data, _ := os.ReadFile(input); string(data) != "original" {
		t.Errorf("input should be unchanged, got %q", data)
	}

	// A normal run afterwards should skip the file.
	cfg.MarkOnly = false
	cfg.InPlace = true
	if err := vshrink.Run(cfg); err != nil {
		t.Errorf("Run() should skip marked file, got error: %v", err)
	}
}

func TestRunMarkOnlyAddsRemuxMarkerToLegacyFile(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	// A file processed by an older version carries only the encoded marker.
	encodedValue := vshrink.FormatXattrValue(time.Now(), "original size: 100, new size: 50")
	if err := vshrink.SetXattr(input, encodedValue); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{Input: input, HandbrakePath: "false", MarkOnly: true}
	if vshrink.IsRemuxed(cfg) {
		t.Fatal("legacy file should not start out remuxed")
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() with MarkOnly error: %v", err)
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("legacy file should be marked remuxed after Run() with MarkOnly")
	}
	// The existing encoded marker should be left as it was.
	if got, err := vshrink.GetXattr(input); err != nil || got != encodedValue {
		t.Errorf("encoded xattr = %q (err %v), want %q", got, err, encodedValue)
	}
}

func TestRunMarkOnlyDir(t *testing.T) {
	dir := t.TempDir()
	videos := []string{filepath.Join(dir, "a.mp4"), filepath.Join(dir, "sub", "b.mkv")}
	for _, v := range videos {
		if err := os.MkdirAll(filepath.Dir(v), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(v, []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := vshrink.Config{Input: dir, HandbrakePath: "false", MarkOnly: true}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() with MarkOnly on dir error: %v", err)
	}
	for _, v := range videos {
		if !vshrink.IsRemuxed(vshrink.Config{Input: v}) {
			t.Errorf("%s should be marked remuxed", v)
		}
		if !vshrink.IsMarked(vshrink.Config{Input: v}) {
			t.Errorf("%s should be marked", v)
		}
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
	if err := os.WriteFile(output, []byte(strings.Repeat("x", len(content))), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.HandbrakePath = "false"
	cfg.FFmpegPath = writeScript(t, dir, "ff.sh", "cp \"$2\" \"$9\"\n")
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
	if !vshrink.IsMarked(cfg) {
		t.Error("file should be marked even when output was discarded")
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked remuxed after the original was remuxed")
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

func TestTempPath(t *testing.T) {
	tests := []struct {
		name   string
		config vshrink.Config
		want   string
	}{
		{
			name:   "default suffix",
			config: vshrink.Config{Input: "/path/to/video.mp4"},
			want:   "/path/to/video.tmp.vshrink.mp4",
		},
		{
			name:   "custom suffix",
			config: vshrink.Config{Input: "/path/to/video.mkv", Suffix: ".small"},
			want:   "/path/to/video.tmp.small.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vshrink.TempPath(tt.config)
			if got != tt.want {
				t.Errorf("TempPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildRemuxArgs(t *testing.T) {
	c := vshrink.Config{Input: "/movies/film.mp4"}
	want := []string{
		"-i", "/movies/film.tmp.vshrink.mp4",
		"-map", "0",
		"-c", "copy",
		"-max_interleave_delta", "0",
		"/movies/film.vshrink.mp4",
	}
	if got := vshrink.BuildRemuxArgs(c, vshrink.TempPath(c)); !reflect.DeepEqual(got, want) {
		t.Errorf("BuildRemuxArgs() = %v, want %v", got, want)
	}
}

// writeScript writes an executable shell script to dir and returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunRemuxesToOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{
		Input: input,
		// HandBrakeCLI stand-in writes the temp file ($7).
		HandbrakePath: writeScript(t, dir, "hb.sh", "echo encoded > \"$7\"\n"),
		// ffmpeg stand-in copies its input ($2) to its output ($9).
		FFmpegPath: writeScript(t, dir, "ff.sh", "cp \"$2\" \"$9\"\n"),
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	data, err := os.ReadFile(vshrink.OutputPath(cfg))
	if err != nil {
		t.Fatalf("output file should exist after remux: %v", err)
	}
	if string(data) != "encoded\n" {
		t.Errorf("output content = %q, want %q", data, "encoded\n")
	}
	if _, err := os.Stat(vshrink.TempPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("temp file should be removed after remux, stat err = %v", err)
	}
}

func TestRunRemuxFailureCleansUp(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := vshrink.Config{
		Input:         input,
		HandbrakePath: writeScript(t, dir, "hb.sh", "touch \"$7\"\n"),
		// ffmpeg stand-in creates a partial output ($9) then fails.
		FFmpegPath: writeScript(t, dir, "ff.sh", "touch \"$9\"\nexit 1\n"),
	}
	err := vshrink.Run(cfg)
	if err == nil {
		t.Fatal("expected error from failing ffmpeg")
	}
	if !strings.Contains(err.Error(), "ffmpeg remux failed") {
		t.Errorf("error message %q does not contain %q", err.Error(), "ffmpeg remux failed")
	}
	for _, p := range []string{vshrink.OutputPath(cfg), vshrink.TempPath(cfg)} {
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Errorf("%s should be removed on remux error, stat err = %v", p, statErr)
		}
	}
}

func TestRunMissingFFmpegSkipsEncode(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "handbrake-ran")
	cfg := vshrink.Config{
		Input:         input,
		HandbrakePath: writeScript(t, dir, "hb.sh", "touch \""+sentinel+"\"\n"),
		FFmpegPath:    "/nonexistent/ffmpeg",
	}
	if err := vshrink.Run(cfg); err == nil {
		t.Fatal("expected error for missing ffmpeg, got nil")
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("HandBrakeCLI should not run when ffmpeg is missing")
	}
}

// encodedMarkerText returns the encoded marker xattr on path without its timestamp.
func encodedMarkerText(t *testing.T, path string) string {
	t.Helper()
	value, err := vshrink.GetXattr(path)
	if err != nil {
		t.Fatalf("GetXattr() error: %v", err)
	}
	_, text, _ := strings.Cut(value, " ")
	return text
}

// inPlaceFixture creates an input file and HandBrakeCLI/ffmpeg stand-ins for an
// in-place run.  The HandBrakeCLI stand-in writes encoded to its output ($7) and
// touches a sentinel; the ffmpeg stand-in records its input ($2) and runs
// ffmpegBody to produce its output ($9).
func inPlaceFixture(t *testing.T, original, encoded, ffmpegBody string) (cfg vshrink.Config, hbSentinel, ffInput string) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(input, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	hbSentinel = filepath.Join(dir, "hb-ran")
	ffInput = filepath.Join(dir, "ff-input")
	cfg = vshrink.Config{
		Input:         input,
		InPlace:       true,
		HandbrakePath: writeScript(t, dir, "hb.sh", "touch \""+hbSentinel+"\"\nprintf '%s' '"+encoded+"' > \"$7\"\n"),
		FFmpegPath:    writeScript(t, dir, "ff.sh", "echo \"$2\" > \""+ffInput+"\"\n"+ffmpegBody),
	}
	return cfg, hbSentinel, ffInput
}

func readTrimmed(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	return strings.TrimSpace(string(data))
}

func TestRunInPlaceEncodesAndRemuxes(t *testing.T) {
	cfg, hbSentinel, ffInput := inPlaceFixture(t, "a much longer original file", "small", "cp \"$2\" \"$9\"\n")
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); err != nil {
		t.Error("HandBrakeCLI should run for an unmarked file")
	}
	if got := readTrimmed(t, ffInput); got != vshrink.TempPath(cfg) {
		t.Errorf("ffmpeg input = %q, want re-encoded file %q", got, vshrink.TempPath(cfg))
	}
	if got := readTrimmed(t, cfg.Input); got != "small" {
		t.Errorf("input content = %q, want re-encoded content", got)
	}
	if !vshrink.IsMarked(cfg) || !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked encoded and remuxed")
	}
	if _, err := os.Stat(vshrink.TempPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("temp file should be removed, stat err = %v", err)
	}
}

func TestRunInPlaceRemuxesOriginalWhenEncodeNotSmaller(t *testing.T) {
	cfg, hbSentinel, ffInput := inPlaceFixture(t, "orig", "a much larger re-encoded file", "cp \"$2\" \"$9\"\n")
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); err != nil {
		t.Error("HandBrakeCLI should run for an unmarked file")
	}
	if got := readTrimmed(t, ffInput); got != cfg.Input {
		t.Errorf("ffmpeg input = %q, want original %q", got, cfg.Input)
	}
	if got := readTrimmed(t, cfg.Input); got != "orig" {
		t.Errorf("input content = %q, want remuxed original", got)
	}
	if _, err := os.Stat(vshrink.TempPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("temp file should be removed, stat err = %v", err)
	}
	if !vshrink.IsMarked(cfg) || !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked encoded and remuxed")
	}
	if got := encodedMarkerText(t, cfg.Input); !strings.Contains(got, "(not replaced)") {
		t.Errorf("encoded marker text = %q, want it to note the original was kept", got)
	}
}

func TestRunInPlaceRemuxOnlyWhenEncoded(t *testing.T) {
	// The ffmpeg stand-in writes a larger file; the remux pass must not compare sizes.
	cfg, hbSentinel, ffInput := inPlaceFixture(t, "orig", "unused", "cat \"$2\" \"$2\" > \"$9\"\n")
	text := "original size: 100, new size: 50 (50.000000%)"
	if err := vshrink.SetXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), text)); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); !os.IsNotExist(err) {
		t.Error("HandBrakeCLI should not run for an already encoded file")
	}
	if got := readTrimmed(t, ffInput); got != cfg.Input {
		t.Errorf("ffmpeg input = %q, want original %q", got, cfg.Input)
	}
	if got := readTrimmed(t, cfg.Input); got != "origorig" {
		t.Errorf("input content = %q, want larger remuxed file", got)
	}
	if !vshrink.IsMarked(cfg) || !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked encoded and remuxed")
	}
	if got := encodedMarkerText(t, cfg.Input); got != text {
		t.Errorf("encoded marker text = %q, want carried over %q", got, text)
	}
}

func TestRunInPlaceRemuxOnlySwapsLeftoverOutput(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
	cfg.HandbrakePath = "false"
	cfg.FFmpegPath = "false"
	if err := vshrink.SetXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), "test")); err != nil {
		t.Fatal(err)
	}
	// A larger leftover remux output is swapped in without a size check.
	if err := os.WriteFile(vshrink.OutputPath(cfg), []byte("origorig"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if got := readTrimmed(t, cfg.Input); got != "origorig" {
		t.Errorf("input content = %q, want leftover output", got)
	}
	if !vshrink.IsMarked(cfg) || !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked encoded and remuxed")
	}
}

func TestRunInPlaceEncodesWhenOnlyRemuxed(t *testing.T) {
	cfg, hbSentinel, ffInput := inPlaceFixture(t, "a much longer original file", "small", "cp \"$2\" \"$9\"\n")
	if err := vshrink.SetRemuxXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), "remuxed")); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); err != nil {
		t.Error("HandBrakeCLI should run for a file that is remuxed but not encoded")
	}
	if got := readTrimmed(t, ffInput); got != vshrink.TempPath(cfg) {
		t.Errorf("ffmpeg input = %q, want re-encoded file %q", got, vshrink.TempPath(cfg))
	}
	if got := readTrimmed(t, cfg.Input); got != "small" {
		t.Errorf("input content = %q, want re-encoded content", got)
	}
	if !vshrink.IsMarked(cfg) || !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked encoded and remuxed")
	}
}

func TestRunInPlaceSkipsEncodedAndRemuxed(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
	cfg.HandbrakePath = "false"
	cfg.FFmpegPath = "false"
	if err := vshrink.MarkSkipped(cfg); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() should skip a fully marked file, got error: %v", err)
	}
	if got := readTrimmed(t, cfg.Input); got != "orig" {
		t.Errorf("input content = %q, want unchanged", got)
	}
}

func TestRunSkipsEncodedFileWithoutInPlace(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
	cfg.InPlace = false
	cfg.HandbrakePath = "false"
	cfg.FFmpegPath = "false"
	if err := vshrink.SetXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), "test")); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() should skip an encoded file without -in-place, got error: %v", err)
	}
	if _, err := os.Stat(vshrink.OutputPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("no output should be written, stat err = %v", err)
	}
}

func TestRunInPlaceSkipsCopiedMarkedFile(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
	cfg.HandbrakePath = "false"
	cfg.FFmpegPath = "false"
	// Markers written long ago; copying the file then gave it a new mtime.
	old := vshrink.FormatXattrValue(time.Now().Add(-24*time.Hour), "test")
	if err := vshrink.SetXattr(cfg.Input, old); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.SetRemuxXattr(cfg.Input, old); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(cfg.Input, now, now); err != nil {
		t.Fatal(err)
	}
	if !vshrink.IsMarked(cfg) || !vshrink.IsRemuxed(cfg) {
		t.Fatal("copied file should still be marked encoded and remuxed")
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() should skip a copied, fully marked file, got error: %v", err)
	}
	if got := readTrimmed(t, cfg.Input); got != "orig" {
		t.Errorf("input content = %q, want unchanged", got)
	}
}

func TestRunRejectsEncodeOnlyWithMuxOnly(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
	cfg.EncodeOnly = true
	cfg.MuxOnly = true
	if err := vshrink.Run(cfg); err == nil {
		t.Error("Run() should reject -encode-only with -mux-only")
	}
}

func TestRunInPlaceEncodeOnly(t *testing.T) {
	cfg, hbSentinel, _ := inPlaceFixture(t, "a much longer original file", "small", "")
	cfg.EncodeOnly = true
	cfg.FFmpegPath = "false"
	// A remux marker file names the original, so it must not survive the swap.
	if err := os.WriteFile(vshrink.RemuxMarkerPath(cfg), []byte("remuxed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); err != nil {
		t.Error("HandBrakeCLI should run")
	}
	if got := readTrimmed(t, cfg.Input); got != "small" {
		t.Errorf("input content = %q, want re-encoded content", got)
	}
	if _, err := os.Stat(vshrink.RemuxMarkerPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("remux marker file should be removed, stat err = %v", err)
	}
	if !vshrink.IsMarked(cfg) || vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked encoded but not remuxed")
	}
	for _, p := range []string{vshrink.TempPath(cfg), vshrink.OutputPath(cfg)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should not exist, stat err = %v", p, err)
		}
	}
}

func TestRunInPlaceEncodeOnlyKeepsOriginalWhenNotSmaller(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "a much larger re-encoded file", "")
	cfg.EncodeOnly = true
	cfg.FFmpegPath = "false"
	if err := vshrink.SetRemuxXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), "remuxed")); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if got := readTrimmed(t, cfg.Input); got != "orig" {
		t.Errorf("input content = %q, want original", got)
	}
	if _, err := os.Stat(vshrink.TempPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("temp file should be removed, stat err = %v", err)
	}
	if !vshrink.IsMarked(cfg) {
		t.Error("original should be marked encoded")
	}
	if got := encodedMarkerText(t, cfg.Input); !strings.Contains(got, "(not replaced)") {
		t.Errorf("encoded marker text = %q, want it to note the original was kept", got)
	}
	if !vshrink.IsRemuxed(cfg) {
		t.Error("the original's remux marker should be left alone")
	}
}

func TestRunInPlaceEncodeOnlySkipsEncoded(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
	cfg.EncodeOnly = true
	cfg.HandbrakePath = "false"
	cfg.FFmpegPath = "false"
	if err := vshrink.SetXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), "test")); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() should skip an encoded file, got error: %v", err)
	}
}

func TestRunInPlaceMuxOnly(t *testing.T) {
	// The ffmpeg stand-in writes a larger file; the remux pass must not compare sizes.
	cfg, hbSentinel, ffInput := inPlaceFixture(t, "orig", "unused", "cat \"$2\" \"$2\" > \"$9\"\n")
	cfg.MuxOnly = true
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); !os.IsNotExist(err) {
		t.Error("HandBrakeCLI should not run with -mux-only")
	}
	if got := readTrimmed(t, ffInput); got != cfg.Input {
		t.Errorf("ffmpeg input = %q, want original %q", got, cfg.Input)
	}
	if got := readTrimmed(t, cfg.Input); got != "origorig" {
		t.Errorf("input content = %q, want larger remuxed file", got)
	}
	if !vshrink.IsRemuxed(cfg) || vshrink.IsMarked(cfg) {
		t.Error("file should be marked remuxed but not encoded")
	}
}

func TestRunInPlaceMuxOnlyCarriesOverEncodedMarker(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "cp \"$2\" \"$9\"\n")
	cfg.MuxOnly = true
	text := "original size: 100, new size: 50 (50.000000%)"
	if err := vshrink.SetXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), text)); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if !vshrink.IsMarked(cfg) || !vshrink.IsRemuxed(cfg) {
		t.Error("file should be marked encoded and remuxed")
	}
	if got := encodedMarkerText(t, cfg.Input); got != text {
		t.Errorf("encoded marker text = %q, want carried over %q", got, text)
	}
}

func TestRunInPlaceMuxOnlySkipsRemuxed(t *testing.T) {
	cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
	cfg.MuxOnly = true
	cfg.HandbrakePath = "false"
	cfg.FFmpegPath = "false"
	if err := vshrink.SetRemuxXattr(cfg.Input, vshrink.FormatXattrValue(time.Now(), "remuxed")); err != nil {
		t.Fatal(err)
	}
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() should skip a remuxed file, got error: %v", err)
	}
}

func TestRunInPlaceSinglePassSkipsLeftoverOutput(t *testing.T) {
	for _, mode := range []string{"encode-only", "mux-only"} {
		t.Run(mode, func(t *testing.T) {
			cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
			cfg.EncodeOnly = mode == "encode-only"
			cfg.MuxOnly = mode == "mux-only"
			cfg.HandbrakePath = "false"
			cfg.FFmpegPath = "false"
			if err := os.WriteFile(vshrink.OutputPath(cfg), []byte("leftover"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := vshrink.Run(cfg); err != nil {
				t.Fatalf("Run() should skip when output exists, got error: %v", err)
			}
			if got := readTrimmed(t, cfg.Input); got != "orig" {
				t.Errorf("input content = %q, want unchanged", got)
			}
			if got := readTrimmed(t, vshrink.OutputPath(cfg)); got != "leftover" {
				t.Errorf("leftover output = %q, want untouched", got)
			}
		})
	}
}

func TestRunEncodeOnlyWithoutInPlace(t *testing.T) {
	cfg, hbSentinel, _ := inPlaceFixture(t, "orig", "encoded", "")
	cfg.InPlace = false
	cfg.EncodeOnly = true
	cfg.FFmpegPath = "/nonexistent/ffmpeg"
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); err != nil {
		t.Error("HandBrakeCLI should run")
	}
	if got := readTrimmed(t, vshrink.OutputPath(cfg)); got != "encoded" {
		t.Errorf("output content = %q, want encoded content", got)
	}
	if _, err := os.Stat(vshrink.TempPath(cfg)); !os.IsNotExist(err) {
		t.Errorf("temp file should not remain, stat err = %v", err)
	}
	if got := readTrimmed(t, cfg.Input); got != "orig" {
		t.Errorf("input content = %q, want unchanged", got)
	}
}

func TestRunMuxOnlyWithoutInPlace(t *testing.T) {
	cfg, hbSentinel, ffInput := inPlaceFixture(t, "orig", "unused", "cp \"$2\" \"$9\"\n")
	cfg.InPlace = false
	cfg.MuxOnly = true
	if err := vshrink.Run(cfg); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(hbSentinel); !os.IsNotExist(err) {
		t.Error("HandBrakeCLI should not run with -mux-only")
	}
	if got := readTrimmed(t, ffInput); got != cfg.Input {
		t.Errorf("ffmpeg input = %q, want original %q", got, cfg.Input)
	}
	if got := readTrimmed(t, vshrink.OutputPath(cfg)); got != "orig" {
		t.Errorf("output content = %q, want remuxed original", got)
	}
}

func TestRunMarkOnlySinglePass(t *testing.T) {
	t.Run("encode-only", func(t *testing.T) {
		cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
		cfg.MarkOnly = true
		cfg.EncodeOnly = true
		if err := vshrink.Run(cfg); err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if !vshrink.IsMarked(cfg) || vshrink.IsRemuxed(cfg) {
			t.Error("-mark -encode-only should record only the encoded marker")
		}
	})
	t.Run("mux-only", func(t *testing.T) {
		cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
		cfg.MarkOnly = true
		cfg.MuxOnly = true
		if err := vshrink.Run(cfg); err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if !vshrink.IsRemuxed(cfg) || vshrink.IsMarked(cfg) {
			t.Error("-mark -mux-only should record only the remux marker")
		}
	})
	t.Run("mux-only on encoded file", func(t *testing.T) {
		cfg, _, _ := inPlaceFixture(t, "orig", "unused", "")
		cfg.MarkOnly = true
		cfg.MuxOnly = true
		value := vshrink.FormatXattrValue(time.Now(), "original size: 100, new size: 50")
		if err := vshrink.SetXattr(cfg.Input, value); err != nil {
			t.Fatal(err)
		}
		if err := vshrink.Run(cfg); err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if !vshrink.IsRemuxed(cfg) {
			t.Error("remux marker should be added")
		}
		if got, err := vshrink.GetXattr(cfg.Input); err != nil || got != value {
			t.Errorf("encoded xattr = %q (err %v), want unchanged %q", got, err, value)
		}
	})
}
