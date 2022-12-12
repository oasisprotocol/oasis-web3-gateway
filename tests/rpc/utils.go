package rpc

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
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
	"github.com/uptrace/bun"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/oasisprotocol/oasis-web3-gateway/filters"
	"github.com/oasisprotocol/oasis-web3-gateway/gas"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
	"github.com/oasisprotocol/oasis-web3-gateway/log"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc"
	"github.com/oasisprotocol/oasis-web3-gateway/server"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
	"github.com/oasisprotocol/oasis-web3-gateway/storage/psql"
	"github.com/oasisprotocol/oasis-web3-gateway/tests"
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

const (
	OasisBlockTimeout = 60 * time.Second
	GasLimit          = uint64(1_000_000) // Minimum gas limit required to pass all tests (not using gas estimation).
)

var (
	db             *psql.PostDB
	GasPrice       = big.NewInt(100_000_000_000) // Minimum gas price: https://github.com/oasisprotocol/emerald-paratime/blob/5c32fcd1147c0e3baa64dd26a3a6a928a946236d/src/lib.rs#L36
	w3             *server.Web3Gateway
	indx           *indexer.Service
	gasPriceOracle gas.Backend
)

func localClient(t *testing.T, ws bool) *ethclient.Client {
	var url string
	var err error
	switch ws {
	case true:
		url, err = w3.GetWSEndpoint()
	case false:
		url, err = w3.GetHTTPEndpoint()
	}
	require.NoError(t, err, "local client url")
	c, err := ethclient.Dial(url)
	require.NoError(t, err, "local client dial")

	return c
}

// Setup spins up web3 gateway.
func Setup() error {
	tests.MustInitConfig()

	if err := log.InitLogging(tests.TestsConfig); err != nil {
		return fmt.Errorf("setup: initialize logging: %w", err)
	}

	// Establish a gRPC connection with the client node.
	conn, err := cmnGrpc.Dial(tests.TestsConfig.NodeAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
	var fundAmount quantity.Quantity
	if err = fundAmount.UnmarshalText([]byte("10_000_000_000_000_000_000_000")); err != nil {
		return fmt.Errorf("failed unmarshalling fund amount: %w", err)
	}
	if err = InitialDeposit(rc, fundAmount, tests.TestKey1.OasisAddress); err != nil {
		return fmt.Errorf("initial deposit failed: %w", err)
	}
	if err = InitialDeposit(rc, fundAmount, tests.TestKey2.OasisAddress); err != nil {
		return fmt.Errorf("initial deposit failed: %w", err)
	}
	if err = InitialDeposit(rc, fundAmount, tests.TestKey3.OasisAddress); err != nil {
		return fmt.Errorf("initial deposit failed: %w", err)
	}

	// Initialize db.
	ctx := context.Background()
	db, err = psql.InitDB(ctx, tests.TestsConfig.Database, true, false)
	if err != nil {
		return fmt.Errorf("failed to initialize DB: %w", err)
	}
	if err = db.RunMigrations(ctx); err != nil {
		return fmt.Errorf("failed to migrate DB: %w", err)
	}
	if err = db.DB.(*bun.DB).DB.Close(); err != nil {
		return fmt.Errorf("failed to close migrations DB: %w", err)
	}

	// Initialize db again, now with configured timeouts.
	var storage storage.Storage
	storage, err = psql.InitDB(ctx, tests.TestsConfig.Database, false, false)
	if err != nil {
		return err
	}
	// Monitoring if enabled.
	if tests.TestsConfig.Gateway.Monitoring.Enabled() {
		storage = psql.NewMetricsWrapper(storage)
	}

	// Create Indexer.
	subBackend, err := filters.NewSubscribeBackend(storage)
	if err != nil {
		return err
	}
	backend := indexer.NewIndexBackend(runtimeID, storage, subBackend)
	indx, backend, err = indexer.New(ctx, backend, rc, runtimeID, storage, tests.TestsConfig)
	if err != nil {
		return err
	}
	indx.Start()

	// Create event system.
	es := filters.NewEventSystem(subBackend)

	// Create Web3 Gateway.
	w3, err = server.New(ctx, tests.TestsConfig.Gateway)
	if err != nil {
		return fmt.Errorf("setup: failed creating server: %w", err)
	}

	gasPriceOracle = gas.New(ctx, backend, core.NewV1(rc))
	if err = gasPriceOracle.Start(); err != nil {
		return fmt.Errorf("setup: failed starting gas price oracle: %w", err)
	}

	w3.RegisterAPIs(rpc.GetRPCAPIs(context.Background(), rc, nil, backend, gasPriceOracle, tests.TestsConfig.Gateway, es))
	w3.RegisterHealthChecks([]server.HealthCheck{indx})

	if err = w3.Start(); err != nil {
		w3.Close()
		return fmt.Errorf("setup: failed to start server: %w", err)
	}

	return nil
}

func Stop() {
	_ = w3.Close()
	indx.Stop()
	gasPriceOracle.Stop()
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

func InitialDeposit(rc client.RuntimeClient, amount quantity.Quantity, to types.Address) error {
	if amount.IsZero() {
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

	ba := types.NewBaseUnits(amount, types.NativeDenomination)
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
	tx.AuthInfo.Fee.Gas = GasLimit
	// Estimate gas usage.
	gas, err := core.NewV1(rc).EstimateGas(ctx, client.RoundLatest, &tx, true)
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

	fmt.Printf("Successfully deposited %v tokens from %s to %s\n", amount, oasisTesting.Alice.Address, to)

	return nil
}
