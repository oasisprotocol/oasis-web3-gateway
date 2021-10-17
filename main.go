package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/oasisprotocol/oasis-core/go/common"
	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"google.golang.org/grpc"

	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc"
	"github.com/starfishlabs/oasis-evm-web3-gateway/server"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage/psql"
)

// In reality these would come from command-line arguments, the environment
// or a configuration file.
const (
	// This is the default runtime ID as used in oasis-net-runner..
	runtimeIDHex = "8000000000000000000000000000000000000000000000000000000000000000"
	// This is the default client node address as set in oasis-net-runner.
	nodeDefaultAddress = "unix:/tmp/eth-runtime-test/net-runner/network/client-0/internal.sock"
)

// The global logger.
var logger = logging.GetLogger("evm-gateway")

func main() {
	// Initialize logging.
	if err := logging.Initialize(os.Stdout, logging.FmtLogfmt, logging.LevelDebug, nil); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to initialize logging: %v\n", err)
		os.Exit(1)
	}

	// Decode hex runtime ID into something we can use.
	var runtimeID common.Namespace
	if err := runtimeID.UnmarshalHex(runtimeIDHex); err != nil {
		logger.Error("malformed runtime ID", "err", err)
		os.Exit(1)
	}

	// Establish a gRPC connection with the client node.
	logger.Info("connecting to local node")
	conn, err := cmnGrpc.Dial(nodeDefaultAddress, grpc.WithInsecure())
	if err != nil {
		logger.Error("failed to establish connection")
		os.Exit(1)
	}
	defer conn.Close()

	// Create the runtime client with account module query helpers.
	rc := client.New(conn, runtimeID)

	// Initialize server config
	cfg, err := conf.InitConfig("./conf/server.yml")
	if err != nil {
		logger.Error("failed to initialize config", err)
		os.Exit(1)
	}

	// Initialize db
	db, err := psql.InitDb(cfg)
	if err != nil {
		logger.Error("failed to initialize db", err)
		os.Exit(1)
	}

	// Create Indexer
	f := indexer.NewPsqlBackend()
	indx, err := indexer.New(f, rc, runtimeID, db)
	if err != nil {
		logger.Error("failed to create indexer", err)
		os.Exit(1)
	}
	indx.Start()

	// Create web3 gateway instance
	srvConfig := defaultServerConfig()
	w3, err := server.New(&srvConfig)
	if err != nil {
		logger.Error("failed to create web3", err)
		os.Exit(1)
	}
	w3.RegisterAPIs(rpc.GetRPCAPIs(context.Background(), rc, logger))

	svr := server.Server{
		Config: cfg,
		Web3:   w3,
		Db:     db,
	}

	svr.Start()

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)

		<-sigc
		logger.Info("Got interrupt, shutting down...")
		go svr.Close()
		go indx.Stop()
	}()

	svr.Wait()
}

func defaultServerConfig() server.Config {
	cfg := server.DefaultConfig
	cfg.HTTPHost = server.DefaultHTTPHost
	cfg.WSHost = server.DefaultWSHost
	cfg.HTTPModules = append(cfg.HTTPModules, "eth")
	cfg.WSModules = append(cfg.WSModules, "eth")
	return cfg
}
