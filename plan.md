# Migration Plan: .vshrink Marker Files → Extended Attributes

## Goal

Replace `.vshrink` marker files with extended attributes (`xattr`) on the video
files themselves, using the `github.com/pkg/xattr` package.  Retain backward
compatibility with both legacy marker formats, actively upgrade old markers to
xattr, and fall back to marker files on filesystems that don't support xattr
(e.g. Windows/NTFS).

---

## Step 1 — Add the `github.com/pkg/xattr` dependency

- `go get github.com/pkg/xattr`
- Verify the module builds and existing tests still pass.

## Step 2 — Define the xattr key, value format, and helpers

Add low-level helpers in a new file `marker.go`:

- Choose an xattr name, e.g. `user.com.rselph.vshrink.done`.
- **Value format:** Because xattrs don't carry their own mtime, embed a
  timestamp in the value so the existing "marker is stale if the video file has
  been modified significantly after the marker was written" logic still works.
  Use a simple single line text format, e.g.:

  ```
  2026-03-25T14:30:00Z original size: 123456, new size: 78901 (64.00%)
  ```

  The `timestamp` field records the wall-clock time when the xattr was written
  (i.e. when processing completed).  The rest is the same human-readable size
  summary already stored in marker files today.

- `SetXattr(path string, value string) error` — sets the xattr on a file.
  Callers pass the fully-formatted value including the timestamp field.
- `GetXattr(path string) (string, error)` — reads the xattr from a file.
- `RemoveXattr(path string) error` - removes the xattr from a file.
- `ParseXattrTimestamp(value string) (time.Time, error)` — extracts the
  `timestamp:` field from an xattr value string.

Each helper wraps `github.com/pkg/xattr` calls and translates
`xattr.ENOTSUP` / `syscall.ENOTSUP` into a clean sentinel error or boolean so
callers don't need to reason about platform specifics.

**Tests:** unit tests for set/get/remove round-trip; test
`ParseXattrTimestamp` round-trips correctly and rejects malformed values.

## Step 3 — Introduce a unified `IsMarked` / `MarkComplete` abstraction

Replace the current `IsMarked`, `MarkerInfo`, and `markComplete` with a
higher-level API that checks all three sources in priority order:

1. **xattr** on the video file (`user.com.rselph.vshrink.done`). If the GetXattr call
returns an error indicating that extended attributes are not supported, this is
logically equivalent to a missing xattr.
2. **Current marker file** (`.vshrink.<basename>.done`).
3. **Legacy marker file** (`.vshrink.done.<basename>`).

The existing time-based staleness check in `runFile` compares the marker's
mtime against the video file's mtime and disregards the marker when the video
is significantly newer (`inInfo.ModTime().Sub(markerInfo.ModTime()) < 1*time.Minute`).
This must continue to work for all three source types:

- **Marker files (current & legacy):** use the marker file's mtime as today.
- **xattr:** parse the `timestamp:` field from the xattr value (since xattrs
  have no mtime of their own) and compare it to the video file's mtime using
  the same 1-minute threshold.

New exported surface:

| Function | Behaviour |
|---|---|
| `IsMarked(c Config) bool` | Returns true if *any* of the three sources indicates the file is done **and** the marker is not stale relative to the video's mtime. |
| `MarkerTime(c Config) (time.Time, error)` | Returns the effective timestamp of the marker — xattr `timestamp:` field, or marker file mtime — for use in the staleness check. Replaces `MarkerInfo`. |
| `MarkComplete(c Config, origInfo, newInfo os.FileInfo) error` | Writes the xattr (including `timestamp: <now>`); falls back to writing a marker file if xattr is unsupported. |

Internally `IsMarked` records *which* source matched so Step 4 can upgrade.

**Tests:** table-driven tests covering every combination of xattr-present,
current-marker-present, legacy-marker-present, and none-present, on a
filesystem that supports xattr.  Include cases where the timestamp/mtime is
stale (video mtime >> marker time) and assert `IsMarked` returns false.

## Step 4 — Active upgrade of old markers to xattr

When `IsMarked` finds a marker file but no xattr:

