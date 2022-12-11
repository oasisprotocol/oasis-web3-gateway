// Package transfer implements the EVM benchmarks.
package transfer

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	consAccClient "github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/consensusaccounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-web3-gateway/benchmarks/api"
	"github.com/oasisprotocol/oasis-web3-gateway/benchmarks/util/keys"
)

type benchEVMTransfers struct{}

type benchState struct {
	account    *keys.ConsensusBenchmarkKey
	evmAccount *keys.EVMBenchmarkKey
	to         common.Address
	chainID    *big.Int
}

func (bench *benchEVMTransfers) Name() string {
	return "evm_transfer"
}

func (bench *benchEVMTransfers) Prepare(ctx context.Context, state *api.State) error {
	state.State = &benchState{
		chainID: big.NewInt(int64(state.Config.ChainID)),
	}
	return nil
}

func (bench *benchEVMTransfers) BulkPrepare(ctx context.Context, states []*api.State) error {
	// Create accounts and ensure all are funded.
	consKeys := keys.ConsensusBenchmarkKeys(len(states))
	evmKeys := keys.EVMBenchmarkKeys(len(states))

	// Get chain context.
	chainInfo, err := states[0].Client.GetInfo(ctx)
	if err != nil {
		return err
	}
	consAcc := consAccClient.NewV1(states[0].Client)
	accs := accounts.NewV1(states[0].Client)

	// Wait that all accounts minted some funds.
	var wg sync.WaitGroup
	errCh := make(chan error, len(states))

	for i := 0; i < len(states); i++ {
		wg.Add(1)

		if s, ok := states[i].State.(*benchState); ok {
			s.account = consKeys[i]
			s.evmAccount = evmKeys[i]
			s.to = common.HexToAddress("0x71C7656EC7ab88b098defB751B7401B5f6d8976F")
		}
		go func(state *api.State) {
			defer wg.Done()

			benchState := state.State.(*benchState)

			// Deposit.
			txb := consAcc.Deposit(&benchState.evmAccount.Address, types.NewBaseUnits(*quantity.NewFromUint64(1_000_000_000_000_000_000), types.NativeDenomination)).SetFeeConsensusMessages(1)
			tx := *txb.GetTransaction()
			// Get current nonce for the signer's account.
			nonce, err := accs.Nonce(ctx, client.RoundLatest, benchState.account.Address)
			if err != nil {
				errCh <- err
				return
			}
			tx.AppendAuthSignature(benchState.account.SigSpec, nonce)
			tx.AuthInfo.Fee.Gas = 100_000
			stx := tx.PrepareForSigning()
			stx.AppendSign(chainInfo.ChainContext, benchState.account.Signer)
			// Submit the signed transaction.
			if _, err := state.Client.SubmitTx(ctx, stx.UnverifiedTransaction()); err != nil {
				errCh <- err
				return
			}

			time.Sleep(10 * time.Second)

			// Ensure consensus accounts has funds.
			bal, err := accs.Balances(ctx, client.RoundLatest, benchState.evmAccount.Address)
			if err != nil {
				errCh <- err
				return
			}
			b := bal.Balances[types.NativeDenomination]
			if b.Cmp(quantity.NewFromUint64(100)) <= 0 {
				errCh <- fmt.Errorf("not enough balance: %d on bench account: %s", b.ToBigInt().Int64(), benchState.account.Address)
				return
			}
			state.Logger.Debug("balance ok", "balance", b, "nonce", nonce, "address", benchState.account.Address, "eth_address", benchState.evmAccount.EthAddress.Hex())
		}(states[i])
	}

	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (bench *benchEVMTransfers) Scenario(ctx context.Context, state *api.State) (uint64, error) {
	benchState := (state.State).(*benchState)

	// Query initial nonces.
	nonce, err := state.ETHClient.NonceAt(ctx, benchState.evmAccount.EthAddress, nil)
	if err != nil {
		return 0, err
	}

	tx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    nonce,
		Value:    big.NewInt(10),
		Gas:      500_000,
		GasPrice: big.NewInt(100_000_000_000),
		To:       &benchState.to,
	})
	signer := ethtypes.LatestSignerForChainID(benchState.chainID)
	signature, err := crypto.Sign(signer.Hash(tx).Bytes(), benchState.evmAccount.EthPrivate)
	if err != nil {
		state.Logger.Error("failed to sign eth transaction", "err", err)
		return 0, err
	}
	signedTx, err := tx.WithSignature(signer, signature)
	if err != nil {
		state.Logger.Error("failed to add signature to transaction", "err", err)
		return 0, err
	}
	if err = state.ETHClient.SendTransaction(ctx, signedTx); err != nil {
		state.Logger.Error("failed to send transaction", "err", err)
		return 0, err
	}

	receipt, err := waitTransaction(ctx, state.ETHClient, signedTx.Hash())
	if err != nil {
		return 0, err
	}
	if receipt.Status == ethtypes.ReceiptStatusFailed {
		return 0, fmt.Errorf("transaction failed: %s", receipt.TxHash)
	}

	state.Logger.Debug("transaction sent", "nonce", nonce, "address", benchState.evmAccount.EthAddress)

	return 1, nil
}

func waitTransaction(ctx context.Context, ec *ethclient.Client, txhash common.Hash) (*ethtypes.Receipt, error) {
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

// Init initializes and registers the benchmark suites.
func Init(cmd *cobra.Command) {
	api.RegisterBenchmark(&benchEVMTransfers{})
}
