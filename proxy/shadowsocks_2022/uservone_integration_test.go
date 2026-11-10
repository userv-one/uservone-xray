package shadowsocks_2022_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	gonet "net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-shadowsocks"
	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	singbuf "github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/metadata"
	"github.com/xtls/xray-core/common/buf"
	clog "github.com/xtls/xray-core/common/log"
	xraynet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/features/routing"
	shadowsocks2022 "github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"github.com/xtls/xray-core/transport"
)

const uservoneIntegrationMethod = "2022-blake3-aes-128-gcm"

var (
	uservoneIntegrationKey      = []byte("0123456789abcdef")
	uservoneIntegrationWrongKey = []byte("fedcba9876543210")
)

type uservoneIntegrationLogSink struct {
	mu    sync.Mutex
	lines []string
}

func (s *uservoneIntegrationLogSink) Handle(msg clog.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, msg.String())
}

func (s *uservoneIntegrationLogSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

type uservoneIntegrationDiscardLogSink struct{}

func (uservoneIntegrationDiscardLogSink) Handle(clog.Message) {}

func uservoneIntegrationCaptureLogs(t *testing.T) *uservoneIntegrationLogSink {
	t.Helper()
	sink := new(uservoneIntegrationLogSink)
	clog.RegisterHandler(sink)
	t.Cleanup(func() {
		clog.RegisterHandler(uservoneIntegrationDiscardLogSink{})
	})
	return sink
}

func uservoneIntegrationAssertNoSecrets(t *testing.T, output string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		forms := []string{secret, base64.StdEncoding.EncodeToString([]byte(secret))}
		if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil {
			forms = append(forms, string(decoded))
		}
		for _, form := range forms {
			if form != "" && strings.Contains(output, form) {
				t.Error("error/log output contains a configured secret")
				return
			}
		}
	}
}

func uservoneIntegrationWaitResult(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Shadowsocks 2022 inbound to stop")
		return nil
	}
}

func uservoneIntegrationContext() context.Context {
	return session.ContextWithInbound(context.Background(), &session.Inbound{})
}

type uservoneIntegrationDispatcher struct {
	onDispatch func(context.Context, xraynet.Destination) (*transport.Link, error)
}

func (d *uservoneIntegrationDispatcher) Type() interface{} {
	return routing.DispatcherType()
}

func (d *uservoneIntegrationDispatcher) Start() error { return nil }
func (d *uservoneIntegrationDispatcher) Close() error { return nil }

func (d *uservoneIntegrationDispatcher) Dispatch(ctx context.Context, destination xraynet.Destination) (*transport.Link, error) {
	return d.onDispatch(ctx, destination)
}

func (d *uservoneIntegrationDispatcher) DispatchLink(context.Context, xraynet.Destination, *transport.Link) error {
	return fmt.Errorf("unexpected DispatchLink call")
}

func uservoneIntegrationNewInbound(t *testing.T, network xraynet.Network) *shadowsocks2022.Inbound {
	t.Helper()
	inbound, err := shadowsocks2022.NewServer(uservoneIntegrationContext(), &shadowsocks2022.ServerConfig{
		Method:  uservoneIntegrationMethod,
		Key:     base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		Network: []xraynet.Network{network},
	})
	if err != nil {
		t.Fatalf("failed to create Shadowsocks 2022 inbound: %v", err)
	}
	return inbound
}

func uservoneIntegrationNewClientMethod(t *testing.T, key []byte) shadowsocks.Method {
	t.Helper()
	method, err := shadowaead_2022.New(uservoneIntegrationMethod, [][]byte{key}, nil)
	if err != nil {
		t.Fatalf("failed to create Shadowsocks 2022 client method: %v", err)
	}
	return method
}

