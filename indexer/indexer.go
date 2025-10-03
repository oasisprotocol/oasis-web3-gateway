package indexer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/source"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
)

const (
	indexerLoopDelay                 = 1 * time.Second
	pruningCheckInterval             = 60 * time.Second
	healthCheckInterval              = 10 * time.Second
	healthCheckIndexerDriftThreshold = 10

	blockIndexTimeout      = 60 * time.Second
	pruneIterationTimeout  = 60 * time.Second
	healthIterationTimeout = 60 * time.Second
)

var (
	metricBlockIndexed = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_web3_gateway_block_indexed", Help: "Indexed block heights."})
	metricBlockPruned  = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_web3_gateway_block_pruned", Help: "Pruned block heights."})
	metricHealthy      = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_web3_gateway_indexer_health", Help: "1 if gateway indexer healthcheck is reporting as healthy, 0 otherwise."})
)

// ErrNotHealthy is the error returned if the gateway is unhealthy.
var ErrNotHealthy = errors.New("not healthy")

// Service is an indexer service.
type Service struct {
	logger *logging.Logger

	runtimeID       common.Namespace
	enablePruning   bool
	pruningStep     uint64
	indexingStart   uint64
	indexingEnd     uint64
	indexingDisable bool

	backend Backend
	client  source.NodeSource

	queryEpochParameters bool
	coreParameters       *core.Parameters
	rtInfo               *core.RuntimeInfoResponse

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
func (s *Service) indexBlock(ctx context.Context, round uint64) error {
	blk, err := s.client.GetBlock(ctx, round)
	if err != nil {
		return fmt.Errorf("querying block: %w", err)
	}

	if s.coreParameters == nil || s.rtInfo == nil || s.queryEpochParameters {
		// Query parameters for block gas limit.
		var params *core.Parameters
		params, err = s.client.CoreParameters(ctx, round)
		if err != nil {
			return fmt.Errorf("querying block parameters: %w", err)
		}
		s.coreParameters = params

		// Query runtime info.
		s.rtInfo, err = s.client.CoreRuntimeInfo(ctx)
		if err != nil {
			return fmt.Errorf("querying runtime info: %w", err)
		}
	}

	txs, err := s.client.GetTransactionsWithResults(ctx, blk.Header.Round)
	if err != nil {
		return fmt.Errorf("querying transactions with results: %w", err)
	}

	err = s.backend.Index(ctx, blk, txs, s.coreParameters, s.rtInfo)
	if err != nil {
		return fmt.Errorf("indexing block: %w", err)
	}
	metricBlockIndexed.Set(float64(blk.Header.Round))

	switch {
	case blk.Header.HeaderType == block.EpochTransition:
		// Epoch transition block, ensure epoch parameters are queried on next normal round.
		s.queryEpochParameters = true
	case s.queryEpochParameters && blk.Header.HeaderType == block.Normal:
		// Epoch parameters were queried in last block, no need to re-query until next epoch.
		s.queryEpochParameters = false
	default:
	}

	return nil
}

func (s *Service) updateHealth(healthy bool) {
	if healthy {
		atomic.StoreUint32(&s.health, 1)
		metricHealthy.Set(float64(1))
	} else {
		atomic.StoreUint32(&s.health, 0)
		metricHealthy.Set(float64(0))
	}
}

// healthWorker is the health check worker.
func (s *Service) healthWorker(ctx context.Context) error {
	s.logger.Debug("starting health check worker")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
			func() {
				intervalCtx, cancel := context.WithTimeout(ctx, healthIterationTimeout)
				defer cancel()

				// Query last indexed round.
				lastIndexed, err := s.backend.QueryLastIndexedRound(intervalCtx)
				if err != nil {
					s.logger.Error("failed to query last indexed round",
						"err", err,
					)
					s.updateHealth(false)
					return
				}

				// Query latest round on the node.
				latestBlk, err := s.client.GetBlock(intervalCtx, client.RoundLatest)
				if err != nil {
					s.logger.Error("failed to query latest block",
						"err", err,
					)
					s.updateHealth(false)
					return
				}
				latestRound := latestBlk.Header.Round

				s.logger.Debug("checking health", "latest_round", latestRound, "latest_indexed", lastIndexed)
				switch {
				case latestRound >= lastIndexed:
					s.updateHealth(latestRound-lastIndexed <= healthCheckIndexerDriftThreshold)
				default:
					// Indexer in front of node - not healthy.
					s.updateHealth(false)
				}
			}()
		}
	}
}

// pruningWorker handles data pruning.
func (s *Service) pruningWorker(ctx context.Context) error {
	s.logger.Debug("starting periodic pruning worker")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pruningCheckInterval):
			func() {
				intervalCtx, cancel := context.WithTimeout(ctx, pruneIterationTimeout)
				defer cancel()

				lastIndexed, err := s.backend.QueryLastIndexedRound(intervalCtx)
				if err != nil {
					s.logger.Error("failed to query last indexed round",
						"err", err,
					)
					return
				}

				if lastIndexed > s.pruningStep {
					round := lastIndexed - s.pruningStep
					if err := s.backend.Prune(intervalCtx, round); err != nil {
						s.logger.Error("failed to prune round",
							"err", err,
							"round", round,
						)
						return
					}
					metricBlockPruned.Set(float64(round))
				}
			}()
		}
	}
}

