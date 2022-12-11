package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
)

func createLocalServer(httpPort, wsPort int, httpPrefix, wsPrefix string) (*Web3Gateway, error) {
	conf := &conf.GatewayConfig{
		HTTP: &conf.GatewayHTTPConfig{
			Host:       "127.0.0.1",
			Port:       httpPort,
			PathPrefix: httpPrefix,
		},
		WS: &conf.GatewayWSConfig{
			Host:       "127.0.0.1",
			Port:       wsPort,
			PathPrefix: wsPrefix,
		},
	}
	server, err := New(context.Background(), conf)
	if err != nil {
		return nil, fmt.Errorf("could not create a new web3 gateway: %w", err)
	}
	return server, nil
}

// wsRequest attempts to open a WebSocket connection to the given URL.
func wsRequest(t *testing.T, url, browserOrigin string) error {
	t.Helper()
	t.Logf("checking WebSocket on %s (origin %q)", url, browserOrigin)

	headers := make(http.Header)
	if browserOrigin != "" {
		headers.Set("Origin", browserOrigin)
	}
	conn, resp, err := websocket.DefaultDialer.Dial(url, headers)
	if conn != nil {
		resp.Body.Close()
		conn.Close()
	}
	return err
}

// rpcRequest performs a JSON-RPC request to the given URL.
func rpcRequest(t *testing.T, url string, extraHeaders ...string) *http.Response {
	t.Helper()
	// Create the request.
	ctx := context.Background()
	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"rpc_modules","params":[]}`))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		t.Fatal("could not create http request:", err)
	}
	req.Header.Set("content-type", "application/json")

	// Apply extra headers.
	if len(extraHeaders)%2 != 0 {
		panic("odd extraHeaders length")
	}
	for i := 0; i < len(extraHeaders); i += 2 {
		key, value := extraHeaders[i], extraHeaders[i+1]
		if strings.ToLower(key) == "host" {
			req.Host = value
		} else {
			req.Header.Set(key, value)
		}
	}

	// Perform the request.
	t.Logf("checking RPC/HTTP on %s %v", url, extraHeaders)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

type rpcPrefixTest struct {
	httpPrefix, wsPrefix string
	// These lists paths on which JSON-RPC should be served / not served.
	wantHTTP   []string
	wantNoHTTP []string
	wantWS     []string
	wantNoWS   []string
}

func (test rpcPrefixTest) check(t *testing.T, server *Web3Gateway) {
	t.Helper()
	httpBase := "http://" + server.http.endpoint
	wsBase := "ws://" + server.ws.endpoint
	for _, path := range test.wantHTTP {
		resp := rpcRequest(t, httpBase+path)
		if resp.StatusCode != 200 {
			t.Errorf("Error: %s: bad status code %d, want 200", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	for _, path := range test.wantNoHTTP {
		resp := rpcRequest(t, httpBase+path)
		if resp.StatusCode != 404 {
			t.Errorf("Error: %s: bad status code %d, want 404", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	for _, path := range test.wantWS {
		err := wsRequest(t, wsBase+path, "")
		if err != nil {
			t.Errorf("Error: %s: WebSocket connection failed: %v", path, err)
		}
	}
	for _, path := range test.wantNoWS {
		err := wsRequest(t, wsBase+path, "")
		if err == nil {
			t.Errorf("Error: %s: WebSocket connection succeeded for path in wantNoWS", path)
		}
	}
}

func TestServerRPCPrefix(t *testing.T) {
	t.Parallel()

	tests := []rpcPrefixTest{
		// No path prefixes.
		{
			httpPrefix: "/", wsPrefix: "/",
			wantHTTP:   []string{"/", "/?p=1", "/test", "/test?p=1", "/test/x", "/test/x?p=1"},
			wantNoHTTP: []string{},
			wantWS:     []string{"/", "/?p=1", "/test", "/test?p=1", "/test/x", "/test/x?p=1"},
			wantNoWS:   []string{},
		},
		// HTTP path prefix.
		{
			httpPrefix: "/testprefix", wsPrefix: "/",
			wantHTTP:   []string{"/testprefix", "/testprefix?p=1", "/testprefix/test", "/testprefix/test?p=1"},
			wantNoHTTP: []string{"/", "/?p=1"},
			wantWS:     []string{"/", "/?p=1", "/test", "/test?p=1"},
			wantNoWS:   []string{},
		},
		// WS path prefix.
		{
			httpPrefix: "/", wsPrefix: "/testprefix",
			wantHTTP:   []string{"/", "/?p=1", "/test/", "/test/?p=1"},
			wantNoHTTP: []string{},
			wantWS:     []string{"/testprefix", "/testprefix?p=1", "/testprefix/test", "/testprefix/test?p=1"},
			wantNoWS:   []string{"/", "/?p=1"},
		},
		// HTTP and WS path prefix.
		{
			httpPrefix: "/testprefix", wsPrefix: "/testprefix",
			wantHTTP:   []string{"/testprefix", "/testprefix?p=1", "/testprefix/test", "/testprefix/test?p=1"},
			wantNoHTTP: []string{"/", "/?p=1"},
			wantWS:     []string{"/testprefix", "/testprefix?p=1", "/testprefix/test", "/testprefix/test?p=1"},
			wantNoWS:   []string{"/", "/?p=1"},
		},
	}

	for _, test := range tests {
		name := fmt.Sprintf("http=%s ws=%s", test.httpPrefix, test.wsPrefix)
		t.Run(name, func(t *testing.T) {
			server, err := createLocalServer(0, 0, test.httpPrefix, test.wsPrefix)
			require.NoError(t, err)
			defer server.Close()

			err = server.Start()
			require.NoError(t, err)
			test.check(t, server)
		})
	}
}
