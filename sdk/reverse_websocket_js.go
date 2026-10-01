//go:build js

package sdk

import (
	"crypto/tls"

	"github.com/coder/websocket"
)

// The browser owns the TLS handshake to the relay and its own trust store, so the
// caller's config has nowhere to go here.
func reverseDialOptions(_ *tls.Config) *websocket.DialOptions {
	return nil
}
