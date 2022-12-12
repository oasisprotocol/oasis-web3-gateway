// Package queries implements the queries EVM benchmarks.
package queries

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/oasisprotocol/oasis-web3-gateway/benchmarks/api"
	"github.com/oasisprotocol/oasis-web3-gateway/benchmarks/util/keys"
)

type benchBlockNumber struct{}

func (bench *benchBlockNumber) Name() string {
	return "evm_blockNumber"
}

func (bench *benchBlockNumber) Prepare(ctx context.Context, state *api.State) error {
	return nil
}

func (bench *benchBlockNumber) BulkPrepare(ctx context.Context, states []*api.State) error {
	return nil
}

func (bench *benchBlockNumber) Scenario(ctx context.Context, state *api.State) (uint64, error) {
	if _, err := state.ETHClient.BlockByNumber(ctx, nil); err != nil {
		state.Logger.Error("BlockByNumber query failed", "err", err)
		return 0, err
	}
	return 1, nil
}

type benchBalance struct{}

type balanceState struct {
	addr common.Address
}

func (bench *benchBalance) Name() string {
	return "evm_balance"
}

func (bench *benchBalance) Prepare(ctx context.Context, state *api.State) error {
	state.State = &balanceState{}
	return nil
}

func (bench *benchBalance) BulkPrepare(ctx context.Context, states []*api.State) error {
	evmKeys := keys.EVMBenchmarkKeys(len(states))
	for i := 0; i < len(states); i++ {
		if s, ok := states[i].State.(*balanceState); ok {
			s.addr = evmKeys[i].EthAddress
		}
	}
	return nil
}

func (bench *benchBalance) Scenario(ctx context.Context, state *api.State) (uint64, error) {
	balanceState := (state.State).(*balanceState)
	if _, err := state.ETHClient.BalanceAt(ctx, balanceState.addr, nil); err != nil {
		state.Logger.Error("BalanceAt query failed", "err", err, "addr", balanceState.addr)
		return 0, err
	}
	return 1, nil
}

// Init initializes and registers the benchmark suites.
func Init(cmd *cobra.Command) {
	api.RegisterBenchmark(&benchBlockNumber{})
	api.RegisterBenchmark(&benchBalance{})
}
