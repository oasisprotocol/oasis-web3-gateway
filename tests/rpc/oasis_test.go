package rpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/oasis-web3-gateway/rpc/oasis"
	"github.com/oasisprotocol/oasis-web3-gateway/tests"
)

func TestOasis_CallDataPublicKey(t *testing.T) {
	if !tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("oasis RPCs are not enabled")
		return
	}
	rpcRes := call(t, "oasis_callDataPublicKey", []string{})
	var res oasis.CallDataPublicKey
	err := json.Unmarshal(rpcRes.Result, &res)
	require.NoError(t, err)
	require.Len(t, res.PublicKey, 32)
	require.NotEmpty(t, res.Checksum)
	require.NotEmpty(t, res.Signature)
}
