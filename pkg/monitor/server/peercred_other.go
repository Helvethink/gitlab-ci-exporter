//go:build !linux && !darwin

package server

import "net"

// peerUID is not implemented on this platform. Access then relies on the 0600
// permissions of the socket file only.
func peerUID(*net.UnixConn) (uint32, error) {
	return 0, errPeerCredentialsUnsupported
}
