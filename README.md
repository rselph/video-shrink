# vshrink

Use [HandBrakeCLI](https://handbrake.fr/docs/en/latest/cli/cli-options.html) to re-encode video
files in bulk for Plex, etc.

## How It Works

There are two ways that you might want to use vshrink.

- If you just give it the name of a file, vshrink uses a HandBrake preset named `vshrink`
(configured in HandBrake's GUI) to re-encode the input file. The output file is written to the same
directory with `.vshrink` inserted before the file extension (e.g. `movie.mp4` →
`movie.vshrink.mp4`). If the output file already exists, it does nothing.

- After each encode, vshrink remuxes the result with
[ffmpeg](https://ffmpeg.org) (`-map 0 -c copy -max_interleave_delta 0`) so the audio and video
tracks are fully interleaved, which avoids caching problems in some players. HandBrake writes to a
temporary file (e.g. `movie.tmp.vshrink.mp4`). The final output file only appears once the remux has
finished, and the temporary file is then deleted. Nothing is re-encoded during the remux.

- When `-in-place` is set, vshrink tries to replace the original file with the new file, effectively
shrinking the file in place. It tries to make sure that nothing will go wrong that will leave you
without either the original file or the final shrunken file. It's also designed so that you can run
it again and again on the same directory without doing any harm. When a file is processed, vshrink
records an extended attribute (`user.com.rselph.vshrink.done`) on the video file itself to remember
that it has already been handled. On filesystems that don't support extended attributes (e.g.
Windows/NTFS), vshrink falls back to an empty marker file named `.vshrink.<filename>.done` in the
same directory. Old-format marker files (`.vshrink.done.<filename>`) are also recognised and are
automatically upgraded to extended attributes when possible. Sometimes, the re-encoded file will
actually be larger than the original. When this happens, vshrink discards it and keeps the original
unless `-ignore-size` is set. The original is still remuxed.

- Separately, vshrink records a second extended attribute (`user.com.rselph.vshrink-remux.done`) to
remember that the file has been remuxed. Its fallback marker file is
`.vshrink-remux.<filename>.done`. With `-in-place`, vshrink decides each pass from the two markers:

  | Re-encoded | Remuxed | What `-in-place` does |
  |------------|---------|-----------------------|
  | no         | either  | Re-encode, then remux (re-encoding undoes any earlier remux) |
  | yes        | no      | Remux only |
  | yes        | yes     | Nothing |

  Only the re-encode pass compares sizes. The remuxed file always replaces the original, since
  remuxing doesn't change the video. Files processed by older versions of vshrink carry only the
  first marker, so an `-in-place` run remuxes them without re-encoding them.

- When `-mark` is set, vshrink doesn't encode anything. It just records both the "already processed"
and "remuxed" markers on each file, so future runs (including `-in-place` runs) leave those files
alone. Files that have only the "already processed" marker (such as files processed by older
versions of vshrink) get the missing "remuxed" marker. Files that already have both are left as they
are.

### Directory Input

If an input argument is a directory, vshrink recurses into it and converts every recognized video
file it finds. This works for both regular and in-place modes. Files that have already been
processed are skipped, apart from the remux pass described above. Vshrink will never re-encode a
file twice.

Recognized video extensions: `.mp4`, `.mkv`, `.avi`, `.mov`, `.m4v`, `.wmv`, `.flv`, `.webm`,
`.mpeg`, `.mpg`, `.ts`, `.m2ts`, `.vob`.

## Getting [HandBrakeCLI](https://handbrake.fr/docs/en/latest/cli/cli-options.html)

The [HandBrakeCLI utility](https://handbrake.fr/docs/en/latest/cli/cli-options.html) isn't
automatically installed by [HandBrake](https://handbrake.fr). It's a separate download. You can get
it at the [HandBrake GitHub page](https://github.com/HandBrake/HandBrake/releases). Go to the latest
release, and download HandBrakeCLI for your platform from the Assets section. You can put it
anywhere, but for greatest convenience I recommend putting it in `/usr/local/bin`.

## Getting [ffmpeg](https://ffmpeg.org)

vshrink also needs `ffmpeg` for the remux step. On macOS, run `brew install ffmpeg` (Homebrew) or `sudo port install ffmpeg` (MacPorts). On Linux, use
your distribution's package manager. Otherwise, download it from
[ffmpeg.org](https://ffmpeg.org/download.html). If it isn't on your `PATH`, use `-ffmpeg` to point
vshrink at it.

## Setting Up Your Preset in HandBrake

By default, vshrink will look for a preset named "vshrink" from the HandBrake UI to do the encoding.
This allows you to set things up just the way you want in HandBrake before vshrink does its thing.

Run `vshrink -print-preset > vshrink.json` to save a suggested preset that you can import into
HandBrake. This makes a good starting point that you can customize as you see fit.

## Running vshrink

```shell
vshrink [options] input-file [input-file...]
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-s`, `-suffix` | `.vshrink` | Suffix inserted before the file extension to form the output file name |
| `-p`, `-preset` | `vshrink` | Name of the HandBrake preset to use |
| `-handbrake` | `HandBrakeCLI` | Path to the HandBrakeCLI executable |
| `-ffmpeg` | `ffmpeg` | Path to the ffmpeg executable used for the remux step |
| `-in-place` | false | Replace the original file with the re-encoded (if smaller) and remuxed file |
| `-ignore-size` | false | With `-in-place`, keep the re-encoded file even if it is larger than the original |
| `-keep` | false | Keep the output file when something goes wrong instead of deleting it |
| `-mark` | false | Mark files as processed without encoding them, so later runs skip them |
| `-continue` | false | Continue processing remaining files after an error |
| `-v` | false | Verbose output (show HandBrakeCLI and ffmpeg stderr) |
| `-no-progress` | false | Disable progress output (suppress HandBrakeCLI stdout) |
| `-print-preset` | false | Print the suggested HandBrake preset JSON to stdout and exit |
