package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

const (
	// dave address generated from github.com/oasisprotocol/oasis-sdk/client-sdk/go/testing
	daveEVMAddr = "0xdce075e1c39b1ae0b75d554558b6451a226ffe00"
	// Zero hex bytes used in jsonrpc
	zeroString = "0x0"
)

var (
	// The dave private key derive from the seed "oasis-runtime-sdk/test-keys: dave"
	daveKey, _ = crypto.HexToECDSA("c0e43d8755f201b715fd5a9ce0034c568442543ae0a0ee1aec2985ffe40edb99")
)

func TestMain(m *testing.M) {

	HOST = "http://localhost:8545"

	// Start all tests
	code := m.Run()
	os.Exit(code)
}

func createRequest(method string, params interface{}) Request {
	return Request{
		Version: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
}

func call(t *testing.T, method string, params interface{}) *Response {
	req, err := json.Marshal(createRequest(method, params))
	require.NoError(t, err)

	time.Sleep(1 * time.Second)
	res, err := http.Post(HOST, "application/json", bytes.NewBuffer(req))
	require.NoError(t, err)

	decoder := json.NewDecoder(res.Body)
	rpcRes := new(Response)
	err = decoder.Decode(&rpcRes)
	require.NoError(t, err)

	err = res.Body.Close()
	require.NoError(t, err)
	require.Nil(t, rpcRes.Error)

	return rpcRes
}

func TestEth_GetBalance(t *testing.T) {

	rpcRes := call(t, "eth_getBalance", []string{daveEVMAddr, zeroString})

	var res hexutil.Big
	err := res.UnmarshalJSON(rpcRes.Result)
	require.NoError(t, err)

	t.Logf("Got balance %s for %s\n", res.String(), daveEVMAddr)

	if res.ToInt().Cmp(big.NewInt(0)) == 0 {
		t.Errorf("expected balance: %d, got: %s", 0, res.String())
	}
}

func getNonce(t *testing.T, from string) hexutil.Uint64 {
	param := []interface{}{from, "latest"}
	rpcRes := call(t, "eth_getTransactionCount", param)

	var nonce hexutil.Uint64
	err := json.Unmarshal(rpcRes.Result, &nonce)
	require.NoError(t, err)
	return nonce
}

func TestEth_GetTransactionCount(t *testing.T) {
	getNonce(t, daveEVMAddr)
}

func localClient() *ethclient.Client {
	HOST = "http://localhost:8545"
	c, _ := ethclient.Dial(HOST)
	return c
}

func TestEth_ChainID(t *testing.T) {
	ec := localClient()

	id, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	t.Logf("chain id: %v", id)
	require.Equal(t, big.NewInt(42261), id)
}

// TestEth_SendRawTransaction post eth raw transaction with ethclient from go-ethereum
func TestEth_SendRawTransaction(t *testing.T) {
	ec := localClient()

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), common.HexToAddress(daveEVMAddr), nil)
	require.Nil(t, err, "get nonce failed")

	// Create transaction
	tx := types.NewTransaction(nonce, common.Address{1}, big.NewInt(1), 22000, big.NewInt(2), nil)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), daveKey)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	ec.SendTransaction(context.Background(), signedTx)
}

func TestEth_GetBlockByNumberAndGetBlockByHash(t *testing.T) {
	ec := localClient()
	ctx := context.Background()

	number := big.NewInt(1)
	blk1, err := ec.BlockByNumber(ctx, number)
	require.NoError(t, err)
	_ = blk1

	param := []interface{}{number.String(), false}
	rpcRes := call(t, "eth_getBlockHash", param)
	var blk_hash interface{}
	err = json.Unmarshal(rpcRes.Result, &blk_hash)
	require.NoError(t, err)
	_ = rpcRes

	blkhash := blk_hash.(string)
	hash := common.HexToHash(blkhash)
	param = []interface{}{hash, false}
	rpcRes = call(t, "eth_getBlockByHash", param)
	blk2 := make(map[string]interface{})
	err = json.Unmarshal(rpcRes.Result, &blk2)
	require.NoError(t, err)
	require.Equal(t, "0x1", blk2["number"].(string))
}

func TestEth_BlockNumber(t *testing.T) {
	ec := localClient()
	ctx := context.Background()

	ret, err := ec.BlockNumber(ctx)
	require.NoError(t, err)
	fmt.Println("The current block number is ", ret)
}
