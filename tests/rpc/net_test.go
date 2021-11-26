package rpc

import (
	"encoding/json"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if err := Setup(); err != nil {
		log.Fatalf("%v", err)
	}

	// Run tests.
	code := m.Run()

	if err := Shutdown(); err != nil {
		log.Printf("%v", err)
	}
	os.Exit(code)
}

func TestNet_Version(t *testing.T) {
	rpcRes := Call(t, "net_version", []string{})
	var res string
	err := json.Unmarshal(rpcRes.Result, &res)

	// 42262 is the default network id
	require.NoError(t, err)
	require.Equal(t, "42262", res)
}
