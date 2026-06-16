package proxy

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type GoProxyServer struct {
	port       int
	proxyID    string
	bindIP     string
	outboundIP string
	netnsName  string
	username   string
	password   string
	auth       AuthValidator
	guard      ConnectionGuard
	listener   net.Listener
	wg         sync.WaitGroup
	quit       chan struct{}
	mu         sync.Mutex
	running    bool
}

type AuthResult struct {
	Allowed    bool
	CustomerID string
	Reason     string
}

type AuthValidator func(proxyID, username, password, clientIP string) AuthResult
type ConnectionGuard func(proxyID, customerID, clientIP, target string) (func(), error)

var customerAuthValidator AuthValidator
var connectionGuard ConnectionGuard

func SetCustomerAuthValidator(v AuthValidator) {
	customerAuthValidator = v
}

func SetConnectionGuard(v ConnectionGuard) {
	connectionGuard = v
}

func NewGoProxyServer(port int, bindIP, outboundIP, username, password string) *GoProxyServer {
	return &GoProxyServer{
		port:       port,
		bindIP:     bindIP,
		outboundIP: outboundIP,
		username:   username,
		password:   password,
		quit:       make(chan struct{}),
	}
}

func NewAuthenticatedGoProxyServer(proxyID string, port int, bindIP, outboundIP, username, password string) *GoProxyServer {
	server := NewGoProxyServer(port, bindIP, outboundIP, username, password)
	server.proxyID = proxyID
	server.auth = customerAuthValidator
	server.guard = connectionGuard
	return server
}

// SetNamespace binds the proxy's outbound dials to a Linux network namespace.
// When set, outbound connections egress through the VPN interface living in
// that namespace instead of the host default route.
func (s *GoProxyServer) SetNamespace(nsName string) {
	s.netnsName = nsName
}

func (s *GoProxyServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	addr := fmt.Sprintf("%s:%d", s.bindIP, s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	s.listener = listener
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

func (s *GoProxyServer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.quit)
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *GoProxyServer) acceptConnections() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				// ignore accept errors on shutdown
				continue
			}
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConnection(c)
		}(conn)
	}
}

func (s *GoProxyServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	firstByte, err := reader.Peek(1)
	if err != nil {
		return
	}

	if firstByte[0] == 0x05 {
		// SOCKS5
		s.handleSocks5(conn, reader)
	} else {
		// HTTP / HTTPS
		s.handleHTTP(conn, reader)
	}
}

