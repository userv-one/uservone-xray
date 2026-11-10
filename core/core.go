// Package core provides an entry point to use Xray core functionalities.
//
// Xray makes it possible to accept incoming network connections with certain
// protocol, process the data, and send them through another connection with
// the same or a difference protocol on demand.
//
// It may be configured to work with multiple protocols at the same time, and
// uses the internal router to tunnel through different inbound and outbound
// connections.
package core

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/xtls/xray-core/common/serial"
)

var (
	Version_x byte = 26
	Version_y byte = 7
	Version_z byte = 28
)

var (
	build    = "Custom"
	codename = "Xray, Penetrates Everything."
	intro    = "uservone keepalive patch; wire-compatible with Xray 26.7.28."
)

func init() {
	// Manually injected
	if build != "Custom" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	var isDirty bool
	var foundBuild bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if len(setting.Value) < 7 {
				return
			}
			build = setting.Value[:7]
			foundBuild = true
		case "vcs.modified":
			isDirty = setting.Value == "true"
		}
	}
	if isDirty && foundBuild {
		build += "-dirty"
	}
}

// Version returns Xray's version as a string, in the form of "x.y.z" where x, y and z are numbers.
// ".z" part may be omitted in regular releases.
func Version() string {
	return fmt.Sprintf("%v.%v.%v", Version_x, Version_y, Version_z)
}

// VersionInfo contains machine-readable build and compatibility information.
type VersionInfo struct {
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	Upstream          string   `json:"upstream"`
	UpstreamCommit    string   `json:"upstream_commit"`
	Fork              string   `json:"fork"`
	Build             string   `json:"build"`
	GoVersion         string   `json:"go_version"`
	OS                string   `json:"os"`
	Arch              string   `json:"arch"`
	WireCompatibility []string `json:"wire_compatibility"`
}

func VersionDetails() VersionInfo {
	return VersionInfo{
		Name:           "uservone Xray " + Version(),
		Version:        Version(),
		Upstream:       "XTLS/Xray-core",
		UpstreamCommit: "5ca6f4b7d4dc20a881d4330e498892697627ec0c",
		Fork:           "uservone-keepalive.1",
		Build:          build,
		GoVersion:      runtime.Version(),
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		WireCompatibility: []string{
			"VLESS", "REALITY", "XTLS Vision", "Shadowsocks 2022", "SOCKS5",
		},
	}
}

// VersionStatement returns a list of strings representing the full version info.
func VersionStatement() []string {
	details := VersionDetails()
	return []string{
		serial.Concat("Xray ", details.Version, " (", codename, ") ", details.UpstreamCommit[:7], " (", details.GoVersion, " ", details.OS, "/", details.Arch, ")"),
		intro,
	}
}
