package rpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNet_Version(t *testing.T) {
	rpcRes := Call(t, "net_version", []string{})
	var res string
	err := json.Unmarshal(rpcRes.Result, &res)

	// 42261 is the default network id
	require.NoError(t, err)
	require.Equal(t, "42261", res)
}