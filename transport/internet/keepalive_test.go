package internet

import (
	"testing"
	"time"
)

func TestListenerKeepAliveDefaults(t *testing.T) {
	config, err := listenerKeepAliveConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enable {
		t.Fatal("TCP keepalive must be enabled for inbound connections by default")
	}
	if config.Idle != 60*time.Second {
		t.Fatalf("idle timeout is %v, want 60s", config.Idle)
	}
	if config.Interval != 15*time.Second {
		t.Fatalf("probe interval is %v, want 15s", config.Interval)
	}
	if config.Count != -1 {
		t.Fatalf("probe count is %d, want the operating system default", config.Count)
	}
}

func TestListenerKeepAliveOverrides(t *testing.T) {
	config, err := listenerKeepAliveConfig(&SocketConfig{
		TcpKeepAliveIdle:     30,
		TcpKeepAliveInterval: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enable {
		t.Fatal("TCP keepalive must remain enabled with positive overrides")
	}
	if config.Idle != 30*time.Second {
		t.Fatalf("idle timeout is %v, want 30s", config.Idle)
	}
	if config.Interval != 10*time.Second {
		t.Fatalf("probe interval is %v, want 10s", config.Interval)
	}
}

func TestListenerKeepAliveCountOverride(t *testing.T) {
	config, err := listenerKeepAliveConfig(&SocketConfig{TcpKeepAliveCount: 5})
	if err != nil {
		t.Fatal(err)
	}
	if config.Count != 5 {
		t.Fatalf("probe count is %d, want 5", config.Count)
	}
}

func TestDialerKeepAliveCountOverride(t *testing.T) {
	config, err := dialerKeepAliveConfig(&SocketConfig{TcpKeepAliveCount: 7})
	if err != nil {
		t.Fatal(err)
	}
	if config.Count != 7 {
		t.Fatalf("probe count is %d, want 7", config.Count)
	}
	if config.Idle != 60*time.Second || config.Interval != 15*time.Second {
		t.Fatalf("uservone defaults changed: idle=%v interval=%v", config.Idle, config.Interval)
	}
}

func TestKeepAliveRejectsMixedSignsWithoutOverflow(t *testing.T) {
	_, err := keepAliveConfig(&SocketConfig{
		TcpKeepAliveIdle:     1<<31 - 1,
		TcpKeepAliveInterval: -2,
	}, time.Second, time.Second)
	if err == nil {
		t.Fatal("mixed keepalive signs must be rejected even when their product overflows int32")
	}
}
