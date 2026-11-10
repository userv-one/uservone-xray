package internet

import (
	"time"

	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
)

func keepAliveConfig(sockopt *SocketConfig, idle, interval time.Duration) (net.KeepAliveConfig, error) {
	config := net.KeepAliveConfig{
		Enable:   true,
		Idle:     idle,
		Interval: interval,
		Count:    -1,
	}
	if sockopt == nil {
		return config, nil
	}
	if (sockopt.TcpKeepAliveIdle < 0 && sockopt.TcpKeepAliveInterval > 0) ||
		(sockopt.TcpKeepAliveIdle > 0 && sockopt.TcpKeepAliveInterval < 0) {
		return net.KeepAliveConfig{}, errors.New("invalid TcpKeepAliveIdle or TcpKeepAliveInterval value: ", sockopt.TcpKeepAliveIdle, " ", sockopt.TcpKeepAliveInterval)
	}
	if sockopt.TcpKeepAliveIdle < 0 || sockopt.TcpKeepAliveInterval < 0 {
		config.Enable = false
		config.Idle = -1
		config.Interval = -1
		return config, nil
	}
	if sockopt.TcpKeepAliveIdle > 0 {
		config.Idle = time.Duration(sockopt.TcpKeepAliveIdle) * time.Second
	}
	if sockopt.TcpKeepAliveInterval > 0 {
		config.Interval = time.Duration(sockopt.TcpKeepAliveInterval) * time.Second
	}
	if sockopt.TcpKeepAliveCount > 0 {
		config.Count = int(sockopt.TcpKeepAliveCount)
	}
	return config, nil
}
