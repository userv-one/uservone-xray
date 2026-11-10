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
	var info map[string]any
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("version-json returned invalid JSON: %v", err)
	}
	expected := map[string]string{
		"name":            "uservone Xray 26.7.28",
		"version":         core.Version(),
		"upstream":        "XTLS/Xray-core",
		"upstream_commit": "5ca6f4b7d4dc20a881d4330e498892697627ec0c",
		"fork":            "uservone-keepalive.1",
	}
	for field, value := range expected {
		if info[field] != value {
			t.Errorf("%s is %v, want %q", field, info[field], value)
		}
	}
	for _, field := range []string{"build", "go_version", "os", "arch"} {
		if info[field] == "" {
			t.Errorf("version-json field %q is empty", field)
		}
	}
	wire, ok := info["wire_compatibility"].([]any)
	if !ok || len(wire) != 5 {
		t.Fatalf("wire_compatibility is %v, want five protocols", info["wire_compatibility"])
	}
}