1. Read the content of the marker file (it contains size stats).
2. Construct the xattr value: prepend `timestamp: <marker-file-mtime>` (to
   preserve the original completion time) followed by the marker file's
   existing content.
3. Call `SetXattr` to write the combined value to the video file.
4. If `SetXattr` succeeds, delete the marker file(s).
5. If `SetXattr` fails (unsupported FS), leave the marker file in place.

Factor this into a helper `UpgradeMarker(c Config) error` that is called from
`IsMarked` (or from the call-sites that invoke `IsMarked`).

During directory walks (`runDir`), also call `UpgradeMarker` for every marked
file so that running vshrink on a previously processed tree converts all old
markers in one pass.

**Tests:**
- Create a temp file with a legacy marker, call `UpgradeMarker`, assert xattr
  is set and marker file is deleted.
- Same for the current-format marker.
- On a simulated unsupported-FS path (or using a build-tag stub), assert the
  marker file is preserved.

## Step 5 — Update `markComplete` and `swapInPlace`

- `markComplete` → call `MarkComplete` (Step 3) instead of creating a marker
  file directly.
- In the `swapInPlace` signal-recovery goroutine, remove the xattr (instead of
  / in addition to removing the marker file) when rolling back an incomplete
  swap.
- In `runFile`, the skip-if-marked check now goes through the unified
  `IsMarked`, which already covers xattr + both marker formats.

**Tests:** update `TestRunInPlaceSwapsWhenOutputSmaller`,
`TestRunInPlaceDiscardsWhenOutputLarger`, and similar tests to assert that
the xattr is set (or marker file exists on unsupported FS) after completion.

## Step 6 — Update `runDir` to stop skipping marker files by name

Currently `runDir` calls `IsMarkerFile(path)` to avoid feeding marker files to
HandBrakeCLI.  Once old markers are upgraded and deleted, new runs won't
produce marker files on xattr-capable systems, so this guard becomes less
important — but keep it for safety on fallback filesystems.  No functional
change needed; just verify the existing guard still works.

## Step 7 — Update tests for full coverage

- Ensure every existing test still passes (marker-file behaviour is preserved
  on fallback paths).
- Add integration-style tests:
  - Process a file → verify xattr set, no marker file.
  - Process a file on a fallback FS → verify marker file, no xattr.
  - Re-run on a directory with old markers → verify markers upgraded to xattr
    and deleted.
  - Verify `IsMarkerFile` still correctly identifies leftover markers so they
    are not re-encoded.

## Step 8 — Update README / documentation

- Document the new `user.com.rselph.vshrink.done` xattr.
- Note the automatic upgrade behaviour.
- Mention the fallback to marker files on unsupported filesystems.
- Remove or de-emphasise references to `.vshrink.done.*` / `.vshrink.*.done`
  files (note they may still appear on NTFS, etc.).

---

## Design Notes

- **xattr key:** `user.com.rselph.vshrink.done` (the `user.` namespace is required on
  Linux; macOS uses the same convention via `pkg/xattr`).
- **xattr value:** a single-line text blob. The first part is
  `timestamp: <RFC 3339>` recording when processing completed (since xattrs
  have no mtime).  Subsequent parts contain the same human-readable size
  summary currently written to marker files, e.g.:
  ```
  2026-03-25T14:30:00Z original size: 123456, new size: 78901 (64.00%)
  ```
- **Staleness check:** `runFile` currently skips a file when
  `inInfo.ModTime().Sub(markerInfo.ModTime()) < 1*time.Minute`.  For xattr
  markers the comparison becomes
  `inInfo.ModTime().Sub(xattrTimestamp) < 1*time.Minute`, using the parsed
  timestamp field.  The threshold and semantics are identical.
- **Upgrade timestamp:** when migrating a marker file to xattr (Step 4), use
  the marker file's mtime as the `timestamp:` value so the staleness window
  is preserved exactly.
- **Fallback detection:** probe once per directory (cache the result for the
  duration of a `runDir` walk) rather than per-file, to avoid repeated syscall
  overhead.
- **Platform builds:** `github.com/pkg/xattr` already handles
  Linux/macOS/FreeBSD; on Windows it returns `ENOTSUP`, which triggers the
  marker-file fallback naturally — no build tags needed.
