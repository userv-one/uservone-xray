# uservone Xray legacy

Minimal production-safe [Xray-core](https://github.com/XTLS/Xray-core) fork based on **v26.7.28**, the version already deployed on userv.one nodes. This branch is independent from `uservone-core` and intentionally does not follow newer upstream releases.

## Provenance and compatibility

- Upstream: `https://github.com/XTLS/Xray-core.git`
- Baseline tag: `v26.7.28`
- Baseline commit: `5ca6f4b7d4dc20a881d4330e498892697627ec0c`
- License: Mozilla Public License 2.0

VLESS, REALITY, XTLS Vision, Shadowsocks 2022, and SOCKS5 wire behavior is unchanged. REALITY `minClientVer`, uTLS fingerprints, subscription formats, and client profiles are not modified.

## Fork changes

- TCP keepalive defaults for inbound and outbound: 60 seconds idle, then a probe every 15 seconds.
- `tcpKeepAliveIdle`, `tcpKeepAliveInterval`, and the additive `tcpKeepAliveCount` socket options.
- `xray version-json` with machine-readable fork, upstream, build, runtime, and wire-compatibility metadata.
- Explicit SOCKS authentication: missing or unknown `auth` is rejected, and `password` requires at least one account. Explicit `noauth` remains available for trusted loopback configurations.

The legacy text banner still starts with `Xray 26.7.28` for existing deployment checks. New automation should use `version-json` instead.

## Build

Go 1.26 or the compatible toolchain declared in `go.mod` is required.

```bash
git clone -b v26.7.28-uservone https://github.com/userv-one/uservone-xray.git
cd uservone-xray
CGO_ENABLED=0 go build -o xray -trimpath -buildvcs=true -ldflags="-s -w" ./main
./xray version
./xray version-json
```

On Windows, use `xray.exe` and `.\xray.exe`.

## Production socket overrides

The fork defaults are overridden by existing uservone-platform values, so current node behavior is preserved:

```json
{
  "streamSettings": {
    "sockopt": {
      "tcpKeepAliveIdle": 20,
      "tcpKeepAliveInterval": 15,
      "tcpKeepAliveCount": 4,
      "tcpUserTimeout": 15000,
      "tcpNoDelay": true
    }
  }
}
```

Negative idle/interval values disable keepalive when neither value is positive; a negative value paired with zero also disables it. Mixed negative and positive values are rejected. A non-positive count leaves the operating-system default unchanged.

## Examples

- `examples/uservone.server.json`: VLESS+Reality with `minClientVer: 1.0.0`, Shadowsocks 2022 TCP+UDP on 8388, and authenticated SOCKS5 with UDP on 1080.
- `examples/uservone.vless-reality.client.json`: Vision, REALITY public key/short ID, SNI, fingerprint, and socket overrides.
- `examples/uservone.vless-reality.url`: XTLS-standard `pbk/sid/fp/sni/spx` parameters and `packetEncoding=xudp`.
- `examples/uservone.shadowsocks.url`: legacy uservone-platform SIP002 form.
- `examples/uservone.shadowsocks-aead2022.url`: standards-compliant AEAD-2022 external example.

Validate configurations without opening listeners:

```bash
go run ./main run -test -config examples/uservone.server.json
go run ./main run -test -config examples/uservone.vless-reality.client.json
```

Examples contain public demonstration keys and credentials; replace them with production values from the platform before deployment.

## Verification

```bash
go test ./...
go test ./transport/internet ./infra/conf ./core ./proxy/socks ./proxy/shadowsocks_2022
go vet ./...
```

Some upstream tests require `resources/geoip.dat` and `resources/geosite.dat`; the release workflow downloads and verifies them.

Release gating additionally requires A/B loopback checks against upstream v26.7.28 with sing-box 1.14.2 and 1.14.4 returning HTTP 204. Any REALITY verification failure blocks publication.

## Release and migration

The release tag is `v26.7.28-uservone.1`. Assets include Linux AMD64/ARM64 binaries, `SHA256SUMS`, `buildinfo.json`, and an SPDX SBOM.

Read [migration and rollback notes](docs/releases/v26.7.28-uservone.1.md) before the canary deployment. Do not enable this release fleet-wide before canary and sing-box/Throne compatibility checks pass.

## License

Mozilla Public License 2.0. See [LICENSE](LICENSE). XTLS attribution and upstream history are preserved.
