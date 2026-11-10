package socks_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	gonet "net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/proxy/socks"
	"github.com/xtls/xray-core/transport"
)

const (
	uservoneRFC1928Timeout  = 5 * time.Second
	uservoneRFC1928Username = "rfc1928-test-user"
	uservoneRFC1928Password = "rfc1928-test-password"
	uservoneRFC1928Domain   = "loopback.rfc1928.test"
)

func TestRFC1928UsernamePasswordAndConnect(t *testing.T) {
	echoAddress := uservoneStartTCPEcho(t)
	dispatcher := uservoneNewLoopbackDispatcher(t)
	proxyAddress := uservoneStartSOCKSServer(t, dispatcher)

	conn := uservoneDialSOCKS(t, proxyAddress)
	echoTCPAddress := echoAddress.(*gonet.TCPAddr)
	request := &protocol.RequestHeader{
		Version: 5,
		Command: protocol.RequestCommandTCP,
		Address: xnet.IPAddress(echoTCPAddress.IP),
		Port:    xnet.Port(echoTCPAddress.Port),
		User:    uservoneRFC1928User(uservoneRFC1928Password),
	}
	udpRequest, err := socks.ClientHandshake(request, conn, conn)
	if err != nil {
		t.Fatalf("RFC 1929 authentication and CONNECT handshake failed: %v", err)
	}
	if udpRequest != nil {
		t.Fatal("CONNECT handshake unexpectedly returned a UDP association")
	}

	uservoneAssertTCPDestination(t, dispatcher.destinations, xnet.TCPDestination(request.Address, request.Port))
	uservoneAssertTCPEcho(t, conn, []byte("authenticated socks5 connect"))
}

func TestRFC1929RejectsWrongPassword(t *testing.T) {
	dispatcher := uservoneNewLoopbackDispatcher(t)
	proxyAddress := uservoneStartSOCKSServer(t, dispatcher)
	conn := uservoneDialSOCKS(t, proxyAddress)

	request := &protocol.RequestHeader{
		Version: 5,
		Command: protocol.RequestCommandTCP,
		Address: xnet.LocalHostIP,
		Port:    9,
		User:    uservoneRFC1928User("incorrect-test-password"),
	}
	if _, err := socks.ClientHandshake(request, conn, conn); err == nil {
		t.Fatal("RFC 1929 authentication with a wrong password unexpectedly succeeded")
	}

	select {
	case destination := <-dispatcher.destinations:
		t.Fatalf("unauthenticated request reached the dispatcher: %v", destination)
	default:
	}
}

func TestRFC1928UDPAssociation(t *testing.T) {
	echoAddress := uservoneStartUDPEcho(t)
	dispatcher := uservoneNewLoopbackDispatcher(t)
	proxyAddress := uservoneStartSOCKSServer(t, dispatcher)
	controlConn := uservoneDialSOCKS(t, proxyAddress)

	request := &protocol.RequestHeader{
		Version: 5,
		Command: protocol.RequestCommandUDP,
		Address: xnet.AnyIP,
		Port:    0,
		User:    uservoneRFC1928User(uservoneRFC1928Password),
	}
	udpAssociation, err := socks.ClientHandshake(request, controlConn, controlConn)
	if err != nil {
		t.Fatalf("UDP ASSOCIATE handshake failed: %v", err)
	}
	if udpAssociation == nil {
		t.Fatal("UDP ASSOCIATE handshake returned no relay endpoint")
	}

	udpConn, err := gonet.DialTimeout("udp", udpAssociation.Destination().NetAddr(), uservoneRFC1928Timeout)
	if err != nil {
		t.Fatalf("failed to connect to the UDP relay: %v", err)
	}
	t.Cleanup(func() { _ = udpConn.Close() })
	if err := udpConn.SetDeadline(time.Now().Add(uservoneRFC1928Timeout)); err != nil {
		t.Fatalf("failed to set UDP relay deadline: %v", err)
	}

	echoUDPAddress := echoAddress.(*gonet.UDPAddr)
	destination := xnet.UDPDestination(xnet.IPAddress(echoUDPAddress.IP), xnet.Port(echoUDPAddress.Port))
	payload := []byte("socks5 udp association")
	packet, err := socks.EncodeUDPPacket(&protocol.RequestHeader{
		Version: 5,
		Command: protocol.RequestCommandUDP,
		Address: destination.Address,
		Port:    destination.Port,
	}, payload)
	if err != nil {
		t.Fatalf("failed to encode the UDP request: %v", err)
	}
	_, writeErr := udpConn.Write(packet.Bytes())
	packet.Release()
	if writeErr != nil {
		t.Fatalf("failed to send the encapsulated UDP request: %v", writeErr)
	}

	uservoneAssertUDPDestination(t, dispatcher.destinations, destination)
	responseBytes := make([]byte, buf.Size)
	n, err := udpConn.Read(responseBytes)
	if err != nil {
		t.Fatalf("failed to read the encapsulated UDP response: %v", err)
	}
	response := buf.FromBytes(responseBytes[:n])
	responseRequest, decodeErr := socks.DecodeUDPPacket(response)
	if decodeErr != nil {
		response.Release()
		t.Fatalf("failed to decode the UDP response: %v", decodeErr)
	}
	if got := responseRequest.Destination(); got != destination {
		response.Release()
		t.Fatalf("UDP response destination = %v, want %v", got, destination)
	}
	if got := string(response.Bytes()); got != string(payload) {
		response.Release()
		t.Fatalf("UDP response payload = %q, want %q", got, payload)
	}
	response.Release()
}

