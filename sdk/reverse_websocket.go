package sdk

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"

	"github.com/coder/websocket"
)

// WebSocketReverseDialer opens the reverse session over a WebSocket instead of a
// TCP socket upgraded to a raw stream.
//
// Nothing above the connection notices: the relay still receives ciphertext it
// cannot read, tenant TLS still terminates in the connector, and the MITM self
// probe still works. The only thing that changes is how the bytes get there - and
// a WebSocket is the one duplex pipe a browser can open, which is what lets the
// connector run inside a WebAssembly page with no sockets at all.
//
// The capability travels in the query string because the browser's WebSocket
// constructor cannot set request headers.
func WebSocketReverseDialer(tlsConfig *tls.Config) ReverseDialer {
	return func(ctx context.Context, endpoint *url.URL, capability string) (net.Conn, error) {
		target := *endpoint
		switch target.Scheme {
		case "https":
			target.Scheme = "wss"
		case "http":
			target.Scheme = "ws"
		}
		query := target.Query()
		query.Set("capability", capability)
		target.RawQuery = query.Encode()

		socket, _, err := websocket.Dial(ctx, target.String(), reverseDialOptions(tlsConfig))
		if err != nil {
			return nil, fmt.Errorf("dial reverse websocket: %w", err)
		}

		// The session outlives this call, so it gets a context of its own.
		return websocket.NetConn(context.Background(), socket, websocket.MessageBinary), nil
	}
}
