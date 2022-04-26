package client

import (
	"context"
	"fmt"
	"math"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/common/pubsub"
	roothash "github.com/oasisprotocol/oasis-core/go/roothash/api"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
)

func clientBlockRanges(
	ctx context.Context,
	cli client.RuntimeClient,
) (uint64, uint64, error) {
	blk, err := cli.GetLastRetainedBlock(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("client: failed to query last retained block: %w", err)
	}
	start := blk.Header.Round

	blk, err = cli.GetBlock(ctx, client.RoundLatest)
	if err != nil {
		return 0, 0, fmt.Errorf("client: failed to query newest block: %w", err)
	}
	return start, blk.Header.Round, nil
}

type archiveClient struct {
	client client.RuntimeClient

	// start/end rounds (inclusive)
	start uint64
	end   uint64
}

func (ac *archiveClient) IsDistinct(other *archiveClient) bool {
	if ac.start <= other.end && ac.end >= other.start {
		return false
	}
	return true
}

func newArchiveClient(
	ctx context.Context,
	cli client.RuntimeClient,
) (*archiveClient, error) {
	ac := &archiveClient{
		client: cli,
	}

	var err error
	if ac.start, ac.end, err = clientBlockRanges(ctx, cli); err != nil {
		return nil, err
	}

	return ac, nil
}

// MultiClient is a SDK runtime client that is backed by a single active
// node, and multiple archive nodes.
//
// Note: Methods that do not have provisions for a round, will always be
// dispatched to the active node, as will pubsub subscriptions.
type MultiClient struct {
	active       client.RuntimeClient
	activeEldest uint64

	archives       []*archiveClient
	archivesEldest uint64
}

// GetInfo returns information about the runtime.
func (mc *MultiClient) GetInfo(
	ctx context.Context,
) (*types.RuntimeInfo, error) {
	return mc.active.GetInfo(ctx)
}

// SubmitTxRaw submits a transaction to the runtime transaction scheduler and waits
// for transaction execution results.
func (mc *MultiClient) SubmitTxRaw(
	ctx context.Context,
	tx *types.UnverifiedTransaction,
) (*types.CallResult, error) {
	return mc.active.SubmitTxRaw(ctx, tx)
}

// SubmitTxRawMeta submits a transaction to the runtime transaction scheduler and waits
// for transaction execution results.
//
// Response includes transaction metadata - e.g. round at which the transaction was included
// in a block.
func (mc *MultiClient) SubmitTxRawMeta(
	ctx context.Context,
	tx *types.UnverifiedTransaction,
) (*client.SubmitTxRawMeta, error) {
	return mc.active.SubmitTxRawMeta(ctx, tx)
}

// SubmitTx submits a transaction to the runtime transaction scheduler and waits
// for transaction execution results.
//
// If there is a possibility that the result is Unknown then the caller must use SubmitTxRaw
// instead as this method will return an error.
func (mc *MultiClient) SubmitTx(
	ctx context.Context,
	tx *types.UnverifiedTransaction,
) (cbor.RawMessage, error) {
	return mc.active.SubmitTx(ctx, tx)
}

// SubmitTx submits a transaction to the runtime transaction scheduler and waits
// for transaction execution results.
//
// If there is a possibility that the result is Unknown then the caller must use SubmitTxRaw
// instead as this method will return an error.
//
// Response includes transaction metadata - e.g. round at which the transaction was included
// in a block.
func (mc *MultiClient) SubmitTxMeta(
	ctx context.Context,
	tx *types.UnverifiedTransaction,
) (*client.SubmitTxMeta, error) {
	return mc.active.SubmitTxMeta(ctx, tx)
}

// SubmitTxNoWait submits a transaction to the runtime transaction scheduler but does
// not wait for transaction execution.
func (mc *MultiClient) SubmitTxNoWait(
	ctx context.Context,
	tx *types.UnverifiedTransaction,
) error {
	return mc.active.SubmitTxNoWait(ctx, tx)
}

// GetGenesisBlock returns the genesis block.
func (mc *MultiClient) GetGenesisBlock(
	ctx context.Context,
) (*block.Block, error) {
	return mc.active.GetGenesisBlock(ctx)
}

// WatchBlocks subscribes to blocks for a specific runtimes.
func (mc *MultiClient) WatchBlocks(
	ctx context.Context,
) (<-chan *roothash.AnnotatedBlock, pubsub.ClosableSubscription, error) {
	return mc.active.WatchBlocks(ctx)
}

// WatchEvents subscribes and decodes runtime events.
func (mc *MultiClient) WatchEvents(
	ctx context.Context,
	decoders []client.EventDecoder,
	includeUndecoded bool,
) (<-chan *client.BlockEvents, error) {
	return mc.active.WatchEvents(ctx, decoders, includeUndecoded)
}

//
// The rest of the routines actually need to handle figuring out which node
// to dispatch the query to.
//

