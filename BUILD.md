# Building EleBBS (Go)

This repository builds three Windows programs:

| Program       | Source          | Purpose                                               |
|---------------|-----------------|-------------------------------------------------------|
| `elebbs.exe`  | `cmd/elebbs`    | The BBS node (one caller per node).                   |
| `eleserv.exe` | `cmd/eleserv`   | FTPS, NNTPS, SSH, telnet, telnets and WSS server that spawns nodes. |
| `elemail.exe` | `cmd/elemail`   | POP3S/SMTPS mail retrieval, tossing and sending.      |

## Requirements

- [Go](https://go.dev/dl/) 1.24 or newer (see `go.mod`).
- Internet access the first time you build, so Go can download the
  `golang.org/x/sys` and `golang.org/x/crypto` modules listed in `go.sum`.
- No C compiler is needed; builds use `CGO_ENABLED=0`.

## Build (Windows, 32-bit)

The released binaries target 32-bit Windows, matching the original EleBBS/W32.
From the repository root in PowerShell:

```powershell
$env:GOOS = "windows"
$env:GOARCH = "386"
$env:CGO_ENABLED = "0"
go build -o elebbs.exe  ./cmd/elebbs
go build -o eleserv.exe ./cmd/eleserv
go build -o elemail.exe ./cmd/elemail
```

From a Command Prompt (`cmd.exe`):

```bat
set GOOS=windows
set GOARCH=386
set CGO_ENABLED=0
go build -o elebbs.exe  .\cmd\elebbs
go build -o eleserv.exe .\cmd\eleserv
go build -o elemail.exe .\cmd\elemail
```

The same commands work from Linux or macOS (cross-compiling), for example:

```sh
GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -o elebbs.exe ./cmd/elebbs
```

For a 64-bit build use `GOARCH=amd64` instead of `386`.

## Tests

```powershell
go test ./internal/...
```

## Installing

Copy the `.exe` files into your EleBBS system directory (the one holding
`CONFIG.RA`), or set the `ELEBBS` (or `RA`) environment variable to that
directory. The programs read the existing RemoteAccess 2.50 / EleBBS
configuration files; no conversion is needed. Running `eleserv.exe` or
`elemail.exe` without options prints their command-line help.
