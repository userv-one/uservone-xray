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
	Version_y byte = 10
	Version_z byte = 31
)

var (
	build    = "Custom"
	codename = "Xray, Penetrates Everything."
	intro    = "uservone core based on XTLS Xray; protocol-compatible with upstream."
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

// VersionInfo contains machine-readable build information.
type VersionInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Codename  string `json:"codename"`
	Build     string `json:"build"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Upstream  string `json:"upstream"`
}

func VersionDetails() VersionInfo {
	return VersionInfo{
		Name:      "uservone Xray",
		Version:   Version(),
		Codename:  codename,
		Build:     build,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Upstream:  "XTLS/Xray-core",
	}
}

// VersionStatement returns a list of strings representing the full version info.
func VersionStatement() []string {
	info := VersionDetails()
	return []string{
		serial.Concat(info.Name, " ", info.Version, " (", info.Codename, ") ", info.Build, " (", info.GoVersion, " ", info.OS, "/", info.Arch, ")"),
		intro,
	}
}
