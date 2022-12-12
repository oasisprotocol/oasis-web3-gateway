package gas

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common/pubsub"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
)

type mockCoreClient struct {
	minGasPrice quantity.Quantity
	shouldFail  bool
}

// EstimateGas implements core.V1.
func (*mockCoreClient) EstimateGas(ctx context.Context, round uint64, tx *types.Transaction, propagateFailures bool) (uint64, error) {
	panic("unimplemented")
}

// EstimateGasForCaller implements core.V1.
func (*mockCoreClient) EstimateGasForCaller(ctx context.Context, round uint64, caller types.CallerAddress, tx *types.Transaction, propagateFailures bool) (uint64, error) {
	panic("unimplemented")
}

// GetEvents implements core.V1.
func (*mockCoreClient) GetEvents(ctx context.Context, round uint64) ([]*core.Event, error) {
	panic("unimplemented")
}

// Parameters implements core.V1.
func (*mockCoreClient) Parameters(ctx context.Context, round uint64) (*core.Parameters, error) {
	panic("unimplemented")
}

// RuntimeInfo implements core.V1.
func (*mockCoreClient) RuntimeInfo(ctx context.Context) (*core.RuntimeInfoResponse, error) {
	panic("unimplemented")
}

// RuntimeInfo implements core.V1.
func (*mockCoreClient) CallDataPublicKey(ctx context.Context) (*core.CallDataPublicKeyResponse, error) {
	panic("unimplemented")
}

// RuntimeInfo implements core.V1.
func (*mockCoreClient) ExecuteReadOnlyTx(ctx context.Context, round uint64, tx *types.UnverifiedTransaction) (*core.ExecuteReadOnlyTxResponse, error) {
	panic("unimplemented")
}

// MinGasPrice implements core.V1.
func (m *mockCoreClient) MinGasPrice(ctx context.Context) (map[types.Denomination]quantity.Quantity, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("failed")
	}
	return map[types.Denomination]quantity.Quantity{
		types.NativeDenomination: m.minGasPrice,
	}, nil
}

type mockBlockEmitter struct {
	notifier *pubsub.Broker
}

func (b *mockBlockEmitter) Emit(data *indexer.BlockData) {
	b.notifier.Broadcast(data)
}

// WatchBlocks implements indexer.BlockWatcher.
func (b *mockBlockEmitter) WatchBlocks(ctx context.Context, buffer int64) (<-chan *indexer.BlockData, pubsub.ClosableSubscription, error) {
	typedCh := make(chan *indexer.BlockData)
	sub := b.notifier.SubscribeBuffered(buffer)
	sub.Unwrap(typedCh)

	return typedCh, sub, nil
}

func emitBlock(emitter *mockBlockEmitter, fullBlock bool, lastTxPrice *quantity.Quantity) {
	// Wait a bit after emitting so that a the block is processed.
	defer time.Sleep(100 * time.Millisecond)

	if fullBlock {
		emitter.Emit(&indexer.BlockData{Block: &model.Block{Header: &model.Header{GasLimit: 10_000, GasUsed: 10_000}}, LastTransactionPrice: lastTxPrice})
		return
	}
	emitter.Emit(&indexer.BlockData{Block: &model.Block{Header: &model.Header{GasLimit: 10_000, GasUsed: 10}}})
}

func TestGasPriceOracle(t *testing.T) {
	require := require.New(t)

	emitter := mockBlockEmitter{
		notifier: pubsub.NewBroker(false),
	}

	coreClient := mockCoreClient{
		minGasPrice: *quantity.NewFromUint64(42),
		shouldFail:  true,
	}

	// Gas price oracle with failing core client.
	gasPriceOracle := New(context.Background(), &emitter, &coreClient)
	require.NoError(gasPriceOracle.Start())

	// Default gas price should be returned by the oracle.
	require.EqualValues(defaultGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return default gas price")

	// Emit a non-full block.
	emitBlock(&emitter, false, nil)

	// Default gas price should be returned by the oracle.
	require.EqualValues(defaultGasPrice.ToBigInt(), gasPriceOracle.GasPrice(), "oracle should return default gas price")

	// Emit a full block.
	emitBlock(&emitter, true, quantity.NewFromUint64(1_000_000_000_000)) // 1000 gwei.

	// 1001 gwei should be returned.
	require.EqualValues(big.NewInt(1_001_000_000_000), gasPriceOracle.GasPrice(), "oracle should return correct gas price")

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
	gasPriceOracle = New(context.Background(), &emitter, &coreClient)
	require.NoError(gasPriceOracle.Start())

	// Wait a bit so that a MinGasPrice query is made.
	time.Sleep(1 * time.Second)
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
