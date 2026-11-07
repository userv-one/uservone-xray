package conf_test

import (
	"encoding/json"
	"testing"

	. "github.com/xtls/xray-core/infra/conf"
)

func TestSocketConfigKeepAlive(t *testing.T) {
	var config SocketConfig
	input := []byte(`{
		"tcpKeepAliveIdle": 60,
		"tcpKeepAliveInterval": 15,
		"tcpKeepAliveCount": 5
	}`)
	if err := json.Unmarshal(input, &config); err != nil {
		t.Fatal(err)
	}
	built, err := config.Build()
	if err != nil {
		t.Fatal(err)
	}
	if built.TcpKeepAliveIdle != 60 || built.TcpKeepAliveInterval != 15 || built.TcpKeepAliveCount != 5 {
		t.Fatalf("keepalive settings were not preserved: idle=%d interval=%d count=%d",
			built.TcpKeepAliveIdle, built.TcpKeepAliveInterval, built.TcpKeepAliveCount)
	}
}
