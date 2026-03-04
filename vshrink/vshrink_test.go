package vshrink_test

import (
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
			name:   "explicit output overrides derivation",
			config: vshrink.Config{Input: "/path/to/video.mp4", Output: "/other/output.mp4"},
			want:   "/other/output.mp4",
		},
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
			name:   "custom preset and explicit output",
			config: vshrink.Config{Input: "video.mp4", Preset: "HQ 1080p30 Surround", Output: "out.mp4"},
			want: []string{
				"--preset-import-gui",
				"--preset", "HQ 1080p30 Surround",
				"-i", "video.mp4",
				"-o", "out.mp4",
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
		// Use the system 'true' command as a stand-in for HandBrakeCLI.
		cfg := vshrink.Config{
			Input:         "video.mp4",
			HandbrakePath: "true",
		}
		if err := vshrink.Run(cfg); err != nil {
			t.Errorf("Run() returned unexpected error: %v", err)
		}
	})

	t.Run("wraps error message on non-zero exit", func(t *testing.T) {
		// Use the system 'false' command to simulate a HandBrakeCLI failure.
		cfg := vshrink.Config{
			Input:         "video.mp4",
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
}
