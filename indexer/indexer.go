package indexer

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"

	"github.com/oasisprotocol/emerald-web3-gateway/filters"
	"github.com/oasisprotocol/emerald-web3-gateway/storage"
)

const (
	indexerLoopDelay                 = 1 * time.Second
	pruningCheckInterval             = 60 * time.Second
	healthCheckInterval              = 10 * time.Second
	healthCheckIndexerDriftThreshold = 10
)

var (
	ErrGetBlockFailed        = errors.New("get block failed")
	ErrGetTransactionsFailed = errors.New("get transactions failed")
	ErrIndexedFailed         = errors.New("index block failed")
	ErrNotHealthy            = errors.New("not healthy")
)

// Service is an indexer service.
type Service struct {
	service.BaseBackgroundService

	runtimeID     common.Namespace
	enablePruning bool
	pruningStep   uint64

	backend Backend
	client  client.RuntimeClient
	core    core.V1

	queryBlockGasLimit bool
	blockGasLimit      uint64

	ctx       context.Context
	cancelCtx context.CancelFunc

	health uint32
}

// Implements server.HealthCheck.
func (s *Service) Health() error {
	if atomic.LoadUint32(&s.health) == 0 {
		return ErrNotHealthy
	}
	return nil
}

// indexBlock indexes given block number.
func (s *Service) indexBlock(round uint64) error {
	blk, err := s.client.GetBlock(s.ctx, round)
	if err != nil {
		return ErrGetBlockFailed
	}

	if s.blockGasLimit == 0 || s.queryBlockGasLimit {
		// Query parameters for block gas limit.
		var params *core.Parameters
		params, err = s.core.Parameters(s.ctx, round)
		if err != nil {
			return fmt.Errorf("querying block gas limit: %w", err)
		}
		s.blockGasLimit = params.MaxBatchGas
	}

	txs, err := s.client.GetTransactionsWithResults(s.ctx, blk.Header.Round)
	if err != nil {
		return ErrGetTransactionsFailed
	}

	err = s.backend.Index(blk, txs, s.blockGasLimit)
	if err != nil {
		return ErrIndexedFailed
	}

	switch {
	case blk.Header.HeaderType == block.EpochTransition:
		// Epoch transition block, ensure block gas is queried on next normal round.
		s.queryBlockGasLimit = true
	case s.queryBlockGasLimit && blk.Header.HeaderType == block.Normal:
		// Block gas was queried, no need to query it until next epoch.
		s.queryBlockGasLimit = false
	default:
	}

	return nil
}

// getRoundLatest returns the latest round.
func (s *Service) getRoundLatest() (uint64, error) {
	blk, err := s.client.GetBlock(s.ctx, client.RoundLatest)
	if err != nil {
		return 0, err
	}

	return blk.Header.Round, nil
}

func (s *Service) updateHealth(healthy bool) {
	if healthy {
		atomic.StoreUint32(&s.health, 1)
	} else {
		atomic.StoreUint32(&s.health, 0)
	}
}

// healthWorker is the health check worker.
func (s *Service) healthWorker() {
	s.Logger.Debug("starting health check worker")
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(healthCheckInterval):
			// Query last indexed round.
			lastIndexed, err := s.backend.QueryLastIndexedRound()
			if err != nil {
				s.Logger.Error("failed to query last indexed round",
					"err", err,
				)
				s.updateHealth(false)
				continue
			}

			// Query latest round on the node.
			latestBlk, err := s.client.GetBlock(s.ctx, client.RoundLatest)
			if err != nil {
				s.Logger.Error("failed to query latest block",
					"err", err,
				)
				s.updateHealth(false)
				continue
			}
			latestRound := latestBlk.Header.Round

			s.Logger.Debug("checking health", "latest_round", latestRound, "latest_indexed", lastIndexed)
			switch {
			case latestRound >= lastIndexed:
				s.updateHealth(latestRound-lastIndexed <= healthCheckIndexerDriftThreshold)
			default:
				// Indexer in front of node - not healthy.
				s.updateHealth(false)
			}
		}
	}
}

// pruningWorker handles data pruning.
func (s *Service) pruningWorker() {
	s.Logger.Debug("starting periodic pruning worker")

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(pruningCheckInterval):
			lastIndexed, err := s.backend.QueryLastIndexedRound()
			if err != nil {
				s.Logger.Error("failed to query last indexed round",
					"err", err,
				)
				continue
			}

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

// indexingWorker is a worker for indexing.
func (s *Service) indexingWorker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(indexerLoopDelay):
		}

		// Query latest round available at the node.
		latest, err := s.getRoundLatest()
		if err != nil {
			s.Logger.Info("failed to query latest round",
				"err", err,
			)
			continue
		}

		var startAt uint64
		// Get last indexed round.
		lastIndexed, err := s.backend.QueryLastIndexedRound()
		switch {
		case errors.Is(err, storage.ErrNoRoundsIndexed):
			// No rounds indexed, start at 0.
			startAt = 0
		case err != nil:
			s.Logger.Error("failed to query last indexed round",
				"err", err,
			)
			continue
		default:
			if latest < lastIndexed {
				panic("This is a new chain, please clear the db first!")
			}
			// Latest round already indexed.
			if latest == lastIndexed {
				continue
			}
			startAt = lastIndexed + 1
		}

		// Get last retained round on the node.
		lastRetainedBlock, err := s.client.GetLastRetainedBlock(s.ctx)
		if err != nil {
			s.Logger.Error("failed to retrieve last retained round",
				"err", err,
			)
			continue
		}
		// Adjust startAt round in case node pruned missing rounds.
		if lastRetainedBlock.Header.Round > startAt {
			startAt = lastRetainedBlock.Header.Round
		}

		for round := startAt; round <= latest; round++ {
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// Try to index block.
			if err = s.indexBlock(round); err != nil {
				s.Logger.Warn("failed to index block",
					"err", err,
					"round", round,
				)
				break
			}
		}
	}
}

// Start starts service.
func (s *Service) Start() {
	go s.indexingWorker()
	go s.healthWorker()

	if s.enablePruning {
		go s.pruningWorker()
	}
}

// Stop stops service.
func (s *Service) Stop() {
	s.cancelCtx()
}

// New creates a new indexer service.
func New(
	ctxBackend context.Context,
	backendFactory BackendFactory,
	client client.RuntimeClient,
	runtimeID common.Namespace,
	storage storage.Storage,
	enablePruning bool,
	pruningStep uint64,
) (*Service, Backend, filters.SubscribeBackend, error) {
	subBackend, err := filters.NewSubscribeBackend(storage)
	if err != nil {
		return nil, nil, nil, err
	}

	ctx, cancelCtx := context.WithCancel(ctxBackend)
	backend, err := backendFactory(ctx, runtimeID, storage, subBackend)
	if err != nil {
		cancelCtx()
		return nil, nil, nil, err
	}

	s := &Service{
		BaseBackgroundService: *service.NewBaseBackgroundService("gateway/indexer"),
		runtimeID:             runtimeID,
		backend:               backend,
		client:                client,
		core:                  core.NewV1(client),
		ctx:                   ctx,
		cancelCtx:             cancelCtx,
		enablePruning:         enablePruning,
		pruningStep:           pruningStep,
	}
	s.Logger = s.Logger.With("runtime_id", s.runtimeID.String())

	return s, backend, subBackend, nil
}
