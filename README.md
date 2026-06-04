# ordnung

`ordnung` is a simple CLI program that renames photos according to their EXIF data.

Videos (`.mov`, `.mp4`, `.m4v`) can be renamed too, using the creation date
embedded in their QuickTime/ISO-BMFF metadata. Video handling is opt-in via the
`-videos` flag.

If a photo has an XMP sidecar next to it (a file with the same base name and an
`.xmp` extension, e.g. `IMG_1234.xmp` beside `IMG_1234.jpg`), the sidecar is
renamed in lockstep with the photo so the two stay paired.

Apple Photos exports of "unmodified originals" produce a Live Photo as a HEIC
(or JPG) still plus a same-named `.mov` video part (e.g. `IMG_1234.mov` beside
`IMG_1234.heic`). By default `ordnung` deletes that `.mov`, collapsing the Live
Photo down to its still frame. Pass `-keep-live-photos` to keep the video
instead (it is left untouched, or renamed alongside other videos if `-videos`
is also set).

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
  extract -> assign -> rename pipeline).
- `internal/ordnung` — the core library: the `Image` model and EXIF date
  extraction (`image.go`), video creation-date extraction (`video.go`),
  target-name generation (`target.go`), and the on-disk rename (`rename.go`),
  each with a matching `_test.go`.
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