func TestRFC1928RemoteDNSConnect(t *testing.T) {
	echoAddress := uservoneStartTCPEcho(t)
	dispatcher := uservoneNewLoopbackDispatcher(t)
	proxyAddress := uservoneStartSOCKSServer(t, dispatcher)
	conn := uservoneDialSOCKS(t, proxyAddress)

	echoPort := xnet.Port(echoAddress.(*gonet.TCPAddr).Port)
	request := &protocol.RequestHeader{
		Version: 5,
		Command: protocol.RequestCommandTCP,
		Address: xnet.DomainAddress(uservoneRFC1928Domain),
		Port:    echoPort,
		User:    uservoneRFC1928User(uservoneRFC1928Password),
	}
	if _, err := socks.ClientHandshake(request, conn, conn); err != nil {
		t.Fatalf("socks5h-style CONNECT handshake failed: %v", err)
	}

	select {
	case destination := <-dispatcher.destinations:
		if !destination.Address.Family().IsDomain() {
			t.Fatalf("dispatcher received address family %v, want a domain", destination.Address.Family())
		}
		if got := destination.Address.Domain(); got != uservoneRFC1928Domain {
			t.Fatalf("dispatcher received domain %q, want %q", got, uservoneRFC1928Domain)
		}
		if destination.Port != echoPort || destination.Network != xnet.Network_TCP {
			t.Fatalf("dispatcher received destination %v, want TCP port %d", destination, echoPort)
		}
	case <-time.After(uservoneRFC1928Timeout):
		t.Fatal("timed out waiting for the remote-DNS CONNECT request")
	}

	uservoneAssertTCPEcho(t, conn, []byte("remote dns payload"))
}

func uservoneRFC1928User(password string) *protocol.MemoryUser {
	return &protocol.MemoryUser{
		Account: &socks.Account{
			Username: uservoneRFC1928Username,
			Password: password,
		},
	}
}

func uservoneDialSOCKS(t *testing.T, address string) gonet.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), uservoneRFC1928Timeout)
	defer cancel()
	conn, err := (&gonet.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		t.Fatalf("failed to dial the loopback SOCKS server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(uservoneRFC1928Timeout)); err != nil {
		t.Fatalf("failed to set the SOCKS client deadline: %v", err)
	}
	return conn
}

func uservoneAssertTCPEcho(t *testing.T, conn gonet.Conn, payload []byte) {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(uservoneRFC1928Timeout)); err != nil {
		t.Fatalf("failed to refresh the TCP test deadline: %v", err)
	}
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("failed to write the TCP echo payload: %v", err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatalf("failed to read the TCP echo response: %v", err)
	}
	if string(response) != string(payload) {
		t.Fatalf("TCP echo response = %q, want %q", response, payload)
	}
}

