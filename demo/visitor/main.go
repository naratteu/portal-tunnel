//go:build !js

// A visitor: dials the relay's SNI port, verifies against its certificate, and asks the
// public hostname for a page. Whatever comes back was produced inside a browser tab.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: visitor <sni-addr> <cert-pem> <public-url>")
		os.Exit(2)
	}
	sniAddr, certPath, publicURL := os.Args[1], os.Args[2], os.Args[3]

	pem, err := os.ReadFile(certPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read certificate:", err)
		os.Exit(1)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)

	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, sniAddr)
			},
			ForceAttemptHTTP2: false,
		},
	}

	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		resp, err := client.Get(publicURL + "/hello")
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("HTTP %d %s\n", resp.StatusCode, resp.Header.Get("Content-Type"))
		fmt.Printf("body: %s\n", body)
		return
	}
	fmt.Fprintln(os.Stderr, "visitor failed:", lastErr)
	os.Exit(1)
}
