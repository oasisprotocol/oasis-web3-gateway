package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/mux"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
)

// HealthCheck is the health checker interface.
type HealthCheck interface {
	// Health checks the health of the service.
	//
	// This method should return quickly and should avoid performing any blocking operations.
	Health() error
}

type Server struct {
	Config *conf.Config
	Web3   *Web3Gateway
	DB     storage.Storage
}

func (s *Server) Start() error {
	return s.Web3.Start()
}

func (s *Server) Wait() {
	s.Web3.Wait()
}

func (s *Server) Close() error {
	return s.Web3.Close()
}

// Web3Gateway is a container on which services can be registered.
type Web3Gateway struct {
	config *conf.GatewayConfig
	logger *logging.Logger

	stop          chan struct{} // Channel to wait for termination notifications
	startStopLock sync.Mutex    // Start/Stop are protected by an additional lock
	state         int           // Tracks state of node lifecycle

	lock    sync.Mutex
	rpcAPIs []rpc.API   // List of APIs currently provided by the node
	http    *httpServer //
	ws      *httpServer //

	healthChecks []HealthCheck
}

const (
	initializingState = iota
	runningState
	closedState
)

var (
	ErrServerStopped = errors.New("web3 gateway server not started")
	ErrServerRunning = errors.New("web3 gateway server already running")
)

func timeoutsFromCfg(cfg *conf.HTTPTimeouts) rpc.HTTPTimeouts {
	timeouts := rpc.DefaultHTTPTimeouts
	if cfg != nil {
		if cfg.Idle != nil {
			timeouts.IdleTimeout = *cfg.Idle
		}
		if cfg.Read != nil {
			timeouts.ReadTimeout = *cfg.Read
		}
		if cfg.Write != nil {
			timeouts.WriteTimeout = *cfg.Write
		}
	}
	return timeouts
}

// New creates a new web3 gateway.
func New(ctx context.Context, conf *conf.GatewayConfig) (*Web3Gateway, error) {
	if conf == nil {
		return nil, fmt.Errorf("missing gateway config")
	}

	server := &Web3Gateway{
		config: conf,
		logger: logging.GetLogger("gateway"),
		stop:   make(chan struct{}),
	}

	// Check HTTP/WS prefixes are valid.
	if err := validatePrefix("HTTP", conf.HTTP.PathPrefix); err != nil {
		return nil, err
	}
	if err := validatePrefix("WebSocket", conf.WS.PathPrefix); err != nil {
		return nil, err
	}

	// Configure RPC servers.
	if conf.HTTP != nil {
		server.http = newHTTPServer(ctx, server.logger.With("server", "http"), timeoutsFromCfg(conf.HTTP.Timeouts))
	}
	if conf.WS != nil {
		server.ws = newHTTPServer(ctx, server.logger.With("server", "ws"), timeoutsFromCfg(conf.WS.Timeouts))
	}

	return server, nil
}

func startPrometheusServer(address string) error {
	router := mux.NewRouter()
	router.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:         address,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	go func() { _ = server.Serve(ln) }()

	return nil
}

// Start Web3Gateway can only be started once.
func (srv *Web3Gateway) Start() error {
	srv.startStopLock.Lock()
	defer srv.startStopLock.Unlock()

	srv.lock.Lock()
	defer srv.lock.Unlock()
	switch srv.state {
	case runningState:
		return ErrServerRunning
	case closedState:
		return ErrServerStopped
	}
	srv.state = runningState
	// start RPC endpoints
	err := srv.startRPC()
	if err != nil {
		srv.stopRPC()
	}

	// Check if RPC endpoint startup failed.
	if err != nil {
		srv.doClose(nil)
		return err
	}

	if srv.config.Monitoring.Enabled() {
		address := srv.config.Monitoring.Address()
		srv.logger.Info("starting prometheus metrics server", "address", address)
		return startPrometheusServer(address)
	}

	return nil
}

