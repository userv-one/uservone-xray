package main

import (
	"encoding/json"
	"testing"

	"github.com/xtls/xray-core/core"
)

func TestVersionJSON(t *testing.T) {
	data, err := versionJSON()
	if err != nil {
		t.Fatal(err)
	}
	var info map[string]string
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("version-json returned invalid JSON: %v", err)
	}
	for _, field := range []string{"name", "version", "build", "go_version", "os", "arch", "upstream"} {
		if info[field] == "" {
			t.Errorf("version-json field %q is empty: %s", field, data)
		}
	}
	if info["name"] != "uservone Xray" {
		t.Errorf("name is %q, want uservone Xray", info["name"])
	}
	if info["version"] != core.Version() {
		t.Errorf("version is %q, want %q", info["version"], core.Version())
	}
}
