package server

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

// Web3Gateway is a container on which services can be registered.
type Web3Gateway struct {
	config *Config
	log    log.Logger

	stop          chan struct{} // Channel to wait for termination notifications
	startStopLock sync.Mutex    // Start/Stop are protected by an additional lock
	state         int           // Tracks state of node lifecycle

	lock          sync.Mutex
	rpcAPIs       []rpc.API   // List of APIs currently provided by the node
	http          *httpServer //
	ws            *httpServer //
	inprocHandler *rpc.Server // In-process RPC request handler to process the API requests
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

// New creates a new web3 gateway.
func New(conf *Config) (*Web3Gateway, error) {
	confCopy := *conf
	conf = &confCopy

	if conf.Logger == nil {
		conf.Logger = log.New()
	}

	server := &Web3Gateway{
		config:        conf,
		inprocHandler: rpc.NewServer(),
		log:           conf.Logger,
		stop:          make(chan struct{}),
	}

	// Check HTTP/WS prefixes are valid.
	if err := validatePrefix("HTTP", conf.HTTPPathPrefix); err != nil {
		return nil, err
	}
	if err := validatePrefix("WebSocket", conf.WSPathPrefix); err != nil {
		return nil, err
	}

	// Configure RPC servers.
	server.http = newHTTPServer(server.log, conf.HTTPTimeouts)
	server.ws = newHTTPServer(server.log, rpc.DefaultHTTPTimeouts)

	return server, nil
}

// Start Web3Gateway can only be started once.
func (srv *Web3Gateway) Start() error {
	srv.startStopLock.Lock()
	defer srv.startStopLock.Unlock()

	srv.lock.Lock()
	switch srv.state {
	case runningState:
		srv.lock.Unlock()
		return ErrServerRunning
	case closedState:
		srv.lock.Unlock()
		return ErrServerStopped
	}
	srv.state = runningState
	// start RPC endpoints
	err := srv.startRPC()
	if err != nil {
		srv.stopRPC()
	}
	srv.lock.Unlock()

	// Check if RPC endpoint startup failed.
	if err != nil {
		srv.doClose(nil)
		return err
	}
	return err
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
		var errs []error
		return srv.doClose(errs)
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
	if srv.config.HTTPHost != "" {
		config := httpConfig{
			CorsAllowedOrigins: srv.config.HTTPCors,
			Vhosts:             srv.config.HTTPVirtualHosts,
			Modules:            srv.config.HTTPModules,
			prefix:             srv.config.HTTPPathPrefix,
		}
		if err := srv.http.setListenAddr(srv.config.HTTPHost, srv.config.HTTPPort); err != nil {
			return err
		}
		if err := srv.http.enableRPC(srv.rpcAPIs, config); err != nil {
			return err
		}
	}

	// Configure WebSocket.
	if srv.config.WSHost != "" {
		server := srv.wsServerForPort(srv.config.WSPort)
		config := wsConfig{
			Modules: srv.config.WSModules,
			Origins: srv.config.WSOrigins,
			prefix:  srv.config.WSPathPrefix,
		}
		if err := server.setListenAddr(srv.config.WSHost, srv.config.WSPort); err != nil {
			return err
		}
		if err := server.enableWS(srv.rpcAPIs, config); err != nil {
			return err
		}
	}

	if err := srv.http.start(); err != nil {
		return err
	}
	return srv.ws.start()
}

func (srv *Web3Gateway) wsServerForPort(port int) *httpServer {
	if srv.config.HTTPHost == "" || srv.http.port == port {
		return srv.http
	}
	return srv.ws
}

func (srv *Web3Gateway) stopRPC() {
	srv.http.stop()
	srv.ws.stop()
	srv.stopInProc()
}

// startInProc registers all RPC APIs on the inproc server.
func (srv *Web3Gateway) startInProc() error {
	for _, api := range srv.rpcAPIs {
		if err := srv.inprocHandler.RegisterName(api.Namespace, api.Service); err != nil {
			return err
		}
	}
	return nil
}

// stopInProc terminates the in-process RPC endpoint.
func (srv *Web3Gateway) stopInProc() {
	srv.inprocHandler.Stop()
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

// RegisterHandler mounts a handler on the given path on the canonical HTTP server.
//
// The name of the handler is shown in a log message when the HTTP server starts
// and should be a descriptive term for the service provided by the handler.
func (srv *Web3Gateway) RegisterHandler(name, path string, handler http.Handler) {
	srv.lock.Lock()
	defer srv.lock.Unlock()

	if srv.state != initializingState {
		panic("can't register HTTP handler on running/stopped server")
	}

	srv.http.mux.Handle(path, handler)
	srv.http.handlerNames[path] = name
}

// Attach creates an RPC client attached to an in-process API handler.
func (srv *Web3Gateway) Attach() (*rpc.Client, error) {
	return rpc.DialInProc(srv.inprocHandler), nil
}

// RPCHandler returns the in-process RPC request handler.
func (srv *Web3Gateway) RPCHandler() (*rpc.Server, error) {
	srv.lock.Lock()
	defer srv.lock.Unlock()

	if srv.state == closedState {
		return nil, ErrServerStopped
	}
	return srv.inprocHandler, nil
}

// HTTPEndpoint returns the URL of the HTTP server. Note that this URL does not
// contain the JSON-RPC path prefix set by HTTPPathPrefix.
func (srv *Web3Gateway) HTTPEndpoint() string {
	return "http://" + srv.http.listenAddr()
}

// WSEndpoint returns the current JSON-RPC over WebSocket endpoint.
func (srv *Web3Gateway) WSEndpoint() string {
	if srv.http.wsAllowed() {
		return "ws://" + srv.http.listenAddr() + srv.http.wsConfig.prefix
	}
	return "ws://" + srv.ws.listenAddr() + srv.ws.wsConfig.prefix
}