// Close stops the Web3Gateway Server and releases resources acquired in
// Web3Gateway Server constructor New.
func (srv *Web3Gateway) Close() error {
	srv.startStopLock.Lock()
	defer srv.startStopLock.Unlock()

	srv.lock.Lock()
	state := srv.state
	srv.lock.Unlock()
	switch state {
	case initializingState:
		// The server was never started.
		return srv.doClose(nil)
	case runningState:
		// The server was started, release resources acquired by Start().
		return srv.doClose([]error{errors.New("error in function close")})
	case closedState:
		return ErrServerStopped
	default:
		panic(fmt.Sprintf("server is in unknown state %d", state))
	}
}

// doClose releases resources acquired by New(), collecting errors.
func (srv *Web3Gateway) doClose(errs []error) error {
	// Unblock n.Wait.
	close(srv.stop)

	// Report any errors that might have occurred.
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("%v", errs)
	}
}

// startRPC is a helper method to configure all the various RPC endpoints during server startup.
func (srv *Web3Gateway) startRPC() error {
	// Configure HTTP.
	if srv.config.HTTP != nil {
		config := httpConfig{
			Modules:            []string{"net", "web3", "eth", "txpool", "oasis"},
			CorsAllowedOrigins: srv.config.HTTP.Cors,
			Vhosts:             srv.config.HTTP.VirtualHosts,
			prefix:             srv.config.HTTP.PathPrefix,
		}
		if err := srv.http.setListenAddr(srv.config.HTTP.Host, srv.config.HTTP.Port); err != nil {
			return err
		}
		if err := srv.http.enableRPC(srv.rpcAPIs, srv.healthChecks, config); err != nil {
			return err
		}
	}

	// Configure WebSocket.
	if srv.config.WS != nil {
		config := wsConfig{
			Modules: []string{"net", "web3", "eth", "txpool", "oasis"},
			Origins: srv.config.WS.Origins,
			prefix:  srv.config.WS.PathPrefix,
		}
		if err := srv.ws.setListenAddr(srv.config.WS.Host, srv.config.WS.Port); err != nil {
			return err
		}
		if err := srv.ws.enableWS(srv.rpcAPIs, config); err != nil {
			return err
		}
	}

	if err := srv.http.start(); err != nil {
		return err
	}
	return srv.ws.start()
}

func (srv *Web3Gateway) stopRPC() {
	srv.http.stop()
	srv.ws.stop()
}

// Wait blocks until the server is closed.
func (srv *Web3Gateway) Wait() {
	<-srv.stop
}

// RegisterAPIs registers the APIs a service provides on the server.
func (srv *Web3Gateway) RegisterAPIs(apis []rpc.API) {
	srv.lock.Lock()
	defer srv.lock.Unlock()

	if srv.state != initializingState {
		panic("can't register APIs on running/stopped server")
	}
	srv.rpcAPIs = append(srv.rpcAPIs, apis...)
}

// RegisterHealthChecks registers the health checks for the /health endpoint.
func (srv *Web3Gateway) RegisterHealthChecks(checks []HealthCheck) {
	srv.lock.Lock()
	defer srv.lock.Unlock()

	if srv.state != initializingState {
		panic("can't register APIs on running/stopped server")
	}
	srv.healthChecks = append(srv.healthChecks, checks...)
}

// GetHTTPEndpoint returns the address of HTTP endpoint.
func (srv *Web3Gateway) GetHTTPEndpoint() (string, error) {
	if srv.http == nil {
		return "", fmt.Errorf("failed to obtain http endpoint")
	}
	return fmt.Sprintf("http://%s", srv.http.endpoint), nil
}

// GetWSEndpoint returns the address of Websocket endpoint.
func (srv *Web3Gateway) GetWSEndpoint() (string, error) {
	if srv.ws == nil {
		return "", fmt.Errorf("failed to obtain websocket endpoint")
	}
	return fmt.Sprintf("ws://%s", srv.ws.endpoint), nil
}
