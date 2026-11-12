package server

import (
	"errors"
	"net"
	"os"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
)

const (
	// maxMonitoringConnections caps simultaneous monitoring client connections.
	maxMonitoringConnections = 8
	// maxStreamsPerConnection caps concurrent RPCs (including telemetry streams) per connection.
	maxStreamsPerConnection = 4
	// maxRequestBytes caps inbound messages; every monitoring request is an empty message.
	maxRequestBytes = 1024
)

// errPeerCredentialsUnsupported is returned by peerUID on platforms without peer
// credentials. Access then relies on the 0600 permissions of the socket file.
var errPeerCredentialsUnsupported = errors.New("peer credentials are not supported on this platform")

// peerAuthenticatingListener accepts only Unix socket connections whose peer runs
// as the same user as the exporter, or as root. Other connections are closed
// before any gRPC data is exchanged.
type peerAuthenticatingListener struct {
	net.Listener
	allowedUID uint32
	peerUID    func(*net.UnixConn) (uint32, error)
}

func newMonitoringListener(l net.Listener) net.Listener {
	return netutil.LimitListener(&peerAuthenticatingListener{
		Listener:   l,
		allowedUID: uint32(os.Getuid()), //nolint:gosec // UIDs are non-negative on platforms with Unix sockets.
		peerUID:    peerUID,
	}, maxMonitoringConnections)
}

// Accept returns the next authenticated connection.
func (l *peerAuthenticatingListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.authorized(conn) {
			return conn, nil
		}
		_ = conn.Close()
	}
}

func (l *peerAuthenticatingListener) authorized(conn net.Conn) bool {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		log.Warn("rejected internal monitoring connection: not a unix socket")
		return false
	}

	uid, err := l.peerUID(unixConn)
	if err != nil {
		if errors.Is(err, errPeerCredentialsUnsupported) {
			return true
		}
		log.WithError(err).Warn("rejected internal monitoring connection: cannot read peer credentials")
		return false
	}
	if uid != l.allowedUID && uid != 0 {
		log.WithField("peer-uid", uid).Warn("rejected internal monitoring connection from another user")
		return false
	}
	return true
}
