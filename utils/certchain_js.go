//go:build js

package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gosuda/portal-tunnel/v2/types"
)

// FetchEndpointCertificateChain asks the relay for the chain over HTTP.
//
// The socket version reads it off a TLS handshake, which a browser cannot do: it owns
// the handshake and never hands the peer chain to the page. The chain is public either
// way - it is what the relay presents to every visitor - so serving it is not a
// disclosure, it is the same bytes by another route.
func FetchEndpointCertificateChain(ctx context.Context, endpoint, _ string) ([]byte, error) {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return nil, errors.New("endpoint is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint url: %w", err)
	}
	parsed.Path = types.PathSDKCertificateChain
	parsed.RawQuery = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch certificate chain: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch certificate chain: unexpected status %d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}
