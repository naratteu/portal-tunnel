//go:build js

// The portal connector, compiled to WebAssembly and running inside a browser tab.
//
// There is no socket anywhere in here. The reverse session rides a WebSocket, tenant
// TLS terminates in this tab, and the service being published is an http.Handler in
// the same runtime - so the public URL is served by a browser.
package main

import (
	"context"
	"io"
	"net/http"
	"syscall/js"

	"github.com/gosuda/portal-tunnel/v2/portal/identity"
	"github.com/gosuda/portal-tunnel/v2/sdk"
)

func main() {
	relayURL := js.Global().Get("PORTAL_RELAY_URL").String()
	name := "browser"
	if value := js.Global().Get("PORTAL_NAME"); value.Type() == js.TypeString && value.String() != "" {
		name = value.String()
	}
	report := func(stage, detail string) {
		js.Global().Call("portalReport", stage, detail)
	}

	go func() {
		ctx := context.Background()

		id, err := identity.Generate(name)
		if err != nil {
			report("error", "identity: "+err.Error())
			return
		}
		report("status", "identity ready")

		// The browser owns the TLS handshake to the relay, so the dialer needs no
		// certificate configuration of its own.
		exposure, err := sdk.Expose(ctx, id, []string{relayURL},
			sdk.WithReverseDialer(sdk.WebSocketReverseDialer(nil)))
		if err != nil {
			report("error", "expose: "+err.Error())
			return
		}
		report("status", "registered, waiting for the tunnel")

		go func() {
			err := sdk.RunHTTP(ctx, exposure, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				report("request", r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = io.WriteString(w, "hello world")
			}), "")
			if err != nil {
				report("error", "serve: "+err.Error())
			}
		}()

		relays, err := exposure.WaitReady(ctx)
		if err != nil {
			report("error", "wait ready: "+err.Error())
			return
		}
		report("ready", relays[0].PublicURL)
	}()

	select {}
}
