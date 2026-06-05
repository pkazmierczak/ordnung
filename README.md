# ordnung

`ordnung` is a simple CLI program that renames photos according to their EXIF data.

Videos (`.mov`, `.mp4`, `.m4v`) can be renamed too, using the creation date
embedded in their QuickTime/ISO-BMFF metadata. Video handling is opt-in via the
`-videos` flag.

If a photo (or a standalone video) has an XMP sidecar next to it (a file with
the same base name and an `.xmp` extension, e.g. `IMG_1234.xmp` beside
`IMG_1234.jpg`), the sidecar is renamed in lockstep with it so the two stay
paired.

Apple Photos exports of "unmodified originals" produce a Live Photo as a HEIC
(or JPG) still plus a same-named `.mov` video part (e.g. `IMG_1234.mov` beside
`IMG_1234.heic`). By default `ordnung` deletes that `.mov`, collapsing the Live
Photo down to its still frame. Pass `-keep-live-photos` to keep the video
instead (it is left untouched, or renamed alongside other videos if `-videos`
is also set).

After the metadata scan, if any files could not be read cleanly, `ordnung`
prints a single grouped report instead of warnings scrolling past the progress
bar: which files are missing capture metadata (and so will be renamed using
their filesystem creation date), and which were skipped (unreadable,
unsupported, or timed out). Before a real run renames anything, it then asks for
confirmation so you can review that report first. The prompt is skipped when
standard input isn't a terminal (so scripts and CI keep working), or when you
pass `-yes` to proceed without asking.

Pass `-report-only` to scan and print that report without printing a plan or
renaming anything — handy for auditing a directory. Either way, `ordnung` exits
non-zero whenever some files were skipped or a rename failed, so scripts can
detect that a directory needs attention. Rename failures are themselves
collected and printed together once the rename pass finishes, rather than
scrolling past the progress bar.

Reading a file's metadata is bounded by a per-file timeout (10s), so a single
unreadable file or an unresponsive filesystem can't hang the whole run.

```
$ ./ordnung -h
```

to see available options.

## Building and installing

`ordnung` is written in Go and requires a recent [Go compiler](https://golang.org/dl/) to be built.

```
$ make
```

to build and

```
$ make install
```

to put the binary in your `/usr/local/bin`.

## Development

The code is organized as a standard Go CLI:

- `cmd/ordnung` — the command-line entry point (flag parsing, the scan ->
  extract -> assign -> rename pipeline, and the post-scan report and
  confirmation prompt in `review.go`).
- `internal/ordnung` — the core library: the `Image` model and EXIF date
  extraction (`image.go`), video creation-date extraction (`video.go`),
  target-name generation (`target.go`), and the on-disk rename (`rename.go`),
  each with a matching `_test.go`. A per-operation IO timeout helper
  (`timeout.go`) bounds every filesystem call, and `extracterror.go` classifies
  why a file's date could not be read so the CLI can group it in the report.
- `internal/version` — build metadata injected at link time by the `Makefile`.

Run the checks with:

```
$ make test    # go test ./...
$ make vet     # go vet ./...
$ make check    # vet + test, as CI runs them
```

Tests use the [shoenig/test](https://github.com/shoenig/test) assertion
library. The convention is to use `must.*` for setup and preconditions whose
failure makes the rest of the test meaningless (it stops the test), and
`test.*` for the actual result assertions (it keeps going), so a single run
surfaces every failure at once.
