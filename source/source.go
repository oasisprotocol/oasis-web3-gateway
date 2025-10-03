// Package source provides the NodeSource interface for fetching data from Oasis nodes.
package source

import (
	"context"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
)

// NodeSource is the interface that provides needed methods for the indexer and gateway to fetch data from.
type NodeSource interface {
	// GetBlock returns the block at the given height.
	GetBlock(ctx context.Context, height uint64) (*block.Block, error)

	// GetTransactionsWithResults returns the transactions with results at the given height.
	GetTransactionsWithResults(ctx context.Context, height uint64) ([]*client.TransactionWithResults, error)

	// GetLastRetained block returns the last retained block.
	GetLastRetainedBlock(ctx context.Context) (*block.Block, error)

	// CoreParameters returns the core module parameters at the given height.
	CoreParameters(ctx context.Context, height uint64) (*core.Parameters, error)

	// CoreRuntimeInfo returns the core module runtime info.
	CoreRuntimeInfo(ctx context.Context) (*core.RuntimeInfoResponse, error)

	// CoreCallDataPublicKey returns the call data public key.
	CoreCallDataPublicKey(ctx context.Context) (*core.CallDataPublicKeyResponse, error)

	// CoreEstimateGasForCaller estimates gas for a transaction from a specific caller.
	CoreEstimateGasForCaller(ctx context.Context, round uint64, caller types.CallerAddress, tx *types.Transaction, propagateFailures bool) (uint64, error)

	// CoreMinGasPrice returns the minimum gas price at the given round.
	CoreMinGasPrice(ctx context.Context, round uint64) (map[types.Denomination]types.Quantity, error)

	// AccountsNonce returns the nonce for an account at the given round.
	AccountsNonce(ctx context.Context, round uint64, address types.Address) (uint64, error)

	// EVMCode returns the code at the given address and round.
	EVMCode(ctx context.Context, round uint64, address []byte) ([]byte, error)

	// EVMBalance returns the balance at the given address and round.
	EVMBalance(ctx context.Context, round uint64, address []byte) (*types.Quantity, error)

	// EVMStorage returns the storage value at the given address, slot, and round.
	EVMStorage(ctx context.Context, round uint64, address []byte, slot []byte) ([]byte, error)

	// EVMSimulateCall simulates a call at the given round.
	EVMSimulateCall(ctx context.Context, round uint64, gasPrice []byte, gasLimit uint64, caller []byte, address []byte, value []byte, data []byte) ([]byte, error)

	// EVMCreate creates a transaction for deploying a contract.
	EVMCreate(value []byte, initCode []byte) *types.Transaction

	// EVMCall creates a transaction for calling a contract.
	EVMCall(address []byte, value []byte, data []byte) *types.Transaction

	// SubmitTxNoWait submits a transaction without waiting for it to be included.
	SubmitTxNoWait(ctx context.Context, tx *types.UnverifiedTransaction) error

	// Close closes the source and releases any resources.
	Close() error
}