func uservoneAssertTCPDestination(t *testing.T, destinations <-chan xnet.Destination, expected xnet.Destination) {
	t.Helper()
	select {
	case destination := <-destinations:
		if destination != expected {
			t.Fatalf("dispatched destination = %v, want %v", destination, expected)
		}
	case <-time.After(uservoneRFC1928Timeout):
		t.Fatal("timed out waiting for the CONNECT request")
	}
}

func uservoneAssertUDPDestination(t *testing.T, destinations <-chan xnet.Destination, expected xnet.Destination) {
	t.Helper()
	select {
	case destination := <-destinations:
		if destination != expected {
			t.Fatalf("dispatched UDP destination = %v, want %v", destination, expected)
		}
	case <-time.After(uservoneRFC1928Timeout):
		t.Fatal("timed out waiting for the UDP request")
	}
}

type uservoneLoopbackDispatcher struct {
	destinations chan xnet.Destination

	mu     sync.Mutex
	conns  map[gonet.Conn]struct{}
	closed bool
}

func uservoneNewLoopbackDispatcher(t *testing.T) *uservoneLoopbackDispatcher {
	t.Helper()
	dispatcher := &uservoneLoopbackDispatcher{
		destinations: make(chan xnet.Destination, 8),
		conns:        make(map[gonet.Conn]struct{}),
	}
	t.Cleanup(func() { _ = dispatcher.Close() })
	return dispatcher
}

func (d *uservoneLoopbackDispatcher) Type() interface{} { return routing.DispatcherType() }
func (d *uservoneLoopbackDispatcher) Start() error      { return nil }

func (d *uservoneLoopbackDispatcher) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	connections := make([]gonet.Conn, 0, len(d.conns))
	for conn := range d.conns {
		connections = append(connections, conn)
	}
	d.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
	return nil
}

func (d *uservoneLoopbackDispatcher) record(destination xnet.Destination) {
	d.destinations <- destination
}

func (d *uservoneLoopbackDispatcher) dial(ctx context.Context, destination xnet.Destination) (gonet.Conn, error) {
	address := destination.Address.String()
	if destination.Address.Family().IsDomain() {
		if destination.Address.Domain() != uservoneRFC1928Domain {
			return nil, fmt.Errorf("unexpected test domain")
		}
		// Resolve the socks5h domain at the remote side without using system DNS.
		address = "127.0.0.1"
	}
	network := destination.Network.SystemString()
	conn, err := (&gonet.Dialer{Timeout: uservoneRFC1928Timeout}).DialContext(
		ctx,
		network,
		gonet.JoinHostPort(address, strconv.Itoa(int(destination.Port))),
	)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(uservoneRFC1928Timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		_ = conn.Close()
		return nil, errors.New("dispatcher is closed")
	}
	d.conns[conn] = struct{}{}
	d.mu.Unlock()
	return conn, nil
}

func (d *uservoneLoopbackDispatcher) untrack(conn gonet.Conn) {
	d.mu.Lock()
	delete(d.conns, conn)
	d.mu.Unlock()
}

func (d *uservoneLoopbackDispatcher) Dispatch(ctx context.Context, destination xnet.Destination) (*transport.Link, error) {
	d.record(destination)
	if destination.Network != xnet.Network_UDP {
		return nil, fmt.Errorf("Dispatch called for non-UDP network %v", destination.Network)
	}
	conn, err := d.dial(ctx, destination)
	if err != nil {
		return nil, err
	}
	return &transport.Link{
		Reader: buf.NewPacketReader(conn),
		Writer: buf.NewWriter(conn),
	}, nil
}

