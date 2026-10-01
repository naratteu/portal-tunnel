//go:build !js

// Runs everything the browser connector demo needs on one machine: a relay, a plain
// HTTP front door for it, and the page that loads the WebAssembly connector.
//
// The front door exists only because the relay here signs its own certificate and a
// browser will not trust that. A relay with a real certificate needs none of this -
// the browser would talk to it directly.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gosuda/portal-tunnel/v2/portal"
	"github.com/gosuda/portal-tunnel/v2/utils"
)

func main() {
	sniPort := envInt("SNI_PORT", 31443)
	frontPort := envInt("FRONT_PORT", 31080)
	pagePort := envInt("PAGE_PORT", 31090)

	stateDir, err := os.MkdirTemp("", "portal-demo-")
	if err != nil {
		log.Fatalf("state dir: %v", err)
	}
	defer os.RemoveAll(stateDir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	relayURL := "https://127.0.0.1:" + strconv.Itoa(sniPort)
	relay, err := portal.NewServer(portal.ServerConfig{
		PortalURL:     relayURL,
		StateDir:      stateDir,
		SNIListenAddr: "127.0.0.1:" + strconv.Itoa(sniPort),
		SNIPort:       sniPort,
	})
	if err != nil {
		log.Fatalf("create relay: %v", err)
	}
	if err := relay.Start(ctx, nil); err != nil {
		log.Fatalf("start relay: %v", err)
	}

	certPath := filepath.Join(stateDir, "fullchain.pem")
	if err := waitForFile(certPath, 15*time.Second); err != nil {
		log.Fatalf("relay certificate: %v", err)
	}
	roots := x509.NewCertPool()
	pem, err := os.ReadFile(certPath)
	if err != nil {
		log.Fatalf("read relay certificate: %v", err)
	}
	roots.AppendCertsFromPEM(pem)

	// Plain HTTP in, relay TLS out. ReverseProxy carries the WebSocket upgrade through,
	// so the reverse session works over it unchanged.
	target, _ := url.Parse(relayURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+strconv.Itoa(sniPort))
		},
		ForceAttemptHTTP2: false,
	}
	frontURL := "http://127.0.0.1:" + strconv.Itoa(frontPort)
	go serve(frontPort, proxy)

	page := http.NewServeMux()
	page.Handle("/", http.FileServer(http.Dir(webDir())))

	// Served straight out of the Go distribution rather than vendored into the tree.
	page.HandleFunc("/wasm_exec.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, filepath.Join(goroot(), "lib", "wasm", "wasm_exec.js"))
	})
	// Reads a relay's certificate chain off a TLS handshake and hands it to the page.
	// Not part of the relay: anything that can open a socket can serve this, because the
	// chain is what the relay already shows every visitor.
	page.HandleFunc("/chain", func(w http.ResponseWriter, r *http.Request) {
		host := r.URL.Query().Get("host")
		if host == "" {
			host = r.URL.Query().Get("")
		}
		if host == "" {
			http.Error(w, "host is required", http.StatusBadRequest)
			return
		}
		chain, err := utils.FetchEndpointCertificateChain(r.Context(), "https://"+host, host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(chain)
	})

	page.HandleFunc("/relay.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprintf(w, "window.PORTAL_RELAY_URL = %q;\n", relayURL)
	})
	go serve(pagePort, page)

	fmt.Printf("RELAY_SNI=127.0.0.1:%d\n", sniPort)
	fmt.Printf("RELAY_CERT=%s\n", certPath)
	fmt.Printf("FRONT=%s\n", frontURL)
	fmt.Printf("PAGE=http://127.0.0.1:%d\n", pagePort)

	<-ctx.Done()
	shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	_ = relay.Shutdown(shutdownCtx)
	_ = relay.Wait()
}

func serve(port int, handler http.Handler) {
	if err := http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port), handler); err != nil {
		log.Printf("listen %d: %v", port, err)
	}
}

func goroot() string {
	if dir := os.Getenv("GOROOT"); dir != "" {
		return dir
	}
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return runtime.GOROOT()
	}
	return strings.TrimSpace(string(out))
}

func webDir() string {
	if dir := os.Getenv("WEB_DIR"); dir != "" {
		return dir
	}
	return "demo/web"
}

func envInt(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return fallback
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%s did not appear", path)
}
