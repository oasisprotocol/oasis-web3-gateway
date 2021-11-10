package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/stretchr/testify/assert"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
)

func createServer(t *testing.T, httpPort, wsPort int) *Web3Gateway {
	conf := &conf.GatewayConfig{
		Http: &conf.GatewayHTTPConfig{
			Host: "127.0.0.1",
			Port: httpPort,
		},
		WS: &conf.GatewayWSConfig{
			Host: "127.0.0.1",
			Port: wsPort,
		},
	}
	server, err := New(conf, logging.GetLogger("test"))
	if err != nil {
		t.Fatalf("could not create a new node: %v", err)
	}
	return server
}

func doHTTPRequest(t *testing.T, req *http.Request) *http.Response {
	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("could not issue a GET request to the given endpoint: %v", err)
	}
	return resp
}

// Tests whether a handler can be successfully mounted on the canonical HTTP server
// on the given prefix
func TestRegisterHandler_Successful(t *testing.T) {
	server := createServer(t, 7878, 7979)

	// create and mount handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("success"))
	})
	server.RegisterHandler("test", "/test", handler)

	// start node
	if err := server.Start(); err != nil {
		t.Fatalf("could not start node: %v", err)
	}

	// create HTTP request
	httpReq, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:7878/test", nil)
	if err != nil {
		t.Error("could not issue new http request ", err)
	}

	// check response
	resp := doHTTPRequest(t, httpReq)
	buf := make([]byte, 7)
	_, err = io.ReadFull(resp.Body, buf)
	if err != nil {
		t.Fatalf("could not read response: %v", err)
	}
	assert.Equal(t, "success", string(buf))
}

// wsRequest attempts to open a WebSocket connection to the given URL.
func wsRequest(t *testing.T, url, browserOrigin string) error {
	t.Helper()
	t.Logf("checking WebSocket on %s (origin %q)", url, browserOrigin)

	headers := make(http.Header)
	if browserOrigin != "" {
		headers.Set("Origin", browserOrigin)
	}
	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if conn != nil {
		conn.Close()
	}
	return err
}

// rpcRequest performs a JSON-RPC request to the given URL.
func rpcRequest(t *testing.T, url string, extraHeaders ...string) *http.Response {
	t.Helper()

	// Create the request.
	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"rpc_modules","params":[]}`))
	req, err := http.NewRequest("POST", url, body)
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
	httpBase := "http://" + server.http.listenAddr()
	wsBase := "ws://" + server.http.listenAddr()

	if server.ws.endpoint != wsBase+test.wsPrefix {
		t.Errorf("Error: node has wrong WSEndpoint %q", server.ws.endpoint)
	}

	for _, path := range test.wantHTTP {
		resp := rpcRequest(t, httpBase+path)
		if resp.StatusCode != 200 {
			t.Errorf("Error: %s: bad status code %d, want 200", path, resp.StatusCode)
		}
	}
	for _, path := range test.wantNoHTTP {
		resp := rpcRequest(t, httpBase+path)
		if resp.StatusCode != 404 {
			t.Errorf("Error: %s: bad status code %d, want 404", path, resp.StatusCode)
		}
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
		// both off
		{
			httpPrefix: "", wsPrefix: "",
			wantHTTP:   []string{"/", "/?p=1"},
			wantNoHTTP: []string{"/test", "/test?p=1"},
			wantWS:     []string{"/", "/?p=1"},
			wantNoWS:   []string{"/test", "/test?p=1"},
		},
		// only http prefix
		{
			httpPrefix: "/testprefix", wsPrefix: "",
			wantHTTP:   []string{"/testprefix", "/testprefix?p=1", "/testprefix/x", "/testprefix/x?p=1"},
			wantNoHTTP: []string{"/", "/?p=1", "/test", "/test?p=1"},
			wantWS:     []string{"/", "/?p=1"},
			wantNoWS:   []string{"/testprefix", "/testprefix?p=1", "/test", "/test?p=1"},
		},
		// only ws prefix
		{
			httpPrefix: "", wsPrefix: "/testprefix",
			wantHTTP:   []string{"/", "/?p=1"},
			wantNoHTTP: []string{"/testprefix", "/testprefix?p=1", "/test", "/test?p=1"},
			wantWS:     []string{"/testprefix", "/testprefix?p=1", "/testprefix/x", "/testprefix/x?p=1"},
			wantNoWS:   []string{"/", "/?p=1", "/test", "/test?p=1"},
		},
		// both set
		{
			httpPrefix: "/testprefix", wsPrefix: "/testprefix",
			wantHTTP:   []string{"/testprefix", "/testprefix?p=1", "/testprefix/x", "/testprefix/x?p=1"},
			wantNoHTTP: []string{"/", "/?p=1", "/test", "/test?p=1"},
			wantWS:     []string{"/testprefix", "/testprefix?p=1", "/testprefix/x", "/testprefix/x?p=1"},
			wantNoWS:   []string{"/", "/?p=1", "/test", "/test?p=1"},
		},
	}

	for _, test := range tests {
		test := test
		name := fmt.Sprintf("http=%s ws=%s", test.httpPrefix, test.wsPrefix)
		t.Run(name, func(t *testing.T) {
			cfg := &conf.GatewayConfig{
				Http: &conf.GatewayHTTPConfig{
					Host:       "127.0.0.1",
					PathPrefix: test.httpPrefix,
				},
				WS: &conf.GatewayWSConfig{
					Host:       "127.0.0.1",
					PathPrefix: test.wsPrefix,
				},
			}
			server, err := New(cfg, logging.GetLogger("test"))
			if err != nil {
				t.Fatal("can't create server:", err)
			}
			defer server.Close()
			if err := server.Start(); err != nil {
				t.Fatal("can't start server:", err)
			}
			test.check(t, server)
		})
	}
}
