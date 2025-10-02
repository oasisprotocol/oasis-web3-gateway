package gas

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/oasis-core/go/common/pubsub"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
	"github.com/oasisprotocol/oasis-web3-gateway/source"
)

type mockNodeSource struct {
	minGasPrice quantity.Quantity
	shouldFail  bool
}

var _ source.NodeSource = (*mockNodeSource)(nil)

func (*mockNodeSource) GetBlock(_ context.Context, _ uint64) (*block.Block, error) {
	panic("unimplemented")
}

func (*mockNodeSource) GetTransactionsWithResults(_ context.Context, _ uint64) ([]*client.TransactionWithResults, error) {
	panic("unimplemented")
}

func (*mockNodeSource) GetLastRetainedBlock(_ context.Context) (*block.Block, error) {
	panic("unimplemented")
}

func (*mockNodeSource) CoreParameters(_ context.Context, _ uint64) (*core.Parameters, error) {
	panic("unimplemented")
}

func (*mockNodeSource) CoreRuntimeInfo(_ context.Context) (*core.RuntimeInfoResponse, error) {
	panic("unimplemented")
}

func (*mockNodeSource) CoreCallDataPublicKey(_ context.Context) (*core.CallDataPublicKeyResponse, error) {
	panic("unimplemented")
}

func (*mockNodeSource) CoreEstimateGasForCaller(_ context.Context, _ uint64, _ types.CallerAddress, _ *types.Transaction, _ bool) (uint64, error) {
	panic("unimplemented")
}

func (m *mockNodeSource) CoreMinGasPrice(_ context.Context, _ uint64) (map[types.Denomination]types.Quantity, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("failed")
	}
	return map[types.Denomination]types.Quantity{
		types.NativeDenomination: m.minGasPrice,
	}, nil
}

func (*mockNodeSource) AccountsNonce(_ context.Context, _ uint64, _ types.Address) (uint64, error) {
	panic("unimplemented")
}

func (*mockNodeSource) EVMCode(_ context.Context, _ uint64, _ []byte) ([]byte, error) {
	panic("unimplemented")
}

func (*mockNodeSource) EVMBalance(_ context.Context, _ uint64, _ []byte) (*types.Quantity, error) {
	panic("unimplemented")
}

func (*mockNodeSource) EVMStorage(_ context.Context, _ uint64, _ []byte, _ []byte) ([]byte, error) {
	panic("unimplemented")
}

func (*mockNodeSource) EVMSimulateCall(_ context.Context, _ uint64, _ []byte, _ uint64, _ []byte, _ []byte, _ []byte, _ []byte) ([]byte, error) {
	panic("unimplemented")
}

func (*mockNodeSource) EVMCreate(_ []byte, _ []byte) *types.Transaction {
	panic("unimplemented")
}

func (*mockNodeSource) EVMCall(_ []byte, _ []byte, _ []byte) *types.Transaction {
	panic("unimplemented")
}

func (*mockNodeSource) SubmitTxNoWait(_ context.Context, _ *types.UnverifiedTransaction) error {
	panic("unimplemented")
}

func (*mockNodeSource) Close() error {
	return nil
}

type mockBlockEmitter struct {
	notifier *pubsub.Broker
}

func (b *mockBlockEmitter) Emit(data *indexer.BlockData) {
	b.notifier.Broadcast(data)
}

// WatchBlocks implements indexer.BlockWatcher.
func (b *mockBlockEmitter) WatchBlocks(_ context.Context, buffer int64) (<-chan *indexer.BlockData, pubsub.ClosableSubscription, error) {
	typedCh := make(chan *indexer.BlockData)
	sub := b.notifier.SubscribeBuffered(buffer)
	sub.Unwrap(typedCh)

	return typedCh, sub, nil
}

