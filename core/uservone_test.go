package core

import (
	"strings"
	"testing"
)

func TestVersionStatementKeepsLegacyBanner(t *testing.T) {
	statement := VersionStatement()
	if len(statement) == 0 {
		t.Fatal("empty version statement")
	}
	if !strings.Contains(statement[0], "Xray 26.7.28") {
		t.Fatalf("legacy version is missing: %q", statement[0])
	}
	if !strings.Contains(statement[0], "5ca6f4b") {
		t.Fatalf("upstream build is missing from the legacy banner: %q", statement[0])
	}
	if strings.HasPrefix(statement[0], "uservone") {
		t.Fatalf("legacy banner must remain unchanged: %q", statement[0])
	}
}
