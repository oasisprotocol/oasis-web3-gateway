package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/server"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage/psql"
)

// In reality these would come from command-line arguments, the environment
// or a configuration file.
const (
	// This is the default runtime ID as used in oasis-net-runner..
	runtimeIDHex = "8000000000000000000000000000000000000000000000000000000000000000"
	// This is the default client node address as set in oasis-net-runner.
	nodeDefaultAddress = "unix:/tmp/minimal-runtime-test/net-runner/network/client-0/internal.sock"
)

// The global logger.
var logger = logging.GetLogger("evm-gateway")

func main() {
	// Initialize logging.
	if err := logging.Initialize(os.Stdout, logging.FmtLogfmt, logging.LevelDebug, nil); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to initialize logging: %v\n", err)
		os.Exit(1)
	}
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
	// Create web3 gateway instance
	w3, err := server.New(&server.DefaultConfig)
	if err != nil {
		logger.Error("failed to create web3", err)
		os.Exit(1)
	}
	// Establish a gRPC connection with the client node.
	logger.Info("Connecting to evm node")

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
	}()

	svr.Wait()
}
