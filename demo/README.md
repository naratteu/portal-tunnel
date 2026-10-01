# The connector in a browser

The portal connector compiled to WebAssembly, publishing an `http.Handler` that lives in a
browser tab. No socket anywhere: the reverse session rides a WebSocket, tenant TLS terminates
in the tab, and the handler answers in the same runtime.

```bash
go run ./demo                       # relay + the page, prints RELAY_SNI / RELAY_CERT / PAGE
# open PAGE in a browser; it prints the public URL it was given
go run ./demo/visitor 127.0.0.1:31443 <RELAY_CERT> <PUBLIC_URL>
# HTTP 200 text/plain; charset=utf-8
# body: hello world
```

## What had to change

**Client, to build and run under `GOOS=js GOARCH=wasm`** — porting work, no protocol change:

- `utils`: `SignalContext` split by platform. A browser has no process signals, and
  `syscall.SIGHUP` does not exist there.
- `utils`: the localhost trust bootstrap split by platform. It dials the relay to read its
  certificate chain off the handshake; in a browser the browser owns the handshake and the
  trust decision, so there is nothing to do and no socket to do it with.
- `utils`: the certificate chain fetched over HTTP instead of off a TLS handshake, for the
  same reason.
- `sdk`: `WithReverseDialer`, so the reverse session can be opened some other way than
  `net.Dial` plus an HTTP/1.1 `Upgrade: raw`.
- `sdk`: `WebSocketReverseDialer`, which is that other way.

**Relay, to let such a connector in** — two additions, neither touching the trust model:

- `/sdk/connect` accepts a WebSocket upgrade alongside the existing hijack. Both end in the
  same `lease.stream.OfferConn(conn)`; the relay still carries ciphertext it cannot read.
  The capability moves to the query string there, because a browser's WebSocket constructor
  cannot set request headers.
- `/sdk/certificate-chain` serves the chain the relay already presents to every visitor. The
  socket client reads it off a handshake, which a browser cannot do.

Tenant TLS still terminates in the connector, the relay still never sees the session keys,
and the `--ban-mitm` self probe still applies. The ask is a door for a connector without
sockets, not a request for the relay to decrypt anything.

## Against a public relay

Not yet. Registration already works — the control plane sends `Access-Control-Allow-Origin: *`,
so a browser gets a lease. It then stops at `/sdk/certificate-chain` (404 there) and would stop
again at `/sdk/connect`, which only hijacks. Both are the relay-side additions above.
