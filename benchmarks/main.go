// Oasis-sdk benchmarking client.
package main

import (
	"os"

	"github.com/oasisprotocol/oasis-core/go/common/logging"

	"github.com/oasisprotocol/oasis-web3-gateway/benchmarks/cmd"
)

func main() {
	cmd.Execute()
}

func init() {
	_ = logging.Initialize(os.Stdout, logging.FmtJSON, logging.LevelDebug, map[string]logging.Level{})
}
