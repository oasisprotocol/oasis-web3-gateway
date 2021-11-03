package indexer

import (
	"context"
	"errors"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

const (
	storageRequestTimeout = 5 * time.Second
	storageRetryTimeout   = 1 * time.Second
	pruningCheckInterval  = 60 * time.Second
)

const RoundLatest = client.RoundLatest

var (
	ErrGetBlockFailed        = errors.New("GetBlock failed")
	ErrGetTransactionsFailed = errors.New("GetTransactions failed")
	ErrIndexedFailed         = errors.New("Index block failed")
)

// Service is an indexer service.
type Service struct {
	service.BaseBackgroundService

	runtimeID     common.Namespace
	enablePruning bool
	pruningStep   uint64

	backend Backend
	client  client.RuntimeClient

	ctx       context.Context
	cancelCtx context.CancelFunc
}

func (s *Service) indexBlock(round uint64) error {
	blk, err := s.client.GetBlock(s.ctx, round)
	if err != nil {
		return ErrGetBlockFailed
	}

	txs, err := s.client.GetTransactionsWithResults(s.ctx, blk.Header.Round)
	if err != nil {
		return ErrGetTransactionsFailed
	}

	err = s.backend.Index(blk, txs)
	if err != nil {
		return ErrIndexedFailed
	}

	return nil
}

func (s *Service) getRoundLatest() (uint64, error) {
	blk, err := s.client.GetBlock(s.ctx, RoundLatest)
	if err != nil {
		return 0, err
	}

	return blk.Header.Round, nil
}

func (s *Service) pruningWorker() {
	s.Logger.Debug("starting periodic pruning worker")

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(pruningCheckInterval):
			lastIndexed := s.backend.QueryLastIndexedRound()

			if lastIndexed > s.pruningStep {
				round := lastIndexed - s.pruningStep
				if err := s.backend.Prune(round); err != nil {
					s.Logger.Error("failed to prune round",
						"err", err,
						"round", round,
					)
				}
			}
		}
	}
}

func (s *Service) indexingWorker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Query latest round available at the node.
		latest, err := s.getRoundLatest()
		if err != nil {
			time.Sleep(storageRequestTimeout)
			s.Logger.Info("failed to query latest round",
				"err", err,
			)
			continue
		}

		lastIndexed := s.backend.QueryLastIndexedRound()
		if latest < lastIndexed {
			panic("This is a new chain, please clear the db first!")
		}

		// Adjust the last indexed round such that it is at least equal to or greater than the last
		// retained round reported by the node as there is no way to request earlier rounds.
		lastRetainedBlock, err := s.client.GetLastRetainedBlock(s.ctx)
		if err != nil {
			time.Sleep(storageRequestTimeout)
			s.Logger.Error("failed to retrieve last retained round",
				"err", err,
			)
			continue
		}
		if lastIndexed < lastRetainedBlock.Header.Round {
			lastIndexed = lastRetainedBlock.Header.Round - 1
		}

		if latest == lastIndexed {
			time.Sleep(storageRetryTimeout)
			continue
		}

		for round := lastIndexed + 1; round <= latest; round++ {
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// Try to index block.
			if err = s.indexBlock(round); err != nil {
				time.Sleep(storageRequestTimeout)
				s.Logger.Warn("failed to index block",
					"err", err,
					"round", round,
				)
				break
			}

			// Update last indexed round for correct resumption.
			if err = s.backend.UpdateLastIndexedRound(round); err != nil {
				s.Logger.Warn("failed to update last indexed round",
					"err", err,
					"round", round,
				)
			}
		}
	}
}

func (s *Service) Start() {
	go s.indexingWorker()

	if s.enablePruning {
		go s.pruningWorker()
	}
}

func (s *Service) Stop() {
	s.cancelCtx()
}

// New creates a new indexer service.
func New(
	backendFactory BackendFactory,
	client client.RuntimeClient,
	runtimeID common.Namespace,
	storage storage.Storage,
	enablePruning bool,
	pruningStep uint64,
) (*Service, Backend, error) {
	backend, err := backendFactory(runtimeID, storage)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	s := &Service{
		BaseBackgroundService: *service.NewBaseBackgroundService("gateway/indexer"),
		runtimeID:             runtimeID,
		backend:               backend,
		client:                client,
		ctx:                   ctx,
		cancelCtx:             cancelCtx,
		enablePruning:         enablePruning,
		pruningStep:           pruningStep,
	}
	s.Logger = s.Logger.With("runtime_id", s.runtimeID.String())

	return s, backend, nil
}
