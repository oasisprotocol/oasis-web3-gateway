package rpc

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"math/big"
	"strings"
	"testing"
	"time"

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
			fmt.Printf("Receipt retrieval failed\n")
		}
		// Wait for the next round.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-queryTicker.C:
		}
	}
}

func TestGetReceipt(t *testing.T) {
	HOST = "http://localhost:8545"
	ec, _ := ethclient.Dial(HOST)
	tx := common.HexToHash("0xe94f62a66e7868a5b0838539c5ca3fc27bb5e6328223d76ca65161ffd81f24fc")
	receipt, err := ec.TransactionReceipt(context.Background(), tx)
	if err != nil {
		log.Fatalln("TransactionReceipt error:", err)
	}
	data, _ := receipt.MarshalJSON()
	fmt.Println(string(data))
}

func TestContractCreation(t *testing.T) {
	HOST = "http://localhost:8545"
	ec, _ := ethclient.Dial(HOST)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID := big.NewInt(42261)
	nonce, err := ec.NonceAt(context.Background(), common.HexToAddress(daveEVMAddr), nil)
	require.Nil(t, err, "get nonce failed")
	fmt.Println("nonce:", nonce)
	// Create transaction
	tx := types.NewContractCreation(nonce, big.NewInt(0), 3000003, big.NewInt(2), code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), daveKey)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	ec.SendTransaction(context.Background(), signedTx)
	fmt.Println(signedTx.Hash().Hex())
	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()
	// waitTransaction(ctx, ec, tx.Hash())
}

func TestContractFailCreation(t *testing.T) {
	HOST = "http://localhost:8545"
	ec, _ := ethclient.Dial(HOST)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID := big.NewInt(42261)
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

	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()
	// waitTransaction(ctx, ec, tx.Hash())
}
