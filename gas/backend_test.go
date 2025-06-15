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
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
)

type mockCoreClient struct {
	minGasPrice quantity.Quantity
	shouldFail  bool
}

// EstimateGas implements core.V1.
func (*mockCoreClient) EstimateGas(_ context.Context, _ uint64, _ *types.Transaction, _ bool) (uint64, error) {
	panic("unimplemented")
}

// EstimateGasForCaller implements core.V1.
func (*mockCoreClient) EstimateGasForCaller(_ context.Context, _ uint64, _ types.CallerAddress, _ *types.Transaction, _ bool) (uint64, error) {
	panic("unimplemented")
}

// GetEvents implements core.V1.
func (*mockCoreClient) GetEvents(_ context.Context, _ uint64) ([]*core.Event, error) {
	panic("unimplemented")
}

// Parameters implements core.V1.
func (*mockCoreClient) Parameters(_ context.Context, _ uint64) (*core.Parameters, error) {
	panic("unimplemented")
}

// RuntimeInfo implements core.V1.
func (*mockCoreClient) RuntimeInfo(_ context.Context) (*core.RuntimeInfoResponse, error) {
	panic("unimplemented")
}

// RuntimeInfo implements core.V1.
func (*mockCoreClient) CallDataPublicKey(_ context.Context) (*core.CallDataPublicKeyResponse, error) {
	panic("unimplemented")
}

// RuntimeInfo implements core.V1.
func (*mockCoreClient) ExecuteReadOnlyTx(_ context.Context, _ uint64, _ *types.UnverifiedTransaction) (*core.ExecuteReadOnlyTxResponse, error) {
	panic("unimplemented")
}

// MinGasPrice implements core.V1.
func (m *mockCoreClient) MinGasPrice(_ context.Context, _ uint64) (map[types.Denomination]quantity.Quantity, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("failed")
	}
	return map[types.Denomination]quantity.Quantity{
		types.NativeDenomination: m.minGasPrice,
	}, nil
}

func (m *mockCoreClient) DecodeEvent(*types.Event) ([]client.DecodedEvent, error) {
	panic("unimplemented")
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
	windowSize := 5
	cfg := &conf.GasConfig{
		BlockFullThreshold: 0.8,
		WindowSize:         uint64(windowSize),
	}
	require := require.New(t)

	emitter := mockBlockEmitter{
		notifier: pubsub.NewBroker(false),
	}

	coreClient := mockCoreClient{
		minGasPrice: *quantity.NewFromUint64(142_000_000_000), // 142 gwei.
		shouldFail:  true,
	}

	// Gas price oracle with failing core client.
	gasPriceOracle := New(context.Background(), cfg, &emitter, &coreClient)
	require.NoError(gasPriceOracle.Start())

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
	gasPriceOracle.Stop()

	// Create a new gas price oracle with working coreClient.
	coreClient.shouldFail = false
	gasPriceOracle = New(context.Background(), cfg, &emitter, &coreClient)
	require.NoError(gasPriceOracle.Start())

	// Emit a non-full block.
	emitBlock(&emitter, false, nil)
	// Emit a non-full block.
	emitBlock(&emitter, false, nil)

	// Wait a bit so that a MinGasPrice query is made.
	time.Sleep(3 * time.Second)
	require.EqualValues(coreClient.minGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return gas reported by the node query")

	// Emit a full block.
	emitBlock(&emitter, true, quantity.NewFromUint64(1_000_000_000_000)) // 1000 gwei.

	// 1001 gwei should be returned.
	require.EqualValues(big.NewInt(1_001_000_000_000), gasPriceOracle.GasPrice(), "oracle should return correct gas price")

	// Emit `windowSize` non-full blocks.
	for i := 0; i < windowSize; i++ {
		emitBlock(&emitter, false, nil)
	}

	require.EqualValues(coreClient.minGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return gas reported by the node query")

	// Emit a full block with a transaction cheaper than the min gas price reported by the oasis node.
	emitBlock(&emitter, true, quantity.NewFromUint64(1)) // 1 wei.

	require.EqualValues(coreClient.minGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return gas reported by the node query")
	gasPriceOracle.Stop()
}