func (s *GoProxyServer) dial(network, address string) (net.Conn, error) {
	// Preferred path: the VPN tunnel lives inside a network namespace, so the
	// outbound socket must be created there to egress via the WireGuard iface.
	if s.netnsName != "" {
		return dialInNamespace(s.netnsName, network, address)
	}

	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Fallback: bind a specific source IP (only works when that IP exists in
	// the proxy's own namespace — e.g. a host-level VPN adapter).
	if s.outboundIP != "" {
		outboundAddr := &net.TCPAddr{
			IP: net.ParseIP(s.outboundIP),
		}
		dialer.LocalAddr = outboundAddr
	}

	return dialer.Dial(network, address)
}

// SOCKS5 handler
func (s *GoProxyServer) handleSocks5(conn net.Conn, reader *bufio.Reader) {
	// Read SOCKS5 greeting
	// [VER, NMETHODS, METHODS]
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return
	}

	nmethods := int(header[1])
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}

	// Determine authentication method
	authMethod := byte(0xFF) // No acceptable methods
	hasNoAuth := false
	hasUserPass := false

	for _, m := range methods {
		if m == 0x00 {
			hasNoAuth = true
		} else if m == 0x02 {
			hasUserPass = true
		}
	}

	if s.username != "" && s.password != "" {
		if hasUserPass {
			authMethod = 0x02
		}
	} else if hasNoAuth {
		authMethod = 0x00
	}

	// Send auth response
	if _, err := conn.Write([]byte{0x05, authMethod}); err != nil {
		return
	}

	if authMethod == 0xFF {
		return
	}

	authResult := AuthResult{Allowed: true}
	if authMethod == 0x02 {
		authResult = s.authenticateSocks5(conn, reader)
		if !authResult.Allowed {
			return
		}
	}

	// Read Request
	// [VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT]
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(reader, reqHeader); err != nil {
		return
	}

	if reqHeader[1] != 0x01 { // Only CONNECT command supported
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Command not supported
		return
	}

	var destHost string
	atyp := reqHeader[3]

	switch atyp {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		destHost = net.IP(ip).String()
	case 0x03: // Domain name
		lenByte, err := reader.ReadByte()
		if err != nil {
			return
		}
		domainName := make([]byte, int(lenByte))
		if _, err := io.ReadFull(reader, domainName); err != nil {
			return
		}
		destHost = string(domainName)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		destHost = net.IP(ip).String()
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Address type not supported
		return
	}

	var portBytes [2]byte
	if _, err := io.ReadFull(reader, portBytes[:]); err != nil {
		return
	}
	destPort := binary.BigEndian.Uint16(portBytes[:])
	destAddr := fmt.Sprintf("%s:%d", destHost, destPort)

	release := func() {}
	if s.guard != nil {
		var err error
		release, err = s.guard(s.proxyID, authResult.CustomerID, remoteIP(conn), destAddr)
		if err != nil {
			conn.Write([]byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			return
		}
	}
	defer release()

	// Dial target
	targetConn, err := s.dial("tcp", destAddr)
	if err != nil {
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}
	defer targetConn.Close()

	// Send success response
	// [VER, REP, RSV, ATYP, BND.ADDR, BND.PORT]
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// Bridge connection
	inBytes, outBytes := s.bridge(conn, targetConn)
	recordProxyUsage(s.proxyID, authResult.CustomerID, inBytes, outBytes)
}

func (s *GoProxyServer) authenticateSocks5(conn net.Conn, reader *bufio.Reader) AuthResult {
	// [VER, ULEN, UNAME, PLEN, PASSWD]
	ver, err := reader.ReadByte()
	if err != nil || ver != 0x01 {
		conn.Write([]byte{0x01, 0x01}) // Failed auth version
		return AuthResult{Allowed: false, Reason: "invalid socks5 auth version"}
	}

	ulen, err := reader.ReadByte()
	if err != nil {
		return AuthResult{Allowed: false, Reason: "invalid socks5 username"}
	}
	username := make([]byte, int(ulen))
	if _, err := io.ReadFull(reader, username); err != nil {
		return AuthResult{Allowed: false, Reason: "invalid socks5 username"}
	}

	plen, err := reader.ReadByte()
	if err != nil {
		return AuthResult{Allowed: false, Reason: "invalid socks5 password"}
	}
	password := make([]byte, int(plen))
	if _, err := io.ReadFull(reader, password); err != nil {
		return AuthResult{Allowed: false, Reason: "invalid socks5 password"}
	}

	if s.auth != nil {
		result := s.auth(s.proxyID, string(username), string(password), remoteIP(conn))
		if result.Allowed {
			conn.Write([]byte{0x01, 0x00})
			return result
		}
		conn.Write([]byte{0x01, 0x01})
		return result
	}

	isValid := false
	if string(username) == s.username {
		if string(password) == s.password {
			isValid = true
		} else {
			err := bcrypt.CompareHashAndPassword([]byte(s.password), password)
			if err == nil {
				isValid = true
			}
		}
	}

	if isValid {
		conn.Write([]byte{0x01, 0x00}) // Success
		return AuthResult{Allowed: true}
	}

	conn.Write([]byte{0x01, 0x01}) // Failure
	return AuthResult{Allowed: false, Reason: "invalid credentials"}
}