func emitBlock(emitter *mockBlockEmitter, fullBlock bool, txPrice *quantity.Quantity) {
	// Wait a bit after emitting so that the block is processed.
	defer time.Sleep(100 * time.Millisecond)

	if fullBlock {
		emitter.Emit(&indexer.BlockData{Block: &model.Block{Header: &model.Header{GasLimit: 10_000, GasUsed: 10_000}}, MedianTransactionGasPrice: txPrice})
		return
	}
	emitter.Emit(&indexer.BlockData{Block: &model.Block{Header: &model.Header{GasLimit: 10_000, GasUsed: 10}}})
}

func TestGasPriceOracle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	windowSize := 5
	cfg := &conf.GasConfig{
		BlockFullThreshold: 0.8,
		WindowSize:         uint64(windowSize),
	}
	require := require.New(t)

	emitter := mockBlockEmitter{
		notifier: pubsub.NewBroker(false),
	}

	nodeSource := mockNodeSource{
		minGasPrice: *quantity.NewFromUint64(142_000_000_000), // 142 gwei.
		shouldFail:  true,
	}

	// Gas price oracle with failing node source.
	gasPriceOracle := New(cfg, &emitter, &nodeSource)
	go gasPriceOracle.Start(ctx)

	// Default gas price should be returned by the oracle.
	require.EqualValues(defaultGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return default gas price")

	fh := gasPriceOracle.FeeHistory(10, 10, []float64{0.25, 0.5})
	require.EqualValues(0, fh.OldestBlock.ToInt().Int64(), "fee history should be empty")
	require.Empty(0, fh.GasUsedRatio, "fee history should be empty")
	require.Empty(0, fh.Reward, "fee history should be empty")

	// Emit a non-full block.
	emitBlock(&emitter, false, nil)

	// Default gas price should be returned by the oracle.
	require.EqualValues(defaultGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return default gas price")

	// Emit a full block.
	emitBlock(&emitter, true, quantity.NewFromUint64(1_000_000_000_000)) // 1000 gwei.

	// 1001 gwei should be returned.
	require.EqualValues(big.NewInt(1_001_000_000_000), gasPriceOracle.GasPrice().ToInt(), "oracle should return correct gas price")

	// Emit a non-full block.
	emitBlock(&emitter, false, nil)

	// 1001 gwei should be returned.
	require.EqualValues(big.NewInt(1_001_000_000_000), gasPriceOracle.GasPrice(), "oracle should return correct gas price")

	// Emit `windowSize` non-full blocks.
	for i := 0; i < windowSize; i++ {
		emitBlock(&emitter, false, nil)
	}

	// Default gas price should be returned by the oracle.
	require.EqualValues(defaultGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return default gas price")

	// Cancel and create a new context for the next oracle.
	cancel()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Create a new gas price oracle with working nodeSource.
	nodeSource.shouldFail = false
	gasPriceOracle = New(cfg, &emitter, &nodeSource)
	go gasPriceOracle.Start(ctx)

	// Emit a non-full block.
	emitBlock(&emitter, false, nil)
	// Emit a non-full block.
	emitBlock(&emitter, false, nil)

	// Wait a bit so that a MinGasPrice query is made.
	time.Sleep(3 * time.Second)
	require.EqualValues(nodeSource.minGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return gas reported by the node query")

	// Emit a full block.
	emitBlock(&emitter, true, quantity.NewFromUint64(1_000_000_000_000)) // 1000 gwei.

	// 1001 gwei should be returned.
	require.EqualValues(big.NewInt(1_001_000_000_000), gasPriceOracle.GasPrice(), "oracle should return correct gas price")

	// Emit `windowSize` non-full blocks.
	for i := 0; i < windowSize; i++ {
		emitBlock(&emitter, false, nil)
	}

	require.EqualValues(nodeSource.minGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return gas reported by the node query")

	// Emit a full block with a transaction cheaper than the min gas price reported by the oasis node.
	emitBlock(&emitter, true, quantity.NewFromUint64(1)) // 1 wei.

	require.EqualValues(nodeSource.minGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return gas reported by the node query")
}
