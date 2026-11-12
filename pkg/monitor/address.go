package monitor

import (
	"errors"
	"fmt"
	"net/url"
)

// ErrUnsupportedListenerAddress is returned for internal monitoring addresses that
// are not Unix domain sockets.
var ErrUnsupportedListenerAddress = errors.New("internal monitoring listener must be a unix socket (unix:///path/to/socket)")

// ValidateListenerAddress checks that the internal monitoring address is a Unix
// domain socket.
//
// The monitoring service has no transport encryption or token authentication, so
// it is only served on a Unix socket. Access is restricted by the socket file
// permissions (0600) and by checking the peer user ID of each connection.
func ValidateListenerAddress(address *url.URL) error {
	if address == nil {
		return errors.New("internal monitoring listener address is required")
	}
	if address.Scheme != "unix" {
		return fmt.Errorf("%w: got scheme %q", ErrUnsupportedListenerAddress, address.Scheme)
	}
	if address.Host != "" {
		return fmt.Errorf("%w: unexpected host %q, use three slashes for an absolute path", ErrUnsupportedListenerAddress, address.Host)
	}
	if address.Path == "" {
		return fmt.Errorf("%w: socket path is empty", ErrUnsupportedListenerAddress)
	}
	return nil
}