func TestShadowsocks2022IntegrationRejectsInvalidConfiguration(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	validKey := base64.StdEncoding.EncodeToString(uservoneIntegrationKey)
	const invalidSecret = "uservone-integration-invalid-password-secret"

	tests := []struct {
		name   string
		client bool
		method string
		key    string
		secret string
	}{
		{name: "server_method", method: "not-a-shadowsocks-2022-method", key: validKey},
		{name: "client_method", client: true, method: "not-a-shadowsocks-2022-method", key: validKey},
		{name: "server_password", method: uservoneIntegrationMethod, key: invalidSecret, secret: invalidSecret},
		{name: "client_password", client: true, method: uservoneIntegrationMethod, key: invalidSecret, secret: invalidSecret},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.client {
				_, err = shadowsocks2022.NewClient(context.Background(), &shadowsocks2022.ClientConfig{Method: tc.method, Key: tc.key})
			} else {
				_, err = shadowsocks2022.NewServer(context.Background(), &shadowsocks2022.ServerConfig{Method: tc.method, Key: tc.key})
			}
			if err == nil {
				t.Fatal("invalid Shadowsocks 2022 configuration was accepted")
			}
			if tc.secret != "" {
				uservoneIntegrationAssertNoSecrets(t, err.Error(), tc.secret)
			}
		})
	}
	uservoneIntegrationAssertNoSecrets(t, logs.String(), invalidSecret, validKey)
}

func uservoneIntegrationTCPEcho(t *testing.T) xraynet.Destination {
	t.Helper()
	listener, err := gonet.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start local TCP echo endpoint: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		t.Cleanup(func() { _ = conn.Close() })
		_, _ = io.Copy(conn, conn)
	}()
	addr := listener.Addr().(*gonet.TCPAddr)
	return xraynet.TCPDestination(xraynet.IPAddress(addr.IP), xraynet.Port(addr.Port))
}

func uservoneIntegrationServeTCP(t *testing.T, inbound *shadowsocks2022.Inbound, dispatcher routing.Dispatcher) (string, <-chan error) {
	t.Helper()
	listener, err := gonet.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start local Shadowsocks 2022 TCP endpoint: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	results := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			results <- err
			return
		}
		t.Cleanup(func() { _ = conn.Close() })
		results <- inbound.Process(uservoneIntegrationContext(), xraynet.Network_TCP, conn, dispatcher)
	}()
	return listener.Addr().String(), results
}