func (d *uservoneLoopbackDispatcher) DispatchLink(ctx context.Context, destination xnet.Destination, link *transport.Link) error {
	d.record(destination)
	if destination.Network != xnet.Network_TCP {
		return fmt.Errorf("DispatchLink called for non-TCP network %v", destination.Network)
	}
	conn, err := d.dial(ctx, destination)
	if err != nil {
		return err
	}
	defer func() {
		d.untrack(conn)
		_ = conn.Close()
	}()

	uploadDone := make(chan error, 1)
	go func() {
		err := buf.Copy(link.Reader, buf.NewWriter(conn))
		if tcpConn, ok := conn.(*gonet.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
		uploadDone <- err
	}()

	downloadErr := buf.Copy(buf.NewReader(conn), link.Writer)
	_ = common.Close(link.Writer)
	_ = conn.Close()
	uploadErr := <-uploadDone
	if downloadErr != nil {
		return downloadErr
	}
	return uploadErr
}

type uservoneSOCKSServer struct {
	listener gonet.Listener
	instance *core.Instance
	cancel   context.CancelFunc
	ctx      context.Context

	acceptDone chan struct{}
	handlers   sync.WaitGroup

	mu    sync.Mutex
	conns map[gonet.Conn]struct{}
}

func uservoneStartSOCKSServer(t *testing.T, dispatcher routing.Dispatcher) string {
	t.Helper()
	instance, err := core.New(&core.Config{})
	if err != nil {
		t.Fatalf("failed to create the Xray test instance: %v", err)
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	ctx := context.WithValue(baseCtx, core.XrayKey(1), instance)
	ctx = context.WithValue(ctx, "cone", false)
	handler, err := socks.NewServer(ctx, &socks.ServerConfig{
		AuthType: socks.AuthType_PASSWORD,
		Accounts: map[string]string{
			uservoneRFC1928Username: uservoneRFC1928Password,
		},
		Address:    xnet.NewIPOrDomain(xnet.LocalHostIP),
		UdpEnabled: true,
	})
	if err != nil {
		cancel()
		_ = instance.Close()
		t.Fatalf("failed to create the SOCKS server: %v", err)
	}
	listener, err := gonet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		_ = instance.Close()
		t.Fatalf("failed to listen for SOCKS connections: %v", err)
	}

	server := &uservoneSOCKSServer{
		listener:   listener,
		instance:   instance,
		cancel:     cancel,
		ctx:        ctx,
		acceptDone: make(chan struct{}),
		conns:      make(map[gonet.Conn]struct{}),
	}
	go server.serve(handler, dispatcher)
	t.Cleanup(server.close)
	return listener.Addr().String()
}

func (s *uservoneSOCKSServer) serve(handler *socks.Server, dispatcher routing.Dispatcher) {
	defer close(s.acceptDone)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.handlers.Add(1)
		go func() {
			defer s.handlers.Done()
			defer func() {
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
				_ = conn.Close()
			}()
			_ = conn.SetDeadline(time.Now().Add(uservoneRFC1928Timeout))
			gateway := xnet.DestinationFromAddr(s.listener.Addr())
			ctx := session.ContextWithInbound(s.ctx, &session.Inbound{
				Source:  xnet.DestinationFromAddr(conn.RemoteAddr()),
				Local:   gateway,
				Gateway: gateway,
			})
			_ = handler.Process(ctx, xnet.Network_TCP, conn, dispatcher)
		}()
	}
}

func (s *uservoneSOCKSServer) close() {
	_ = s.listener.Close()
	s.cancel()
	<-s.acceptDone
	s.mu.Lock()
	connections := make([]gonet.Conn, 0, len(s.conns))
	for conn := range s.conns {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
	s.handlers.Wait()
	_ = s.instance.Close()
}

func uservoneStartTCPEcho(t *testing.T) gonet.Addr {
	t.Helper()
	listener, err := gonet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start the TCP echo listener: %v", err)
	}
	done := make(chan struct{})
	accepted := make(chan gonet.Conn, 1)
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(uservoneRFC1928Timeout))
		buffer := make([]byte, 1024)
		for {
			n, err := conn.Read(buffer)
			if n > 0 {
				if _, writeErr := conn.Write(buffer[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case conn := <-accepted:
			_ = conn.Close()
		default:
		}
		<-done
	})
	return listener.Addr()
}

func uservoneStartUDPEcho(t *testing.T) gonet.Addr {
	t.Helper()
	conn, err := gonet.ListenUDP("udp", &gonet.UDPAddr{IP: gonet.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to start the UDP echo listener: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, buf.Size)
		for {
			n, remote, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			if _, err := conn.WriteToUDP(buffer[:n], remote); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		<-done
	})
	return conn.LocalAddr()
}
