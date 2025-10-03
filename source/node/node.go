// Package node implements the oasis-node backed source.
package node

import (
	"context"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/oasisprotocol/oasis-web3-gateway/source"
)

var _ source.NodeSource = (*Source)(nil)

type Source struct {
	client   client.RuntimeClient
	core     core.V1
	evm      evm.V1
	accounts accounts.V1
}

// CoreParameters implements source.NodeSource.
func (s *Source) CoreParameters(ctx context.Context, height uint64) (*core.Parameters, error) {
	return s.core.Parameters(ctx, height)
}

// CoreRuntimeInfo implements source.NodeSource.
func (s *Source) CoreRuntimeInfo(ctx context.Context) (*core.RuntimeInfoResponse, error) {
	return s.core.RuntimeInfo(ctx)
}

// CoreCallDataPublicKey implements source.NodeSource.
func (s *Source) CoreCallDataPublicKey(ctx context.Context) (*core.CallDataPublicKeyResponse, error) {
	return s.core.CallDataPublicKey(ctx)
}

// CoreEstimateGasForCaller implements source.NodeSource.
func (s *Source) CoreEstimateGasForCaller(ctx context.Context, round uint64, caller types.CallerAddress, tx *types.Transaction, propagateFailures bool) (uint64, error) {
	return s.core.EstimateGasForCaller(ctx, round, caller, tx, propagateFailures)
}

// CoreMinGasPrice implements source.NodeSource.
func (s *Source) CoreMinGasPrice(ctx context.Context, round uint64) (map[types.Denomination]types.Quantity, error) {
	return s.core.MinGasPrice(ctx, round)
}

// AccountsNonce implements source.NodeSource.
func (s *Source) AccountsNonce(ctx context.Context, round uint64, address types.Address) (uint64, error) {
	return s.accounts.Nonce(ctx, round, address)
}

// GetBlock implements source.NodeSource.
func (s *Source) GetBlock(ctx context.Context, height uint64) (*block.Block, error) {
	return s.client.GetBlock(ctx, height)
}

// GetLastRetainedBlock implements source.NodeSource.
func (s *Source) GetLastRetainedBlock(ctx context.Context) (*block.Block, error) {
	return s.client.GetLastRetainedBlock(ctx)
}

// GetTransactionsWithResults implements source.NodeSource.
func (s *Source) GetTransactionsWithResults(ctx context.Context, height uint64) ([]*client.TransactionWithResults, error) {
	return s.client.GetTransactionsWithResults(ctx, height)
}

// EVMCode implements source.NodeSource.
func (s *Source) EVMCode(ctx context.Context, round uint64, address []byte) ([]byte, error) {
	return s.evm.Code(ctx, round, address)
}

// EVMBalance implements source.NodeSource.
func (s *Source) EVMBalance(ctx context.Context, round uint64, address []byte) (*types.Quantity, error) {
	return s.evm.Balance(ctx, round, address)
}

// EVMStorage implements source.NodeSource.
func (s *Source) EVMStorage(ctx context.Context, round uint64, address []byte, slot []byte) ([]byte, error) {
	return s.evm.Storage(ctx, round, address, slot)
}

// EVMSimulateCall implements source.NodeSource.
func (s *Source) EVMSimulateCall(ctx context.Context, round uint64, gasPrice []byte, gasLimit uint64, caller []byte, address []byte, value []byte, data []byte) ([]byte, error) {
	return s.evm.SimulateCall(ctx, round, gasPrice, gasLimit, caller, address, value, data)
}

// EVMCreate implements source.NodeSource.
func (s *Source) EVMCreate(value []byte, initCode []byte) *types.Transaction {
	return evm.NewV1(s.client).Create(value, initCode).GetTransaction()
}

// EVMCall implements source.NodeSource.
func (s *Source) EVMCall(address []byte, value []byte, data []byte) *types.Transaction {
	return evm.NewV1(s.client).Call(address, value, data).GetTransaction()
}

// SubmitTxNoWait implements source.NodeSource.
func (s *Source) SubmitTxNoWait(ctx context.Context, tx *types.UnverifiedTransaction) error {
	return s.client.SubmitTxNoWait(ctx, tx)
}

// Close implements source.NodeSource.
func (s *Source) Close() error {
	// Nothing to close for the node source.
	return nil
}

// New creates a new node backed source.
func New(client client.RuntimeClient) *Source {
	return &Source{
		client:   client,
		core:     core.NewV1(client),
		evm:      evm.NewV1(client),
		accounts: accounts.NewV1(client),
	}
}
