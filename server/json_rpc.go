package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/mux"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/rs/cors"
)

// httpConfig is the JSON-RPC/HTTP configuration.
type httpConfig struct {
	Modules            []string
	CorsAllowedOrigins []string
	Vhosts             []string
	prefix             string // path prefix on which to mount http handler
}

// wsConfig is the JSON-RPC/Websocket configuration.
type wsConfig struct {
	Origins []string
	Modules []string
	prefix  string // path prefix on which to mount ws handler
}

// httpServer handle http connection and rpc requests.
type httpServer struct {
	ctx      context.Context
	logger   *logging.Logger
	timeouts rpc.HTTPTimeouts

	server *http.Server

	// rpcHandler holds the whole http handler
	rpcHandler http.Handler
	// rpcServer handle json rpc requests
	rpcServer *rpc.Server

	httpConfig httpConfig
	wsConfig   wsConfig

	// These are set by setListenAddr.
	endpoint string
	host     string
	port     int
}

func newHTTPServer(ctx context.Context, logger *logging.Logger, timeouts rpc.HTTPTimeouts) *httpServer {
	h := &httpServer{ctx: ctx, logger: logger, timeouts: timeouts}
	return h
}

// setListenAddr configures the listening address of the server.
// The address can only be set while the server isn't running.
func (h *httpServer) setListenAddr(host string, port int) error {
	h.host, h.port = host, port
	h.endpoint = fmt.Sprintf("%s:%d", host, port)
	return nil
}

// healthCheckHTTP is the HTTP health check endpoint handler.
func healthCheckHandler(healthChecks []HealthCheck) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		for _, h := range healthChecks {
			if err := h.Health(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

// start starts the HTTP server if it is enabled and not already running.
func (h *httpServer) start() error {
	if h.endpoint == "" {
		h.logger.Info("RPC endpoint not specified")
		return nil
	}

	// Initialize the server.
	if h.timeouts == (rpc.HTTPTimeouts{}) {
		h.timeouts = rpc.DefaultHTTPTimeouts
	}
	CheckTimeouts(&h.timeouts)
	h.server = &http.Server{
		Handler:      h.rpcHandler,
		ReadTimeout:  h.timeouts.ReadTimeout,
		WriteTimeout: h.timeouts.WriteTimeout,
		IdleTimeout:  h.timeouts.IdleTimeout,
	}

	// Start the server.
	listener, err := net.Listen("tcp", h.endpoint)
	if err != nil {
		h.logger.Error("tcp listen failed", "err", err)
		return err
	}

	//nolint:errcheck
	go h.server.Serve(listener)
	// Random port is determined by the server. Retrieve it.
	if h.port == 0 {
		h.endpoint = listener.Addr().String()
		_, portStr, err := net.SplitHostPort(h.endpoint)
		if err != nil {
			h.logger.Error("Splitting host:port of listener failed")
			return err
		}
		h.port, err = strconv.Atoi(portStr)
		if err != nil {
			h.logger.Error("Parsing of listener port failed. Is '%s' a service name instead?", portStr)
			return err
		}
	}

	h.logger.Info("HTTP server started",
		"provided_endpoint", h.endpoint,
		"endpoint", listener.Addr(),
		"prefix", h.httpConfig.prefix,
		"cors", strings.Join(h.httpConfig.CorsAllowedOrigins, ","),
	)
	return nil
}

// validatePrefix checks if 'path' is a valid configuration value for the RPC prefix option.
func validatePrefix(what, path string) error {
	if path == "" {
		return nil
	}
	if path[0] != '/' {
		return fmt.Errorf(`%s RPC path prefix %q does not contain leading "/"`, what, path)
	}
	if strings.ContainsAny(path, "?#") {
		// This is just to avoid confusion. While these would match correctly (i.e. they'd
		// match if URL-escaped into path), it's not easy to understand for users when
		// setting that on the command line.
		return fmt.Errorf("%s RPC path prefix %q contains URL meta-characters", what, path)
	}
	return nil
}

// stop shuts down the HTTP server.
func (h *httpServer) stop() {
	if err := h.server.Shutdown(h.ctx); err != nil {
		h.logger.Error("Error while shutting down HTTP server: %w", err)
	}
	h.logger.Info("HTTP server stopped", "endpoint", h.endpoint)
}

// enableRPC turns on JSON-RPC over HTTP on the server.
func (h *httpServer) enableRPC(apis []rpc.API, healthChecks []HealthCheck, config httpConfig) error {
	// Create RPC server and handler.
	srv := rpc.NewServer()
	if err := RegisterApis(apis, config.Modules, srv, false); err != nil {
		return err
	}
	h.httpConfig = config

	h.rpcServer = srv
	router := mux.NewRouter()
	router.PathPrefix(h.httpConfig.prefix).HandlerFunc(h.rpcServer.ServeHTTP).Methods("POST")
	h.rpcHandler = newCorsHandler(router, h.httpConfig.CorsAllowedOrigins)

	router.HandleFunc("/health", healthCheckHandler(healthChecks)).Methods("GET")

	return nil
}

// enableWS turns on JSON-RPC over WebSocket on the server.
func (h *httpServer) enableWS(apis []rpc.API, config wsConfig) error {
	// Create RPC server and handler.
	srv := rpc.NewServer()
	if err := RegisterApis(apis, config.Modules, srv, false); err != nil {
		return err
	}
	h.wsConfig = config

	h.rpcServer = srv
	router := mux.NewRouter()
	router.PathPrefix(h.wsConfig.prefix).HandlerFunc(h.rpcServer.WebsocketHandler(config.Origins).ServeHTTP)
	h.rpcHandler = router

	return nil
}

func newCorsHandler(srv http.Handler, allowedOrigins []string) http.Handler {
	// disable CORS support if user has not specified a custom CORS configuration
	if len(allowedOrigins) == 0 {
		return srv
	}
	c := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{http.MethodPost, http.MethodGet},
		AllowedHeaders: []string{"*"},
		MaxAge:         600,
	})
	return c.Handler(srv)
}

// RegisterApis registers all of the APIs exposed by the services.
func RegisterApis(apis []rpc.API, modules []string, srv *rpc.Server, exposeAll bool) error {
	// Generate the allow list based on the allowed modules
	allowList := make(map[string]bool)
	for _, module := range modules {
		allowList[module] = true
	}
	// Register all the APIs exposed by the services
	for _, api := range apis {
		if exposeAll || allowList[api.Namespace] || (len(allowList) == 0 && api.Public) {
			if err := srv.RegisterName(api.Namespace, api.Service); err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckTimeouts ensures that timeout values are meaningful.
func CheckTimeouts(timeouts *rpc.HTTPTimeouts) {
	if timeouts.ReadTimeout < time.Second {
		timeouts.ReadTimeout = rpc.DefaultHTTPTimeouts.ReadTimeout
	}
	if timeouts.WriteTimeout < time.Second {
		timeouts.WriteTimeout = rpc.DefaultHTTPTimeouts.WriteTimeout
	}
	if timeouts.IdleTimeout < time.Second {
		timeouts.IdleTimeout = rpc.DefaultHTTPTimeouts.IdleTimeout
	}
}
