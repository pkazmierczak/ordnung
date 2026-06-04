# ordnung

`ordnung` is a simple CLI program that renames photos according to their EXIF data.

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
  extraction (`image.go`), target-name generation (`name.go`), and the
  on-disk rename (`rename.go`), each with a matching `_test.go`.
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
