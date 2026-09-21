# Building EleBBS-Go

Go 1.24 or newer is the only requirement. Nothing here uses cgo, so no C
compiler is involved.

    go build ./cmd/...

The three executables — `elebbs`, `eleserv` and `elemail` — land in the
current directory.

## Cross-compiling

Set `GOOS` and `GOARCH`. A 32-bit Windows build, the shape most existing
EleBBS setups expect:

    GOOS=windows GOARCH=386 go build ./cmd/...

## Tests

    go test ./...

Some tests currently fail on Linux, where the DOS path handling leaves
backslashes in filenames.
