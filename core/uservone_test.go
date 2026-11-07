package core

import (
	"strings"
	"testing"
)

func TestVersionStatementBrand(t *testing.T) {
	statement := strings.Join(VersionStatement(), "\n")
	if !strings.Contains(statement, "uservone Xray "+Version()) {
		t.Fatalf("version statement does not identify the uservone fork: %q", statement)
	}
	if !strings.Contains(statement, "XTLS") {
		t.Fatalf("version statement must preserve upstream attribution: %q", statement)
	}
}