func (mc *MultiClient) clientForRound(
	round uint64,
) client.RuntimeClient {
	// Fast path, check the active node first, before hitting up the
	// archives.
	if round == client.RoundLatest {
		return mc.active
	}
	if round >= mc.activeEldest {
		return mc.active
	}

	// Scan through the archive nodes till we find the appropriate one.
	for _, an := range mc.archives {
		if round >= an.start && round <= an.end {
			return an.client
		}
	}

	// No archive node available that can service the request.  Just
	// hit up the active one for the error.
	return mc.active
}

// GetBlock fetches the given runtime block.
func (mc *MultiClient) GetBlock(
	ctx context.Context,
	round uint64,
) (*block.Block, error) {
	cli := mc.clientForRound(round)
	return cli.GetBlock(ctx, round)
}

// GetLastRetainedBlock returns the last retained block.
func (mc *MultiClient) GetLastRetainedBlock(
	ctx context.Context,
) (*block.Block, error) {
	// There are several extremely nonsensical configurations that
	// the user can provide, particularly anything with both pruning
	// and archive nodes.  But "don't do that then, idiot".
	if len(mc.archives) == 0 {
		return mc.active.GetLastRetainedBlock(ctx)
	}

	// Ok, presumably, we have contiguous history, so doing this
	// actually makes sense and users won't encounter the gap.
	//
	// Since the height of the last retained block doesn't change
	// for archive nodes, just return it.
	return mc.GetBlock(ctx, mc.archivesEldest)
}

// GetTransactions returns all transactions that are part of a given block.
func (mc *MultiClient) GetTransactions(
	ctx context.Context,
	round uint64,
) ([]*types.UnverifiedTransaction, error) {
	cli := mc.clientForRound(round)
	return cli.GetTransactions(ctx, round)
}

// GetTransactionsWithResults returns all transactions that are part of a given block together
// with their results and emitted events.
func (mc *MultiClient) GetTransactionsWithResults(
	ctx context.Context,
	round uint64,
) ([]*client.TransactionWithResults, error) {
	cli := mc.clientForRound(round)
	return cli.GetTransactionsWithResults(ctx, round)
}

// GetEventsRaw returns all events emitted in a given block.
func (mc *MultiClient) GetEventsRaw(
	ctx context.Context,
	round uint64,
) ([]*types.Event, error) {
	cli := mc.clientForRound(round)
	return cli.GetEventsRaw(ctx, round)
}

// GetEvents returns and decodes events emitted in a given block with the provided decoders.
func (mc *MultiClient) GetEvents(
	ctx context.Context,
	round uint64,
	decoders []client.EventDecoder,
	includeUndecoded bool,
) ([]client.DecodedEvent, error) {
	cli := mc.clientForRound(round)
	return cli.GetEvents(ctx, round, decoders, includeUndecoded)
}

// Query makes a runtime-specific query.
func (mc *MultiClient) Query(
	ctx context.Context,
	round uint64,
	method string,
	args interface{},
	rsp interface{},
) error {
	cli := mc.clientForRound(round)
	return cli.Query(ctx, round, method, args, rsp)
}

// NewMulti creates a new RuntimeClient that is a composite of an active node
// (generating new blocks), and archive nodes (already fully indexed).
//
// Note: All the nodes must be functional and responsive when this routine
// is called.
func NewMulti(
	ctx context.Context,
	active client.RuntimeClient,
	archives []client.RuntimeClient,
) (client.RuntimeClient, error) {
	activeInfo, err := active.GetInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("client: failed to query active rt info: %w", err)
	}

	mc := &MultiClient{
		active:         active,
		archives:       make([]*archiveClient, 0, len(archives)),
		archivesEldest: math.MaxUint64,
	}
	if mc.activeEldest, _, err = clientBlockRanges(ctx, active); err != nil {
		return nil, err
	}

	for i := range archives {
		client := archives[i]
		archiveInfo, err := client.GetInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("client: failed to query archive rt info: %w", err)
		}
		if !archiveInfo.ID.Equal(&activeInfo.ID) {
			return nil, fmt.Errorf("client: runtime ID mismatch: got %v expected %v", archiveInfo.ID, activeInfo.ID)
		}

		ac, err := newArchiveClient(ctx, client)
		if err != nil {
			return nil, err
		}
		for _, existing := range mc.archives {
			if !ac.IsDistinct(existing) {
				return nil, fmt.Errorf("client: multiple archive nodes provide overlapping state")
			}
		}

		mc.archives = append(mc.archives, ac)
		if mc.archivesEldest > ac.start {
			mc.archivesEldest = ac.start
		}
	}

	// This could sort mc.archives (probably in reverse order), and
	// check that history is contiguous across all the archive nodes,
	// but there is only going to be one archive node for a while
	// if any.

	return mc, nil
}
