package e2e_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gosuda/portal-tunnel/v2/portal"
	"github.com/gosuda/portal-tunnel/v2/portal/identity"
	"github.com/gosuda/portal-tunnel/v2/sdk"
)

// The reverse session over a WebSocket instead of a socket upgraded to a raw stream.
// This is the transport a connector without sockets has to use; the test proves the
// rest of the stack does not care which one carried the bytes.
func TestReverseSessionOverWebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sniPort := harnessPort(t)
	stateDir := t.TempDir()
	relayURL := "https://127.0.0.1:" + strconv.Itoa(sniPort)

	relay, err := portal.NewServer(portal.ServerConfig{
		PortalURL:     relayURL,
		StateDir:      stateDir,
		SNIListenAddr: "127.0.0.1:" + strconv.Itoa(sniPort),
		SNIPort:       sniPort,
	})
	if err != nil {
		t.Fatalf("create relay: %v", err)
	}
	if err := relay.Start(ctx, nil); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = relay.Shutdown(shutdownCtx)
		_ = relay.Wait()
	})

	certPath := filepath.Join(stateDir, "fullchain.pem")
	relayTLS, err := relayTLSConfig(certPath)
	if err != nil {
		t.Fatalf("relay tls config: %v", err)
	}

	clientIdentity, err := identity.Generate("e2e-websocket")
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	exposure, err := sdk.Expose(ctx, clientIdentity, []string{relayURL},
		sdk.WithReverseDialer(sdk.WebSocketReverseDialer(relayTLS)))
	if err != nil {
		t.Fatalf("expose: %v", err)
	}
	t.Cleanup(func() { _ = exposure.Close() })

	// No local service and no socket to it: the handler runs in this process, the way
	// it would inside a WebAssembly page.
	served := make(chan error, 1)
	go func() {
		served <- sdk.RunHTTP(ctx, exposure, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "hello world")
		}), "")
	}()

	readyCtx, readyCancel := context.WithTimeout(ctx, 30*time.Second)
	defer readyCancel()
	relays, err := exposure.WaitReady(readyCtx)
	if err != nil {
		t.Fatalf("tunnel did not become ready: %v", err)
	}
	publicURL := relays[0].PublicURL
	t.Logf("public URL: %s", publicURL)

	var body string
	for attempt := 0; attempt < 30; attempt++ {
		if got, ok := tenantGet("127.0.0.1:"+strconv.Itoa(sniPort), certPath, publicURL); ok {
			body = got
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if body != "hello world" {
		t.Fatalf("tenant request returned %q, want %q", body, "hello world")
	}
}

func relayTLSConfig(certPath string) (*tls.Config, error) {
	pem, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	return &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, nil
}
