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

// In reality these would come from command-line arguments, the environment
// or a configuration file.
const (
	// This is the default client node address as set in oasis-net-runner.
	nodeDefaultAddress = "unix:/tmp/eth-runtime-test/net-runner/network/client-0/internal.sock"
	// This is the default runtime ID as used in oasis-net-runner.
	defaultRuntimeIdHex = "8000000000000000000000000000000000000000000000000000000000000000"
	// This is the default pruningstep.
	defaultPruningStep = 100000
)

var (
	// Node unix socket path
	nodeAddr string
	// Ethereum network id
	chainId uint
	// Runtime identifier.
	runtimeIDHex string
	// Switch for pruning in indexer
	enablePruning bool
	// Pruning step for indexer.
	// When the pruning function is enabled,
	// all information with a block number less than (latest- pruningStep) will be deleted.
	pruningStep uint64
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
	rootCmd.Flags().StringVar(&nodeAddr, "addr", nodeDefaultAddress, "address of node socket path")
	rootCmd.Flags().UintVar(&chainId, "chainid", 42261, "ethereum network id")
	rootCmd.Flags().StringVar(&runtimeIDHex, "runtime-id", defaultRuntimeIdHex, "runtime id in hex")
	rootCmd.Flags().BoolVar(&enablePruning, "enablePruning", false, "enable pruning feature for indexer")
	rootCmd.Flags().Uint64Var(&pruningStep, "pruningStep", defaultPruningStep, "information with blockNumber less then (latestBlockNumber-pruningStep) will be deleted")
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

	// Decode hex runtime ID into something we can use.
	var runtimeID common.Namespace
	if err := runtimeID.UnmarshalHex(runtimeIDHex); err != nil {
		logger.Error("malformed runtime ID", "err", err)
		os.Exit(1)
	}

	// Establish a gRPC connection with the client node.
	logger.Info("connecting to local node", "addr", nodeAddr)
	conn, err := cmnGrpc.Dial(nodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("failed to establish connection", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Create the runtime client with account module query helpers.
	rc := client.New(conn, runtimeID)

	// Initialize server config
	cfg, err := conf.InitConfig("./conf/server.yml")
	if err != nil {
		logger.Error("failed to initialize config", "err", err)
		os.Exit(1)
	}

	// Initialize db
	db, err := psql.InitDb(cfg)
	if err != nil {
		logger.Error("failed to initialize db", "err", err)
		os.Exit(1)
	}

	// Create Indexer
	f := indexer.NewPsqlBackend()
	indx, backend, err := indexer.New(f, rc, runtimeID, db, enablePruning, pruningStep)
	if err != nil {
		logger.Error("failed to create indexer", err)
		os.Exit(1)
	}
	indx.Start()

	// Create web3 gateway instance
	srvConfig := defaultServerConfig()
	srvConfig.ChainId = chainId
	w3, err := server.New(&srvConfig)
	if err != nil {
		logger.Error("failed to create web3", err)
		os.Exit(1)
	}
	w3.RegisterAPIs(rpc.GetRPCAPIs(context.Background(), rc, logger, backend, srvConfig))

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
