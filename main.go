package main

import (
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/oasisprotocol/oasis-core/go/common/logging"
    "github.com/starfishlabs/oasis-evm-web3-gateway/server"
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

    w3, _ := server.New(&server.DefaultConfig)

    // Establish a gRPC connection with the client node.
    logger.Info("Connecting to evm node")

    w3.Start()

    go func() {
        sigc := make(chan os.Signal, 1)
        signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
        defer signal.Stop(sigc)

        <-sigc
        logger.Info("Got interrupt, shutting down...")
        go w3.Close()
    }()

    w3.Wait()
}
