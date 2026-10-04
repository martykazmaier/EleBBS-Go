# EleBBS-Go

A port of the EleBBS bulletin board system from Pascal to Go.

EleBBS was written by Maarten Bekers (1997-2003) as a RemoteAccess 2.50
compatible BBS. EleBBS-Go keeps that design: it reads the same configuration,
message base (JAM), file base, user base, menus, language files and Q-A
scripts, so an existing EleBBS or RemoteAccess setup works without conversion.

## Download

Ready-to-run builds are on the
[Releases page](https://github.com/martykazmaier/EleBBS-Go/releases):

| File | For |
|------|-----|
| `...-win32.zip` | Windows (32-bit, runs on 64-bit Windows too) |
| `...-linux-386.tar.gz` | Linux, 32-bit PCs |
| `...-linux-amd64.tar.gz` | Linux, 64-bit PCs |
| `...-linux-arm64.tar.gz` | Linux on 64-bit ARM (for example Raspberry Pi 4/5) |

The Windows build is the one in daily use. The Linux builds are new and not
yet tested on a live system.

## The programs

| Program | What it does |
|---------|--------------|
| `elebbs` | The BBS itself. One copy runs per caller (node). |
| `eleserv` | Answers incoming connections and starts a node for each caller: telnet, telnets (secure telnet), SSH and secure WebSocket (WSS) for web terminals such as fTelnet. Also serves the file areas over FTPS and the message areas over NNTPS. |
| `elemail` | Internet e-mail: fetches mail over POP3S and sends it over SMTPS. |

Run any program with `-?` to see its options.

## Installing

1. Unpack the release into your EleBBS system directory (the one holding
   `CONFIG.RA`), or set the `ELEBBS` environment variable to that directory.
2. Start `eleserv` with the services you want, for example:

   ```
   eleserv -TELNET -TELNETS -SSH -WSS -WSSPORT:11235
   ```

### eleserv options

| Option | Meaning |
|--------|---------|
| `-TELNET` | Telnet, on the port set in `TELNET.ELE` (default 23). |
| `-TELNETS`, `-TELNETSPORT:<port>` | Secure telnet (default port 992). |
| `-SSH`, `-SSHPORT:<port>` | SSH (default port 22). Callers log in with their BBS name and password. |
| `-WSS`, `-WSSPORT:<port>` | Secure WebSocket for web terminals (default port 11235). |
| `-FTPS`, `-FTPPORT:<port>` | Secure FTP for the file areas (default port 990). |
| `-PASVPORTS:<low>-<high>` | Port range for FTPS passive data connections (default 1025-65535). Open this range in your firewall. |
| `-PASVSRVIP:<address>` | Address given to FTPS clients for passive connections, such as your public IP when behind a router. |
| `-PASVOFFSET:<n>` | Add `n` to the passive port given to clients, for routers that forward to different port numbers. |
| `-NNTPS`, `-NNTSPORT:<port>` | Secure news for the message areas (default port 563). |
| `-CERT:<file>`, `-KEY:<file>` | TLS certificate for telnets, WSS, FTPS and NNTPS. Without these, `eleserv.pem` (or `eleserv.crt` + `eleserv.key`) in the system directory is used. |

SSH host keys (`eleserv_hostkey`, `eleserv_hostkey_rsa`, `eleserv_hostkey_dsa`)
are created in the system directory the first time SSH starts. Ed25519, RSA and
DSA keys are offered, so both current clients and older terminal programs such
as NetRunner and SyncTERM can connect.

All connection types share the node numbers and session limit set in
`TELNET.ELE`.

## Building from source

See [BUILD.md](BUILD.md).

## Credits

- Original EleBBS: Maarten Bekers. The original Pascal source is at
  [github.com/mbek/elebbs](https://github.com/mbek/elebbs).
- Go port: Martin ([martykazmaier](https://github.com/martykazmaier)).
