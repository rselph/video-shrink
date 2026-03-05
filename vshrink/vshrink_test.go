package vshrink_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rselph/video-shrink/vshrink"
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