func TestShadowsocks2022IntegrationTCP(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	echoDest := uservoneIntegrationTCPEcho(t)
	inbound := uservoneIntegrationNewInbound(t, xraynet.Network_TCP)

	var dispatches atomic.Int32
	upstreams := make(chan gonet.Conn, 1)
	dispatcher := &uservoneIntegrationDispatcher{onDispatch: func(_ context.Context, dest xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		if dest.NetAddr() != echoDest.NetAddr() || dest.Network != xraynet.Network_TCP {
			return nil, fmt.Errorf("unexpected TCP destination %s", dest.NetAddr())
		}
		upstream, err := gonet.DialTimeout("tcp4", dest.NetAddr(), 2*time.Second)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = upstream.Close() })
		upstreams <- upstream
		return &transport.Link{Reader: buf.NewReader(upstream), Writer: buf.NewWriter(upstream)}, nil
	}}
	proxyAddr, results := uservoneIntegrationServeTCP(t, inbound, dispatcher)

	client, err := gonet.DialTimeout("tcp4", proxyAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to local Shadowsocks 2022 TCP endpoint: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("failed to set TCP test deadline: %v", err)
	}
	method := uservoneIntegrationNewClientMethod(t, uservoneIntegrationKey)
	secureClient := method.DialEarlyConn(client, metadata.ParseSocksaddr(echoDest.NetAddr()))
	t.Cleanup(func() { _ = secureClient.Close() })
	payload := []byte("uservone Shadowsocks 2022 TCP integration payload")
	if _, err := secureClient.Write(payload); err != nil {
		t.Fatalf("failed to write Shadowsocks 2022 TCP request: %v", err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(secureClient, response); err != nil {
		t.Fatalf("failed to read Shadowsocks 2022 TCP response: %v", err)
	}
	if !bytes.Equal(response, payload) {
		t.Fatalf("TCP echo payload mismatch: got %q, want %q", response, payload)
	}

	_ = client.Close()
	_ = (<-upstreams).Close()
	processErr := uservoneIntegrationWaitResult(t, results)
	if got := dispatches.Load(); got != 1 {
		t.Fatalf("TCP dispatcher call count = %d, want 1", got)
	}
	uservoneIntegrationAssertNoSecrets(t, logs.String()+"\n"+fmt.Sprint(processErr),
		string(uservoneIntegrationKey), base64.StdEncoding.EncodeToString(uservoneIntegrationKey))
}

func TestShadowsocks2022IntegrationRejectsWrongTCPPassword(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	inbound := uservoneIntegrationNewInbound(t, xraynet.Network_TCP)
	var dispatches atomic.Int32
	dispatcher := &uservoneIntegrationDispatcher{onDispatch: func(context.Context, xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		return nil, fmt.Errorf("wrong-password request reached dispatcher")
	}}
	proxyAddr, results := uservoneIntegrationServeTCP(t, inbound, dispatcher)
	client, err := gonet.DialTimeout("tcp4", proxyAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to local Shadowsocks 2022 TCP endpoint: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("failed to set wrong-password TCP deadline: %v", err)
	}
	method := uservoneIntegrationNewClientMethod(t, uservoneIntegrationWrongKey)
	secureClient := method.DialEarlyConn(client, metadata.ParseSocksaddr("127.0.0.1:9"))
	t.Cleanup(func() { _ = secureClient.Close() })
	if _, err := secureClient.Write([]byte("must be rejected")); err != nil {
		t.Fatalf("failed to send wrong-password TCP request: %v", err)
	}
	_ = client.Close()
	processErr := uservoneIntegrationWaitResult(t, results)
	if processErr == nil {
		t.Fatal("wrong-password TCP request was accepted")
	}
	if got := dispatches.Load(); got != 0 {
		t.Fatalf("wrong-password TCP request reached dispatcher %d times", got)
	}
	uservoneIntegrationAssertNoSecrets(t, logs.String()+"\n"+processErr.Error(),
		string(uservoneIntegrationKey), base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		string(uservoneIntegrationWrongKey), base64.StdEncoding.EncodeToString(uservoneIntegrationWrongKey))
}

func uservoneIntegrationUDPEcho(t *testing.T) xraynet.Destination {
	t.Helper()
	conn, err := gonet.ListenUDP("udp4", &gonet.UDPAddr{IP: gonet.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("failed to start local UDP echo endpoint: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buffer := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			if _, err := conn.WriteToUDP(buffer[:n], addr); err != nil {
				return
			}
		}
	}()
	addr := conn.LocalAddr().(*gonet.UDPAddr)
	return xraynet.UDPDestination(xraynet.IPAddress(addr.IP), xraynet.Port(addr.Port))
}

type uservoneIntegrationUDPServerConn struct {
	*gonet.UDPConn
	destination *gonet.UDPAddr
}

func (c *uservoneIntegrationUDPServerConn) Write(payload []byte) (int, error) {
	return c.WriteToUDP(payload, c.destination)
}

func uservoneIntegrationServeUDP(t *testing.T, inbound *shadowsocks2022.Inbound, dispatcher routing.Dispatcher) (*gonet.UDPConn, *gonet.UDPConn, <-chan error) {
	t.Helper()
	server, err := gonet.ListenUDP("udp4", &gonet.UDPAddr{IP: gonet.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("failed to start local Shadowsocks 2022 UDP endpoint: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	client, err := gonet.DialUDP("udp4", nil, server.LocalAddr().(*gonet.UDPAddr))
	if err != nil {
		t.Fatalf("failed to connect local Shadowsocks 2022 UDP client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	conn := &uservoneIntegrationUDPServerConn{UDPConn: server, destination: client.LocalAddr().(*gonet.UDPAddr)}
	results := make(chan error, 1)
	go func() {
		results <- inbound.Process(uservoneIntegrationContext(), xraynet.Network_UDP, conn, dispatcher)
	}()
	return client, server, results
}

func TestShadowsocks2022IntegrationUDP(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	echoDest := uservoneIntegrationUDPEcho(t)
	inbound := uservoneIntegrationNewInbound(t, xraynet.Network_UDP)

	var dispatches atomic.Int32
	upstreams := make(chan gonet.Conn, 1)
	dispatcher := &uservoneIntegrationDispatcher{onDispatch: func(_ context.Context, dest xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		if dest.NetAddr() != echoDest.NetAddr() || dest.Network != xraynet.Network_UDP {
			return nil, fmt.Errorf("unexpected UDP destination %s", dest.NetAddr())
		}
		upstream, err := gonet.DialTimeout("udp4", dest.NetAddr(), 2*time.Second)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = upstream.Close() })
		upstreams <- upstream
		return &transport.Link{Reader: buf.NewPacketReader(upstream), Writer: buf.NewWriter(upstream)}, nil
	}}
	client, server, results := uservoneIntegrationServeUDP(t, inbound, dispatcher)
	method := uservoneIntegrationNewClientMethod(t, uservoneIntegrationKey)
	secureClient := method.DialPacketConn(client)
	t.Cleanup(func() { _ = secureClient.Close() })
	payload := []byte("uservone Shadowsocks 2022 UDP integration payload")
	request := singbuf.NewSize(2048 + len(payload))
	request.Resize(1024, 0)
	if _, err := request.Write(payload); err != nil {
		request.Release()
		t.Fatalf("failed to build Shadowsocks 2022 UDP request: %v", err)
	}
	if err := secureClient.WritePacket(request, metadata.ParseSocksaddr(echoDest.NetAddr())); err != nil {
		t.Fatalf("failed to send Shadowsocks 2022 UDP request: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("failed to set UDP test deadline: %v", err)
	}
	response := singbuf.New()
	defer response.Release()
	responseSource, err := secureClient.ReadPacket(response)
	if err != nil {
		t.Fatalf("failed to read Shadowsocks 2022 UDP response: %v", err)
	}
	if !bytes.Equal(response.Bytes(), payload) {
		t.Fatalf("UDP echo payload mismatch: got %q, want %q", response.Bytes(), payload)
	}
	if responseSource.String() != echoDest.NetAddr() {
		t.Fatalf("UDP response destination = %s, want %s", responseSource.String(), echoDest.NetAddr())
	}

	_ = client.Close()
	_ = (<-upstreams).Close()
	_ = server.Close()
	processErr := uservoneIntegrationWaitResult(t, results)
	if got := dispatches.Load(); got != 1 {
		t.Fatalf("UDP dispatcher call count = %d, want 1", got)
	}
	uservoneIntegrationAssertNoSecrets(t, logs.String()+"\n"+fmt.Sprint(processErr),
		string(uservoneIntegrationKey), base64.StdEncoding.EncodeToString(uservoneIntegrationKey))
}

func TestShadowsocks2022IntegrationRejectsWrongUDPPassword(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	inbound := uservoneIntegrationNewInbound(t, xraynet.Network_UDP)
	var dispatches atomic.Int32
	dispatcher := &uservoneIntegrationDispatcher{onDispatch: func(context.Context, xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		return nil, fmt.Errorf("wrong-password packet reached dispatcher")
	}}
	client, server, results := uservoneIntegrationServeUDP(t, inbound, dispatcher)
	method := uservoneIntegrationNewClientMethod(t, uservoneIntegrationWrongKey)
	secureClient := method.DialPacketConn(client)
	t.Cleanup(func() { _ = secureClient.Close() })
	request := singbuf.NewSize(2048 + len("must be rejected"))
	request.Resize(1024, 0)
	if _, err := request.Write([]byte("must be rejected")); err != nil {
		request.Release()
		t.Fatalf("failed to build wrong-password UDP packet: %v", err)
	}
	if err := secureClient.WritePacket(request, metadata.ParseSocksaddr("127.0.0.1:9")); err != nil {
		t.Fatalf("failed to send wrong-password UDP packet: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("failed to set UDP rejection deadline: %v", err)
	}
	response := singbuf.New()
	defer response.Release()
	if _, err := secureClient.ReadPacket(response); err == nil {
		t.Fatal("wrong-password UDP packet received a response")
	} else if netErr, ok := err.(gonet.Error); !ok || !netErr.Timeout() {
		t.Fatalf("wrong-password UDP read failed unexpectedly: %v", err)
	}
	if got := dispatches.Load(); got != 0 {
		t.Fatalf("wrong-password UDP packet reached dispatcher %d times", got)
	}

	_ = client.Close()
	_ = server.Close()
	processErr := uservoneIntegrationWaitResult(t, results)
	if processErr == nil {
		t.Fatal("wrong-password UDP packet was accepted")
	}
	uservoneIntegrationAssertNoSecrets(t, logs.String()+"\n"+processErr.Error(),
		string(uservoneIntegrationKey), base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		string(uservoneIntegrationWrongKey), base64.StdEncoding.EncodeToString(uservoneIntegrationWrongKey))
}
