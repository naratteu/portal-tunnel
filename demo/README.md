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

Tried on the relays in the project's own `registry.json`, from a browser, with the
certificate chain supplied from off the relay, and an identity name randomised per run so
nothing squats a name someone wants.

Seven of the eight behave identically, across both release versions in the registry
(v2.5.0 and v2.5.1, protocol 10). The eighth, `portal.thumbgo.kr`, rate-limited the probe
with 429 and still does after a backoff - it registered fine, but the run never reached the
reverse session. That is the operator's prerogative and was not pursued further.

| step | public relays |
| --- | --- |
| register a lease | works — the control plane sends `Access-Control-Allow-Origin: *`, and the relay issues a real capability |
| keyless materials | works — the chain is public, so it can come from anywhere that can complete a handshake with the relay; `/chain` in this demo does exactly that, off the relay |
| reverse session | **rejected**: `wss://<relay>/sdk/connect failed: Unexpected response code: 403` |

So `/sdk/certificate-chain` is a convenience, not a requirement — a connector can obtain the
chain by other means, which this demo does against the real relays.

The reverse session is the only thing that genuinely needs the relay, and it needs two small
pieces, both of them necessary:

1. **Somewhere to put the capability.** `handleConnect` reads it from the
   `X-Portal-Reverse-Capability` header, and a browser's WebSocket constructor cannot set
   request headers. It can offer subprotocols, so the client offers
   `["portal.reverse.v1", "<capability>"]` and the relay selects only the marker — which also
   keeps a bearer token out of access logs and browser history, where a query string would
   have put it.

2. **A WebSocket handshake behind it.** Moving the capability alone is not enough. With only
   the first piece the relay authorises the session, hijacks, answers
   `101 Switching Protocols / Upgrade: raw` and logs `sdk reverse connected` — while the
   browser rejects the handshake with `'Upgrade' header value is not 'WebSocket': raw`. The
   relay then holds a session the client has already abandoned.

Neither piece touches the trust model: the relay still carries ciphertext it cannot read,
tenant TLS still terminates in the connector, and `--ban-mitm` still applies.
