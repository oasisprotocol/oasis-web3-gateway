package rpc

import (
	"context"
	_ "embed"
	"math/big"
	"reflect"
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

/*
contract ExampleRequire {
    constructor () {
        require(false, "ExampleErrorRequire");
    }
}
*/
//go:embed contracts/evm_constructor_require.hex
var evmConstructorRequireHex string

/*
contract ExampleCustomError {
    error Example(string x);
    constructor () {
        revert Example("ExampleCustomError");
    }
}
*/
//go:embed contracts/evm_constructor_revert.hex
var evmConstructorRevertHex string

// extractDataFromUnexportedError extracts the "Data" field from *rpc.jsonError
// that is not exported using reflection.
func extractDataFromUnexportedError(err error) string {
	if err == nil {
		return ""
	}

	val := reflect.ValueOf(err)
	if val.Kind() == reflect.Ptr && !val.IsNil() {
		// Assuming jsonError is a struct
		errVal := val.Elem()

		// Check if the struct has a field named "Data".
		dataField := errVal.FieldByName("Data")
		if dataField.IsValid() && dataField.CanInterface() {
			// Assuming the data field is a string
			return dataField.Interface().(string)
		}
	}

	return ""
}

func skipIfNotSapphire(t *testing.T, ec *ethclient.Client) {
	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	switch chainID.Uint64() {
	case 0x5afd, 0x5afe, 0x5aff: // For sapphire chain IDs, do nothing
	default:
		t.Skip("Skipping contract constructor tests on non-Sapphire chain")
	}
}

// Verifies  that require(false,"...") in constructor works as expected.
func testContractConstructorError(t *testing.T, contractCode string, expectedError string) {
	ec := localClient(t, false)
	skipIfNotSapphire(t, ec)

	t.Logf("compiled contract: %s", contractCode)
	code := common.FromHex(strings.TrimSpace(contractCode))

	callMsg := ethereum.CallMsg{
		From: tests.TestKey1.EthAddress,
		Gas:  0,
		Data: code,
	}

	// Estimate contract creation gas requirements.
	gasCost, err := ec.EstimateGas(context.Background(), callMsg)
	require.Nil(t, err, "estimate gas failed")
	require.Greater(t, gasCost, uint64(0), "gas estimate must be positive")
	t.Logf("gas estimate for contract creation: %d", gasCost)

	// Evaluate the contract creation via `eth_call`.
	callMsg.Gas = 250000 // gasCost
	result, err := ec.CallContract(context.Background(), callMsg, nil)
	require.NotNil(t, err, "constructor expected to throw error")
	require.Equal(t, expectedError, extractDataFromUnexportedError(err))
	require.Nil(t, result)
}

func TestContractConstructorRequire(t *testing.T) {
	testContractConstructorError(t, evmConstructorRequireHex, "0x08c379a0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000134578616d706c654572726f725265717569726500000000000000000000000000")
}

func TestContractConstructorRevert(t *testing.T) {
	testContractConstructorError(t, evmConstructorRevertHex, "0x547796ff000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000124578616d706c65437573746f6d4572726f720000000000000000000000000000")
}

// Verify contract creation can be simulated and returns an address.
func TestContractCreateInCall(t *testing.T) {
	ec := localClient(t, false)
	skipIfNotSapphire(t, ec)

	t.Logf("compiled contract: %s", evmSolTestCompiledHex)
	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	gasPrice, err := ec.SuggestGasPrice(context.Background())
	require.Nil(t, err, "get gasPrice failed")

	callMsg := ethereum.CallMsg{
		From:     tests.TestKey1.EthAddress,
		Gas:      0,
		GasPrice: gasPrice,
		Data:     code,
	}

	// Estimate contract creation gas requirements.
	gasCost, err := ec.EstimateGas(context.Background(), callMsg)
	require.NoError(t, err, "estimate gas failed")
	require.Greater(t, gasCost, uint64(0), "gas estimate must be positive")
	t.Logf("gas estimate for contract creation: %d", gasCost)

	// Evaluate the contract creation via `eth_call`.
	callMsg.Gas = 250000 // gasCost
	result, err := ec.CallContract(context.Background(), callMsg, nil)
	require.Nil(t, err, "call contract")

	require.NotNil(t, result)
	require.Equal(t, len(result), 20)
}

func testContractCreation(t *testing.T, value *big.Int, txEIP int) uint64 {
	ec := localClient(t, false)

	t.Logf("compiled contract: %s", evmSolTestCompiledHex)
	code := common.FromHex(strings.TrimSpace(evmSolTestCompiledHex))

	chainID, err := ec.ChainID(context.Background())
	require.Nil(t, err, "get chainid")

	nonce, err := ec.NonceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.Nil(t, err, "get nonce failed")

	// Create transaction.
	gasPrice, err := ec.SuggestGasPrice(context.Background())
	require.Nil(t, err, "get price failed")
	t.Logf("suggested gas price: %d", gasPrice)

	// Estimate contract creation gas requirements.
	callMsg := ethereum.CallMsg{
		From:     tests.TestKey1.EthAddress,
		Gas:      0,
		GasPrice: gasPrice,
		Data:     code,
	}
	gasLimit, err := ec.EstimateGas(context.Background(), callMsg)
	require.NoError(t, err, "estimate gas failed")
	t.Logf("gas estimate for contract creation: %d", gasLimit)

	var tx *types.Transaction
	switch txEIP {
	case 155:
		tx = types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			Value:    value,
			Gas:      gasLimit,
			GasPrice: gasPrice,
			Data:     code,
		})
	case 1559:
		skipIfNotSapphire(t, ec) // No emerald release yet with fix for non-legacy txs.
		tx = types.NewTx(&types.DynamicFeeTx{
			Nonce:     nonce,
			Value:     value,
			Gas:       gasLimit,
			GasFeeCap: new(big.Int).Add(gasPrice, big.NewInt(100_000_000_000_000_000)),
			GasTipCap: big.NewInt(500_000_000),
			Data:      code,
		})
	case 2930:
		tx = types.NewTx(&types.AccessListTx{
			Nonce:    nonce,
			Value:    value,
			Gas:      gasLimit,
			GasPrice: gasPrice,
			Data:     code,
		})
	default:
		require.Fail(t, "Unsupported transaction EIP-%d", txEIP)
	}

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

	// Ensure the effective gas price is what was returned from `eth_gasPrice`.
	t.Logf("Effective gas price: %d, gas used: %d", receipt.EffectiveGasPrice, receipt.GasUsed)

	balanceAfter, err := ec.BalanceAt(context.Background(), tests.TestKey1.EthAddress, nil)
	require.NoError(t, err)

	actualSpent := new(big.Int).Sub(balanceBefore, balanceAfter)
	t.Logf("Spent for gas: %s", actualSpent)

	// Always require that some gas was spent.
	require.Positive(t, balanceBefore.Cmp(balanceAfter))

	// Ensure that the spent was exactly the effective gas price * gas used.
	require.EqualValues(t, actualSpent, new(big.Int).Mul(big.NewInt(int64(receipt.GasUsed)), receipt.EffectiveGasPrice), "`effective gas price * gas used` should match actual spent")

	// If the contract deployed successfully, check that unspent gas was returned.
	if receipt.Status == 1 {
		minBalanceAfter := new(big.Int).Sub(balanceBefore, new(big.Int).Mul(big.NewInt(int64(GasLimit)), GasPrice))
		require.Positive(t, balanceAfter.Cmp(minBalanceAfter))
	}

	return receipt.Status
}

