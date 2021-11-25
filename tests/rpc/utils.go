package rpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	cmnEth "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/oasisprotocol/oasis-core/go/common"
	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/ed25519"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	consAccClient "github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/consensusaccounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	oasisTesting "github.com/oasisprotocol/oasis-sdk/client-sdk/go/testing"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/starfishlabs/oasis-evm-web3-gateway/filters"
	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/log"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc"
	"github.com/starfishlabs/oasis-evm-web3-gateway/server"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage/psql"
	"github.com/starfishlabs/oasis-evm-web3-gateway/tests"

	"github.com/uptrace/bun"
)

type Request struct {
	Version string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Response struct {
	Error  *Error          `json:"error"`
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
}

const OasisBlockTimeout = 35 * time.Second

var (
	db *psql.PostDB
	w3 *server.Web3Gateway
)

// Setup spins up web3 gateway.
func Setup() error {
	tests.MustInitConfig()

	if err := log.InitLogging(tests.TestsConfig); err != nil {
		return fmt.Errorf("setup: initialize logging: %w", err)
	}

	// Establish a gRPC connection with the client node.
	conn, err := cmnGrpc.Dial(tests.TestsConfig.NodeAddress, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("setup: failed to establish gRPC connection with oasis-node: %w", err)
	}

	// Decode hex runtime ID into something we can use.
	var runtimeID common.Namespace
	if err = runtimeID.UnmarshalHex(tests.TestsConfig.RuntimeID); err != nil {
		return fmt.Errorf("malformed runtime ID: %w", err)
	}

	// Create the Oasis runtime client.
	rc := client.New(conn, runtimeID)
	_, err = rc.GetInfo(context.Background())
	if err != nil {
		logging.GetLogger("main").Error("It seems oasis-node is not running. To spin up oasis-node locally you can run tests/tools/spinup-oasis-stack.sh")
		return fmt.Errorf("failed connecting to oasis-node: %w", err)
	}

	// Fund test accounts.
	if err = InitialDeposit(rc, 1000000000000, tests.TestKey1.OasisAddress); err != nil {
		return fmt.Errorf("initial deposit failed: %w", err)
	}
	if err = InitialDeposit(rc, 1000000000000, tests.TestKey2.OasisAddress); err != nil {
		return fmt.Errorf("initial deposit failed: %w", err)
	}
	if err = InitialDeposit(rc, 1000000000000, tests.TestKey3.OasisAddress); err != nil {
		return fmt.Errorf("initial deposit failed: %w", err)
	}

	// Initialize db.
	ctx := context.Background()
	db, err = psql.InitDB(ctx, tests.TestsConfig.Database)
	if err != nil {
		return fmt.Errorf("failed to initialize DB: %w", err)
	}

	// Create Indexer.
	f := indexer.NewIndexBackend()
	indx, backend, subBackend, err := indexer.New(ctx, f, rc, runtimeID, db, tests.TestsConfig.EnablePruning, tests.TestsConfig.PruningStep)
	if err != nil {
		return fmt.Errorf("failed to create indexer: %w", err)
	}
	indx.Start()

	// Create event system.
	es := filters.NewEventSystem(subBackend)

	// Create Web3 Gateway.
	w3, err = server.New(ctx, tests.TestsConfig.Gateway)
	if err != nil {
		return fmt.Errorf("setup: failed creating server: %w", err)
	}

	w3.RegisterAPIs(rpc.GetRPCAPIs(context.Background(), rc, backend, tests.TestsConfig.Gateway, es))
	w3.RegisterHealthChecks([]server.HealthCheck{indx})

	if err = w3.Start(); err != nil {
		w3.Close()
		return fmt.Errorf("setup: failed to start server: %w", err)
	}
	return nil
}

func waitForDepositEvent(ch <-chan *client.BlockEvents, from types.Address, nonce uint64, to types.Address, amount types.BaseUnits) error {
	for {
		select {
		case bev := <-ch:
			for _, ev := range bev.Events {
				ae, ok := ev.(*consAccClient.Event)
				if !ok {
					continue
				}
				if ae.Deposit == nil {
					continue
				}
				if !ae.Deposit.From.Equal(from) {
					continue
				}
				if ae.Deposit.Nonce != nonce {
					continue
				}
				if !ae.Deposit.To.Equal(to) {
					continue
				}
				if ae.Deposit.Amount.Amount.Cmp(&amount.Amount) != 0 {
					continue
				}
				if ae.Deposit.Amount.Denomination != amount.Denomination {
					continue
				}
				return nil
			}

		case <-time.After(OasisBlockTimeout):
			return fmt.Errorf("timeout waiting for event")
		}
	}
}

func InitialDeposit(rc client.RuntimeClient, amount uint64, to types.Address) error {
	if amount == 0 {
		return fmt.Errorf("no deposit amount provided")
	}
	if rc == nil {
		return fmt.Errorf("no runtime client provided")
	}

	signer := oasisTesting.Alice.Signer
	extraGas := uint64(0)
	flag.Parse()

	consAcc := consAccClient.NewV1(rc)

	ctx, cancelFn := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancelFn()

	ba := types.NewBaseUnits(*quantity.NewFromUint64(amount), types.NativeDenomination)
	txb := consAcc.Deposit(&to, ba).SetFeeConsensusMessages(1)
	tx := *txb.GetTransaction()

	// Get chain context.
	chainInfo, err := rc.GetInfo(ctx)
	if err != nil {
		return err
	}

	// Get current nonce for the signer's account.
	ac := accounts.NewV1(rc)
	nonce, err := ac.Nonce(ctx, client.RoundLatest, types.NewAddress(types.NewSignatureAddressSpecEd25519(signer.Public().(ed25519.PublicKey))))
	if err != nil {
		return err
	}
	tx.AppendAuthSignature(types.NewSignatureAddressSpecEd25519(signer.Public().(ed25519.PublicKey)), nonce)

	// Estimate gas.
	// Set the starting gas to something high, so we don't run out.
	tx.AuthInfo.Fee.Gas = 1000000
	// Estimate gas usage.
	gas, err := core.NewV1(rc).EstimateGas(ctx, client.RoundLatest, &tx)
	if err != nil {
		return fmt.Errorf("unable to estimate gas: %w", err)
	}
	// Specify only as much gas as was estimated.
	tx.AuthInfo.Fee.Gas = gas + extraGas

	// Sign the transaction.
	stx := tx.PrepareForSigning()
	if err = stx.AppendSign(chainInfo.ChainContext, signer); err != nil {
		return err
	}

	consAccounts := consAccClient.NewV1(rc)
	acCh, err := rc.WatchEvents(context.Background(), []client.EventDecoder{consAccounts}, false)
	if err != nil {
		return err
	}

	// Submit the signed transaction.
	if _, err = rc.SubmitTx(ctx, stx.UnverifiedTransaction()); err != nil {
		return err
	}

	if err = waitForDepositEvent(acCh, oasisTesting.Alice.Address, nonce, to, ba); err != nil {
		return fmt.Errorf("ensuring alice deposit runtime event: %w", err)
	}

	fmt.Printf("Successfully deposited %d tokens from %s to %s\n", amount, oasisTesting.Alice.Address, to)

	return nil
}

// Shutdown stops web3 gateway.
func Shutdown() error {
	if err := model.TruncateModel(context.Background(), db.DB.(*bun.DB)); err != nil {
		return fmt.Errorf("db cleanup failed: %w", err)
	}

	return w3.Close()
}

func GetAddress() ([]byte, error) {
	rpcRes, err := CallWithError("eth_accounts", []string{})
	if err != nil {
		return nil, err
	}

	var res []hexutil.Bytes
	err = json.Unmarshal(rpcRes.Result, &res)
	if err != nil {
		return nil, err
	}

	return res[0], nil
}

func CreateRequest(method string, params interface{}) Request {
	return Request{
		Version: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
}

func Call(t *testing.T, method string, params interface{}) *Response {
	req, err := json.Marshal(CreateRequest(method, params))
	require.NoError(t, err)

	var rpcRes *Response
	time.Sleep(1 * time.Second)

	url, err := w3.GetHTTPEndpoint()
	require.NoError(t, err)
	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(req))
	if err != nil {
		require.NoError(t, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	res, err := client.Do(httpReq)
	if err != nil {
		require.NoError(t, fmt.Errorf("could not perform request: %w", err))
	}

	decoder := json.NewDecoder(res.Body)
	rpcRes = new(Response)
	err = decoder.Decode(&rpcRes)
	require.NoError(t, err)

	err = res.Body.Close()
	require.NoError(t, err)
	require.Nil(t, rpcRes.Error)

	return rpcRes
}

func CallWithError(method string, params interface{}) (*Response, error) {
	req, err := json.Marshal(CreateRequest(method, params))
	if err != nil {
		return nil, err
	}

	var rpcRes *Response
	time.Sleep(1 * time.Second)

	url, err := w3.GetHTTPEndpoint()
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(req))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("could not perform request: %w", err)
	}

	decoder := json.NewDecoder(res.Body)
	rpcRes = new(Response)
	err = decoder.Decode(&rpcRes)
	if err != nil {
		return nil, err
	}

	err = res.Body.Close()
	if err != nil {
		return nil, err
	}

	if rpcRes.Error != nil {
		return nil, fmt.Errorf(rpcRes.Error.Message)
	}

	return rpcRes, nil
}

// HexToBigInt turns a 0x prefixed hex string to a big.Int.
func HexToBigInt(t *testing.T, in string) *big.Int {
	s := in[2:]
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return big.NewInt(0).SetBytes(b)
}

// sendTestTransaction sends a dummy transaction.
func SendTestTransaction(t *testing.T, addr []byte) hexutil.Bytes {
	param := make([]map[string]string, 1)
	param[0] = make(map[string]string)
	param[0]["from"] = "0x" + fmt.Sprintf("%x", addr)
	param[0]["to"] = "0x1122334455667788990011223344556677889900"
	param[0]["value"] = "0x1"

	rpcRes := Call(t, "personal_unlockAccount", []interface{}{param[0]["from"], ""})
	require.Nil(t, rpcRes.Error)

	rpcRes = Call(t, "eth_sendTransaction", param)
	require.Nil(t, rpcRes.Error)

	var hash hexutil.Bytes
	err := json.Unmarshal(rpcRes.Result, &hash)
	require.NoError(t, err)
	return hash
}

// deployTestContract deploys a contract that emits an event in the constructor.
func DeployTestContract(t *testing.T, addr []byte) (hexutil.Bytes, map[string]interface{}) {
	param := make([]map[string]string, 1)
	param[0] = make(map[string]string)
	param[0]["from"] = "0x" + fmt.Sprintf("%x", addr)
	param[0]["data"] = "0x6080604052348015600f57600080fd5b5060117f775a94827b8fd9b519d36cd827093c664f93347070a554f65e4a6f56cd73889860405160405180910390a2603580604b6000396000f3fe6080604052600080fdfea165627a7a723058206cab665f0f557620554bb45adf266708d2bd349b8a4314bdff205ee8440e3c240029"
	param[0]["gas"] = "0x200000"

	rpcRes := Call(t, "personal_unlockAccount", []interface{}{param[0]["from"], ""})
	require.Nil(t, rpcRes.Error)

	rpcRes = Call(t, "eth_sendTransaction", param)
	require.Nil(t, rpcRes.Error)

	var hash hexutil.Bytes
	err := json.Unmarshal(rpcRes.Result, &hash)
	require.NoError(t, err)

	receipt := WaitForReceipt(t, hash)
	require.NotNil(t, receipt, "transaction failed")
	require.Equal(t, "0x1", receipt["status"].(string))

	return hash, receipt
}

func DeployTestContractWithFunction(t *testing.T, addr []byte) hexutil.Bytes {
	// pragma solidity ^0.5.1;

	// contract Test {
	//     event Hello(uint256 indexed world);
	//     event TestEvent(uint256 indexed a, uint256 indexed b);

	//     uint256 myStorage;

	//     constructor() public {
	//         emit Hello(17);
	//     }

	//     function test(uint256 a, uint256 b) public {
	//         myStorage = a;
	//         emit TestEvent(a, b);
	//     }
	// }

	bytecode := "0x608060405234801561001057600080fd5b5060117f775a94827b8fd9b519d36cd827093c664f93347070a554f65e4a6f56cd73889860405160405180910390a260d08061004d6000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c8063eb8ac92114602d575b600080fd5b606060048036036040811015604157600080fd5b8101908080359060200190929190803590602001909291905050506062565b005b8160008190555080827ff3ca124a697ba07e8c5e80bebcfcc48991fc16a63170e8a9206e30508960d00360405160405180910390a3505056fea265627a7a723158201d94d2187aaf3a6790527b615fcc40970febf0385fa6d72a2344848ebd0df3e964736f6c63430005110032"

	param := make([]map[string]string, 1)
	param[0] = make(map[string]string)
	param[0]["from"] = "0x" + fmt.Sprintf("%x", addr)
	param[0]["data"] = bytecode
	param[0]["gas"] = "0x200000"

	rpcRes := Call(t, "personal_unlockAccount", []interface{}{param[0]["from"], ""})
	require.Nil(t, rpcRes.Error)

	rpcRes = Call(t, "eth_sendTransaction", param)

	var hash hexutil.Bytes
	err := json.Unmarshal(rpcRes.Result, &hash)
	require.NoError(t, err)

	receipt := WaitForReceipt(t, hash)
	require.NotNil(t, receipt, "transaction failed")
	require.Equal(t, "0x1", receipt["status"].(string))

	return hash
}

func GetTransactionReceipt(t *testing.T, hash hexutil.Bytes) map[string]interface{} {
	param := []string{hash.String()}
	rpcRes := Call(t, "eth_getTransactionReceipt", param)

	receipt := make(map[string]interface{})
	err := json.Unmarshal(rpcRes.Result, &receipt)
	require.NoError(t, err)

	return receipt
}

func WaitForReceipt(t *testing.T, hash hexutil.Bytes) map[string]interface{} {
	for i := 0; i < 12; i++ {
		receipt := GetTransactionReceipt(t, hash)
		if receipt != nil {
			return receipt
		}

		time.Sleep(time.Second)
	}

	return nil
}

func GetNonce(t *testing.T, block string) hexutil.Uint64 {
	from, err := GetAddress()
	require.NoError(t, err)

	param := []interface{}{hexutil.Bytes(from), block}
	rpcRes := Call(t, "eth_getTransactionCount", param)

	var nonce hexutil.Uint64
	err = json.Unmarshal(rpcRes.Result, &nonce)
	require.NoError(t, err)
	return nonce
}

func UnlockAllAccounts(t *testing.T) {
	var accts []cmnEth.Address
	rpcRes := Call(t, "eth_accounts", []map[string]string{})
	err := json.Unmarshal(rpcRes.Result, &accts)
	require.NoError(t, err)

	for _, acct := range accts {
		t.Logf("account: %v", acct)
		rpcRes = Call(t, "personal_unlockAccount", []interface{}{acct, ""})
		var unlocked bool
		err = json.Unmarshal(rpcRes.Result, &unlocked)
		require.NoError(t, err)
		require.True(t, unlocked)
	}
}
