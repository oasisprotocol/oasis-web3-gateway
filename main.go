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
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/log"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc"
	"github.com/starfishlabs/oasis-evm-web3-gateway/server"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage/psql"
)

var (
	// Path to the configuration file.
	configFile string
	// Oasis-web3-gateway root command.
	rootCmd = &cobra.Command{
		Use:   "oasis-evm-web3-gateway",
		Short: "oasis-evm-web3-gateway",
		RunE:  exec,
	}
)

func init() {
	rootCmd.Flags().StringVar(&configFile, "config", "./conf/server.yml", "path to the config.yml file")
}

func main() {
	_ = rootCmd.Execute()
}

func exec(cmd *cobra.Command, args []string) error {
	if err := runRoot(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}

func runRoot() error {
	ctx := context.Background()
	// Initialize server config
	cfg, err := conf.InitConfig(configFile)
	if err != nil {
		return err
	}
	// Initialize logging.
	if err = log.InitLogging(cfg); err != nil {
		return err
	}

	logger := logging.GetLogger("main")

	// Decode hex runtime ID into something we can use.
	var runtimeID common.Namespace
	if err = runtimeID.UnmarshalHex(cfg.RuntimeID); err != nil {
		logger.Error("malformed runtime ID", "err", err)
		return err
	}

	// Establish a gRPC connection with the client node.
	logger.Info("connecting to local node", "addr", cfg.NodeAddress)
	conn, err := cmnGrpc.Dial(cfg.NodeAddress, grpc.WithInsecure())
	if err != nil {
		logger.Error("failed to establish connection", "err", err)
		return err
	}
	defer conn.Close()

	// Create the runtime client with account module query helpers.
	rc := client.New(conn, runtimeID)

	// Initialize db
	db, err := psql.InitDB(ctx, cfg.Database)
	if err != nil {
		logger.Error("failed to initialize db", "err", err)
		return err
	}

	// Create Indexer
	f := indexer.NewPsqlBackend()
	indx, backend, err := indexer.New(ctx, f, rc, runtimeID, db, cfg.EnablePruning, cfg.PruningStep)
	if err != nil {
		logger.Error("failed to create indexer", err)
		return err
	}
	indx.Start()

	// Create web3 gateway instance
	w3, err := server.New(ctx, cfg.Gateway)
	if err != nil {
		logger.Error("failed to create web3", err)
		return err
	}
	w3.RegisterAPIs(rpc.GetRPCAPIs(ctx, rc, backend, cfg.Gateway))
	w3.RegisterHealthChecks([]server.HealthCheck{indx})

	svr := server.Server{
		Config: cfg,
		Web3:   w3,
		DB:     db,
	}

	if err = svr.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to start Web3 server: %v\n", err)
		return err
	}

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

	return nil
}
