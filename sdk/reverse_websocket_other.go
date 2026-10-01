//go:build !js

package sdk

import (
	"crypto/tls"
	"net/http"

	"github.com/coder/websocket"
)

// Off the browser, the relay's certificate has to be trusted explicitly - in tests
// it is a local CA.
func reverseDialOptions(tlsConfig *tls.Config) *websocket.DialOptions {
	if tlsConfig == nil {
		return nil
	}
	return &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}},
	}
}
