package main

import (
	"encoding/json"
	"fmt"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/main/commands/base"
)

var cmdVersionJSON = &base.Command{
	UsageLine: "{{.Exec}} version-json",
	Short:     "Show current version as JSON",
	Long: `Version-json prints machine-readable build information for uservone Xray.
	`,
	Run: executeVersionJSON,
}

func versionJSON() ([]byte, error) {
	return json.Marshal(core.VersionDetails())
}

func executeVersionJSON(cmd *base.Command, args []string) {
	data, err := versionJSON()
	if err != nil {
		base.Fatalf("failed to encode version information: %s", err)
	}
	fmt.Println(string(data))
}
