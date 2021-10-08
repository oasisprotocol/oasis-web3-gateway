package indexer

import (
	"context"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
)

const (
	storageRequestTimeout = 5 * time.Second
	storageRetryTimeout   = 120 * time.Second
)

var stopFlag bool

// Service is an indexer service.
type Service struct {
	service.BaseBackgroundService
	runtimeID common.Namespace
	backend   Backend
	client    client.RuntimeClient
	ctx       context.Context
	cancelCtx context.CancelFunc
	stopFlag  bool
	stopCh    chan struct{}
}

func (s *Service) watchBlockWorker() {
	defer s.BaseBackgroundService.Stop()

	s.Logger.Info("started indexer for runtime")

	// Start watching blocks.
	blocksCh, blocksSub, err := s.client.WatchBlocks(s.ctx)
	if err != nil {
		s.Logger.Error("failed to subscribe to blocks",
			"err", err,
		)
		return
	}
	defer blocksSub.Close()

	for {
		select {
		case <-s.stopCh:
			s.Logger.Info("stop requested, terminating indexer")
			return
		case annBlk := <-blocksCh:
			// New blocks to index.
			blk := annBlk.Block

			var txs []*types.UnverifiedTransaction
			// TODO:
			off := cmnBackoff.NewExponentialBackOff()
			off.MaxElapsedTime = storageRetryTimeout

			err = backoff.Retry(func() error {
				txs, err := s.client.GetTransactions(s.ctx, blk.Header.Round)
			}, off)

			if err != nil {
				s.Logger.Error("can't get transactions through client")
				continue
			}

			if err = s.backend.Index(blk.Header.Round, blk.Header.EncodedHash(), txs); err != nil {
				s.Logger.Error("failed to index",
					"err", err,
					"round", blk.Header.Round,
				)
				continue
			}
		}
	}
}

func (s *Service) periodIndexWorker() {
	for {
		if stopFlag == true {
			break
		}

		latest := uint64(0) // TODO: s.client.RoundLatest
		indexed := s.backend.QueryIndexedRound()

		if latest == indexed {
			time.Sleep(storageRetryTimeout)
			continue
		}

		for {
			if stopFlag == true || latest == indexed {
				break
			}

			indexed++
			_, err1 := s.backend.QueryBlockHash(indexed)
			if err1 == nil {
				indexed++
				continue
			}

			blk, err2 := s.client.GetBlock(s.ctx, indexed)
			if err2 != nil {
				indexed--
				time.Sleep(storageRequestTimeout)
				continue
			}

			txs, err3 := s.client.GetTransactions(s.ctx, blk.Header.Round)
			if err3 != nil {
				indexed--
				time.Sleep(storageRequestTimeout)
				continue
			}

			if err := s.backend.Index(blk.Header.Round, blk.Header.EncodedHash(), txs); err != nil {
				s.Logger.Error("failed to index",
					"err", err,
					"round", blk.Header.Round,
				)
				indexed--
				time.Sleep(storageRetryTimeout)
				continue
			}
		}
	}
}

func (s *Service) Start() {
	go s.watchBlockWorker()
	go s.periodIndexWorker()
}

func (s *Service) Stop() {
	s.cancelCtx()
	close(s.stopCh)
	stopFlag = true
}

// New creates a new indexer service.
func New(dataDir string, runtimeID common.Namespace, backendFactory BackendFactory, client client.RuntimeClient) (*Service, error) {
	backend, err := backendFactory(dataDir, runtimeID, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	s := &Service{
		BaseBackgroundService: *service.NewBaseBackgroundService("gateway/indexer"),
		runtimeID:             runtimeID,
		backend:               backend,
		client:                client,
		ctx:                   ctx,
		cancelCtx:             cancelCtx,
		stopCh:                make(chan struct{}),
		stopFlag:              false,
	}
	s.Logger = s.Logger.With("runtime_id", s.runtimeID.String())

	return s, nil
}
