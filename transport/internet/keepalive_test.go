package internet

import (
	"testing"
	"time"

	"github.com/xtls/xray-core/common/net"
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
	for _, test := range []struct {
		name     string
		idle     int32
		interval int32
	}{
		{name: "positive idle", idle: 1<<31 - 1, interval: -2},
		{name: "positive interval", idle: -2, interval: 1<<31 - 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := keepAliveConfig(&SocketConfig{
				TcpKeepAliveIdle:     test.idle,
				TcpKeepAliveInterval: test.interval,
			}, time.Second, time.Second)
			if err == nil {
				t.Fatal("mixed keepalive signs must be rejected even when their product overflows int32")
			}
		})
	}
}

func TestKeepAliveNegativeValuesDisable(t *testing.T) {
	for name, configure := range map[string]func(*SocketConfig) (net.KeepAliveConfig, error){
		"listener": listenerKeepAliveConfig,
		"dialer":   dialerKeepAliveConfig,
	} {
		t.Run(name, func(t *testing.T) {
			config, err := configure(&SocketConfig{TcpKeepAliveIdle: -1, TcpKeepAliveInterval: -1, TcpKeepAliveCount: 4})
			if err != nil {
				t.Fatal(err)
			}
			if config.Enable {
				t.Fatal("negative idle and interval must disable TCP keepalive")
			}
			if config.Idle != -1 || config.Interval != -1 {
				t.Fatalf("disabled keepalive timings changed: idle=%v interval=%v", config.Idle, config.Interval)
			}
			if config.Count != -1 {
				t.Fatalf("disabled keepalive count is %d, want the operating system default", config.Count)
			}
		})
	}
}

func TestKeepAliveNegativeWithZeroDisables(t *testing.T) {
	for _, test := range []struct {
		name     string
		idle     int32
		interval int32
	}{
		{name: "negative idle", idle: -1, interval: 0},
		{name: "negative interval", idle: 0, interval: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := keepAliveConfig(&SocketConfig{
				TcpKeepAliveIdle:     test.idle,
				TcpKeepAliveInterval: test.interval,
			}, time.Minute, 15*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if config.Enable {
				t.Fatal("a negative idle or interval must disable keepalive when the other value is zero")
			}
		})
	}
}

func TestKeepAliveNonPositiveCountUsesOSDefault(t *testing.T) {
	for _, count := range []int32{0, -1} {
		config, err := keepAliveConfig(&SocketConfig{TcpKeepAliveCount: count}, time.Minute, 15*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if config.Count != -1 {
			t.Fatalf("count %d produced probe count %d, want the operating system default", count, config.Count)
		}
	}
}
