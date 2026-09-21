# EleBBS-Go

A Go port of EleBBS, the RemoteAccess-compatible bulletin board system
written in Pascal by Maarten Bekers. It reads the same data files the
original does — `CONFIG.RA` and the rest of the RA 2.50 packed records,
`USERS.BBS` with its `USERSIDX.BBS` index, `.MNU` menus, `.RA` language
files, `.ANS`/`.ASC` display files and JAM message bases — so an existing
EleBBS or RemoteAccess installation runs without conversion. The port
tracks EleBBS 0.11.b1 and reports itself as `EleBBS/W32 v0.11.b1-go`.

Three programs are published here:

- **elebbs** — the BBS itself. Run it for a local logon, hand it a socket
  from a front end with `-XT -H<handle>`, or let it take telnet callers
  directly with `-LISTEN=:port`.
- **eleserv** — the server suite: FTPS, NNTPS, SSH, and a telnet listener
  that spawns a session per caller. Logins go through `USERS.BBS`, so
  callers use their BBS accounts.
- **elemail** — the internet mail gateway. Collects over POP3S and sends
  over SMTPS, tossing messages into the local message bases.

EleBBS configuration lives in `CONFIG.RA`. Each program looks for it in
the current directory, or in the path named by the `ELEBBS` environment
variable.

## Building

Go 1.24 or newer, no C compiler: `go build ./cmd/...`. [BUILD.md](BUILD.md)
covers cross-compiling and the test suite.

## License

EleBBS is distributed under the Q Public License version 1.0; see
[LICENSE](LICENSE). Copyright 1997-2003 Maarten Bekers. The Go port is by
Marty Kazmaier.
