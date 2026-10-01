# The connector in a browser

The portal connector compiled to WebAssembly, publishing an `http.Handler` that lives in a
browser tab. No socket anywhere: the reverse session rides a WebSocket, tenant TLS terminates
in the tab, and the handler answers in the same runtime.

```bash
GOOS=js GOARCH=wasm go build -o demo/web/connector.wasm ./demo/wasm
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
  The capability rides a WebSocket subprotocol there, because a browser's constructor cannot
  set request headers but can offer subprotocols - and unlike a query string, that keeps a
  bearer token out of access logs and browser history. The client offers
  `["portal.reverse.v1", "<capability>"]` and the relay selects only the marker, so the
  handshake response never echoes it back.
- `/sdk/certificate-chain` serves the chain the relay already presents to every visitor. The
  socket client reads it off a handshake, which a browser cannot do.

Tenant TLS still terminates in the connector, the relay still never sees the session keys,
and the `--ban-mitm` self probe still applies. The ask is a door for a connector without
sockets, not a request for the relay to decrypt anything.

## Against a public relay

Tried, on the stock `gosunuts.xyz` from the registry. It gets further than expected and
stops in exactly one place.

| step | stock public relay |
| --- | --- |
| register a lease | works — the control plane sends `Access-Control-Allow-Origin: *`, and the relay issued a real capability for `browser:0xd4e2…` |
| keyless materials | works — the certificate chain is public, so it can come from anywhere that can complete a handshake with the relay. `/chain` in this demo does exactly that, off the relay |
| reverse session | **rejected**: `wss://gosunuts.xyz/sdk/connect?capability=… failed: Unexpected response code: 403` |

So `/sdk/certificate-chain` is a convenience, not a requirement — a connector can obtain the
chain by other means, and this demo proves it against the real relay.

The one thing that genuinely needs the relay is the reverse session. `handleConnect` reads the
capability from the `X-Portal-Reverse-Capability` header, which a browser's WebSocket
constructor cannot set, so it is already unauthorized before transport is considered - and
there is no WebSocket branch behind it either.

That makes the whole ask a single change with one shape: accept the reverse session over a
WebSocket, with the capability in a subprotocol rather than a header. The relay still receives
ciphertext it cannot read, tenant TLS still terminates in the connector, and `--ban-mitm`
still applies.
