# uservone Xray fork

uservone Xray tracks [XTLS/Xray-core](https://github.com/XTLS/Xray-core) and keeps its wire protocols compatible with upstream. The fork currently focuses on server-side TCP connection management and explicit build identity.

## Provenance

- Upstream: `https://github.com/XTLS/Xray-core.git`
- Baseline: `1c8bf4067b881e5f21442882013d0617e4d5dc6d` from `main`
- License: Mozilla Public License 2.0; see `LICENSE`

## Changes from upstream

- TCP keepalive is enabled by default for every TCP inbound, including VLESS+Reality, Shadowsocks, and SOCKS5. The defaults are 60 seconds before the first probe and 15 seconds between probes.
- `sockopt.tcpKeepAliveCount` configures the number of unanswered probes before the operating system drops a connection. It works for both inbound and outbound TCP connections.
- Existing `tcpKeepAliveIdle` and `tcpKeepAliveInterval` values override the corresponding defaults. Setting either value to a negative number disables keepalive for that listener or dialer; mixed signs are rejected.
- `xray version` identifies the binary as `uservone Xray` while retaining the XTLS upstream version and build revision.

No VLESS, REALITY, Shadowsocks, or SOCKS wire formats are changed. The new protobuf field uses number 24 and is additive.

## Per-connection configuration

```json
{
  "streamSettings": {
    "sockopt": {
      "tcpKeepAliveIdle": 60,
      "tcpKeepAliveInterval": 15,
      "tcpKeepAliveCount": 5
    }
  }
}
```

Omit `sockopt` to use the fork defaults. A zero or negative `tcpKeepAliveCount` leaves the operating-system default unchanged. Keepalive support and exact timing semantics remain operating-system dependent.

## Example server

`examples/uservone.server.json` contains VLESS+Reality on port 443, Shadowsocks 2022 on port 8443, and a loopback-only SOCKS5 inbound on port 1080.

The UUID and keys in that file are public demonstration values. Replace them before deployment:

```bash
xray uuid
xray x25519
openssl rand -base64 16
```

Validate the example without opening listeners:

```bash
go run ./main run -test -config examples/uservone.server.json
```

## Build

```bash
CGO_ENABLED=0 go build -o xray -trimpath -buildvcs=true -ldflags="-s -w" ./main
./xray version
```

On Windows, use `xray.exe` and set `CGO_ENABLED=0` with the shell's environment syntax.

## Updating from upstream

```bash
git fetch upstream
git merge upstream/main
go run ./infra/vprotogen -pwd .
go test ./...
```

Keep the fork branch small and review conflicts in `transport/internet/keepalive.go`, the listener/dialer, `infra/conf/transport_sockopt.go`, and `core/core.go`.
