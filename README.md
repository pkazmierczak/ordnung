# ordnung

`ordnung` is a simple CLI program that renames photos according to their EXIF data.

Videos (`.mov`, `.mp4`, `.m4v`) can be renamed too, using the creation date
embedded in their QuickTime/ISO-BMFF metadata. Video handling is opt-in via the
`videos` setting in the config file (see [Configuration](#configuration)).

If a photo (or a standalone video) has an XMP sidecar next to it (a file with
the same base name and an `.xmp` extension, e.g. `IMG_1234.xmp` beside
`IMG_1234.jpg`), the sidecar is renamed in lockstep with it so the two stay
paired.

Apple Photos exports of "unmodified originals" produce a Live Photo as a HEIC
(or JPG) still plus a same-named `.mov` video part (e.g. `IMG_1234.mov` beside
`IMG_1234.heic`). By default `ordnung` deletes that `.mov`, collapsing the Live
Photo down to its still frame. Set `keep_live_photos = true` in the config to
keep the video instead (it is left untouched, or renamed alongside other videos
if `videos` is also enabled).

While it works, `ordnung` reports progress live on a single, continuously
updating line — a spinner while it discovers files, then a progress bar naming
the file whose metadata is being read (and, on a real run, the file being
renamed) — rather than scrolling thousands of log lines. When standard error
isn't a terminal (a pipe, a redirect, CI), it falls back to plain status lines
instead, and the machine-readable dry-run plan always goes to standard output
untouched, so `ordnung dir > plan.txt` stays clean and parseable.

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

## Configuration

Preferences live in a [TOML](https://toml.io) config file rather than in a long
list of flags. By default it is read from `~/.config/ordnung/config.toml` (or
`$XDG_CONFIG_HOME/ordnung/config.toml` when that variable is set); point
`-config` at a different path to override it.

The first time you run `ordnung` with no config file, an interactive wizard
offers to create one. You can re-run that wizard at any time with:

```
$ ./ordnung -configure
```

Off a terminal (a pipe, a redirect, `/dev/null`, CI, cron) the wizard is never
shown: `ordnung` falls back to built-in defaults so unattended runs keep working
without a config file.

The file holds these settings:

```toml
# Renaming pattern. Tokens are replaced with the photo's capture date:
#   YYYY  four-digit year   (2021)
#   MM    two-digit month   (07)
#   DD    two-digit day     (15)
# Use "/" to create sub-directories.
pattern = "YYYY-MM-DD"

# How many files to read and rename concurrently.
workers = 4

# Keep Apple Live Photo .mov parts instead of deleting them.
keep_live_photos = false

# Also rename video files (.mov/.mp4/.m4v) by their embedded creation date.
videos = false

# Default to a dry run (preview only). Override per-run with -dry=false.
dry_run = true
```

The **pattern** is the headline feature: build any convention you like out of
the `YYYY`, `MM`, and `DD` tokens, using `/` to nest into sub-directories. A
`_<time>-NN` suffix (a within-day ordering token plus a collision counter) and
the original extension are always appended, so a photo taken on 15 July 2021
becomes, for a few example patterns:

| Pattern               | Result                              |
| --------------------- | ----------------------------------- |
| `YYYY-MM-DD`          | `2021-07-15_29730-00.jpg`           |
| `YYYY/MM/DD`          | `2021/07/15_29730-00.jpg`           |
| `YYYY/MM-DD`          | `2021/07-15_29730-00.jpg`           |
| `YYYY/MM/YYYY-MM-DD`  | `2021/07/2021-07-15_29730-00.jpg`   |

The remaining options stay as flags, since they are per-run choices: `-dry`
(which overrides the config's `dry_run` for a single run), `-yes`,
`-report-only`, and `-config` / `-configure`.

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
  extract -> assign -> rename pipeline, the post-scan report and confirmation
  prompt in `review.go`, and the live progress UI — spinner, progress bars, and
  colored status — in `ui.go`). Config resolution (load the file, run the
  first-run wizard, or fall back to defaults) lives in `config.go`, and the
  interactive setup wizard itself in `wizard.go`.
- `internal/ordnung` — the core library: the `Image` model and EXIF date
  extraction (`image.go`), video creation-date extraction (`video.go`),
  renaming-pattern expansion and validation (`patterns.go`), target-name
  generation (`target.go`), and the on-disk rename (`rename.go`), each with a
  matching `_test.go`. A per-operation IO timeout helper (`timeout.go`) bounds
  every filesystem call, and `extracterror.go` classifies why a file's date
  could not be read so the CLI can group it in the report.
- `internal/config` — the persistent settings: the `Config` struct, defaults,
  validation, and reading/writing the commented TOML file (`config.go`).
- `internal/version` — build metadata injected at link time by the `Makefile`.

The interactive output (the discovery spinner, the metadata/rename progress
bars, and the colored status lines) is built on
[pterm](https://github.com/pterm/pterm). It auto-degrades to plain text when
standard error isn't a terminal, so piped and CI output stays readable.

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
