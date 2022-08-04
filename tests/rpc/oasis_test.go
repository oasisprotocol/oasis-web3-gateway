package rpc

import (
	"encoding/json"
	"testing"

	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/emerald-web3-gateway/tests"
)

func TestOasis_CallDataPublicKey(t *testing.T) {
	if !tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("oasis RPCs are not enabled")
		return
	}
	rpcRes := call(t, "oasis_callDataPublicKey", []string{})
	var res core.CallDataPublicKeyResponse
	err := json.Unmarshal(rpcRes.Result, &res)
	require.NoError(t, err)
	require.Len(t, res.PublicKey.PublicKey, 32)
	require.NotEmpty(t, res.PublicKey.Checksum)
	require.NotEmpty(t, res.PublicKey.Signature)
}
