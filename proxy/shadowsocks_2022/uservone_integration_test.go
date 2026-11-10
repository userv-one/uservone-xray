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

	"github.com/xtls/xray-core/common/buf"
	clog "github.com/xtls/xray-core/common/log"
	xraynet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/features/routing"
	. "github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"github.com/xtls/xray-core/transport"
)

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
		{name: "server_password", method: MethodAES128GCM, key: invalidSecret, secret: invalidSecret},
		{name: "client_password", client: true, method: MethodAES128GCM, key: invalidSecret, secret: invalidSecret},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.client {
				_, err = NewClient(context.Background(), &ClientConfig{Method: tc.method, Key: tc.key})
			} else {
				_, err = NewServer(context.Background(), &ServerConfig{Method: tc.method, Key: tc.key})
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
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()
	addr := listener.Addr().(*gonet.TCPAddr)
	return xraynet.TCPDestination(xraynet.IPAddress(addr.IP), xraynet.Port(addr.Port))
}

func uservoneIntegrationServeTCP(t *testing.T, inbound *Inbound, dispatcher routing.Dispatcher) (string, <-chan error) {
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
		results <- inbound.Process(newTestContext(), xraynet.Network_TCP, conn, dispatcher)
	}()
	return listener.Addr().String(), results
}

func uservoneIntegrationReadPayload(t *testing.T, reader buf.Reader) []byte {
	t.Helper()
	mb, err := reader.ReadMultiBuffer()
	if err != nil {
		t.Fatalf("failed to read Shadowsocks 2022 response payload: %v", err)
	}
	defer buf.ReleaseMulti(mb)
	var payload []byte
	for _, b := range mb {
		payload = append(payload, b.Bytes()...)
	}
	return payload
}

func TestShadowsocks2022IntegrationTCP(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	echoDest := uservoneIntegrationTCPEcho(t)
	inbound, err := NewServer(newTestContext(), &ServerConfig{
		Method:  MethodAES128GCM,
		Key:     base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		Network: []xraynet.Network{xraynet.Network_TCP},
	})
	if err != nil {
		t.Fatalf("failed to create Shadowsocks 2022 TCP inbound: %v", err)
	}

	var dispatches atomic.Int32
	dispatcher := &dummyDispatcher{onDispatch: func(_ context.Context, dest xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		if dest.NetAddr() != echoDest.NetAddr() || dest.Network != xraynet.Network_TCP {
			return nil, fmt.Errorf("unexpected TCP destination")
		}
		upstream, err := gonet.DialTimeout("tcp4", dest.NetAddr(), 2*time.Second)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = upstream.Close() })
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

	method, err := GetCipherMethod(MethodAES128GCM)
	if err != nil {
		t.Fatalf("failed to load TCP test cipher: %v", err)
	}
	salt := bytes.Repeat([]byte{0x42}, method.KeySaltLength)
	payload := []byte("uservone Shadowsocks 2022 TCP integration payload")
	if _, err := WriteTCPRequest(client, method, [][]byte{uservoneIntegrationKey}, echoDest, salt, payload); err != nil {
		t.Fatalf("failed to write Shadowsocks 2022 TCP request: %v", err)
	}
	response, err := ReadTCPResponse(client, method, uservoneIntegrationKey, salt)
	if err != nil {
		t.Fatalf("failed to read Shadowsocks 2022 TCP response: %v", err)
	}
	if got := uservoneIntegrationReadPayload(t, response); !bytes.Equal(got, payload) {
		t.Fatalf("TCP echo payload mismatch: got %q, want %q", got, payload)
	}

	_ = client.Close()
	processErr := uservoneIntegrationWaitResult(t, results)
	if got := dispatches.Load(); got != 1 {
		t.Fatalf("TCP dispatcher call count = %d, want 1", got)
	}
	uservoneIntegrationAssertNoSecrets(t, logs.String()+"\n"+fmt.Sprint(processErr),
		string(uservoneIntegrationKey), base64.StdEncoding.EncodeToString(uservoneIntegrationKey))
}

