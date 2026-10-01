//go:build !js

package utils

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
)

// A relay on localhost signs its own certificate, so its chain is fetched once and
// trusted for this process.
func bootstrapRelayTrust(ctx context.Context, relayURL *url.URL, serverName string) (*x509.CertPool, error) {
	if !IsLocalRelayHost(serverName) {
		return nil, nil
	}
	rootCAPEM, err := FetchEndpointCertificateChain(ctx, relayURL.String(), serverName)
	if err != nil {
		return nil, fmt.Errorf("bootstrap localhost relay trust: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(rootCAPEM) {
		return nil, errors.New("failed to parse relay root ca")
	}
	return pool, nil
}
