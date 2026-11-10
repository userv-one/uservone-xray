# uservone Xray

Protocol-compatible [Xray-core](https://github.com/XTLS/Xray-core) fork maintained for [userv.one](https://userv.one). The project keeps upstream VLESS, REALITY, Shadowsocks, and SOCKS behavior while adding server-oriented TCP connection management.

## Fork features

- TCP keepalive enabled by default for every TCP inbound and outbound: 60 seconds idle, then one probe every 15 seconds.
- Existing `tcpKeepAliveIdle` and `tcpKeepAliveInterval` settings can override the defaults.
- Additional `tcpKeepAliveCount` setting controls unanswered probes for inbound and outbound TCP connections.
- Consistent listener/dialer validation without integer overflow on extreme values.
- Explicit `uservone Xray` build identity with the upstream Xray version and Git revision.

No wire-protocol changes are made. Configurations without the new `tcpKeepAliveCount` field remain compatible with upstream Xray.

## Quick start

Go 1.27 or a compatible toolchain declared in `go.mod` is required.

```bash
git clone https://github.com/userv-one/uservone-xray.git
cd uservone-xray
CGO_ENABLED=0 go build -o xray -trimpath -buildvcs=true -ldflags="-s -w" ./main
./xray version
./xray version-json
```

On Windows, build `xray.exe` and run `.\xray.exe version`.

## TCP keepalive configuration

The fork applies keepalive automatically to TCP inbounds, including VLESS+Reality, Shadowsocks, and SOCKS5. No repeated `sockopt` block is required.

To customize an individual listener or dialer:

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

Setting either `tcpKeepAliveIdle` or `tcpKeepAliveInterval` to a negative value disables keepalive only when both values are non-positive; mixed positive/negative signs are rejected. A zero or negative `tcpKeepAliveCount` uses the operating-system default. Exact keepalive behavior depends on the operating system.

See [uservone.md](uservone.md) for detailed defaults, compatibility notes, and update instructions.

## Example configuration

[`examples/uservone.server.json`](examples/uservone.server.json) contains:

- VLESS + XTLS Vision + REALITY on TCP port 443;
- Shadowsocks 2022 with TCP and UDP on port 8388;
- loopback-only SOCKS5 with mandatory username/password authentication and UDP on port 1080.

The VLESS client fixture and subscription examples also cover REALITY public key/short ID, SNI, fingerprint, `packetEncoding=xudp`, and the SIP002 Shadowsocks format.

Validate it without opening listeners:

```bash
go run ./main run -test -config examples/uservone.server.json
```

The example contains public demonstration UUIDs and keys. Generate new values with `xray uuid`, `xray x25519`, and `openssl rand -base64 16` before deployment.

## Releases

Tagged releases build Linux AMD64 and ARM64 binaries, `SHA256SUMS`, `buildinfo.json`, and an SPDX SBOM. See the [v26.10.31-uservone.1 migration and rollback notes](docs/releases/v26.10.31-uservone.1.md).

## Development checks

```bash
go run ./infra/vprotogen -pwd . -mode check
go test ./...
go vet ./core ./transport/internet
```

Some upstream tests require `resources/geoip.dat` and `resources/geosite.dat`. The GitHub Actions workflows obtain these assets through the Xray asset-update pipeline.

## Upstream synchronization

```bash
git fetch upstream
git merge upstream/main
go run ./infra/vprotogen -pwd .
go test ./...
```

The upstream remote is `https://github.com/XTLS/Xray-core.git`. Protocol implementation, documentation, and release history remain available in the [upstream repository](https://github.com/XTLS/Xray-core).

## License

Mozilla Public License 2.0. See [LICENSE](LICENSE). Xray-core and XTLS attribution is retained; fork-specific changes are documented in this repository.
