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

	"github.com/oasisprotocol/oasis-web3-gateway/tests"
)

const ethTimeout = 15 * time.Second

// We store the compiled EVM bytecode for the SimpleSolEVMTest in a separate
// file (in hex) to preserve readability of this file.
//
//go:embed contracts/evm_sol_test_compiled.hex
var evmSolTestCompiledHex string

// We store the compiled EVM bytecode for the SimpleERC20EVMTest in a separate
// file (in hex) to preserve readability of this file.
//
//go:embed contracts/evm_erc20_test_compiled.hex
var evmERC20TestCompiledHex string

/*
pragma solidity ^0.8.3;

contract Events {
    event Log(address indexed sender, string message);
    event AnotherLog();

    constructor() {
        emit Log(msg.sender, "Hello World!");
        emit Log(msg.sender, "Hello EVM!");
        emit AnotherLog();
    }
}
*/
//go:embed contracts/evm_events_test_compiled.hex
var evmEventsTestCompiledHex string

func waitTransaction(ctx context.Context, ec *ethclient.Client, txhash common.Hash) (*types.Receipt, error) {
	queryTicker := time.NewTicker(time.Second)
	defer queryTicker.Stop()

	for {
		receipt, err := ec.TransactionReceipt(ctx, txhash)
		if receipt != nil {
			return receipt, nil
		}
		if err != ethereum.NotFound {
			return nil, err
		}
		// Wait for the next round.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-queryTicker.C:
		}
	}
}

func testContractCreation(t *testing.T, value *big.Int) uint64 {
	ec := localClient(t, false)

	t.Logf("compiled contract: %s", evmSolTestCompiledHex)
	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.Nil(t, err, "get nonce failed")

	// Create transaction
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		Value:    value,
		Gas:      GasLimit,
		GasPrice: GasPrice,
		Data:     code,
	})
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), tests.TestKey1.Private)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	balanceBefore, err := ec.BalanceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.NoError(t, err)

	err = ec.SendTransaction(context.Background(), signedTx)
	require.Nil(t, err, "send transaction failed")

	ctx, cancel := context.WithTimeout(context.Background(), ethTimeout)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	require.NoError(t, err)

	t.Logf("Contract address: %s", receipt.ContractAddress)

	balanceAfter, err := ec.BalanceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.NoError(t, err)

	t.Logf("Spent for gas: %s", new(big.Int).Sub(balanceBefore, balanceAfter))

	// Always require that some gas was spent.
	require.Positive(t, balanceBefore.Cmp(balanceAfter))

	// If the contract deployed successfully, check that unspent gas was returned.
	if receipt.Status == 1 {
		minBalanceAfter := new(big.Int).Sub(balanceBefore, new(big.Int).Mul(big.NewInt(int64(GasLimit)), GasPrice))
		require.Positive(t, balanceAfter.Cmp(minBalanceAfter))
	}

	return receipt.Status
}

func TestContractCreation(t *testing.T) {
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("contract tests w/ c10lity require compat lib to be integrated")
		return
	}
	status := testContractCreation(t, big.NewInt(0))
	require.Equal(t, uint64(1), status)
}

func TestContractFailCreation(t *testing.T) {
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("contract tests w/ c10lity require compat lib to be integrated")
		return
	}
	status := testContractCreation(t, big.NewInt(1))
	require.Equal(t, uint64(0), status)
}

func TestEth_EstimateGas(t *testing.T) {
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("contract tests w/ c10lity require compat lib to be integrated")
		return
	}
	ec := localClient(t, false)
	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.Nil(t, err, "get nonce failed")

	// Build call args for estimate gas
	msg := ethereum.CallMsg{
		From:  tests.TestKey1.EthAddress,
		Value: big.NewInt(0),
		Data:  code,
	}
	gas, err := ec.EstimateGas(context.Background(), msg)
	require.Nil(t, err, "gas estimation")
	require.NotZero(t, gas)
	t.Logf("estimate gas: %v", gas)

	// Create transaction
	tx := types.NewContractCreation(nonce, big.NewInt(0), gas, GasPrice, code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), tests.TestKey1.Private)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	err = ec.SendTransaction(context.Background(), signedTx)
	require.Nil(t, err, "send transaction failed")

	ctx, cancel := context.WithTimeout(context.Background(), ethTimeout)
	defer cancel()

	_, err = waitTransaction(ctx, ec, signedTx.Hash())
	require.NoError(t, err)

	// EstimateGas should fail for a failing transaction.
	msg = ethereum.CallMsg{
		From:  common.HexToAddress("0x0000000000000000000000000000000000000000"),
		Value: big.NewInt(100000),
		Data:  []byte{0, 1, 2, 3},
	}
	_, err = ec.EstimateGas(context.Background(), msg)
	require.Error(t, err, "gas estimation should fail")
}

