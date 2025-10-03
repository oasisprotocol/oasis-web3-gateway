// Package pebble implements a source backend using pebbledb cache.
package pebble

import (
	"context"
	"fmt"

	"github.com/cockroachdb/pebble"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/oasisprotocol/oasis-web3-gateway/source"
)

var _ source.NodeSource = (*Source)(nil)

func makeKey(method string, params ...interface{}) []byte {
	return cbor.Marshal([]interface{}{method, params})
}

// Source is a pebble backend source.
type Source struct {
	db *pebble.DB

	source source.NodeSource
}

func getFromCacheOrFetch[T any, F func(context.Context) (T, error)](ctx context.Context, db *pebble.DB, key []byte, volatile bool, fetch F) (T, error) {
	if volatile {
		return fetch(ctx)
	}

	var val T
	// Try fetching from cache first.
	data, closer, err := db.Get(key)
	switch err {
	case nil:
		// Continues below.
	case pebble.ErrNotFound:
		// Key not found, fetch via the client.
		fetched, err := fetch(ctx)
		if err != nil {
			return val, fmt.Errorf("fetching data: %w", err)
		}
		// Store in cache.
		_ = db.Set(key, cbor.Marshal(fetched), pebble.Sync)
		return fetched, nil
	default:
		return val, err
	}

	defer func() { _ = closer.Close() }()
	if err := cbor.Unmarshal(data, &val); err != nil {
		return val, fmt.Errorf("cached data: %w", err)
	}
	return val, nil
}

func (s *Source) GetBlock(ctx context.Context, height uint64) (*block.Block, error) {
	key := makeKey("GetBlock", height)
	return getFromCacheOrFetch(ctx, s.db, key, height == client.RoundLatest, func(ctx context.Context) (*block.Block, error) {
		return s.source.GetBlock(ctx, height)
	})
}

func (s *Source) GetTransactionsWithResults(ctx context.Context, height uint64) ([]*client.TransactionWithResults, error) {
	key := makeKey("GetTransactionsWithResults", height)
	return getFromCacheOrFetch(ctx, s.db, key, height == client.RoundLatest, func(ctx context.Context) ([]*client.TransactionWithResults, error) {
		txs, err := s.source.GetTransactionsWithResults(ctx, height)
		if err != nil {
			return nil, err
		}
		return txs, nil
	})
}

func (s *Source) GetLastRetainedBlock(ctx context.Context) (*block.Block, error) {
	key := makeKey("GetLastRetainedBlock")
	return getFromCacheOrFetch(ctx, s.db, key, true, func(ctx context.Context) (*block.Block, error) {
		return s.source.GetLastRetainedBlock(ctx)
	})
}

// CoreParameters implements source.NodeSource.
func (s *Source) CoreParameters(ctx context.Context, height uint64) (*core.Parameters, error) {
	key := makeKey("CoreParameters", height)
	return getFromCacheOrFetch(ctx, s.db, key, height == client.RoundLatest, func(ctx context.Context) (*core.Parameters, error) {
		return s.source.CoreParameters(ctx, height)
	})
}

// CoreRuntimeInfo implements source.NodeSource.
func (s *Source) CoreRuntimeInfo(ctx context.Context) (*core.RuntimeInfoResponse, error) {
	key := makeKey("CoreRuntimeInfo")
	// Ideally runtime info would accept a height parameter, but since it doesn't at the moment
	// we should treat this as volatile, but we don't as we need to cache it for tests.
	return getFromCacheOrFetch(ctx, s.db, key, false, func(ctx context.Context) (*core.RuntimeInfoResponse, error) {
		return s.source.CoreRuntimeInfo(ctx)
	})
}

// CoreCallDataPublicKey implements source.NodeSource.
func (s *Source) CoreCallDataPublicKey(ctx context.Context) (*core.CallDataPublicKeyResponse, error) {
	key := makeKey("CoreCallDataPublicKey")
	// Ideally runtime info would accept a height parameter, but since it doesn't at the moment
	// we should treat this as volatile, but we don't as we need to cache it for tests.
	return getFromCacheOrFetch(ctx, s.db, key, false, func(ctx context.Context) (*core.CallDataPublicKeyResponse, error) {
		return s.source.CoreCallDataPublicKey(ctx)
	})
}

