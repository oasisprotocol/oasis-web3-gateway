// Package main implements the main entry point for the Oasis Web3 Gateway.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	_ "net/http/pprof" // nolint:gosec
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptrace/bun"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/oasisprotocol/oasis-core/go/common"
	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/oasisprotocol/oasis-web3-gateway/archive"
	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/db/migrations"
	"github.com/oasisprotocol/oasis-web3-gateway/filters"
	"github.com/oasisprotocol/oasis-web3-gateway/gas"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
	"github.com/oasisprotocol/oasis-web3-gateway/log"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc"
	"github.com/oasisprotocol/oasis-web3-gateway/server"
	"github.com/oasisprotocol/oasis-web3-gateway/source"
	srcNode "github.com/oasisprotocol/oasis-web3-gateway/source/node"
	"github.com/oasisprotocol/oasis-web3-gateway/source/pebble"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
	"github.com/oasisprotocol/oasis-web3-gateway/storage/psql"
	"github.com/oasisprotocol/oasis-web3-gateway/version"
	"github.com/oasisprotocol/oasis-web3-gateway/worker"
)

var (
	// Path to the configuration file.
	configFile string
	// Allow unsafe operations.
	allowUnsafe bool

	// Oasis-web3-gateway root command.
	rootCmd = &cobra.Command{
		Use:     "oasis-web3-gateway",
		Short:   "oasis-web3-gateway",
		Version: version.Software,
		RunE:    exec,
	}

	// Truncate DB.
	truncateCmd = &cobra.Command{
		Use:   "truncate-db",
		Short: "truncate-db",
		RunE:  truncateExec,
	}

	// Migrate DB.
	migrateCmd = &cobra.Command{
		Use:   "migrate-db",
		Short: "migrate-db",
		RunE:  migrateExec,
	}
)

func initVersions() {
	cobra.AddTemplateFunc("toolchain", func() any { return version.Toolchain })
	cobra.AddTemplateFunc("sdk", func() any { return version.GetOasisSDKVersion() })

	rootCmd.SetVersionTemplate(`Software version: {{.Version}}
Oasis SDK version: {{ sdk }}
Go toolchain version: {{ toolchain }}
`)
}

func init() {
	initVersions()

	rootCmd.Flags().StringVar(&configFile, "config", "./conf/server.yml", "path to the config.yml file")

	truncateCmd.Flags().StringVar(&configFile, "config", "./conf/server.yml", "path to the config.yml file")
	truncateCmd.Flags().BoolVar(&allowUnsafe, "unsafe", false, "allow destructive unsafe operations")
	rootCmd.AddCommand(truncateCmd)

	migrateCmd.Flags().StringVar(&configFile, "config", "./conf/server.yml", "path to the config.yml file")
	rootCmd.AddCommand(migrateCmd)
}

func main() {
	_ = rootCmd.Execute()
}