// indexingWorker is a worker for indexing.
func (s *Service) indexingWorker(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(indexerLoopDelay):
		}

		done := func() bool {
			blockCtx, cancel := context.WithTimeout(ctx, blockIndexTimeout)
			defer cancel()

			// Get last indexed round.
			var startAt uint64
			lastIndexed, err := s.backend.QueryLastIndexedRound(blockCtx)
			switch {
			case errors.Is(err, storage.ErrNoRoundsIndexed):
				// No rounds indexed, start at 0.
				startAt = 0
			case err != nil:
				s.logger.Error("failed to query last indexed round",
					"err", err,
				)
				return false
			default:
				startAt = lastIndexed + 1
			}

			// Adjust startAt based on configuration and node state.
			switch s.indexingStart {
			case 0:
				// Not set, get last retained round on the node.
				lastRetainedBlock, err := s.client.GetLastRetainedBlock(blockCtx)
				if err != nil {
					s.logger.Error("failed to retrieve last retained round",
						"err", err,
					)
					return false
				}
				// Adjust startAt round in case node pruned missing rounds.
				if lastRetainedBlock.Header.Round > startAt {
					startAt = lastRetainedBlock.Header.Round
				}
			default:
				// indexing_start is set, use that.
				if s.indexingStart > startAt {
					startAt = s.indexingStart
				}
			}

			// Configure end round.
			var latest uint64
			switch s.indexingEnd {
			case 0:
				// Not set, query latest round available on the node.
				latestBlk, err := s.client.GetBlock(blockCtx, client.RoundLatest)
				if err != nil {
					s.logger.Error("failed to query latest block",
						"err", err,
					)
					return false
				}
				latest = latestBlk.Header.Round
			default:
				// indexing_end is set, use that.
				latest = s.indexingEnd

				// If we reached the end, stop the service.
				if startAt > latest {
					s.logger.Info("reached indexing end, stopping indexer",
						"indexing_end", s.indexingEnd,
					)
					return true
				}
			}

			// Following code uses a new context with new timeout.
			cancel()
			for round := startAt; round <= latest; round++ {
				select {
				case <-ctx.Done():
					return false
				default:
				}
				s.logger.Info("indexing round", "round", round, "start", startAt, "end", latest)

				indexCtx, cancel := context.WithTimeout(ctx, blockIndexTimeout)
				// Try to index block.
				if err = s.indexBlock(indexCtx, round); err != nil {
					s.logger.Warn("failed to index block",
						"err", err,
						"round", round,
					)
					cancel()
					break
				}
				cancel()
			}

			return false
		}()
		if done {
			s.updateHealth(true)
			return nil
		}
	}
}

// Start starts service.
func (s *Service) Start(ctx context.Context) error {
	// TODO/NotYawning: Non-archive nodes that have the indexer disabled
	// likely want to use a different notion of healthy, and probably also
	// want to start a worker that monitors the database for changes.
	if s.indexingDisable {
		if _, err := s.backend.QueryLastIndexedRound(ctx); err != nil {
			s.logger.Error("indexer disabled and no rounds indexed, this will never work",
				"err", err,
			)
			return err
		}
		s.updateHealth(true)
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 4)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Stop the service if indexer exists.
		defer cancel()

		if err := s.indexingWorker(ctx); err != nil {
			errCh <- fmt.Errorf("indexing worker: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.healthWorker(ctx); err != nil {
			errCh <- fmt.Errorf("health worker: %w", err)
		}
	}()

	if s.enablePruning {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.pruningWorker(ctx); err != nil {
				errCh <- fmt.Errorf("pruning worker: %w", err)
			}
		}()
	}

	select {
	case err := <-errCh:
		s.logger.Error("worker stopped", "err", err)
	case <-ctx.Done():
		s.logger.Debug("context done")
	}
	cancel()

	// Ensure source is closed.
	_ = s.client.Close()

	// Wait for services to shutdown.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// New creates a new indexer service.
func New(
	ctxBackend context.Context,
	backend Backend,
	source source.NodeSource,
	runtimeID common.Namespace,
	cfg *conf.Config,
) (*Service, error) {
	s := &Service{
		logger:          logging.GetLogger("indexer"),
		runtimeID:       runtimeID,
		backend:         backend,
		client:          source,
		enablePruning:   cfg.EnablePruning,
		pruningStep:     cfg.PruningStep,
		indexingStart:   cfg.IndexingStart,
		indexingEnd:     cfg.IndexingEnd,
		indexingDisable: cfg.IndexingDisable,
	}

	return s, nil
}
