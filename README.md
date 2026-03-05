# video-shrink

Use HandBrakeCLI to re-encode video files in bulk for Plex, etc.

## Usage

```shell
vshrink [options] input-file [input-file...]
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-o`, `-output` | _(derived)_ | Set output file name (incompatible with directory input and `-in-place`) |
| `-s`, `-suffix` | `.vshrink` | Suffix inserted before the file extension when `-output` is not set |
| `-p`, `-preset` | `vshrink` | Name of the HandBrake preset to use |
| `-handbrake` | `HandBrakeCLI` | Path to the HandBrakeCLI executable |
| `-in-place` | false | Replace the original file with the output when the output is smaller (incompatible with `-output`) |
| `-keep` | false | Keep the output file on error instead of deleting it |
| `-continue` | false | Continue processing remaining files after an error |
| `-v` | false | Verbose output (show HandBrakeCLI stderr) |
| `-no-progress` | false | Disable progress output (suppress HandBrakeCLI stdout) |

## Behavior

By default, vshrink uses a HandBrake preset named `vshrink` (configured in HandBrake's GUI) to
re-encode the input file. The output file is written to the same directory with `.vshrink` inserted
before the file extension (e.g. `movie.mp4` → `movie.vshrink.mp4`).

If the output file already exists, the file is skipped. When `-in-place` is set and the output
already exists, vshrink proceeds directly to the in-place swap step without re-encoding.

### Directory input

If an input argument is a directory, vshrink recurses into it and converts every recognized video
file it finds. Files whose names already contain the suffix are skipped to avoid re-encoding
previously converted outputs. Directory input is not compatible with the `-output` option.

Recognized video extensions: `.mp4`, `.mkv`, `.avi`, `.mov`, `.m4v`, `.wmv`, `.flv`, `.webm`,
`.mpeg`, `.mpg`, `.ts`, `.m2ts`, `.vob`.

### In-place replacement

When `-in-place` is set, vshrink replaces the original file with the converted output only if the
output is smaller. If the output is not smaller, it is discarded and the original is left untouched.

The swap is done safely in three steps so the original is never destroyed before a valid replacement
exists. If the process is interrupted mid-swap, vshrink attempts to restore the original file.
