//go:build js

package utils

import (
	"context"
	"crypto/x509"
	"net/url"
)

// In a browser the TLS handshake belongs to the browser: it picks the trust store and
// there is no socket to probe the relay's chain over. Whatever it decides stands.
func bootstrapRelayTrust(_ context.Context, _ *url.URL, _ string) (*x509.CertPool, error) {
	return nil, nil
}
