package rpc

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/oasis-web3-gateway/tests"
)

func TestMain(m *testing.M) {
	if err := Setup(); err != nil {
		log.Fatalf("%v", err)
	}

	// Run tests.
	code := m.Run()

	Stop()

	os.Exit(code)
}

func TestNet_Version(t *testing.T) {
	rpcRes := call(t, "net_version", []string{})
	var res string
	err := json.Unmarshal(rpcRes.Result, &res)

	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%v", tests.TestsConfig.Gateway.ChainID), res)
}