func TestContractCreation155(t *testing.T) {
	status := testContractCreation(t, big.NewInt(0), 155)
	require.Equal(t, uint64(1), status)
}

func TestContractFailCreation155(t *testing.T) {
	status := testContractCreation(t, big.NewInt(1), 155)
	require.Equal(t, uint64(0), status)
}

func TestContractCreation1559(t *testing.T) {
	status := testContractCreation(t, big.NewInt(0), 1559)
	require.Equal(t, uint64(1), status)
}

func TestContractFailCreation1559(t *testing.T) {
	status := testContractCreation(t, big.NewInt(1), 1559)
	require.Equal(t, uint64(0), status)
}

func TestContractCreation2930(t *testing.T) {
	status := testContractCreation(t, big.NewInt(0), 2930)
	require.Equal(t, uint64(1), status)
}

func TestContractFailCreation2930(t *testing.T) {
	status := testContractCreation(t, big.NewInt(1), 2930)
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

	// NOTE: Gas used may vary depending on the nonce value was so this may need updates if other
	//       tests submit more or less transactions.
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		require.EqualValues(t, uint64(103647), receipt.GasUsed, "expected contract creation gas used")
	} else {
		require.EqualValues(t, uint64(103648), receipt.GasUsed, "expected contract creation gas used")
	}
	require.Equal(t, uint64(1), receipt.Status)

	storedCode, err := ec.CodeAt(context.Background(), receipt.ContractAddress, nil)
	require.Nil(t, err, "get code")
	require.NotEmpty(t, storedCode)
}

func TestEth_GetStorageZero(t *testing.T) {
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

	key := common.HexToHash("0x0")
	res, err := ec.StorageAt(context.Background(), receipt.ContractAddress, key, nil)
	require.NoError(t, err, "get storage at")
	require.Equal(t, len(res), 32, "storage at should return 32 bytes (even if zero)")
}

func TestEth_Call(t *testing.T) {
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
	// NOTE: Gas used may vary depending on the nonce value was so this may need updates if other
	//       tests submit more or less transactions.
	if tests.TestsConfig.Gateway.ExposeOasisRPCs {
		require.EqualValues(t, uint64(52499), receipt.GasUsed, "ERC20-transfer expected gas use")
	} else {
		require.EqualValues(t, uint64(52500), receipt.GasUsed, "ERC20-transfer expected gas use")
	}

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