// HTTP handler
func (s *GoProxyServer) handleHTTP(conn net.Conn, reader *bufio.Reader) {
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}

	// Verify authentication
	authResult := AuthResult{Allowed: true}
	if s.username != "" && s.password != "" {
		auth := req.Header.Get("Proxy-Authorization")
		if auth == "" {
			conn.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"Proxy\"\r\n\r\n"))
			return
		}
		authResult = s.authenticateHTTP(auth, remoteIP(conn))
		if !authResult.Allowed {
			conn.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"Proxy\"\r\n\r\n"))
			return
		}
	}

	if req.Method == http.MethodConnect {
		// HTTPS Tunneling
		destAddr := req.RequestURI
		release := func() {}
		if s.guard != nil {
			var err error
			release, err = s.guard(s.proxyID, authResult.CustomerID, remoteIP(conn), destAddr)
			if err != nil {
				conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\nTarget blocked or connection limit exceeded"))
				return
			}
		}
		defer release()
		targetConn, err := s.dial("tcp", destAddr)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer targetConn.Close()

		_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			return
		}

		inBytes, outBytes := s.bridge(conn, targetConn)
		recordProxyUsage(s.proxyID, authResult.CustomerID, inBytes, outBytes)
	} else {
		// HTTP Proxying
		destAddr := req.Host
		if !strings.Contains(destAddr, ":") {
			destAddr = destAddr + ":80"
		}
		release := func() {}
		if s.guard != nil {
			var err error
			release, err = s.guard(s.proxyID, authResult.CustomerID, remoteIP(conn), destAddr)
			if err != nil {
				conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\nTarget blocked or connection limit exceeded"))
				return
			}
		}
		defer release()

		targetConn, err := s.dial("tcp", destAddr)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer targetConn.Close()

		// Forward the request to target
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Proxy-Connection")
		err = req.Write(targetConn)
		if err != nil {
			return
		}

		// Bridge connection (with remaining body/response)
		inBytes, outBytes := s.bridge(conn, targetConn)
		recordProxyUsage(s.proxyID, authResult.CustomerID, inBytes, outBytes)
	}
}

func (s *GoProxyServer) authenticateHTTP(authHeader string, clientIP string) AuthResult {
	const prefix = "Basic "
	if !strings.HasPrefix(authHeader, prefix) {
		return AuthResult{Allowed: false, Reason: "missing basic auth"}
	}
	payload, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
	if err != nil {
		return AuthResult{Allowed: false, Reason: "invalid basic auth"}
	}
	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		return AuthResult{Allowed: false, Reason: "invalid basic auth"}
	}
	if s.auth != nil {
		result := s.auth(s.proxyID, pair[0], pair[1], clientIP)
		if result.Allowed {
			return result
		}
	}
	if pair[0] != s.username {
		return AuthResult{Allowed: false, Reason: "invalid credentials"}
	}
	if pair[1] == s.password {
		return AuthResult{Allowed: true}
	}
	err = bcrypt.CompareHashAndPassword([]byte(s.password), []byte(pair[1]))
	return AuthResult{Allowed: err == nil}
}

func (s *GoProxyServer) bridge(client, target net.Conn) (uint64, uint64) {
	errChan := make(chan error, 2)
	countClient := &countingConn{Conn: client}
	countTarget := &countingConn{Conn: target}
	go func() {
		_, err := io.Copy(countTarget, client)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(countClient, target)
		errChan <- err
	}()
	<-errChan
	return countTarget.written, countClient.written
}

type countingConn struct {
	net.Conn
	written uint64
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.written += uint64(n)
	return n, err
}

type UsageRecorder func(proxyID, customerID string, inBytes, outBytes uint64)

var usageRecorder UsageRecorder

func SetUsageRecorder(v UsageRecorder) {
	usageRecorder = v
}

func recordProxyUsage(proxyID, customerID string, inBytes, outBytes uint64) {
	if usageRecorder != nil {
		usageRecorder(proxyID, customerID, inBytes, outBytes)
	}
}

func remoteIP(conn net.Conn) string {
	if conn == nil || conn.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return conn.RemoteAddr().String()
	}
	return host
}