func exec(_ *cobra.Command, _ []string) error {
	if err := runRoot(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}

func truncateExec(_ *cobra.Command, _ []string) error {
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
	logger := logging.GetLogger("truncate-db")

	if !allowUnsafe {
		logger.Error("unsafe commands not allowed, ensure `--unsafe` is set")
		return fmt.Errorf("unsafe not allowed")
	}

	// Initialize db.
	db, err := psql.InitDB(ctx, cfg.Database, true, false)
	if err != nil {
		logger.Error("failed to initialize db", "err", err)
		return err
	}

	// Truncate database.
	if err := migrations.DropTables(ctx, db.DB.(*bun.DB)); err != nil {
		logger.Error("failed to truncate db", "err", err)
		return err
	}

	logger.Info("database truncated")

	return nil
}

func migrateExec(_ *cobra.Command, _ []string) error {
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
	logger := logging.GetLogger("migrate-db")

	// Initialize db.
	db, err := psql.InitDB(ctx, cfg.Database, true, false)
	if err != nil {
		logger.Error("failed to initialize db", "err", err)
		return err
	}

	// Run migrations.
	if err = db.RunMigrations(ctx); err != nil {
		return fmt.Errorf("failed to migrate DB: %w", err)
	}

	return nil
}

//nolint:gocyclo
func runRoot() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Enable pprof if configured.
	if cfg.PprofAddr != "" {
		go func() {
			if err = http.ListenAndServe(cfg.PprofAddr, nil); err != nil { // nolint:gosec
				logging.GetLogger("pprof").Error("server stopped", "err", err)
			}
		}()
	}

	var runtimeID common.Namespace
	if err = runtimeID.UnmarshalHex(cfg.RuntimeID); err != nil {
		return fmt.Errorf("invalid runtime ID: %w", err)
	}

	// Establish a gRPC connection with the client node.
	logger.Info("connecting to node", "addr", cfg.NodeAddress)
	var dialOpts []grpc.DialOption
	switch cmnGrpc.IsLocalAddress(cfg.NodeAddress) {
	case true:
		// No TLS for local connections.
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case false:
		creds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}
	conn, err := cmnGrpc.Dial(cfg.NodeAddress, dialOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to node: %w", err)
	}
	defer conn.Close() // nolint:errcheck

	// Create the runtime client with account module query helpers.
	rc := client.New(conn, runtimeID)

	// For now, "disable" write access to the DB in a kind of kludgy way
	// if the indexer is disabled.  Yes this means that no migrations
	// can be done.  Deal with it.
	dbReadOnly := cfg.IndexingDisable

	if !dbReadOnly {
		// Initialize db for migrations (higher timeouts).
		db, err := psql.InitDB(ctx, cfg.Database, true, dbReadOnly)
		if err != nil {
			return fmt.Errorf("failed to initialize migrations DB: %w", err)
		}
		// Run migrations.
		if err = db.RunMigrations(ctx); err != nil {
			return fmt.Errorf("failed to migrate DB: %w", err)
		}
		if err = db.DB.(*bun.DB).DB.Close(); err != nil {
			return fmt.Errorf("failed to close migrations DB: %w", err)
		}
	}

	// Initialize db again, now with configured timeouts.
	var storage storage.Storage
	rawStorage, err := psql.InitDB(ctx, cfg.Database, false, dbReadOnly)
	if err != nil {
		return fmt.Errorf("failed to initialize DB: %w", err)
	}
	storage = rawStorage

	// Configure storage monitoring if enabled.
	if cfg.Gateway != nil && cfg.Gateway.Monitoring.Enabled() {
		storage = psql.NewMetricsWrapper(storage)
	}

	// Initialize services.
	errCh := make(chan error, 4)
	var wg sync.WaitGroup

	// Start log tx_index fixer worker.
	if bunDB, ok := rawStorage.DB.(*bun.DB); ok {
		logTxIndexFixer := worker.NewLogTxIndexFixer(bunDB)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := logTxIndexFixer.Start(ctx); err != nil {
				errCh <- fmt.Errorf("logs index fixer: %w", err)
			}
		}()
	}

	// Create indexer backend.
	subBackend, err := filters.NewSubscribeBackend(storage)
	if err != nil {
		return err
	}

	backend := indexer.NewIndexBackend(runtimeID, storage, subBackend)
	backend = indexer.NewCachingBackend(backend, cfg.Cache)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := backend.(*indexer.CachingBackend).Start(ctx); err != nil {
			errCh <- fmt.Errorf("backend: %w", err)
		}
	}()

	// Create source (node + optional pebble cache).
	var src source.NodeSource
	src = srcNode.New(rc)
	if cfg.NodeCachePath != nil {
		src, err = pebble.New(src, *cfg.NodeCachePath)
		if err != nil {
			return fmt.Errorf("creating pebble source: %w", err)
		}
	}

	// Create Indexer.
	indexer, err := indexer.New(ctx, backend, src, runtimeID, cfg)
	if err != nil {
		logger.Error("failed to create indexer", err)
		return err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()

		// If no server is running, stop the process when the indexer finishes.
		if cfg.Gateway == nil {
			defer cancel()
		}
		if err := indexer.Start(ctx); err != nil {
			errCh <- fmt.Errorf("indexer: %w", err)
		}
	}()

	// Initialize gas price oracle.
	gasPriceOracle := gas.New(cfg.Gas, backend, src)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := gasPriceOracle.Start(ctx); err != nil {
			errCh <- fmt.Errorf("gas price oracle: %w", err)
		}
	}()

	// Start Web3 gateway if configured.
	if cfg.Gateway != nil {
		logger.Info("Starting Web3 gateway")

		// Create event system for gateway.
		es := filters.NewEventSystem(subBackend)

		// Create archive client if configured.
		var archiveClient *archive.Client
		if cfg.ArchiveURI != "" {
			if archiveClient, err = archive.New(ctx, cfg.ArchiveURI, cfg.ArchiveHeightMax); err != nil {
				return fmt.Errorf("creating archive client: %w", err)
			}
		}

		// Create web3 gateway instance.
		w3, err := server.New(ctx, cfg.Gateway)
		if err != nil {
			return fmt.Errorf("creating web3 gateway: %w", err)
		}

		// Register APIs and health checks.
		apis, checks := rpc.GetRPCAPIs(ctx, src, archiveClient, backend, gasPriceOracle, cfg.Gateway, es)
		w3.RegisterAPIs(apis)
		checks = append(checks, indexer)
		w3.RegisterHealthChecks(checks)

		// Start the gateway server.
		server := server.Server{
			Config: cfg,
			Web3:   w3,
			DB:     storage,
		}

		if err = server.Start(); err != nil {
			return fmt.Errorf("starting web3 server: %w", err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			closed := make(chan struct{})
			go func() {
				server.Wait()
				close(closed)
			}()

			select {
			case <-ctx.Done():
				_ = server.Close()
				errCh <- ctx.Err()
			case <-closed:
				errCh <- fmt.Errorf("web3 server stopped")
			}
		}()
	}

	logger.Info("started all services")

	// Ensure clean shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case err := <-errCh:
		logger.Error("service stopped", "err", err)
	case <-stop:
		logger.Info("received signal to stop")
	case <-ctx.Done():
		logger.Info("context done")
	}
	cancel()

	logger.Info("waiting for services to shutdown")

	// Wait for services to shutdown.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("services shut down")
	case <-time.After(30 * time.Second):
		logger.Error("services did not shutdown in time, forcing exit")
		os.Exit(1)
	}

	return nil
}
