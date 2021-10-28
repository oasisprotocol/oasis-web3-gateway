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
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc"
	"github.com/starfishlabs/oasis-evm-web3-gateway/server"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage/psql"
)

var (
	// Path to the configuration file.
	configFile string
	// Oasis-web3-gateway root command
	rootCmd = &cobra.Command{
		Use:   "oasis-evm-web3-gateway",
		Short: "oasis-evm-web3-gateway",
		Run:   runRoot,
	}
)

// The global logger.
var logger = logging.GetLogger("evm-gateway")

func init() {
	rootCmd.Flags().StringVar(&configFile, "config", "./conf/server.yml", "path to the config.yml file")
}

func main() {
	rootCmd.Execute()
}

func runRoot(cmd *cobra.Command, args []string) {
	// Initialize logging.
	if err := logging.Initialize(os.Stdout, logging.FmtLogfmt, logging.LevelDebug, nil); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to initialize logging: %v\n", err)
		os.Exit(1)
	}

	// Initialize server config
	cfg, err := conf.InitConfig(configFile)
	if err != nil {
		logger.Error("failed to initialize config", "err", err)
		os.Exit(1)
	}

	// Decode hex runtime ID into something we can use.
	var runtimeID common.Namespace
	if err := runtimeID.UnmarshalHex(cfg.RuntimeID); err != nil {
		logger.Error("malformed runtime ID", "err", err)
		os.Exit(1)
	}

	// Establish a gRPC connection with the client node.
	logger.Info("connecting to local node", "addr", cfg.NodeAddress)
	conn, err := cmnGrpc.Dial(cfg.NodeAddress, grpc.WithInsecure())
	if err != nil {
		logger.Error("failed to establish connection", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Create the runtime client with account module query helpers.
	rc := client.New(conn, runtimeID)

	// Initialize db
	db, err := psql.InitDb(cfg.PostDb)
	if err != nil {
		logger.Error("failed to initialize db", "err", err)
		os.Exit(1)
	}

	// Create Indexer
	f := indexer.NewPsqlBackend()
	indx, backend, err := indexer.New(f, rc, runtimeID, db, cfg.EnablePruning, cfg.PruningStep)
	if err != nil {
		logger.Error("failed to create indexer", err)
		os.Exit(1)
	}
	indx.Start()

	// Create web3 gateway instance
	w3, err := server.New(cfg.Gateway)
	if err != nil {
		logger.Error("failed to create web3", err)
		os.Exit(1)
	}
	w3.RegisterAPIs(rpc.GetRPCAPIs(context.Background(), rc, logger, backend, cfg.Gateway))

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