func TestEth_GetCode(t *testing.T) {
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("contract tests w/ c10lity require compat lib to be integrated")
		return
	}
	ec := localClient(t, false)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.Nil(t, err, "get nonce failed")
	t.Logf("got nonce: %v", nonce)

	// Create transaction
	tx := types.NewContractCreation(nonce, big.NewInt(0), GasLimit, GasPrice, code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), tests.TestKey1.Private)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	err = ec.SendTransaction(context.Background(), signedTx)
	require.Nil(t, err, "send transaction failed")

	ctx, cancel := context.WithTimeout(context.Background(), ethTimeout)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	require.NoError(t, err)
	t.Logf("SignedTx hash: %s", signedTx.Hash().Hex())
	t.Logf("Contract address: %s", receipt.ContractAddress)

	require.EqualValues(t, uint64(103630), receipt.GasUsed, "expected contract creation gas used")
	require.Equal(t, uint64(1), receipt.Status)

	storedCode, err := ec.CodeAt(context.Background(), receipt.ContractAddress, nil)
	require.Nil(t, err, "get code")
	require.NotEmpty(t, storedCode)
}

func TestEth_Call(t *testing.T) {
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("contract tests w/ c10lity require compat lib to be integrated")
		return
	}
	abidata := `
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

	ec := localClient(t, false)

	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.Nil(t, err, "get nonce failed")
	t.Logf("got nonce: %v", nonce)

	// Create transaction
	tx := types.NewContractCreation(nonce, big.NewInt(0), GasLimit, GasPrice, code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), tests.TestKey1.Private)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	err = ec.SendTransaction(context.Background(), signedTx)
	require.Nil(t, err, "send transaction failed")

	ctx, cancel := context.WithTimeout(context.Background(), ethTimeout)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	require.NoError(t, err)
	t.Logf("SignedTx hash: %s", signedTx.Hash().Hex())
	t.Logf("Contract address: %s", receipt.ContractAddress)

	require.Equal(t, uint64(1), receipt.Status)

	calldata, err := testabi.Pack("name")
	require.NoError(t, err)
	t.Logf("calldata: %x", calldata)

	msg := ethereum.CallMsg{
		To:   &receipt.ContractAddress,
		Data: calldata,
	}
	out, err := ec.CallContract(context.Background(), msg, nil)
	require.NoError(t, err)
	t.Logf("contract call return: %x", out)

	ret, err := testabi.Unpack("name", out)
	require.Nil(t, err)
	require.Equal(t, "test", ret[0])
}

// TestERC20 deploy erc20 with no constructor.
//
//	  pragma solidity ^0.8.0;
//	  import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
//	  contract TestToken is ERC20 {
//		   constructor() ERC20("Test", "TST") public {
//		     _mint(msg.sender, 1000000 * (10 ** uint256(decimals())));
//		   }
//	  }
func TestERC20(t *testing.T) {
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		t.Skip("contract tests w/ c10lity require compat lib to be integrated")
		return
	}
	testabi, _ := abi.JSON(strings.NewReader(erc20abi))

	ec := localClient(t, false)

	code := common.FromHex(strings.TrimSpace(evmERC20TestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.Nil(t, err, "get nonce failed")

	// Deploy ERC20 contract
	tx := types.NewContractCreation(nonce, big.NewInt(0), GasLimit, GasPrice, code)
	signer := types.LatestSignerForChainID(chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), tests.TestKey1.Private)
	require.Nil(t, err, "sign tx")

	signedTx, err := tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")

	err = ec.SendTransaction(context.Background(), signedTx)
	require.Nil(t, err, "send transaction failed")

	ctx, cancel := context.WithTimeout(context.Background(), ethTimeout)
	defer cancel()

	receipt, err := waitTransaction(ctx, ec, signedTx.Hash())
	require.NoError(t, err)
	require.Equal(t, uint64(1), receipt.Status)
	tokenAddr := receipt.ContractAddress
	t.Logf("ERC20 address: %s", tokenAddr.Hex())

	// Make transfer token transaction
	nonce, err = ec.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.Nil(t, err, "get nonce failed")
	transferCall, err := testabi.Pack("transfer", common.Address{1}, big.NewInt(10))
	if err != nil {
		t.Error(err)
	}

	tx = types.NewTransaction(nonce, tokenAddr, big.NewInt(0), GasLimit, GasPrice, transferCall)
	signer = types.LatestSignerForChainID(chainID)
	signature, err = crypto.Sign(signer.Hash(tx).Bytes(), tests.TestKey1.Private)
	require.Nil(t, err, "sign tx")
	signedTx, err = tx.WithSignature(signer, signature)
	require.Nil(t, err, "pack tx")
	err = ec.SendTransaction(context.Background(), signedTx)
	require.Nil(t, err, "send transaction failed")

	receipt, err = waitTransaction(context.Background(), ec, signedTx.Hash())
	require.NoError(t, err)
	require.Equal(t, uint64(1), receipt.Status)
	require.NotEmpty(t, receipt.Logs, "ERC20-transfer receipt should contain the emitted log")
	require.EqualValues(t, uint64(49700), receipt.GasUsed, "ERC20-transfer expected gas use")

	// Get balance of token receiver
	balanceOfCall, err := testabi.Pack("balanceOf", common.Address{1})
	require.NoError(t, err)
	msg := ethereum.CallMsg{
		To:   &tokenAddr,
		Data: balanceOfCall,
	}
	out, err := ec.CallContract(context.Background(), msg, nil)
	require.NoError(t, err)
	t.Logf("contract call return: %x", out)

	ret, err := testabi.Unpack("balanceOf", out)
	require.Nil(t, err)
	require.Equal(t, big.NewInt(10), ret[0])
}
