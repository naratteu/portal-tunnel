//go:build !js

package utils

import "context"

// FetchEndpointCertificateChain returns the certificate chain an endpoint presents,
// read straight off a TLS handshake with it.
func FetchEndpointCertificateChain(ctx context.Context, endpoint, serverName string) ([]byte, error) {
	return fetchEndpointCertificateChainOverTLS(ctx, endpoint, serverName)
}