// CoreEstimateGasForCaller implements source.NodeSource.
func (s *Source) CoreEstimateGasForCaller(ctx context.Context, round uint64, caller types.CallerAddress, tx *types.Transaction, propagateFailures bool) (uint64, error) {
	key := makeKey("CoreEstimateGasForCaller", round, caller, tx, propagateFailures)
	return getFromCacheOrFetch(ctx, s.db, key, round == client.RoundLatest, func(ctx context.Context) (uint64, error) {
		return s.source.CoreEstimateGasForCaller(ctx, round, caller, tx, propagateFailures)
	})
}

// CoreMinGasPrice implements source.NodeSource.
func (s *Source) CoreMinGasPrice(ctx context.Context, round uint64) (map[types.Denomination]types.Quantity, error) {
	key := makeKey("CoreMinGasPrice", round)
	return getFromCacheOrFetch(ctx, s.db, key, round == client.RoundLatest, func(ctx context.Context) (map[types.Denomination]types.Quantity, error) {
		return s.source.CoreMinGasPrice(ctx, round)
	})
}

// AccountsNonce implements source.NodeSource.
func (s *Source) AccountsNonce(ctx context.Context, round uint64, address types.Address) (uint64, error) {
	key := makeKey("AccountsNonce", round, address)
	return getFromCacheOrFetch(ctx, s.db, key, round == client.RoundLatest, func(ctx context.Context) (uint64, error) {
		return s.source.AccountsNonce(ctx, round, address)
	})
}

// EVMCode implements source.NodeSource.
func (s *Source) EVMCode(ctx context.Context, round uint64, address []byte) ([]byte, error) {
	key := makeKey("EVMCode", round, address)
	return getFromCacheOrFetch(ctx, s.db, key, round == client.RoundLatest, func(ctx context.Context) ([]byte, error) {
		return s.source.EVMCode(ctx, round, address)
	})
}

// EVMBalance implements source.NodeSource.
func (s *Source) EVMBalance(ctx context.Context, round uint64, address []byte) (*types.Quantity, error) {
	key := makeKey("EVMBalance", round, address)
	return getFromCacheOrFetch(ctx, s.db, key, round == client.RoundLatest, func(ctx context.Context) (*types.Quantity, error) {
		return s.source.EVMBalance(ctx, round, address)
	})
}

// EVMStorage implements source.NodeSource.
func (s *Source) EVMStorage(ctx context.Context, round uint64, address []byte, slot []byte) ([]byte, error) {
	key := makeKey("EVMStorage", round, address, slot)
	return getFromCacheOrFetch(ctx, s.db, key, round == client.RoundLatest, func(ctx context.Context) ([]byte, error) {
		return s.source.EVMStorage(ctx, round, address, slot)
	})
}

// EVMSimulateCall implements source.NodeSource.
func (s *Source) EVMSimulateCall(ctx context.Context, round uint64, gasPrice []byte, gasLimit uint64, caller []byte, address []byte, value []byte, data []byte) ([]byte, error) {
	key := makeKey("EVMSimulateCall", round, gasPrice, gasLimit, caller, address, value, data)
	return getFromCacheOrFetch(ctx, s.db, key, round == client.RoundLatest, func(ctx context.Context) ([]byte, error) {
		return s.source.EVMSimulateCall(ctx, round, gasPrice, gasLimit, caller, address, value, data)
	})
}

// EVMCreate implements source.NodeSource.
func (s *Source) EVMCreate(value []byte, initCode []byte) *types.Transaction {
	// Transaction creation is a local operation, just delegate.
	return s.source.EVMCreate(value, initCode)
}

// EVMCall implements source.NodeSource.
func (s *Source) EVMCall(address []byte, value []byte, data []byte) *types.Transaction {
	// Transaction creation is a local operation, just delegate.
	return s.source.EVMCall(address, value, data)
}

// SubmitTxNoWait implements source.NodeSource.
func (s *Source) SubmitTxNoWait(ctx context.Context, tx *types.UnverifiedTransaction) error {
	// Transaction submission should not go through cache.
	return fmt.Errorf("SubmitTxNoWait not supported on pebble source")
}

// Close closes the pebble source backend.
func (s *Source) Close() error {
	return s.db.Close()
}

// New creates a new pebble source backend.
func New(source source.NodeSource, path string) (*Source, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, err
	}

	return &Source{
		db:     db,
		source: source,
	}, nil
}
