package rpc

import (
	"context"
	_ "embed"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

// We store the compiled EVM bytecode for the SimpleSolEVMTest in a separate
// file (in hex) to preserve readability of this file.
//go:embed contracts/evm_sol_test_compiled.hex
var evmSolTestCompiledHex string

// We store the compiled EVM bytecode for the SimpleERC20EVMTest in a separate
// file (in hex) to preserve readability of this file.
//go:embed contracts/evm_erc20_test_compiled.hex
var evmERC20TestCompiledHex string

func waitTransaction(ctx context.Context, ec *ethclient.Client, txhash common.Hash) (*types.Receipt, error) {
	queryTicker := time.NewTicker(time.Second)
	defer queryTicker.Stop()

	for {
		receipt, err := ec.TransactionReceipt(ctx, txhash)
		if receipt != nil {
			return receipt, nil
		}
		if err != nil {
			// fmt.Printf("Receipt retrieval failed: %s\n", err)
		}
		// Wait for the next round.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-queryTicker.C:
		}
	}
}

func TestContractCreation(t *testing.T) {
	HOST = "http://localhost:8545"
	ec, _ := ethclient.Dial(HOST)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), common.HexToAddress(daveEVMAddr), nil)
	require.Nil(t, err, "get nonce failed")

	// Create transaction
	tx := types.NewContractCreation(nonce, big.NewInt(0), 3000003, big.NewInt(2), code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), daveKey)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	ec.SendTransaction(context.Background(), signedTx)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	if err != nil {
		t.Errorf("get receipt failed: %s", err)
		return
	}
	// t.Logf("Contract address: %s", receipt.ContractAddress)

	require.Equal(t, receipt.Status, uint64(1))
}

func TestContractFailCreation(t *testing.T) {
	HOST = "http://localhost:8545"
	ec, _ := ethclient.Dial(HOST)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), common.HexToAddress(daveEVMAddr), nil)
	require.Nil(t, err, "get nonce failed")

	// Create transaction with transfer which is not allowed in solidity non payale
	tx := types.NewContractCreation(nonce, big.NewInt(1), 3000000, big.NewInt(2), code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), daveKey)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	ec.SendTransaction(context.Background(), signedTx)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	if err != nil {
		t.Errorf("get receipt failed: %s", err)
		return
	}
	// t.Logf("failed creation receipt: %#v", receipt)

	require.Equal(t, receipt.Status, uint64(0))
}

func TestEth_GetCode(t *testing.T) {
	HOST = "http://localhost:8545"
	ec, _ := ethclient.Dial(HOST)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), common.HexToAddress(daveEVMAddr), nil)
	require.Nil(t, err, "get nonce failed")

	// Create transaction
	tx := types.NewContractCreation(nonce, big.NewInt(0), 3000003, big.NewInt(2), code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), daveKey)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	ec.SendTransaction(context.Background(), signedTx)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	if err != nil {
		t.Errorf("get receipt failed: %s", err)
		return
	}
	t.Logf("Contract address: %s", receipt.ContractAddress)

	require.Equal(t, uint64(1), receipt.Status)

	storedCode, err := ec.CodeAt(context.Background(), receipt.ContractAddress, nil)
	require.Nil(t, err, "get code")
	require.NotEmpty(t, storedCode)
}

func TestEth_Call(t *testing.T) {

	var abidata = `
		[
			{
				"inputs": [],
				"stateMutability": "nonpayable",
				"type": "constructor"
			},
			{
				"inputs": [],
				"name": "name",
				"outputs": [
					{
						"internalType": "string",
						"name": "",
						"type": "string"
					}
				],
				"stateMutability": "view",
				"type": "function"
			}
		]
	`
	testabi, _ := abi.JSON(strings.NewReader(abidata))

	HOST = "http://localhost:8545"
	ec, _ := ethclient.Dial(HOST)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), common.HexToAddress(daveEVMAddr), nil)
	require.Nil(t, err, "get nonce failed")

	// Create transaction
	tx := types.NewContractCreation(nonce, big.NewInt(0), 3000000, big.NewInt(2), code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), daveKey)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	ec.SendTransaction(context.Background(), signedTx)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	if err != nil {
		t.Errorf("get receipt failed: %s", err)
		return
	}
	// t.Logf("Contract address: %s", receipt.ContractAddress)

	require.Equal(t, receipt.Status, uint64(1))

	calldata, err := testabi.Pack("name");
	if err != nil {
		t.Error(err)
	}
	t.Logf("calldata: %x", calldata)

	msg := ethereum.CallMsg{
		From:  common.HexToAddress(daveEVMAddr),
		To:    &receipt.ContractAddress,
		Gas:   3000000,
		GasPrice: big.NewInt(2),
		Value: big.NewInt(0),
		Data: calldata,
	}

	out, err := ec.CallContract(context.Background(), msg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("contract call return: %x", out)

	ret, err := testabi.Unpack("name", out)
	require.Nil(t, err)
	require.Equal(t, "test", ret[0])
}