func TestShadowsocks2022IntegrationRejectsWrongTCPPassword(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	inbound, err := NewServer(newTestContext(), &ServerConfig{
		Method:  MethodAES128GCM,
		Key:     base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		Network: []xraynet.Network{xraynet.Network_TCP},
	})
	if err != nil {
		t.Fatalf("failed to create Shadowsocks 2022 TCP inbound: %v", err)
	}
	var dispatches atomic.Int32
	dispatcher := &dummyDispatcher{onDispatch: func(context.Context, xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		return nil, fmt.Errorf("wrong-password request reached dispatcher")
	}}
	proxyAddr, results := uservoneIntegrationServeTCP(t, inbound, dispatcher)
	client, err := gonet.DialTimeout("tcp4", proxyAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to local Shadowsocks 2022 TCP endpoint: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	method, err := GetCipherMethod(MethodAES128GCM)
	if err != nil {
		t.Fatalf("failed to load TCP test cipher: %v", err)
	}
	salt := bytes.Repeat([]byte{0x24}, method.KeySaltLength)
	dest := xraynet.TCPDestination(xraynet.LocalHostIP, 9)
	_, writeErr := WriteTCPRequest(client, method, [][]byte{uservoneIntegrationWrongKey}, dest, salt, []byte("must be rejected"))
	_ = client.Close()
	if writeErr != nil {
		t.Fatalf("failed to send wrong-password TCP request: %v", writeErr)
	}
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

func uservoneIntegrationServeUDP(t *testing.T, inbound *Inbound, dispatcher routing.Dispatcher) (*gonet.UDPConn, *gonet.UDPConn, <-chan error) {
	t.Helper()
	server, err := gonet.ListenUDP("udp4", &gonet.UDPAddr{IP: gonet.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("failed to start local Shadowsocks 2022 UDP endpoint: %v", err)
	}
	client, err := gonet.DialUDP("udp4", nil, server.LocalAddr().(*gonet.UDPAddr))
	if err != nil {
		_ = server.Close()
		t.Fatalf("failed to connect local Shadowsocks 2022 UDP client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	conn := &xraynet.PacketConnWrapper{PacketConn: server, Dest: client.LocalAddr()}
	results := make(chan error, 1)
	go func() {
		results <- inbound.Process(newTestContext(), xraynet.Network_UDP, conn, dispatcher)
	}()
	return client, server, results
}

func uservoneIntegrationUDPSession(t *testing.T, key []byte) (*ClientUDPSession, *CipherMethod) {
	t.Helper()
	method, err := GetCipherMethod(MethodAES128GCM)
	if err != nil {
		t.Fatalf("failed to load UDP test cipher: %v", err)
	}
	codec, err := NewUDPPacketCodec(method, [][]byte{key})
	if err != nil {
		t.Fatalf("failed to create UDP client codec: %v", err)
	}
	session, err := codec.NewClientSession()
	if err != nil {
		t.Fatalf("failed to create UDP client session: %v", err)
	}
	return session, method
}

func TestShadowsocks2022IntegrationUDP(t *testing.T) {
	logs := uservoneIntegrationCaptureLogs(t)
	echoDest := uservoneIntegrationUDPEcho(t)
	inbound, err := NewServer(newTestContext(), &ServerConfig{
		Method:  MethodAES128GCM,
		Key:     base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		Network: []xraynet.Network{xraynet.Network_UDP},
	})
	if err != nil {
		t.Fatalf("failed to create Shadowsocks 2022 UDP inbound: %v", err)
	}

	var dispatches atomic.Int32
	dispatcher := &dummyDispatcher{onDispatch: func(_ context.Context, dest xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		if dest.NetAddr() != echoDest.NetAddr() || dest.Network != xraynet.Network_UDP {
			return nil, fmt.Errorf("unexpected UDP destination")
		}
		upstream, err := gonet.DialTimeout("udp4", dest.NetAddr(), 2*time.Second)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = upstream.Close() })
		return &transport.Link{Reader: buf.NewPacketReader(upstream), Writer: buf.NewWriter(upstream)}, nil
	}}
	client, server, results := uservoneIntegrationServeUDP(t, inbound, dispatcher)
	session, _ := uservoneIntegrationUDPSession(t, uservoneIntegrationKey)
	payload := []byte("uservone Shadowsocks 2022 UDP integration payload")
	packet, err := session.EncodePacket(echoDest, payload)
	if err != nil {
		t.Fatalf("failed to encode Shadowsocks 2022 UDP request: %v", err)
	}
	if _, err := client.Write(packet.Bytes()); err != nil {
		packet.Release()
		t.Fatalf("failed to send Shadowsocks 2022 UDP request: %v", err)
	}
	packet.Release()
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("failed to set UDP test deadline: %v", err)
	}
	buffer := make([]byte, 65535)
	n, err := client.Read(buffer)
	if err != nil {
		t.Fatalf("failed to read Shadowsocks 2022 UDP response: %v", err)
	}
	decoded, err := session.DecodePacket(buffer[:n])
	if err != nil {
		t.Fatalf("failed to decode Shadowsocks 2022 UDP response: %v", err)
	}
	if !bytes.Equal(decoded.Payload, payload) {
		t.Fatalf("UDP echo payload mismatch: got %q, want %q", decoded.Payload, payload)
	}
	if decoded.Destination.NetAddr() != echoDest.NetAddr() {
		t.Fatalf("UDP response destination = %s, want %s", decoded.Destination.NetAddr(), echoDest.NetAddr())
	}

	_ = client.Close()
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
	inbound, err := NewServer(newTestContext(), &ServerConfig{
		Method:  MethodAES128GCM,
		Key:     base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		Network: []xraynet.Network{xraynet.Network_UDP},
	})
	if err != nil {
		t.Fatalf("failed to create Shadowsocks 2022 UDP inbound: %v", err)
	}
	var dispatches atomic.Int32
	dispatcher := &dummyDispatcher{onDispatch: func(context.Context, xraynet.Destination) (*transport.Link, error) {
		dispatches.Add(1)
		return nil, fmt.Errorf("wrong-password packet reached dispatcher")
	}}
	client, server, results := uservoneIntegrationServeUDP(t, inbound, dispatcher)
	session, _ := uservoneIntegrationUDPSession(t, uservoneIntegrationWrongKey)
	dest := xraynet.UDPDestination(xraynet.LocalHostIP, 9)
	packet, err := session.EncodePacket(dest, []byte("must be rejected"))
	if err != nil {
		t.Fatalf("failed to encode wrong-password UDP packet: %v", err)
	}
	if _, err := client.Write(packet.Bytes()); err != nil {
		packet.Release()
		t.Fatalf("failed to send wrong-password UDP packet: %v", err)
	}
	packet.Release()
	if err := client.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("failed to set UDP rejection deadline: %v", err)
	}
	buffer := make([]byte, 65535)
	if _, err := client.Read(buffer); err == nil {
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
	uservoneIntegrationAssertNoSecrets(t, logs.String()+"\n"+fmt.Sprint(processErr),
		string(uservoneIntegrationKey), base64.StdEncoding.EncodeToString(uservoneIntegrationKey),
		string(uservoneIntegrationWrongKey), base64.StdEncoding.EncodeToString(uservoneIntegrationWrongKey))
}
