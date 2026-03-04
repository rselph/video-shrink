# video-shrink

Use HandBrakeCLI to re-encode video files in bulk for Plex, etc.

## Usage

```shell
vshrink [options] input-file
    -o, -output file        Set output file name (not valid when input-file is a directory)
    -s, -suffix name        Use "name" instead of ".vshrink" for output files
[[future feature]]    -in-place               Attempt to replace the original file with the converted file
    -p, -preset             Name of the HandBrake preset to use
    -handbrake              Path to the HandBrakeCLI executable
```

By default video-shrink will use a conversion profile named "vshrink" from HandBrake's GUI to
re-encode the input file.  The output file will be a video file in the same directory with
".vshrink" added to the filename, before the type suffix. There are lots of options to change these
default behaviors.

If the input-file is actually a directory, video-shrink will search recursively for video files and
convert each one as it finds it.  Files whose names already contain the suffix are skipped to avoid
re-encoding previously converted outputs.  This behavior is not compatible with the -output option.

[[future feature]] When using the -in-place option, video-shrink won't touch the original file if it is smaller than
the converted file.
