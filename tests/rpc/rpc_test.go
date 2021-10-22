package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
	"math/big"

	"github.com/stretchr/testify/require"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	// dave address generated from github.com/oasisprotocol/oasis-sdk/client-sdk/go/testing
	daveEVMAddr = "0xdce075e1c39b1ae0b75d554558b6451a226ffe00"
	// Zero hex bytes used in jsonrpc
	zeroString = "0x0"
)

var (
	// The dave private key derive from the seed "oasis-runtime-sdk/test-keys: dave"
	daveKey, _  = crypto.HexToECDSA("c0e43d8755f201b715fd5a9ce0034c568442543ae0a0ee1aec2985ffe40edb99")
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
	fmt.Printf("ret: %v", rpcRes)
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

// TestEth_SendRawTransaction post eth raw transaction with ethclient from go-ethereum
func TestEth_SendRawTransaction(t *testing.T) {
	ec := localClient()

	chainID := big.NewInt(42261)
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

// func TestETH_GetBlockTransactionCountByHash(t *testing.T) {
// 	txHash := sendTestTransaction(t)

// 	receipt := waitForReceipt(t, txHash)
// 	require.NotNil(t, receipt, "transaction failed")
// 	require.Equal(t, "0x1", receipt["status"].(string))

// 	blockHash := receipt["blockHash"].(string)
// 	param := []string{blockHash}
// 	rpcRes := call(t, "eth_getBlockTransactionCountByHash", param)

// 	var res hexutil.Uint
// 	err := res.UnmarshalJSON(rpcRes.Result)
// 	require.NoError(t, err)
// 	require.Equal(t, "0x1", res.String())
// }

// func TestETH_GetBlockTransactionCountByHash_BlockHashNotFound(t *testing.T) {
// 	anyBlockHash := "0xb3b20624f8f0f86eb50dd04688409e5cea4bd02d700bf6e79e9384d47d6a5a35"
// 	param := []string{anyBlockHash}
// 	rpcRes := call(t, "eth_getBlockTransactionCountByHash", param)

// 	var result interface{}
// 	err := json.Unmarshal(rpcRes.Result, &result)
// 	require.NoError(t, err)
// 	require.Nil(t, result)
// }

// func TestETH_GetTransactionByBlockHashAndIndex(t *testing.T) {
// 	txHash := sendTestTransaction(t)

// 	receipt := waitForReceipt(t, txHash)
// 	require.NotNil(t, receipt, "transaction failed")
// 	require.Equal(t, "0x1", receipt["status"].(string))
// 	blockHash := receipt["blockHash"].(string)

// 	param := []string{blockHash, "0x0"}
// 	rpcRes := call(t, "eth_getTransactionByBlockHashAndIndex", param)

// 	tx := make(map[string]interface{})
// 	err := json.Unmarshal(rpcRes.Result, &tx)
// 	require.NoError(t, err)
// 	require.NotNil(t, tx)
// 	require.Equal(t, blockHash, tx["blockHash"].(string))
// 	require.Equal(t, "0x0", tx["transactionIndex"].(string))
// }

// func TestETH_GetTransactionByBlockHashAndIndex_BlockHashNotFound(t *testing.T) {
// 	anyBlockHash := "0xb3b20624f8f0f86eb50dd04688409e5cea4bd02d700bf6e79e9384d47d6a5a35"

// 	param := []string{anyBlockHash, "0x0"}
// 	rpcRes := call(t, "eth_getTransactionByBlockHashAndIndex", param)

// 	var result interface{}
// 	err := json.Unmarshal(rpcRes.Result, &result)
// 	require.NoError(t, err)
// 	require.Nil(t, result)
// }

// func TestEth_chainId(t *testing.T) {
// 	rpcRes := call(t, "eth_chainId", []string{})

// 	var res hexutil.Uint
// 	err := res.UnmarshalJSON(rpcRes.Result)
// 	require.NoError(t, err)
// 	require.NotEqual(t, "0x0", res.String())
// }

// func TestEth_blockNumber(t *testing.T) {
// 	rpcRes := call(t, "eth_blockNumber", []string{})

// 	var res hexutil.Uint64
// 	err := res.UnmarshalJSON(rpcRes.Result)
// 	require.NoError(t, err)

// 	t.Logf("Got block number: %s\n", res.String())
// }

// func TestEth_coinbase(t *testing.T) {
// 	zeroAddress := hexutil.Bytes(common.Address{}.Bytes())
// 	rpcRes := call(t, "eth_coinbase", []string{})

// 	var res hexutil.Bytes
// 	err := res.UnmarshalJSON(rpcRes.Result)
// 	require.NoError(t, err)

// 	t.Logf("Got coinbase block proposer: %s\n", res.String())
// 	require.NotEqual(t, zeroAddress.String(), res.String(), "expected: not %s got: %s\n", zeroAddress.String(), res.String())
// }

// func TestEth_GetCode(t *testing.T) {
// 	expectedRes := hexutil.Bytes{}
// 	rpcRes := call(t, "eth_getCode", []string{addrA, zeroString})

// 	var code hexutil.Bytes
// 	err := code.UnmarshalJSON(rpcRes.Result)

// 	require.NoError(t, err)

// 	t.Logf("Got code [%X] for %s\n", code, addrA)
// 	require.True(t, bytes.Equal(expectedRes, code), "expected: %X got: %X", expectedRes, code)
// }

// // sendTestTransaction sends a dummy transaction
// func sendTestTransaction(t *testing.T) hexutil.Bytes {
// 	param := make([]map[string]string, 1)
// 	param[0] = make(map[string]string)
// 	param[0]["from"] = "0x" + fmt.Sprintf("%x", from)
// 	param[0]["to"] = "0x1122334455667788990011223344556677889900"
// 	param[0]["value"] = "0x1"
// 	param[0]["gasPrice"] = "0x1"
// 	rpcRes := call(t, "eth_sendTransaction", param)

// 	var hash hexutil.Bytes
// 	err := json.Unmarshal(rpcRes.Result, &hash)
// 	require.NoError(t, err)
// 	return hash
// }

// func TestEth_GetTransactionReceipt(t *testing.T) {
// 	hash := sendTestTransaction(t)

// 	receipt := waitForReceipt(t, hash)

// 	require.NotNil(t, receipt, "transaction failed")
// 	require.Equal(t, "0x1", receipt["status"].(string))
// 	require.Equal(t, []interface{}{}, receipt["logs"].([]interface{}))
// }

// // deployTestContract deploys a contract that emits an event in the constructor
// func deployTestContract(t *testing.T) (hexutil.Bytes, map[string]interface{}) {
// 	param := make([]map[string]string, 1)
// 	param[0] = make(map[string]string)
// 	param[0]["from"] = "0x" + fmt.Sprintf("%x", from)
// 	param[0]["data"] = "0x6080604052348015600f57600080fd5b5060117f775a94827b8fd9b519d36cd827093c664f93347070a554f65e4a6f56cd73889860405160405180910390a2603580604b6000396000f3fe6080604052600080fdfea165627a7a723058206cab665f0f557620554bb45adf266708d2bd349b8a4314bdff205ee8440e3c240029"
// 	param[0]["gas"] = "0x200000"
// 	param[0]["gasPrice"] = "0x1"

// 	rpcRes := call(t, "eth_sendTransaction", param)

// 	var hash hexutil.Bytes
// 	err := json.Unmarshal(rpcRes.Result, &hash)
// 	require.NoError(t, err)

// 	receipt := waitForReceipt(t, hash)
// 	require.NotNil(t, receipt, "transaction failed")
// 	require.Equal(t, "0x1", receipt["status"].(string))

// 	return hash, receipt
// }

// // hash of Hello event
// var helloTopic = "0x775a94827b8fd9b519d36cd827093c664f93347070a554f65e4a6f56cd738898"

// // world parameter in Hello event
// var worldTopic = "0x0000000000000000000000000000000000000000000000000000000000000011"

// func TestEth_EstimateGas(t *testing.T) {
// 	param := make([]map[string]string, 1)
// 	param[0] = make(map[string]string)
// 	param[0]["from"] = "0x" + fmt.Sprintf("%x", from)
// 	param[0]["to"] = "0x1122334455667788990011223344556677889900"
// 	param[0]["value"] = "0x1"
// 	param[0]["gas"] = "0x5209"
// 	rpcRes := call(t, "eth_estimateGas", param)
// 	require.NotNil(t, rpcRes)

// 	var gas string
// 	err := json.Unmarshal(rpcRes.Result, &gas)
// 	require.NoError(t, err, string(rpcRes.Result))
// 	require.Equal(t, "0x5208", gas)
// }

func TestEth_GetBlockByNumber(t *testing.T) {
	param := []interface{}{"0x1", false}
	rpcRes := call(t, "eth_getBlockByNumber", param)

	block := make(map[string]interface{})
	err := json.Unmarshal(rpcRes.Result, &block)
	require.NoError(t, err)
	require.Equal(t, "0x", block["extraData"].(string))
	require.Equal(t, []interface{}{}, block["uncles"].([]interface{}))
}

// func TestEth_GetBlockByHash(t *testing.T) {
// 	param := []interface{}{"0x1", false}
// 	rpcRes := call(t, "eth_getBlockByNumber", param)

// 	block := make(map[string]interface{})
// 	err := json.Unmarshal(rpcRes.Result, &block)
// 	require.NoError(t, err)
// 	blockHash := block["hash"].(string)

// 	param = []interface{}{blockHash, false}
// 	rpcRes = call(t, "eth_getBlockByHash", param)
// 	block = make(map[string]interface{})
// 	err = json.Unmarshal(rpcRes.Result, &block)
// 	require.NoError(t, err)
// 	require.Equal(t, "0x1", block["number"].(string))
// }
